package e2e

import (
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// be's PASSWORD_PATTERN `^(?=.*[a-z])(?=.*[A-Z])(?=.*[0-9])(?=.*[^A-Za-z0-9\s])\S{8,64}$`,
// split into its lookaheads because RE2 has none: all five must match.
var policy = []*regexp.Regexp{
	regexp.MustCompile(`[a-z]`),
	regexp.MustCompile(`[A-Z]`),
	regexp.MustCompile(`[0-9]`),
	regexp.MustCompile(`[^A-Za-z0-9\s]`),
	regexp.MustCompile(`^\S{8,64}$`),
}

// prefixShape is what the HMAC part must be: 20 base64url characters with
// '-' and '_' removed, so every policy class comes from passwordSuffix alone.
var prefixShape = regexp.MustCompile(`^[A-Za-z0-9]{20}$`)

func TestPassword_MatchesPolicy(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	key := []byte("k")
	for i := 0; i < 1000; i++ {
		login := "u" + strconv.Itoa(rng.Int())
		role := []string{"agent", "sale", "administrator"}[rng.Intn(3)]
		period := rng.Int63n(1000)
		p := Password(key, login, role, period)
		prefix, ok := strings.CutSuffix(p, passwordSuffix)
		if !ok || !prefixShape.MatchString(prefix) {
			t.Fatalf("Password(%q,%q,%d)=%q: want 20 alphanumerics then %q", login, role, period, p, passwordSuffix)
		}
		for j, re := range policy {
			if !re.MatchString(p) {
				t.Fatalf("Password(%q,%q,%d)=%q fails policy part %d (%s)", login, role, period, p, j, re)
			}
		}
		if p2 := Password(key, login, role, period); p2 != p {
			t.Fatalf("not deterministic: %q vs %q", p, p2)
		}
	}
}

func TestPassword_KnownValue(t *testing.T) {
	// openssl: printf 'alice:agent:225' | openssl dgst -sha256 -hmac testkey -binary | base64 | tr -d '+/=' | cut -c1-20
	if got, want := Password([]byte("testkey"), "alice", "agent", 225), "mWPKXjzU3rQTQLy9J7kPAa1!"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if Password([]byte("testkey"), "alice", "agent", 226) == Password([]byte("testkey"), "alice", "agent", 225) {
		t.Fatal("period must change the password")
	}
}

func TestPeriod(t *testing.T) {
	// date -u -j -f '%Y-%m-%d %H:%M:%S' '2026-10-15 12:00:00' +%s = 1792065600; /86400/90 = 230
	if got := Period(time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)); got != 230 {
		t.Fatalf("got %d want 230", got)
	}
}
