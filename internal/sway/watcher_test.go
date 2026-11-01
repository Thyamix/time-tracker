package sway

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/iotest"
	"time"
)

func TestFraming(t *testing.T) {
	wire := []byte{'i', '3', '-', 'i', 'p', 'c', 2, 0, 0, 0, 0, 0, 0, 128, '{', '}'}
	var got bytes.Buffer
	if err := writeFrame(&got, workspaceEvent, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), wire) {
		t.Fatalf("frame = %x, want %x", got.Bytes(), wire)
	}
	kind, payload, err := readFrame(iotest.OneByteReader(bytes.NewReader(wire)))
	if err != nil || kind != workspaceEvent || string(payload) != `{}` {
		t.Fatalf("readFrame = %d, %q, %v", kind, payload, err)
	}

	badMagic := bytes.Clone(wire)
	badMagic[0] = 'x'
	tooLarge := bytes.Clone(wire[:14])
	binary.LittleEndian.PutUint32(tooLarge[6:10], maxPayload+1)
	for name, frame := range map[string][]byte{
		"magic": badMagic, "length": tooLarge, "header": wire[:13], "payload": wire[:15],
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := readFrame(bytes.NewReader(frame)); err == nil {
				t.Fatal("accepted invalid frame")
			}
		})
	}
	if err := writeFrame(io.Discard, getWorkspaces, make([]byte, maxPayload+1)); err == nil {
		t.Fatal("accepted oversized outgoing frame")
	}
}

func TestRequestRejectsEventAsReply(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer server.Close()
		kind, payload, err := readFrame(server)
		if err != nil || kind != getWorkspaces || len(payload) != 0 {
			t.Errorf("request = %d, %q, %v", kind, payload, err)
			return
		}
		if err := writeFrame(server, workspaceEvent, []byte(`{}`)); err != nil {
			t.Error(err)
		}
	}()
	if _, err := request(client, getWorkspaces, nil); err == nil {
		t.Fatal("accepted workspace event as query reply")
	}
	<-done
}

func TestFocusedWorkspace(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		want    observation
		wantErr bool
	}{
		{"focused not visible", `[{"name":"other","num":2,"visible":true,"focused":false},{"name":"work","num":7,"focused":true}]`, observation{"work", 7}, false},
		{"named workspace", `[{"name":"renamed","num":-1,"focused":true}]`, observation{"renamed", -1}, false},
		{"only visible", `[{"name":"other","num":2,"visible":true}]`, observation{}, true},
		{"empty", `[]`, observation{}, true},
		{"null", `null`, observation{}, true},
		{"object", `{}`, observation{}, true},
		{"malformed", `[`, observation{}, true},
		{"missing name", `[{"num":1,"focused":true}]`, observation{}, true},
		{"empty name", `[{"name":"","num":1,"focused":true}]`, observation{}, true},
		{"missing number", `[{"name":"work","focused":true}]`, observation{}, true},
		{"invalid number", `[{"name":"work","num":"1","focused":true}]`, observation{}, true},
		{"multiple focused", `[{"name":"a","num":1,"focused":true},{"name":"b","num":2,"focused":true}]`, observation{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name, number, err := focusedWorkspace([]byte(tc.payload))
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, want error %v", err, tc.wantErr)
			}
			if !tc.wantErr && (observation{name, number}) != tc.want {
				t.Fatalf("focus = %q, %d, want %+v", name, number, tc.want)
			}
		})
	}
}

func TestWatchSnapshots(t *testing.T) {
	s := newFakeIPC(t, `{"success":true}`)
	t.Setenv("SWAYSOCK", s.path)
	w := startWatch(t)
	subscription := receive(t, s.subscriptions)
	receive(t, s.requests).reply <- fakeReply{getWorkspaces, `[{"name":"visible","num":9,"visible":true},{"name":"work","num":1,"focused":true}]`}
	if got := receive(t, w.observations); got != (observation{"work", 1}) {
		t.Fatalf("initial focus = %+v", got)
	}

	// The event's current workspace is deliberately wrong. Only a fresh query
	// can report the rename correctly.
	if err := writeFrame(subscription, workspaceEvent, []byte(`{"change":"rename","current":{"name":"stale","num":8}}`)); err != nil {
		t.Fatal(err)
	}
	receive(t, s.requests).reply <- fakeReply{getWorkspaces, `[{"name":"renamed","num":1,"focused":true}]`}
	if got := receive(t, w.observations); got != (observation{"renamed", 1}) {
		t.Fatalf("event focus = %+v", got)
	}

	// Even without another event, an unchanged focus must be observed again.
	receive(t, s.requests).reply <- fakeReply{getWorkspaces, `[{"name":"renamed","num":1,"focused":true}]`}
	if got := receive(t, w.observations); got != (observation{"renamed", 1}) {
		t.Fatalf("heartbeat focus = %+v", got)
	}
	w.cancel()
	receive(t, w.disconnections)
	receive(t, w.done)
	assertNoObservation(t, w)
}

