package media

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"time"
)

// AssetPath is the route prefix under which Resonans serves its own asset bytes.
const AssetPath = "/_assets/"

// AssetHandler serves stored asset bytes at /_assets/<key>. It exists so a
// self-hosted Resonans has a working public asset URL with nothing but a
// volume: before it, an install without Bunny stored uploads in memory and
// pointed every asset URL at a route that did not exist.
//
// With Bunny configured this route is not mounted — the CDN serves the bytes —
// so the handler only ever fronts a backend that can read them back.
func AssetHandler(b ByteServer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, _, err := b.Open(r.PathValue("key"))
		if err != nil {
			// A malformed key and a missing object answer alike: telling a
			// prober that their traversal syntax parsed is itself an answer.
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrBadKey) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "asset unavailable", http.StatusInternalServerError)
			return
		}
		defer func() { _ = f.Close() }()

		// Re-derive the type from the BYTES, with the same policy the upload
		// used (http.DetectContentType reads at most 512 bytes, so this sniff
		// is the identical judgement). Serving the stored metadata instead
		// would trust a row to describe a file; this way a byte sequence the
		// policy forbids cannot be served even if it reached the disk by some
		// other path than an upload.
		head := make([]byte, 512)
		n, err := io.ReadFull(f, head)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			http.Error(w, "asset unavailable", http.StatusInternalServerError)
			return
		}
		mime, _, err := DetectType(head[:n])
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			http.Error(w, "asset unavailable", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", mime)
		// These bytes are visitor-supplied and served from the same origin as
		// the editor API. The type allowlist already excludes every text/* and
		// SVG type, so nothing script-carrying gets this far; nosniff plus a
		// sandbox keeps a future addition to that list from silently becoming
		// an execution surface here.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		// Keys carry a UUID, so a key names one immutable byte sequence forever.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

		// ServeContent gives range requests and conditional handling; it leaves
		// the Content-Type we set alone. The zero modtime omits Last-Modified,
		// which an immutable key does not need.
		http.ServeContent(w, r, "", time.Time{}, f)
	})
}
