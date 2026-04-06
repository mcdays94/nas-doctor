package api

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
)

//go:embed icons
var iconsFS embed.FS

// serveIcon writes an icon PNG to the response. Falls back to default.png.
func serveIcon(w http.ResponseWriter, name string) {
	filePath := path.Join("icons", name+".png")
	data, err := iconsFS.ReadFile(filePath)
	if err != nil {
		// fallback to default
		data, err = iconsFS.ReadFile("icons/default.png")
		if err != nil {
			http.Error(w, "icon not found", http.StatusNotFound)
			return
		}
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

// ListIcons returns available icon names.
func ListIcons() []string {
	var names []string
	entries, err := fs.ReadDir(iconsFS, "icons")
	if err != nil {
		return names
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if n == "default.png" {
			continue
		}
		// strip .png
		if len(n) > 4 && n[len(n)-4:] == ".png" {
			names = append(names, n[:len(n)-4])
		}
	}
	return names
}
