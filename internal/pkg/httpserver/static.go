package httpserver

import (
	"html"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type staticHandler struct {
	root       string
	listing    bool
	fileServer http.Handler
}

type noListingFileSystem struct {
	root string
}

func (fs noListingFileSystem) Open(name string) (http.File, error) {
	file, err := http.Dir(fs.root).Open(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if info.IsDir() {
		file.Close()
		return nil, os.ErrNotExist
	}
	return file, nil
}

func newStaticHandler(root string, listing bool) http.Handler {
	var fileHandler http.Handler
	if listing {
		fileHandler = http.FileServer(http.Dir(root))
	} else {
		fileHandler = http.FileServer(noListingFileSystem{root: root})
	}
	return &staticHandler{
		root:       root,
		listing:    listing,
		fileServer: fileHandler,
	}
}

func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Accept-Ranges", "bytes")

	cleanPath := filepath.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
	target := filepath.Join(h.root, filepath.FromSlash(cleanPath))
	if !strings.HasPrefix(target, filepath.Clean(h.root)) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		h.fileServer.ServeHTTP(w, r)
		return
	}
	if !h.listing {
		http.NotFound(w, r)
		return
	}

	index := filepath.Join(target, "index.html")
	if _, err := os.Stat(index); err == nil {
		h.fileServer.ServeHTTP(w, r)
		return
	}
	h.renderListing(w, r, cleanPath, target)
}

func (h *staticHandler) renderListing(w http.ResponseWriter, r *http.Request, requestPath string, target string) {
	entries, err := os.ReadDir(target)
	if err != nil {
		http.Error(w, "unable to read directory", http.StatusForbidden)
		return
	}

	names := make([]string, 0, len(entries))
	directories := map[string]bool{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		names = append(names, entry.Name())
		directories[entry.Name()] = entry.IsDir()
	}
	sort.Strings(names)

	var builder strings.Builder
	builder.WriteString("<!DOCTYPE html><html lang=\"en\"><head><meta charset=\"utf-8\">")
	builder.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">")
	builder.WriteString("<title>Index of " + html.EscapeString(requestPath) + "</title>")
	builder.WriteString("<style>body{font-family:system-ui,sans-serif;margin:2rem}li{margin:.25rem 0}a{text-decoration:none}</style>")
	builder.WriteString("</head><body><h1>Index of " + html.EscapeString(requestPath) + "</h1><ul>")
	if requestPath != "/" {
		builder.WriteString("<li><a href=\"../\">../</a></li>")
	}
	for _, name := range names {
		suffix := ""
		if directories[name] {
			suffix = "/"
		}
		href := filepath.ToSlash(filepath.Join(requestPath, name)) + suffix
		builder.WriteString("<li><a href=\"" + html.EscapeString(href) + "\">" + html.EscapeString(name) + suffix + "</a></li>")
	}
	builder.WriteString("</ul></body></html>")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(builder.String()))
}
