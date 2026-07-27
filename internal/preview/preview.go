// Package preview issues + verifies short-lived signed preview tokens. A token
// grants read access to a single tenant's draft document (type+slug) for a
// bounded window, so the editor can render unpublished content in the frontend's
// preview mode without exposing drafts publicly. HMAC-SHA256 signed; no DB.
package preview

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ErrInvalid is returned for a malformed, tampered, or expired token.
var ErrInvalid = errors.New("preview: invalid token")

// Claims are the token's scope: which tenant's (type, slug) draft, until when.
type Claims struct {
	Tenant string `json:"t"`
	Type   string `json:"dt"`
	Slug   string `json:"s"`
	Exp    int64  `json:"e"` // unix seconds
}

// Sign returns a token "payload.mac" for the claims.
func Sign(secret []byte, c Claims) string {
	payload, _ := json.Marshal(c)
	p := base64.RawURLEncoding.EncodeToString(payload)
	return p + "." + mac(secret, p)
}

// Verify checks the signature + expiry and returns the claims.
func Verify(secret []byte, token string, now time.Time) (Claims, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return Claims{}, ErrInvalid
	}
	if !hmac.Equal([]byte(mac(secret, parts[0])), []byte(parts[1])) {
		return Claims{}, ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrInvalid
	}
	var c Claims
	if err := json.Unmarshal(raw, &c); err != nil {
		return Claims{}, ErrInvalid
	}
	if now.Unix() > c.Exp {
		return Claims{}, ErrInvalid
	}
	return c, nil
}

func mac(secret []byte, msg string) string {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(msg))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
