package media

import (
	"errors"
	"net/http"
	"sort"
	"strings"
)

// ErrUnsupportedType is returned when an upload's CONTENT is not an allowed
// type. What the client claims (Content-Type, file extension) is never trusted:
// an SVG or HTML file with a script, stored with its claimed type, would run on
// the media origin (riff KH-1011).
var ErrUnsupportedType = errors.New("media: unsupported file type")

// allowedTypes maps a sniffed MIME type to the extension the storage key gets.
//
// Measured with http.DetectContentType on go1.25.13 before choosing:
//   - an SVG with <script> sniffs as "text/plain", with an <?xml?> head as
//     "text/xml"; HTML with a UTF-8 BOM, or with text before its first tag, also
//     sniffs as "text/plain". So no text/* type is allowed: each of them would
//     let script-carrying markup in under an innocent name.
//   - MP4 and AVIF are not reliably recognized, so they are not listed rather
//     than accepted as application/octet-stream. Video goes through the video
//     plugin (Vimeo), not asset upload.
var allowedTypes = map[string]string{
	"image/png":       ".png",
	"image/jpeg":      ".jpg",
	"image/gif":       ".gif",
	"image/webp":      ".webp",
	"application/pdf": ".pdf",
	"application/zip": ".zip",
	"audio/mpeg":      ".mp3",
}

// DetectType returns the MIME type of data, judged from its bytes, and the
// canonical extension for it — or ErrUnsupportedType.
func DetectType(data []byte) (mime, ext string, err error) {
	sniffed := http.DetectContentType(data)
	if i := strings.IndexByte(sniffed, ';'); i >= 0 {
		sniffed = sniffed[:i]
	}
	ext, ok := allowedTypes[sniffed]
	if !ok {
		return "", "", ErrUnsupportedType
	}
	return sniffed, ext, nil
}

// AllowedTypes lists the accepted MIME types, sorted, for error messages.
func AllowedTypes() []string {
	out := make([]string, 0, len(allowedTypes))
	for t := range allowedTypes {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