func TestWatchInvalidFocus(t *testing.T) {
	for _, payload := range []string{`[]`, `[{"name":"visible","num":2,"visible":true}]`, `not json`} {
		t.Run(payload, func(t *testing.T) {
			s := newFakeIPC(t, `{"success":true}`)
			t.Setenv("SWAYSOCK", s.path)
			w := startWatch(t)
			receive(t, s.requests).reply <- fakeReply{getWorkspaces, payload}
			receive(t, w.disconnections)
			assertNoObservation(t, w)
		})
	}
}

func TestWatchRejectedSubscription(t *testing.T) {
	for _, reply := range []string{`{"success":false}`, `{}`, `null`, `not json`} {
		t.Run(reply, func(t *testing.T) {
			s := newFakeIPC(t, reply)
			t.Setenv("SWAYSOCK", s.path)
			w := startWatch(t)
			receive(t, w.disconnections)
			assertNoObservation(t, w)
			select {
			case <-s.requests:
				t.Fatal("queried workspaces before successful subscription")
			default:
			}
		})
	}
}

func TestWatchSubscriptionFailureInterruptsQuery(t *testing.T) {
	s := newFakeIPC(t, `{"success":true}`)
	t.Setenv("SWAYSOCK", s.path)
	w := startWatch(t)
	subscription := receive(t, s.subscriptions)
	receive(t, s.requests) // Leave the query unanswered.
	if err := subscription.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-w.disconnections:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("subscription failure did not interrupt the pending query")
	}
	assertNoObservation(t, w)
}

func TestWatchCancellation(t *testing.T) {
	for _, phase := range []string{"subscription", "query", "retry"} {
		t.Run(phase, func(t *testing.T) {
			t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
			t.Setenv("SWAYSOCK", "")
			var s *fakeIPC
			if phase != "retry" {
				reply := `{"success":true}`
				if phase == "subscription" {
					reply = ""
				}
				s = newFakeIPC(t, reply)
				t.Setenv("SWAYSOCK", s.path)
			}
			w := startWatch(t)
			switch phase {
			case "subscription":
				receive(t, s.subscriptions)
			case "query":
				receive(t, s.requests)
			case "retry":
				receive(t, w.disconnections)
			}
			w.cancel()
			select {
			case <-w.done:
			case <-time.After(500 * time.Millisecond):
				t.Fatal("cancellation did not stop Watch promptly")
			}
			if phase != "retry" {
				receive(t, w.disconnections)
			}
			assertNoObservation(t, w)
		})
	}
}

func TestWatchQueryTimeout(t *testing.T) {
	s := newFakeIPC(t, `{"success":true}`)
	t.Setenv("SWAYSOCK", s.path)
	w := startWatch(t)
	receive(t, s.requests)
	receive(t, w.disconnections)
	assertNoObservation(t, w)
}

