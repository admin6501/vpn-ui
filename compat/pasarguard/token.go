// Package pasarguard validates existing PasarGuard subscription credentials.
package pasarguard

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

var ErrToken = errors.New("invalid subscription token")

type Payload struct {
	UserID   int
	Username string
	IssuedAt time.Time
}

func decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

// Verify accepts the legacy SHA256 formats, HS256 subscription JWTs and v3 HMAC.
// The signing secret must be imported from the original database, never replaced.
func Verify(token, secret string, now time.Time) (Payload, error) {
	if secret == "" || len(token) < 15 || len(token) > 8192 {
		return Payload{}, ErrToken
	}
	parts := strings.Split(token, ".")
	if len(parts) == 3 {
		header, err := decode(parts[0])
		if err != nil {
			return Payload{}, ErrToken
		}
		var h struct {
			Alg string `json:"alg"`
		}
		if json.Unmarshal(header, &h) != nil || h.Alg != "HS256" {
			return Payload{}, ErrToken
		}
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(parts[0] + "." + parts[1]))
		signature, err := decode(parts[2])
		if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
			return Payload{}, ErrToken
		}
		data, err := decode(parts[1])
		if err != nil {
			return Payload{}, ErrToken
		}
		var claims struct {
			Access string `json:"access"`
			Sub    string `json:"sub"`
			Iat    int64  `json:"iat"`
			Exp    *int64 `json:"exp"`
			Nbf    *int64 `json:"nbf"`
		}
		if json.Unmarshal(data, &claims) != nil || claims.Access != "subscription" || claims.Sub == "" || claims.Iat <= 0 || claims.Iat > now.Unix() || (claims.Exp != nil && now.Unix() >= *claims.Exp) || (claims.Nbf != nil && now.Unix() < *claims.Nbf) {
			return Payload{}, ErrToken
		}
		return Payload{Username: claims.Sub, IssuedAt: time.Unix(claims.Iat, 0)}, nil
	}
	var body string
	if len(parts) == 2 {
		body = parts[0]
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(body))
		signature, err := decode(parts[1])
		if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
			return Payload{}, ErrToken
		}
	} else if len(parts) == 1 {
		body = token[:len(token)-10]
		signature := token[len(token)-10:]
		hash := sha256.Sum256([]byte(body + secret))
		legacy := base64.RawURLEncoding.EncodeToString(hash[:])[:10]
		hexadecimal := hex.EncodeToString(hash[:])[:10]
		if !hmac.Equal([]byte(signature), []byte(legacy)) && !hmac.Equal([]byte(signature), []byte(hexadecimal)) {
			return Payload{}, ErrToken
		}
	} else {
		return Payload{}, ErrToken
	}
	data, err := decode(body)
	if err != nil {
		return Payload{}, ErrToken
	}
	fields := strings.Split(string(data), ",")
	p := Payload{}
	var timestamp string
	if len(fields) == 3 && (fields[0] == "v2" || fields[0] == "v3") {
		p.UserID, err = strconv.Atoi(fields[1])
		if err != nil || p.UserID <= 0 {
			return Payload{}, ErrToken
		}
		timestamp = fields[2]
	} else if len(fields) == 2 && fields[0] != "" {
		p.Username = fields[0]
		timestamp = fields[1]
	} else {
		return Payload{}, ErrToken
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || seconds <= 0 {
		return Payload{}, ErrToken
	}
	p.IssuedAt = time.Unix(seconds, 0)
	return p, nil
}

func (p Payload) ValidAfter(createdAt, revokedAt time.Time) bool {
	return !p.IssuedAt.Before(createdAt) && (revokedAt.IsZero() || !p.IssuedAt.Before(revokedAt))
}
