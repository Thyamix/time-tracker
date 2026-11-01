package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"timetrack/internal/api"
	"timetrack/internal/db"
	"timetrack/internal/sway"
	"timetrack/internal/tracking"
)

//go:embed web/dist
var webDist embed.FS

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "7332"
	}
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		home, _ := os.UserHomeDir()
		dbPath = home + "/.local/share/timetrack/timetrack.db"
	}
	// Expand ~ manually
	if strings.HasPrefix(dbPath, "~/") {
		home, _ := os.UserHomeDir()
		dbPath = home + dbPath[1:]
	}

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	tracker, err := tracking.New(database)
	if err != nil {
		log.Fatalf("failed to initialize tracking: %v", err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		sway.Watch(ctx, tracker.Observe, tracker.Disconnect)
	}()
	server := api.NewServer(database, tracker)

	mux := http.NewServeMux()

	// API routes
	mux.Handle("/api/", server.Router())

	// Static frontend — serve from embedded web/dist
	distFS, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Fatalf("failed to get dist fs: %v", err)
	}
	fileServer := http.FileServer(http.FS(distFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// SPA fallback: if file not found serve index.html
		_, err := distFS.Open(strings.TrimPrefix(r.URL.Path, "/"))
		if err != nil || r.URL.Path == "/" {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})

	addr := "127.0.0.1:" + port
	fmt.Printf("timetrack listening on http://%s\n", addr)
	httpServer := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	serveDone := make(chan error, 1)
	go func() { serveDone <- httpServer.ListenAndServe() }()
	select {
	case err := <-serveDone:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server: %v", err)
		}
	case <-ctx.Done():
	}
	shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err := httpServer.Shutdown(shutdown); err != nil {
		log.Printf("HTTP shutdown: %v", err)
	}
	cancel()
	<-watcherDone
}
