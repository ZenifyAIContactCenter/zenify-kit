package tui

import (
	"errors"
	"os/exec"
	"strings"
)

var errNotFound = errors.New("gh not found")

func detectGH() error { return detectGHWith(exec.LookPath) }

func detectGHWith(lookPath func(string) (string, error)) error {
	if _, err := lookPath("gh"); err != nil {
		return errors.New("GitHub CLI (gh) not found — install: https://cli.github.com then re-run `zenify up`")
	}
	return nil
}

type authState int

const (
	authLoggedIn authState = iota
	authLoggedOut
	authUnreachable
)

// networkMarkers là các chuỗi xuất hiện trong output `gh auth status` khi
// máy không kết nối được GitHub (phân biệt với chưa-login).
var networkMarkers = []string{
	"could not connect",
	"dial tcp",
	"no such host",
	"connection refused",
	"timeout",
	"network is unreachable",
}

// parseAuthState phân loại output `gh auth status` thành 3 trạng thái. err là
// lỗi process của `gh auth status` (non-nil khi exit≠0). offline được nhận
// diện bằng err≠nil KÈM một marker mạng trong output; nếu không có marker thì
// coi như chưa-login (an toàn: đẩy về luồng login).
func parseAuthState(out string, err error) (account string, state authState) {
	lower := strings.ToLower(out)
	if err != nil {
		for _, m := range networkMarkers {
			if strings.Contains(lower, m) {
				return "", authUnreachable
			}
		}
	}
	if strings.Contains(out, "not logged in") || strings.Contains(out, "not logged into") {
		return "", authLoggedOut
	}
	// "Logged in to github.com account <name>"
	if i := strings.Index(out, "account "); i >= 0 {
		rest := out[i+len("account "):]
		if f := strings.Fields(rest); len(f) > 0 {
			return f[0], authLoggedIn
		}
	}
	if strings.Contains(out, "Logged in") {
		return "", authLoggedIn
	}
	return "", authLoggedOut
}

func ghAuthStatus() (string, authState) {
	out, err := exec.Command("gh", "auth", "status").CombinedOutput()
	return parseAuthState(string(out), err)
}

func loginCmd() *exec.Cmd { return exec.Command("gh", "auth", "login", "--web") }
