package tui

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/huh"
)

var errNotFound = errors.New("gh not found")

func detectGH() error { return detectGHWith(exec.LookPath) }

func detectGHWith(lookPath func(string) (string, error)) error {
	if _, err := lookPath("gh"); err != nil {
		return errors.New("GitHub CLI (gh) not found — install: https://cli.github.com then re-run `zenify up`")
	}
	return nil
}

func detectGit() error { return detectGitWith(exec.LookPath) }

func detectGitWith(lookPath func(string) (string, error)) error {
	if _, err := lookPath("git"); err != nil {
		return errors.New("git không tìm thấy — cài Xcode Command Line Tools (`xcode-select --install`) hoặc git rồi chạy lại `zenify up`")
	}
	return nil
}

// ghGuideErr là lỗi guide-only khi thiếu gh (giữ nguyên thông điệp P1).
var ghGuideErr = errors.New("GitHub CLI (gh) not found — install: https://cli.github.com then re-run `zenify up`")

// offerInstallGH chạy khi gh thiếu ở chế độ interactive: hỏi cài qua Homebrew,
// nếu có brew và user đồng ý thì chạy InstallRunner rồi re-detect. Mọi đường
// khác (không brew / từ chối) trả ghGuideErr.
func offerInstallGH(cfg OnboardConfig) error {
	lookPath := cfg.lookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if _, err := lookPath("brew"); err != nil {
		return ghGuideErr // không có brew → không tự cài được
	}

	confirm := cfg.ConfirmFn
	if confirm == nil {
		confirm = huhConfirm
	}
	ok, err := confirm("gh chưa có. Cài gh qua Homebrew ngay?")
	if err != nil {
		return err
	}
	if !ok {
		return ghGuideErr
	}

	run := cfg.InstallRunner
	if run == nil {
		run = brewInstall
	}
	if err := run("gh"); err != nil {
		return err
	}

	detect := cfg.DetectGHFn
	if detect == nil {
		detect = detectGH
	}
	return detect() // re-detect sau khi cài
}

// huhConfirm là ConfirmFn thật dùng huh.
func huhConfirm(prompt string) (bool, error) {
	v := false
	err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title(prompt).Value(&v),
	)).Run()
	return v, err
}

// brewInstall là InstallRunner thật: chạy `brew install <tool>` với stdio bám
// terminal để user thấy tiến trình.
func brewInstall(tool string) error {
	cmd := exec.Command("brew", "install", tool)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
	"i/o timeout",
	"context deadline exceeded",
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
