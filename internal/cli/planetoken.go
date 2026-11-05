package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
)

// planeTokenDeps are the boundaries askPlanePAT touches; tests replace them.
type planeTokenDeps struct {
	ask   func(prompt string) (string, error) // masked line from the terminal
	open  func(url string) error              // the token page in a browser
	check func(base, token string) (owner string, err error)
}

func defaultPlaneTokenDeps() planeTokenDeps {
	return planeTokenDeps{
		ask: func(prompt string) (string, error) {
			_, _ = fmt.Fprint(os.Stdout, prompt)
			b, err := term.ReadPassword(int(os.Stdin.Fd()))
			_, _ = fmt.Fprintln(os.Stdout)
			return string(b), err
		},
		open: func(url string) error {
			name := "xdg-open"
			if runtime.GOOS == "darwin" {
				name = "open"
			}
			return exec.Command(name, url).Start() //nolint:gosec // G204 -- fixed opener; url is the manifest's Plane URL plus a fixed path
		},
		check: checkPlanePAT,
	}
}

// planeTokenURL is the Plane CE page that creates a personal access token
// (apps/web/app/routes/redirects/core/api-tokens.tsx at v1.4.2).
func planeTokenURL(base string) string {
	return strings.TrimRight(base, "/") + "/settings/profile/api-tokens/"
}

// planeHTTP never follows a redirect: Go keeps X-API-Key on a redirect to
// another host, so a 3xx is reported instead.
var planeHTTP = &http.Client{
	Timeout:       10 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// errPlaneRejected is Plane refusing the token itself; any other check error
// (network, 5xx, odd answer) is not the token's fault.
var errPlaneRejected = errors.New("plane từ chối token này") //znf:allow-lang

// checkPlanePAT asks Plane whose token this is. Plane CE reads the PAT from
// X-API-Key and answers 403 for a bad one (measured on 1.4.2).
func checkPlanePAT(base, token string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(base, "/")+"/api/v1/users/me/", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-API-Key", token)
	resp, err := planeHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("không gọi được Plane: %w", err) //znf:allow-lang
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return "", errPlaneRejected
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("plane trả %s", resp.Status) //znf:allow-lang
	}
	var me struct {
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&me); err != nil {
		return "", fmt.Errorf("plane trả dữ liệu lạ: %w", err) //znf:allow-lang
	}
	if strings.TrimSpace(me.Email) == "" {
		return "", errors.New("plane trả dữ liệu lạ: thiếu email") //znf:allow-lang
	}
	return strings.TrimSpace(me.DisplayName + " <" + me.Email + ">"), nil
}

// askPlanePAT opens the token page, takes the pasted token (two tries), checks
// it with Plane and saves it to <ws>/.zenify/.env. An empty answer skips; only
// a token Plane rejects is asked again, any other check error stops asking.
func askPlanePAT(w io.Writer, ws, base string, d planeTokenDeps) bool {
	link := planeTokenURL(base)
	_, _ = fmt.Fprintf(w, "zenify-plane: cần Plane token của bạn. Mở %s → Add personal access token, copy rồi dán vào đây (Enter để bỏ qua)\n", link) //znf:allow-lang
	_ = d.open(link)
	for range 2 {
		tok, err := d.ask("Plane token: ")
		tok = strings.TrimSpace(tok)
		if err != nil || tok == "" {
			return false
		}
		owner, err := d.check(base, tok)
		if errors.Is(err, errPlaneRejected) {
			_, _ = fmt.Fprintf(w, "zenify-plane: %v\n", err)
			continue
		}
		if err != nil {
			_, _ = fmt.Fprintf(w, "zenify-plane: %v; chạy lại zenify up --apply sau\n", err) //znf:allow-lang
			return false
		}
		path := devenv.Path(ws)
		f, err := devenv.Load(path)
		if err == nil {
			f.Set(planePATKey, tok)
			err = f.Save(path)
		}
		if err != nil {
			_, _ = fmt.Fprintf(w, "zenify-plane: không lưu được token: %v\n", err) //znf:allow-lang
			return false
		}
		_, _ = fmt.Fprintf(w, "zenify-plane: đã nhận token của %s, lưu vào %s\n", owner, path) //znf:allow-lang
		return true
	}
	return false
}

// planePATSetFor is the patSet up hands registerPlaneMCP: the env/file lookup,
// and, in a real terminal, askPlanePAT when that finds nothing.
func planePATSetFor(w io.Writer, ws, base string, interactive bool, d planeTokenDeps) func() bool {
	return func() bool {
		if planePATSet(os.Getenv, ws) {
			return true
		}
		return interactive && askPlanePAT(w, ws, base, d)
	}
}
