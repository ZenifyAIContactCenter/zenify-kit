package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

var testKey = []byte("test-key-0123456789abcdef0123456")

var t0 = time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC)

// sign builds an HS256 JWT by hand, independent of Issue.
func sign(t *testing.T, key []byte, header, claims string) string {
	t.Helper()
	enc := base64.RawURLEncoding
	in := enc.EncodeToString([]byte(header)) + "." + enc.EncodeToString([]byte(claims))
	m := hmac.New(sha256.New, key)
	m.Write([]byte(in))
	return in + "." + enc.EncodeToString(m.Sum(nil))
}

func TestJWT_RoundTrip(t *testing.T) {
	tok, exp, _ := Issue(testKey, "octocat", "lead", t0)
	if !exp.Equal(t0.Add(15 * time.Minute)) {
		t.Errorf("exp = %v, want t0+15m", exp)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts", len(parts))
	}
	hb, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var h map[string]any
	if err := json.Unmarshal(hb, &h); err != nil || h["alg"] != "HS256" || h["typ"] != "JWT" {
		t.Errorf("header = %s", hb)
	}
	cb, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var c map[string]any
	if err := json.Unmarshal(cb, &c); err != nil {
		t.Fatal(err)
	}
	// 2026-10-15T08:00:00Z = 1792051200
	if c["sub"] != "octocat" || c["aud"] != "zenify-broker" || c["iat"] != float64(1792051200) || c["exp"] != float64(1792051200+900) {
		t.Errorf("claims = %s", cb)
	}
	login, _, err := Verify(testKey, tok, t0.Add(14*time.Minute))
	if err != nil || login != "octocat" {
		t.Errorf("Verify = %q, %v", login, err)
	}
	// A hand-built token with the same claims verifies too.
	hand := sign(t, testKey, `{"alg":"HS256","typ":"JWT"}`, `{"sub":"hubot","role":"dev","aud":"zenify-broker","exp":1792052100,"iat":1792051200}`)
	if login, _, err := Verify(testKey, hand, t0); err != nil || login != "hubot" {
		t.Errorf("Verify(hand) = %q, %v", login, err)
	}
}

func TestJWT_RejectsExpired(t *testing.T) {
	tok, _, _ := Issue(testKey, "octocat", "lead", t0)
	if _, _, err := Verify(testKey, tok, t0.Add(15*time.Minute)); err == nil {
		t.Error("token accepted at exp")
	}
	if _, _, err := Verify(testKey, tok, t0.Add(time.Hour)); err == nil {
		t.Error("token accepted after exp")
	}
}

func TestJWT_RejectsWrongAud(t *testing.T) {
	tok := sign(t, testKey, `{"alg":"HS256","typ":"JWT"}`, `{"sub":"octocat","aud":"other","exp":1792052100,"iat":1792051200}`)
	if _, _, err := Verify(testKey, tok, t0); err == nil {
		t.Error("wrong aud accepted")
	}
	noAud := sign(t, testKey, `{"alg":"HS256","typ":"JWT"}`, `{"sub":"octocat","exp":1792052100,"iat":1792051200}`)
	if _, _, err := Verify(testKey, noAud, t0); err == nil {
		t.Error("missing aud accepted")
	}
}

func TestJWT_RejectsBadSig(t *testing.T) {
	tok, _, _ := Issue(testKey, "octocat", "lead", t0)
	if _, _, err := Verify([]byte("other-key-0123456789abcdef012345678"), tok, t0); err == nil {
		t.Error("token signed with another key accepted")
	}
	parts := strings.Split(tok, ".")
	forged := sign(t, testKey, `{"alg":"HS256","typ":"JWT"}`, `{"sub":"admin","aud":"zenify-broker","exp":1792052100,"iat":1792051200}`)
	swapped := parts[0] + "." + strings.Split(forged, ".")[1] + "." + parts[2]
	if _, _, err := Verify(testKey, swapped, t0); err == nil {
		t.Error("payload swap accepted")
	}
	none := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + parts[1] + "."
	if _, _, err := Verify(testKey, none, t0); err == nil {
		t.Error("alg=none accepted")
	}
	for _, s := range []string{"", "a.b", "a.b.c.d", "!!.!!.!!"} {
		if _, _, err := Verify(testKey, s, t0); err == nil {
			t.Errorf("malformed %q accepted", s)
		}
	}
	if _, _, err := Verify(nil, tok, t0); err == nil {
		t.Error("empty key accepted")
	}
}

func TestIssue_RefusesShortKey(t *testing.T) {
	short := testKey[:MinKeyLen-1]
	if tok, _, err := Issue(short, "octocat", "lead", t0); !errors.Is(err, ErrKeyTooShort) || tok != "" {
		t.Fatalf("Issue(31-byte key) = %q, %v; want ErrKeyTooShort", tok, err)
	}
	hand := sign(t, short, `{"alg":"HS256","typ":"JWT"}`, `{"sub":"octocat","aud":"zenify-broker","exp":1792052100,"iat":1792051200}`)
	if _, _, err := Verify(short, hand, t0); err == nil {
		t.Fatal("Verify accepted a token under a 31-byte key")
	}
}

// A token issued before roles existed, or with a role the broker does not
// know, is refused.
func TestJWT_RejectsMissingOrUnknownRole(t *testing.T) {
	for name, claims := range map[string]string{
		"no role":      `{"sub":"octocat","aud":"zenify-broker","exp":1792052100,"iat":1792051200}`,
		"unknown role": `{"sub":"octocat","role":"admin","aud":"zenify-broker","exp":1792052100,"iat":1792051200}`,
	} {
		tok := sign(t, testKey, `{"alg":"HS256","typ":"JWT"}`, claims)
		if _, _, err := Verify(testKey, tok, t0); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, role, err := Verify(testKey, sign(t, testKey, `{"alg":"HS256","typ":"JWT"}`,
		`{"sub":"octocat","role":"dev","aud":"zenify-broker","exp":1792052100,"iat":1792051200}`), t0); err != nil || role != "dev" {
		t.Errorf("dev: role %q, %v", role, err)
	}
}
