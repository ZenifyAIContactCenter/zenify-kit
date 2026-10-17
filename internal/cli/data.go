package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/brokerclient"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/datarun"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/spf13/cobra"
)

// dataEnvNames are the six variables `data run` grants a child; any of them
// inherited from the parent is removed first so only the broker's values stay.
var dataEnvNames = []string{"MONGO_URL", "MYSQL_URL", "REDIS_URL", "LUMI_MYSQL_URL", "QDRANT_URL", "LUMI_APP_URL"}

// dataDeps are the process boundaries `zenify data run` touches; tests replace them.
type dataDeps struct {
	environ func() []string
	now     func() time.Time
}

func defaultDataDeps() dataDeps {
	return dataDeps{environ: os.Environ, now: time.Now}
}

func newDataCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "data",
		Short: "Chạy script dữ liệu với credential do broker cấp", //znf:allow-lang
	}
	c.AddCommand(defaultDataDeps().runCmd())
	return c
}

func dataErr(code int, format string, a ...any) error {
	return exitcode.New(code, fmt.Errorf("data run: "+format, a...))
}

func (d dataDeps) runCmd() *cobra.Command {
	var stg, prd, write, lumi bool
	c := &cobra.Command{
		Use:                   "run (--stg|--prd) [--write] [--lumi] -- <lệnh> [args…]",          //znf:allow-lang
		Short:                 "Chạy lệnh với credential DB từ broker, che giá trị trong output", //znf:allow-lang
		SilenceUsage:          true,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if stg == prd {
				return dataErr(exitcode.BadArgs, "cần đúng một trong --stg hoặc --prd") //znf:allow-lang
			}
			if cmd.ArgsLenAtDash() != 0 || len(args) == 0 {
				return dataErr(exitcode.BadArgs, "cần `--` rồi lệnh cần chạy") //znf:allow-lang
			}
			env := "stg"
			if prd {
				env = "prd"
			}
			return d.run(cmd, env, write, lumi, args)
		},
	}
	f := c.Flags()
	f.BoolVar(&stg, "stg", false, "staging credentials")
	f.BoolVar(&prd, "prd", false, "production credentials (lead only)")
	f.BoolVar(&write, "write", false, "write credentials; needs a matching dry-run first")
	f.BoolVar(&lumi, "lumi", false, "also vend the lumi-agent credentials")
	return c
}

