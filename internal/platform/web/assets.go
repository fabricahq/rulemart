// Serve the embedded static files under a path that changes whenever any of them does.

package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
)

//go:embed static
var embedded embed.FS

// assets serves the files in static/ at /_static/<version>/<file>, where version is a digest of every file. A
// release's files can then be cached for a year, and the stylesheet's relative font URLs stay in the same version.
type assets struct {
	files   fs.FS
	version string
}

// newAssets returns the embedded static files and their version.
func newAssets() (*assets, error) {
	files, err := fs.Sub(embedded, "static")
	if err != nil {
		return nil, fmt.Errorf("open static files: %v", err)
	}
	digest := sha256.New()
	err = fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		fmt.Fprintf(digest, "%s %d\n", name, len(content))
		digest.Write(content)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("digest static files: %v", err)
	}
	return &assets{files: files, version: hex.EncodeToString(digest.Sum(nil))[:12]}, nil
}

// url returns the path that serves the static file name.
func (a *assets) url(name string) string {
	return "/_static/" + a.version + "/" + name
}

// iconURL returns the path that serves the icon file, a path under static/icons such as devicon/go-plain.svg.
func (a *assets) iconURL(file string) string {
	return a.url("icons/" + file)
}

// immutable caches a file for a year: its path changes whenever its content does.
const immutable = "public, max-age=31536000, immutable"

// ServeHTTP answers GET /_static/{version}/{file...}. A request for another version, such as from a page cached
// before a deployment, gets this release's file, cached briefly, so the page still renders.
func (a *assets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	content, err := fs.ReadFile(a.files, name)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid) {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "The file couldn't be read.", http.StatusInternalServerError)
		return
	}
	cache := immutable
	if r.PathValue("version") != a.version {
		cache = pageCache
	}
	writeFile(w, r, contentType(name), cache, content)
}

// faviconPattern is the route of the icon browsers ask for at the site's root, which has no version to cache by.
const faviconPattern = "GET /favicon.ico"

// faviconCache keeps the root icon a day, since its address stays the same when it changes.
const faviconCache = "public, max-age=86400"

// serveFavicon answers GET /favicon.ico with the static favicon.ico.
func (a *assets) serveFavicon(w http.ResponseWriter, r *http.Request) {
	content, err := fs.ReadFile(a.files, "favicon.ico")
	if err != nil {
		http.Error(w, "The file couldn't be read.", http.StatusInternalServerError)
		return
	}
	writeFile(w, r, contentType("favicon.ico"), faviconCache, content)
}

// writeFile answers with content, a file of the media type contentType that's the same for every visitor, cached as
// cache says.
func writeFile(w http.ResponseWriter, r *http.Request, contentType, cache string, content []byte) {
	header := w.Header()
	header.Set("Content-Type", contentType)
	header.Set("Cache-Control", cache)
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(content)
	}
}

// contentType returns a static file's media type from its extension, the same on every system for the types the
// site serves, whatever its MIME table holds.
func contentType(name string) string {
	switch path.Ext(name) {
	case ".ico":
		return "image/x-icon"
	case ".png":
		return "image/png"
	case ".svg":
		return "image/svg+xml"
	case ".css":
		return "text/css; charset=utf-8"
	case ".woff2":
		return "font/woff2"
	case ".js":
		return "text/javascript; charset=utf-8"
	}
	if t := mime.TypeByExtension(path.Ext(name)); t != "" {
		return t
	}
	return "application/octet-stream"
}
