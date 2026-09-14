// Package update resolves the latest published zenify release and tells the
// caller whether the running binary is behind. It is deliberately free of any
// dependency on internal/cli so both the session-start hook and the `update`
// subcommand can share it.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

// DefaultURL is the GitHub "latest release" page. GitHub answers it with a
// 302 to /releases/tag/<tag> — no auth, no API rate limit — so the tag is read
// from the Location header without following the redirect.
const DefaultURL = "https://github.com/ZenifyAIContactCenter/zenify-kit/releases/latest"

// CacheFile is the JSON file under the kit home (~/.zenify) that remembers the
// last check, so session-start touches the network at most once a day.
const CacheFile = "update-check.json"

const (
	freshTTL = 24 * time.Hour // a successful check is trusted for a day
	failTTL  = time.Hour      // a failed check backs off for an hour
)

// ErrBackoff is returned while a previous failed check is still inside its
// back-off window: nothing was fetched, nothing is known.
var ErrBackoff = errors.New("update: last check failed, backing off")

// Options drives Check. Zero values fall back: Client → http.DefaultClient,
// Now → time.Now, URL → DefaultURL, CacheDir "" → no cache at all.
type Options struct {
	Current  string
	CacheDir string
	URL      string
	Client   *http.Client
	Now      func() time.Time
	Force    bool // skip a valid cache and hit the network
}

// Result is what Check found. Err != nil always comes with Newer == false.
type Result struct {
	Current string
	Latest  string
	Newer   bool
	Err     error
}

type cacheEntry struct {
	CheckedAt time.Time `json:"checkedAt"`
	Latest    string    `json:"latest"`
}

// Latest GETs url without following redirects and returns the last path
// segment after "/tag/" of the Location header — the release tag.
func Latest(ctx context.Context, client *http.Client, url string) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) //nolint:gosec // G107 -- url is DefaultURL or the ZENIFY_UPDATE_URL test override, never user input
	if err != nil {
		return "", err
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck // nothing to do with a close error on a discarded body
	switch resp.StatusCode {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
	default:
		return "", fmt.Errorf("update: %s returned %d, expected a redirect", url, resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	i := strings.LastIndex(loc, "/tag/")
	if i < 0 {
		return "", fmt.Errorf("update: Location %q has no /tag/ segment", loc)
	}
	tag := strings.TrimSuffix(loc[i+len("/tag/"):], "/")
	if tag == "" {
		return "", fmt.Errorf("update: Location %q has an empty tag", loc)
	}
	return tag, nil
}

// Check answers "is a newer release out?" using the cache when it is fresh
// and the network otherwise. It never panics and never returns Newer=true
// alongside an error.
func Check(o Options) Result {
	res := Result{Current: o.Current}
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	if o.URL == "" {
		o.URL = DefaultURL
	}
	if o.Client == nil {
		o.Client = http.DefaultClient
	}

	if !o.Force {
		if c, ok := loadCache(o.CacheDir, now()); ok {
			if c.Latest == "" {
				res.Err = ErrBackoff
				return res
			}
			res.Latest = c.Latest
			res.Newer = version.Newer(c.Latest, o.Current)
			return res
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeoutOf(o.Client))
	defer cancel()
	latest, err := Latest(ctx, o.Client, o.URL)
	if werr := saveCache(o.CacheDir, cacheEntry{CheckedAt: now(), Latest: latest}); werr != nil && err == nil {
		err = werr
	}
	if err != nil {
		res.Err = err
		return res
	}
	res.Latest = latest
	res.Newer = version.Newer(latest, o.Current)
	return res
}

// timeoutOf mirrors the client's own Timeout into the request context so a
// Client without one still cannot hang the hook forever.
func timeoutOf(c *http.Client) time.Duration {
	if c != nil && c.Timeout > 0 {
		return c.Timeout
	}
	return 2 * time.Second
}

// loadCache returns the entry and ok=true only when it is still inside its TTL.
func loadCache(dir string, now time.Time) (cacheEntry, bool) {
	var c cacheEntry
	if dir == "" {
		return c, false
	}
	raw, err := os.ReadFile(filepath.Join(dir, CacheFile)) //nolint:gosec // G304 -- path is <kit home>/update-check.json, computed from the kit's own home, not user input
	if err != nil {
		return c, false
	}
	if err := json.Unmarshal(raw, &c); err != nil || c.CheckedAt.IsZero() {
		return c, false
	}
	ttl := freshTTL
	if c.Latest == "" {
		ttl = failTTL
	}
	if now.Sub(c.CheckedAt) >= ttl {
		return c, false
	}
	return c, true
}

func saveCache(dir string, c cacheEntry) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301 -- kit home dir, same mode the rest of the kit uses for it
		return err
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, CacheFile), raw, 0o644) //nolint:gosec // G306 -- non-secret cache (a version string + timestamp) under the kit's own home
}
