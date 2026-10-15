// Package auth issues and verifies broker JWTs, checks GitHub org membership
// and guards protected endpoints.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Audience and Lifetime are fixed by the contract.
const (
	Audience = "zenify-broker"
	Lifetime = 15 * time.Minute
)

// ErrJWTInvalid covers every reason a broker JWT is refused (bad shape,
// signature, audience or expiry); the caller answers 401 either way.
var ErrJWTInvalid = errors.New("invalid broker token")

var b64 = base64.RawURLEncoding

const jwtHeader = `{"alg":"HS256","typ":"JWT"}`

type claims struct {
	Sub string `json:"sub"`
	Aud string `json:"aud"`
	Exp int64  `json:"exp"`
	Iat int64  `json:"iat"`
}

func mac(key []byte, signingInput string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(signingInput))
	return m.Sum(nil)
}

// Issue signs an HS256 JWT for login, valid for Lifetime from now.
func Issue(key []byte, login string, now time.Time) (token string, exp time.Time) {
	exp = now.Add(Lifetime)
	payload, _ := json.Marshal(claims{Sub: login, Aud: Audience, Exp: exp.Unix(), Iat: now.Unix()})
	in := b64.EncodeToString([]byte(jwtHeader)) + "." + b64.EncodeToString(payload)
	return in + "." + b64.EncodeToString(mac(key, in)), exp
}

// Verify checks signature, alg, audience and expiry and returns the login.
func Verify(key []byte, token string, now time.Time) (login string, err error) {
	if len(key) == 0 {
		return "", ErrJWTInvalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", ErrJWTInvalid
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, mac(key, parts[0]+"."+parts[1])) {
		return "", ErrJWTInvalid
	}
	var h struct {
		Alg string `json:"alg"`
	}
	hb, err := b64.DecodeString(parts[0])
	if err != nil || json.Unmarshal(hb, &h) != nil || h.Alg != "HS256" {
		return "", ErrJWTInvalid
	}
	var c claims
	cb, err := b64.DecodeString(parts[1])
	if err != nil || json.Unmarshal(cb, &c) != nil {
		return "", ErrJWTInvalid
	}
	if c.Aud != Audience || c.Sub == "" || now.Unix() >= c.Exp {
		return "", ErrJWTInvalid
	}
	return c.Sub, nil
}
