package rest

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dashboard/*
var dashboardFS embed.FS

// dashboardContent returns the embedded dashboard filesystem.
// It removes the "web/dashboard/" prefix so that index.html is at the root.
func dashboardContent() http.FileSystem {
	// Create a sub-filesystem that strips the "web/dashboard/" prefix
	subFS, err := fs.Sub(dashboardFS, "dashboard")
	if err != nil {
		panic("rest: failed to create dashboard sub-filesystem: " + err.Error())
	}
	return http.FS(subFS)
}

// ServeDashboard registers a handler for the dashboard at the root path.
// It serves index.html for the root and static files from the embedded filesystem.
func ServeDashboard(mux *http.ServeMux) {
	fs := dashboardContent()

	// Serve index.html for root path
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			f, err := fs.Open("index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer f.Close()

			stat, err := f.Stat()
			if err != nil {
				http.NotFound(w, r)
				return
			}

			http.ServeContent(w, r, "index.html", stat.ModTime(), f)
			return
		}

		// For other paths, try to serve static files
		http.FileServer(fs).ServeHTTP(w, r)
	})
}