func TestWatchReconnectRediscoversEnvironment(t *testing.T) {
	t.Setenv("SWAYSOCK", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	w := startWatch(t)
	receive(t, w.disconnections)

	for i := 1; i <= 2; i++ {
		s := newFakeIPC(t, `{"success":true}`)
		t.Setenv("SWAYSOCK", s.path)
		receive(t, s.requests).reply <- fakeReply{getWorkspaces, fmt.Sprintf(`[{"name":"work","num":%d,"focused":true}]`, i)}
		if got := receive(t, w.observations); got != (observation{"work", i}) {
			t.Fatalf("reconnected focus = %+v", got)
		}
		if err := receive(t, s.subscriptions).Close(); err != nil {
			t.Fatal(err)
		}
		receive(t, w.disconnections)
	}
}

func TestConnectDiscovery(t *testing.T) {
	for _, scenario := range []string{"fallback", "invalid env", "stale env", "ambiguous", "env preferred", "regular file"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_RUNTIME_DIR", dir)
			t.Setenv("SWAYSOCK", "")
			path := filepath.Join(dir, fmt.Sprintf("sway-ipc.%d.1.sock", os.Getuid()))
			listenUnix(t, path)
			switch scenario {
			case "invalid env":
				t.Setenv("SWAYSOCK", filepath.Join(dir, "missing.sock"))
			case "stale env":
				stalePath := filepath.Join(dir, fmt.Sprintf("sway-ipc.%d.2.sock", os.Getuid()))
				stale := listenUnix(t, stalePath)
				stale.SetUnlinkOnClose(false)
				if err := stale.Close(); err != nil {
					t.Fatal(err)
				}
				t.Setenv("SWAYSOCK", stalePath)
			case "ambiguous", "env preferred":
				listenUnix(t, filepath.Join(dir, fmt.Sprintf("sway-ipc.%d.2.sock", os.Getuid())))
				if scenario == "env preferred" {
					t.Setenv("SWAYSOCK", path)
				}
			case "regular file":
				file := filepath.Join(dir, fmt.Sprintf("sway-ipc.%d.2.sock", os.Getuid()))
				if err := os.WriteFile(file, nil, 0600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("SWAYSOCK", file)
			}
			conn, got, err := connect(context.Background())
			if conn != nil {
				conn.Close()
			}
			if scenario == "ambiguous" {
				if err == nil {
					t.Fatal("selected an ambiguous socket")
				}
			} else if err != nil || got != path {
				t.Fatalf("connect = %q, %v, want %q", got, err, path)
			}
		})
	}
}

type observation struct {
	name   string
	number int
}

type watchRun struct {
	observations   chan observation
	disconnections chan struct{}
	done           chan struct{}
	cancel         context.CancelFunc
}

func startWatch(t *testing.T) *watchRun {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	w := &watchRun{make(chan observation, 16), make(chan struct{}, 16), make(chan struct{}), cancel}
	go func() {
		defer close(w.done)
		Watch(ctx, func(name string, number int) { w.observations <- observation{name, number} }, func() { w.disconnections <- struct{}{} })
	}()
	t.Cleanup(func() {
		cancel()
		receive(t, w.done)
	})
	return w
}

func assertNoObservation(t *testing.T, w *watchRun) {
	t.Helper()
	select {
	case got := <-w.observations:
		t.Fatalf("unexpected observation: %+v", got)
	default:
	}
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(4 * time.Second):
		t.Fatal("timed out waiting for IPC activity")
		var zero T
		return zero
	}
}

type fakeReply struct {
	kind    uint32
	payload string
}

type fakeRequest struct {
	reply chan fakeReply
}

type fakeIPC struct {
	path          string
	subscriptions chan net.Conn
	requests      chan fakeRequest
}

func listenUnix(t *testing.T, path string) *net.UnixListener {
	t.Helper()
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	return listener
}

func newFakeIPC(t *testing.T, subscriptionReply string) *fakeIPC {
	t.Helper()
	s := &fakeIPC{filepath.Join(t.TempDir(), "ipc.sock"), make(chan net.Conn, 8), make(chan fakeRequest, 8)}
	listener := listenUnix(t, s.path)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				stop := context.AfterFunc(ctx, func() { conn.Close() })
				defer stop()
				for {
					kind, payload, err := readFrame(conn)
					if err != nil {
						return
					}
					switch kind {
					case subscribe:
						if string(payload) != `["workspace"]` {
							t.Errorf("subscription payload = %q", payload)
							return
						}
						select {
						case s.subscriptions <- conn:
						case <-ctx.Done():
							return
						}
						if subscriptionReply == "" {
							<-ctx.Done()
							return
						}
						if err := writeFrame(conn, subscribe, []byte(subscriptionReply)); err != nil {
							return
						}
					case getWorkspaces:
						if len(payload) != 0 {
							t.Errorf("get_workspaces payload = %q", payload)
							return
						}
						req := fakeRequest{make(chan fakeReply, 1)}
						select {
						case s.requests <- req:
						case <-ctx.Done():
							return
						}
						select {
						case reply := <-req.reply:
							if err := writeFrame(conn, reply.kind, []byte(reply.payload)); err != nil {
								return
							}
						case <-ctx.Done():
							return
						}
					default:
						t.Errorf("unexpected request type %d", kind)
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() {
		cancel()
		listener.Close()
		wg.Wait()
	})
	return s
}
