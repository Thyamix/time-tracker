package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"timetrack/internal/api"
	"timetrack/internal/db"
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

	server := api.NewServer(database)

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
	log.Fatal(http.ListenAndServe(addr, mux))
}
