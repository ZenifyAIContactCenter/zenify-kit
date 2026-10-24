package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/playwright"
	"github.com/spf13/cobra"
)

var (
	browserSessionRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	browserPathRe    = regexp.MustCompile(`(^|[^/\w.-])\.playwright-cli/`)
	// browserBlocked are subcommands that would touch every session or the shared install.
	browserBlocked = map[string]bool{"close-all": true, "kill-all": true, "install": true}
)

// parseBrowserArgs extracts the session name and the pass-through args (session
// tokens removed). The subcommand is the first token that is not a session token, --raw or --json;
// any other dash token before it is refused (it could take a value and hide the subcommand).
func parseBrowserArgs(args []string) (session string, rest []string, sub string, err error) {
	have := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case strings.HasPrefix(a, "-s="), strings.HasPrefix(a, "--session="):
			_, session, _ = strings.Cut(a, "=")
			have = true
		case a == "-s" || a == "--session":
			if i+1 >= len(args) {
				return "", nil, "", fmt.Errorf("browser: %s thiếu tên phiên", a) //znf:allow-lang
			}
			i++
			session, have = args[i], true
		default:
			if sub == "" {
				switch {
				case a == "--raw" || a == "--json":
				case strings.HasPrefix(a, "-"):
					return "", nil, "", fmt.Errorf("browser: %q đứng trước lệnh con không được phép, chỉ nhận --raw và --json", a) //znf:allow-lang
				default:
					sub = a
				}
			}
			rest = append(rest, a)
		}
	}
	if !have {
		return "", nil, "", errors.New("browser: thiếu -s=<phiên>, ví dụ: zenify browser -s=<phiên> open <url>") //znf:allow-lang
	}
	return session, rest, sub, nil
}

// pathRewriter rewrites relative .playwright-cli/ paths to absolute ones, line by line.
type pathRewriter struct {
	w    io.Writer
	repl string
	buf  []byte
}

func (p *pathRewriter) Write(b []byte) (int, error) {
	p.buf = append(p.buf, b...)
	for {
		i := bytes.IndexByte(p.buf, '\n')
		if i < 0 {
			return len(b), nil
		}
		if err := p.emit(p.buf[:i+1]); err != nil {
			return 0, err
		}
		p.buf = p.buf[i+1:]
	}
}

func (p *pathRewriter) flush() error {
	if len(p.buf) == 0 {
		return nil
	}
	err := p.emit(p.buf)
	p.buf = nil
	return err
}

func (p *pathRewriter) emit(line []byte) error {
	_, err := io.WriteString(p.w, browserPathRe.ReplaceAllString(string(line), p.repl))
	return err
}

// runBrowser implements `browser -s=<session> <subcommand> …` and returns the exit code.
func runBrowser(args []string, home string, stdout, stderr io.Writer, run func(c *exec.Cmd) error) int {
	session, rest, sub, err := parseBrowserArgs(args)
	if err == nil && !browserSessionRe.MatchString(session) {
		err = fmt.Errorf("browser: tên phiên %q không hợp lệ, chỉ nhận chữ, số, _ và -", session) //znf:allow-lang
	}
	if err == nil && browserBlocked[sub] {
		err = fmt.Errorf("browser: lệnh %q bị chặn vì ảnh hưởng mọi phiên hoặc bản cài chung", sub) //znf:allow-lang
	}
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 2
	}
	if !playwright.CLIInstalled(home) {
		fmt.Fprintln(stderr, "browser: playwright-cli chưa cài, chạy `zenify up` trước") //znf:allow-lang
		return 1
	}
	sessDir := filepath.Join(playwright.SessionsDir(home), session)
	if err := os.MkdirAll(sessDir, 0o755); err != nil { //nolint:gosec // G301 -- session dir holds no secrets
		fmt.Fprintln(stderr, "browser:", err)
		return 1
	}
	c := exec.Command("node", append([]string{playwright.CLIScript(home), "-s=" + session}, rest...)...) //nolint:gosec // G204 -- fixed binary; script path from home
	c.Dir = sessDir
	c.Stdin = os.Stdin
	c.Stderr = stderr
	c.Env = []string{} // non-nil: nil would inherit the parent's
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if k != "NO_UPDATE_NOTIFIER" && !isE2EEnvKey(k) {
			c.Env = append(c.Env, kv)
		}
	}
	c.Env = append(c.Env, "NO_UPDATE_NOTIFIER=1")
	rw := &pathRewriter{w: stdout, repl: "${1}" + strings.ReplaceAll(sessDir, "$", "$$") + "/.playwright-cli/"}
	c.Stdout = rw
	runErr := run(c)
	_ = rw.flush()
	if runErr == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(runErr, &ee) {
		return ee.ExitCode()
	}
	fmt.Fprintln(stderr, "browser:", runErr)
	return 1
}

func isE2EEnvKey(k string) bool {
	for _, e := range e2eEnvKeys {
		if k == e {
			return true
		}
	}
	return false
}

func newBrowserCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "browser -s=<phiên> <lệnh> [args…]",                                              //znf:allow-lang
		Short:              "Điều khiển trình duyệt qua playwright-cli đã ghim, mỗi phiên một thư mục riêng", //znf:allow-lang
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			if code := runBrowser(args, home, cmd.OutOrStdout(), cmd.ErrOrStderr(), func(c *exec.Cmd) error { return c.Run() }); code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}
}
