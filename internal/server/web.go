package server

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed all:static
var staticContent embed.FS

// webRouter creates a router that represents the routes under /ui
func webRouter() (chi.Router, error) {
	r := chi.NewRouter()
	r.Use(middleware.StripSlashes)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate, max-age=0")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
			next.ServeHTTP(w, r)
		})
	})

	// direct routes for html pages to provide clean URLs
	r.Get("/", func(w http.ResponseWriter, r *http.Request) { serveHTML(w, r, "static/index.html") })
	r.Get("/tools", func(w http.ResponseWriter, r *http.Request) { serveHTML(w, r, "static/tools.html") })
	r.Get("/toolsets", func(w http.ResponseWriter, r *http.Request) { serveHTML(w, r, "static/toolsets.html") })

	// handler for all other static files/assets
	staticFS, _ := fs.Sub(staticContent, "static")
	r.Handle("/*", http.StripPrefix("/ui", http.FileServer(http.FS(staticFS))))

	return r, nil
}

func serveHTML(w http.ResponseWriter, r *http.Request, filepath string) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	file, err := staticContent.Open(filepath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading file: %v", err), http.StatusInternalServerError)
		return
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return
	}
	http.ServeContent(w, r, fileInfo.Name(), fileInfo.ModTime(), bytes.NewReader(fileBytes))
}
