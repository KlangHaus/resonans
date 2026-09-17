// Package bridge serves the visual-editing bridge script that a frontend loads
// on its preview route. The script is embedded, so the version a site loads is
// the version of the Resonans it talks to — no separate package to keep in sync.
package bridge

import (
	_ "embed"
	"net/http"
)

//go:embed resonans-bridge.js
var script []byte

// Path is where the script is served. The protocol version is in the path, so
// a breaking protocol change gets a new path instead of silently changing the
// behavior of every site that already loads v1.
const Path = "/bridge/v1/resonans-bridge.js"

// Handler serves the script. It is public (it contains no tenant data) and is
// loaded cross-origin via <script src>, which needs no CORS headers.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(script)
	})
}
