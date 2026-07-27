package preview

import (
	"errors"
	"testing"
	"time"
)

func TestSignVerifyRoundtrip(t *testing.T) {
	secret := []byte("s3cret-preview-key")
	c := Claims{Tenant: "11111111-1111-1111-1111-111111111111", Type: "lesson", Slug: "home", Exp: time.Now().Add(time.Minute).Unix()}
	got, err := Verify(secret, Sign(secret, c), time.Now())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Tenant != c.Tenant || got.Type != "lesson" || got.Slug != "home" {
		t.Errorf("claims = %+v", got)
	}
}

func TestVerifyExpired(t *testing.T) {
	secret := []byte("s")
	tok := Sign(secret, Claims{Slug: "x", Exp: time.Now().Add(-time.Minute).Unix()})
	if _, err := Verify(secret, tok, time.Now()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid (expired)", err)
	}
}

func TestVerifyTamperedOrWrongSecret(t *testing.T) {
	secret := []byte("s")
	tok := Sign(secret, Claims{Slug: "x", Exp: time.Now().Add(time.Minute).Unix()})
	if _, err := Verify([]byte("other-secret"), tok, time.Now()); !errors.Is(err, ErrInvalid) {
		t.Error("wrong secret should not verify")
	}
	if _, err := Verify(secret, "nonsense", time.Now()); !errors.Is(err, ErrInvalid) {
		t.Error("garbage token should not verify")
	}
	if _, err := Verify(secret, tok+"tamper", time.Now()); !errors.Is(err, ErrInvalid) {
		t.Error("tampered mac should not verify")
	}
}
