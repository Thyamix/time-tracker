// Package sway observes the focused workspace through Sway's Unix socket IPC.
package sway

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	getWorkspaces  uint32 = 1
	subscribe      uint32 = 2
	workspaceEvent uint32 = 1 << 31
	maxPayload            = 4 << 20
	ipcTimeout            = time.Second
	watchInterval         = time.Second
)

// Watch runs until ctx is canceled. It reports fresh focused-workspace snapshots
// after subscribing, on workspace events, and every second, even if unchanged.
// IPC failures (including missing or ambiguous sockets) call disconnected and
// retry after one second. Cancellation also calls disconnected unless already
// disconnected. Callbacks run serially on the calling goroutine and must return
// promptly. No observation is fabricated when focus cannot be read.
func Watch(ctx context.Context, observe func(name string, number int), disconnected func()) {
	for {
		if ctx.Err() == nil {
			_ = watch(ctx, observe)
		}
		disconnected()
		if ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(watchInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func watch(ctx context.Context, observe func(string, int)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	eventsConn, path, err := connect(ctx)
	if err != nil {
		return err
	}
	defer eventsConn.Close()
	stopEvents := context.AfterFunc(ctx, func() { eventsConn.Close() })
	defer stopEvents()

	payload, err := request(eventsConn, subscribe, []byte(`["workspace"]`))
	if err != nil {
		return err
	}
	var reply struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(payload, &reply); err != nil || !reply.Success {
		return errors.New("workspace subscription rejected")
	}
	if err := eventsConn.SetDeadline(time.Time{}); err != nil {
		return err
	}

	events := make(chan struct{}, 1)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer cancel()
		for {
			// An idle subscription has no timeout. Once a frame starts, its
			// remaining bytes must arrive within the IPC deadline.
			if err := eventsConn.SetReadDeadline(time.Time{}); err != nil {
				return
			}
			var first [1]byte
			if _, err := io.ReadFull(eventsConn, first[:]); err != nil {
				return
			}
			if err := eventsConn.SetReadDeadline(time.Now().Add(ipcTimeout)); err != nil {
				return
			}
			kind, payload, err := readFrame(io.MultiReader(bytes.NewReader(first[:]), eventsConn))
			if err != nil || kind != workspaceEvent || !json.Valid(payload) {
				return
			}
			select {
			case events <- struct{}{}:
			default:
			}
		}
	}()
	defer func() {
		cancel()
		eventsConn.Close()
		<-readerDone
	}()

	queries, err := (&net.Dialer{Timeout: ipcTimeout}).DialContext(ctx, "unix", path)
	if err != nil {
		return err
	}
	defer queries.Close()
	stopQueries := context.AfterFunc(ctx, func() { queries.Close() })
	defer stopQueries()

	ticker := time.NewTicker(watchInterval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		payload, err := request(queries, getWorkspaces, nil)
		if err != nil {
			return err
		}
		name, number, err := focusedWorkspace(payload)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		observe(name, number)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		case <-events:
		}
	}
}

func connect(ctx context.Context) (net.Conn, string, error) {
	dialer := net.Dialer{Timeout: ipcTimeout}
	env := os.Getenv("SWAYSOCK")
	if isSocket(env) {
		conn, err := dialer.DialContext(ctx, "unix", env)
		if err == nil {
			return conn, env, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	paths, err := filepath.Glob(filepath.Join(runtimeDir, fmt.Sprintf("sway-ipc.%d.*.sock", os.Getuid())))
	if err != nil {
		return nil, "", err
	}
	var candidates []string
	for _, path := range paths {
		// Do not retry an environment socket that already failed to dial.
		if filepath.Clean(path) != filepath.Clean(env) && isSocket(path) {
			candidates = append(candidates, path)
		}
	}
	if len(candidates) != 1 {
		return nil, "", fmt.Errorf("found %d candidate Sway sockets, want exactly one", len(candidates))
	}
	conn, err := dialer.DialContext(ctx, "unix", candidates[0])
	return conn, candidates[0], err
}

func isSocket(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode()&os.ModeSocket != 0
}

func request(conn net.Conn, kind uint32, payload []byte) ([]byte, error) {
	if err := conn.SetDeadline(time.Now().Add(ipcTimeout)); err != nil {
		return nil, err
	}
	if err := writeFrame(conn, kind, payload); err != nil {
		return nil, err
	}
	replyKind, reply, err := readFrame(conn)
	if err != nil {
		return nil, err
	}
	if replyKind != kind {
		return nil, fmt.Errorf("unexpected IPC reply type %d, want %d", replyKind, kind)
	}
	return reply, nil
}

func readFrame(r io.Reader) (uint32, []byte, error) {
	var header [14]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	if string(header[:6]) != "i3-ipc" {
		return 0, nil, errors.New("invalid IPC magic")
	}
	length := binary.LittleEndian.Uint32(header[6:10])
	if length > maxPayload {
		return 0, nil, errors.New("IPC payload too large")
	}
	payload := make([]byte, int(length))
	_, err := io.ReadFull(r, payload)
	return binary.LittleEndian.Uint32(header[10:14]), payload, err
}

func writeFrame(w io.Writer, kind uint32, payload []byte) error {
	if len(payload) > maxPayload {
		return errors.New("IPC payload too large")
	}
	var header [14]byte
	copy(header[:6], "i3-ipc")
	binary.LittleEndian.PutUint32(header[6:10], uint32(len(payload)))
	binary.LittleEndian.PutUint32(header[10:14], kind)
	_, err := io.Copy(w, io.MultiReader(bytes.NewReader(header[:]), bytes.NewReader(payload)))
	return err
}

func focusedWorkspace(payload []byte) (string, int, error) {
	var workspaces []struct {
		Name    *string `json:"name"`
		Number  *int    `json:"num"`
		Focused bool    `json:"focused"`
	}
	if err := json.Unmarshal(payload, &workspaces); err != nil {
		return "", 0, err
	}
	name, number, found := "", 0, false
	for _, workspace := range workspaces {
		if !workspace.Focused {
			continue
		}
		if found || workspace.Name == nil || *workspace.Name == "" || workspace.Number == nil {
			return "", 0, errors.New("invalid focused workspace")
		}
		name, number, found = *workspace.Name, *workspace.Number, true
	}
	if !found {
		return "", 0, errors.New("no focused workspace")
	}
	return name, number, nil
}
