package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

//go:embed static
var staticFiles embed.FS

func loadEnv(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func main() {
	loadEnv(".env")

	mux := http.NewServeMux()

	client := NewGitHubClient(os.Getenv("GITHUB_TOKEN"))
	h := &Handler{gh: client, cache: NewCache(2 * time.Minute)}

	static, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("static: %v", err)
	}
	files := http.FileServerFS(static)

	mux.HandleFunc("GET /{$}", h.index)
	mux.HandleFunc("GET /activity", h.activity)
	mux.HandleFunc("GET /api/activity", h.activity)
	mux.Handle("GET /static/", http.StripPrefix("/static/", files))

	addr := os.Getenv("PORT")
	if addr == "" {
		addr = "8080"
	}
	srv := &http.Server{
		Addr:              ":" + addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("listening on http://localhost:%s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