func (d dataDeps) run(cmd *cobra.Command, env string, write, lumi bool, argv []string) error {
	bc, ok, err := brokerFromFlags("", "")
	if err != nil {
		return dataErr(exitcode.Fail, "%v", err)
	}
	if !ok {
		return dataErr(exitcode.BadArgs, "data run needs a broker")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return dataErr(exitcode.Fail, "%v", err)
	}
	ws := workspaceOrCwd("", cmd.ErrOrStderr())
	store := datarun.Store{Dir: filepath.Join(ws, ".zenify", "data-runs"), Now: d.now}
	key, err := datarun.Key(argv, cwd, openForKey)
	if err != nil {
		return dataErr(exitcode.Fail, "%v", err)
	}
	var recID string
	if write {
		recID, err = store.FindForWrite(key, env, lumi)
		if errors.Is(err, datarun.ErrNoDryRun) {
			return dataErr(exitcode.NeedsDryRun, "--write needs a dry-run of the same command first (within %d minutes); run: zenify data run --%s -- %s", //znf:allow-lang
				int(datarun.Window.Minutes()), env, strings.Join(argv, " "))
		}
		if err != nil {
			return dataErr(exitcode.Fail, "%v", err)
		}
	}

	mode := api.CredsModeRead
	if write {
		mode = api.CredsModeWrite
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), brokerCallTimeout)
	var resp api.CredsResponse
	err = bc.Post(ctx, "/creds", api.CredsRequest{Env: env, Mode: mode, Lumi: lumi}, &resp)
	cancel()
	if err != nil {
		var ae *brokerclient.APIError
		if errors.As(err, &ae) && ae.Code == api.CodeLeadOnly {
			home := bc.Home
			if home == "" {
				home, _ = os.UserHomeDir()
			}
			_, _, role, _ := brokerclient.CachedIdentity(home)
			return dataErr(exitcode.Fail, "prd needs role lead (yours: %s)", role)
		}
		err = brokerCallErr(err)
		return dataErr(exitcode.Code(err), "%v", err)
	}

	var secrets []string
	var granted []string
	for k, v := range resp.Values {
		if v == "" {
			continue
		}
		secrets = append(append(secrets, v), urlHosts(v)...)
		granted = append(granted, k+"="+v)
	}
	var base []string
	for _, kv := range d.environ() {
		name, _, _ := strings.Cut(kv, "=")
		if isProdKey(name, nil) || slices.Contains(dataEnvNames, name) {
			continue
		}
		base = append(base, kv)
	}

	out, errw := cmd.OutOrStdout(), cmd.ErrOrStderr()
	var logFile *os.File
	if write {
		logFile, err = openWriteLog(ws, cwd, env, argv, d.now())
		if err != nil {
			return dataErr(exitcode.Fail, "%v", err)
		}
		defer func() { _ = logFile.Close() }()
		lw := &lockedWriter{w: logFile}
		out, errw = io.MultiWriter(out, lw), io.MultiWriter(errw, lw)
	}
	if write {
		if err := store.MarkUsed(recID); err != nil {
			if errors.Is(err, datarun.ErrNoDryRun) {
				return dataErr(exitcode.NeedsDryRun, "the dry-run record was already used; run the dry-run again") //znf:allow-lang
			}
			return dataErr(exitcode.Fail, "%v", err)
		}
	}

	c := exec.Command(argv[0], argv[1:]...) //nolint:gosec // G204 -- the user's own command, run on purpose
	c.Env = append(base, granted...)
	c.Stdin = os.Stdin
	mo := devenv.NewMaskWriter(out, secrets)
	me := devenv.NewMaskWriter(errw, secrets)
	c.Stdout, c.Stderr = mo, me
	runErr := c.Run()
	_ = mo.Close()
	_ = me.Close()
	var ee *exec.ExitError
	code := 0
	switch {
	case errors.As(runErr, &ee):
		code = ee.ExitCode()
	case runErr != nil:
		return dataErr(exitcode.Fail, "%v", runErr)
	}
	if !write {
		_, login, _, _ := brokerclient.CachedIdentity(cacheHome(bc))
		if rerr := store.RecordDryRun(key, env, lumi, login, code); rerr != nil && code == 0 {
			return dataErr(exitcode.Fail, "cannot record the dry-run: %v", rerr) //znf:allow-lang
		}
	}
	if code != 0 {
		return exitcode.New(code, fmt.Errorf("data run: command exited with code %d", code))
	}
	return nil
}

// openForKey is the datarun.Key seam: it opens each argv file the key hashes.
func openForKey(p string) (io.ReadCloser, error) {
	return os.Open(p) //nolint:gosec // G304 -- a file the user named in their own command
}

func cacheHome(bc *brokerclient.Client) string {
	if bc.Home != "" {
		return bc.Home
	}
	h, _ := os.UserHomeDir()
	return h
}

// lockedWriter serialises the stdout and stderr copies into one log file.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// openWriteLog creates the --write log (FR-4.4): next to the script when it
// lives under <ws>/docs/scripts/<repo>/, else under <ws>/.zenify/data-runs/.
func openWriteLog(ws, cwd, env string, argv []string, now time.Time) (*os.File, error) {
	dir, base := filepath.Join(ws, ".zenify", "data-runs"), "run"
	scripts := filepath.Join(ws, "docs", "scripts")
	for _, a := range argv {
		p := a
		if !filepath.IsAbs(p) {
			p = filepath.Join(cwd, p)
		}
		if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		base = filepath.Base(p)
		if rel, err := filepath.Rel(scripts, filepath.Clean(p)); err == nil && !strings.HasPrefix(rel, "..") && filepath.Dir(rel) != "." {
			dir = filepath.Join(scripts, strings.Split(rel, string(filepath.Separator))[0])
		}
		break
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301 -- docs/scripts is a shared, readable workspace dir
		return nil, err
	}
	name := fmt.Sprintf("%s.%s.%s.write.log", base, now.UTC().Format("20060102T150405Z"), env)
	return os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304 -- path built from the workspace and a script basename
}
