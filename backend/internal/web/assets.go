// Package web holds the static assets served next to the HTML pages.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
)

//go:embed static
var embedded embed.FS

// Static is the asset tree mounted under /static.
var Static, _ = fs.Sub(embedded, "static")

var versions = func() map[string]string {
	out := map[string]string{}
	_ = fs.WalkDir(Static, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(Static, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[path] = hex.EncodeToString(sum[:4])
		return nil
	})
	return out
}()

// Asset returns the URL of a static file with a content hash, so browsers can
// cache it for a long time and still pick up a new version after a deploy.
func Asset(name string) string {
	return "/static/" + name + "?v=" + versions[name]
}
