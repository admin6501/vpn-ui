package pasarguard

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"
)

func TestExistingSubscriptionFormats(t *testing.T) {
	secret := "test-secret"
	now := time.Unix(1700000100, 0)
	enc := base64.RawURLEncoding.EncodeToString
	body := enc([]byte("v2,123,1700000000"))
	hash := sha256.Sum256([]byte(body + secret))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	tokens := []string{body + hex.EncodeToString(hash[:])[:10], body + enc(hash[:])[:10], body + "." + enc(mac.Sum(nil))}
	for _, token := range tokens {
		p, err := Verify(token, secret, now)
		if err != nil || p.UserID != 123 || p.IssuedAt.Unix() != 1700000000 {
			t.Fatalf("format rejected: %v", err)
		}
		if _, err := Verify(token, "wrong", now); err == nil {
			t.Fatal("accepted wrong key")
		}
		if p.ValidAfter(time.Unix(1700000001, 0), time.Time{}) {
			t.Fatal("accepted token older than account")
		}
		if p.ValidAfter(time.Time{}, time.Unix(1700000001, 0)) {
			t.Fatal("accepted revoked token")
		}
	}
	header := enc([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := enc([]byte(`{"access":"subscription","sub":"alice","iat":1700000000,"exp":1700000200}`))
	input := header + "." + claims
	mac = hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(input))
	token := input + "." + enc(mac.Sum(nil))
	p, err := Verify(token, secret, now)
	if err != nil || p.Username != "alice" {
		t.Fatal("JWT rejected", err)
	}
	if _, err := Verify(token, secret, time.Unix(1700000300, 0)); err == nil {
		t.Fatal("accepted expired JWT")
	}
}

func TestInvalidTokens(t *testing.T) {
	for _, token := range []string{"", "../admin", "eyJhbGciOiJub25lIn0.e30.", "not-a-token-at-all", "a.b.c.d"} {
		if _, err := Verify(token, "secret", time.Now()); err == nil {
			t.Fatal("accepted invalid token")
		}
	}
}

func TestDisplayTokenUsesImportedIdentityAndRevocationBoundary(t *testing.T) {
	created := time.Unix(1700000000, 500000000)
	revoked := created.Add(time.Hour)
	token, err := DisplayToken(123, created, revoked, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := Verify(token, "test-secret", revoked.Add(time.Hour))
	if err != nil || payload.UserID != 123 || !payload.ValidAfter(created, revoked) {
		t.Fatal("invalid display token", err)
	}
	again, _ := DisplayToken(123, created, revoked, "test-secret")
	if token != again {
		t.Fatal("display token changes between reads")
	}
	if _, err := Verify(token, "wrong", time.Now()); err == nil {
		t.Fatal("wrong signing key accepted")
	}
}
