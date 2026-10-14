package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/devenv"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/manifest"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var envKeyName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// envDeps are the process boundaries `zenify env` touches; tests replace them.
type envDeps struct {
	workspace func() (string, bool)
	manifest  func(workspace string) (*manifest.Manifest, error)
	isTTY     func() bool
	getenv    func(string) string
	environ   func() []string
	prompt    func([]string) (map[string]string, error) // nil → the real masked form
	pick      func([]string) (string, error)            // nil → the real key picker
	editor    func(path string) error                   // nil → $EDITOR
}

func defaultEnvDeps() envDeps {
	return envDeps{
		workspace: func() (string, bool) {
			cwd, _ := os.Getwd()
			ws, _, ok := resolveWorkspace(cwd, "", os.Getenv, os.UserHomeDir)
			return ws, ok
		},
		manifest: func(ws string) (*manifest.Manifest, error) {
			m, _, err := loadKitManifest("", filepath.Join(ws, ".zenify-overlay.yaml"))
			return m, err
		},
		isTTY:   func() bool { return term.IsTerminal(int(os.Stdin.Fd())) },
		getenv:  os.Getenv,
		environ: os.Environ,
		pick:    tui.PickKey,
		editor:  runEditor,
	}
}

var envLong = strings.Join([]string{
	"Biến môi trường cá nhân cho tool dev và agent (db-read, e2e, visual, MCP). Đây không phải env chạy app.", //znf:allow-lang
	"Lưu ở <workspace>/.zenify/.env, quyền 0600; env hiện có luôn thắng giá trị trong file.",                  //znf:allow-lang
	"",
	"  zenify env                 form ẩn: key toàn cục còn trống, rồi chọn key bất kỳ để điền hoặc sửa", //znf:allow-lang
	"  zenify env list            tên, có/trống, nguồn, mô tả; chỉ in giá trị của key không phải secret", //znf:allow-lang
	"  zenify env set KEY         nhập ẩn một key (--desc ghi dòng mô tả)",                               //znf:allow-lang
	"  zenify env edit            mở file bằng $EDITOR",                                                  //znf:allow-lang
	"  zenify env exec -- <lệnh>  chạy lệnh với key trong file, che giá trị secret trong output",         //znf:allow-lang
}, "\n")

func newEnvCmd() *cobra.Command { return newEnvCmdWith(defaultEnvDeps()) }

func newEnvCmdWith(d envDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "env",
		Short:         "Biến môi trường cá nhân cho tool dev và agent (không phải env chạy app)", //znf:allow-lang
		Long:          envLong,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(cmd *cobra.Command, _ []string) error { return d.runForm() },
	}
	cmd.AddCommand(d.listCmd(), d.setCmd(), d.editCmd(), d.execCmd())
	return cmd
}

var errNeedTTY = errors.New("cần terminal thật để nhập giá trị ẩn") //znf:allow-lang

// load resolves the workspace, the manifest keys and the credential file.
func (d envDeps) load() (ws string, m *manifest.Manifest, f *devenv.File, err error) {
	ws, ok := d.workspace()
	if !ok {
		return "", nil, nil, errors.New("chưa có workspace (chạy zenify up trước)") //znf:allow-lang
	}
	if m, err = d.manifest(ws); err != nil {
		return "", nil, nil, err
	}
	if f, err = devenv.Load(devenv.Path(ws)); err != nil {
		return "", nil, nil, err
	}
	return ws, m, f, nil
}

// checkValue refuses a value that would inject another line into the file.
func checkValue(key, v string) error {
	if strings.ContainsAny(v, "\r\n") {
		return fmt.Errorf("giá trị của %s có ký tự xuống dòng, không ghi", key) //znf:allow-lang
	}
	return nil
}

// metaFor returns the manifest entry for name, or a secret placeholder for a local key.
func metaFor(keys []manifest.SecretKey, name string) manifest.SecretKey {
	for _, k := range keys {
		if k.Name == name {
			return k
		}
	}
	return manifest.SecretKey{Name: name, Secret: true}
}

// askValues runs the masked form for keys and writes every non-empty answer.
// It validates all answers before touching f.
func (d envDeps) askValues(keys []manifest.SecretKey, f *devenv.File) (int, error) {
	vals, err := tui.PromptSecrets(keys, d.prompt)
	if err != nil {
		return 0, err
	}
	for _, k := range keys {
		if err := checkValue(k.Name, vals[k.Name]); err != nil {
			return 0, err
		}
	}
	n := 0
	for _, k := range keys {
		if v := vals[k.Name]; v != "" {
			f.Set(k.Name, v)
			n++
		}
	}
	return n, nil
}

func (d envDeps) runForm() error {
	if !d.isTTY() {
		return errNeedTTY
	}
	ws, m, f, err := d.load()
	if err != nil {
		return err
	}
	var empty []manifest.SecretKey
	for _, k := range m.SecretKeys {
		if v, _ := f.Get(k.Name); v == "" {
			empty = append(empty, k)
		}
	}
	all := m.AllKeys()
	if len(empty) > 0 {
		if _, err := d.askValues(empty, f); err != nil {
			return err
		}
		if err := f.Save(devenv.Path(ws)); err != nil {
			return err
		}
	}
	pick := d.pick
	if pick == nil {
		pick = tui.PickKey
	}
	for {
		names := manifest.KeyNames(all)
		for _, k := range f.Keys() {
			if !contains(names, k) {
				names = append(names, k)
			}
		}
		name, err := pick(names)
		if err != nil {
			return err
		}
		if name == "" {
			return nil
		}
		if _, err := d.askValues([]manifest.SecretKey{metaFor(all, name)}, f); err != nil {
			return err
		}
		if err := f.Save(devenv.Path(ws)); err != nil {
			return err
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (d envDeps) listCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "list",
		Short:        "Liệt kê key: có/trống, nguồn, mô tả (không in giá trị secret)", //znf:allow-lang
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, m, f, err := d.load()
			if err != nil {
				return err
			}
			listEnv(cmd.OutOrStdout(), m.AllKeys(), f)
			return nil
		},
	}
}

// listEnv prints one line per key: NAME state source [value if not secret] description.
// A key in the file but not in keys is "local" and treated as secret.
func listEnv(w io.Writer, keys []manifest.SecretKey, f *devenv.File) {
	line := func(k manifest.SecretKey, source, desc string) {
		v, _ := f.Get(k.Name)
		state := "empty"
		if v != "" {
			state = "set"
		}
		cols := []string{k.Name, state, source}
		if !k.Secret && v != "" {
			cols = append(cols, v)
		}
		if desc != "" {
			cols = append(cols, desc)
		}
		_, _ = fmt.Fprintln(w, strings.Join(cols, "  "))
	}
	known := map[string]bool{}
	for _, k := range keys {
		known[k.Name] = true
		line(k, "manifest", k.Description)
	}
	for _, name := range f.Keys() {
		if !known[name] {
			line(manifest.SecretKey{Name: name, Secret: true}, "local", f.Desc(name))
		}
	}
}

func (d envDeps) setCmd() *cobra.Command {
	var desc string
	cmd := &cobra.Command{
		Use:          "set KEY",
		Short:        "Nhập ẩn giá trị một key vào .zenify/.env", //znf:allow-lang
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if !d.isTTY() {
				return errNeedTTY
			}
			if !envKeyName.MatchString(key) {
				return fmt.Errorf("tên key không hợp lệ: %q", key) //znf:allow-lang
			}
			ws, m, f, err := d.load()
			if err != nil {
				return err
			}
			n, err := d.askValues([]manifest.SecretKey{metaFor(m.AllKeys(), key)}, f)
			if err != nil {
				return err
			}
			if _, present := f.Get(key); n == 0 && !present {
				return fmt.Errorf("giá trị của %s trống, không ghi", key) //znf:allow-lang
			}
			if desc != "" {
				f.SetDesc(key, desc)
			}
			return f.Save(devenv.Path(ws))
		},
	}
	cmd.Flags().StringVar(&desc, "desc", "", "dòng mô tả ghi phía trên key") //znf:allow-lang
	return cmd
}

func (d envDeps) editCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "edit",
		Short:        "Mở .zenify/.env bằng $EDITOR", //znf:allow-lang
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			ws, ok := d.workspace()
			if !ok {
				return errors.New("chưa có workspace (chạy zenify up trước)") //znf:allow-lang
			}
			path := devenv.Path(ws)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				f, lerr := devenv.Load(path)
				if lerr != nil {
					return lerr
				}
				if err := f.Save(path); err != nil { // creates the file 0600
					return err
				}
			}
			ed := d.editor
			if ed == nil {
				ed = runEditor
			}
			return ed(path)
		},
	}
}

// runEditor opens path in $EDITOR (default vi) attached to the terminal.
func runEditor(path string) error {
	parts := strings.Fields(os.Getenv("EDITOR"))
	if len(parts) == 0 {
		parts = []string{"vi"}
	}
	c := exec.Command(parts[0], append(parts[1:], path)...) //nolint:gosec // G204 -- the user's own $EDITOR
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

func (d envDeps) execCmd() *cobra.Command {
	return &cobra.Command{
		Use:                   "exec -- <lệnh> [args…]",                                                //znf:allow-lang
		Short:                 "Chạy lệnh với key trong .zenify/.env, che giá trị secret trong output", //znf:allow-lang
		Args:                  cobra.MinimumNArgs(1),
		SilenceUsage:          true,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, m, f, err := d.load()
			if err != nil {
				return err
			}
			lookup := devenv.Lookup(d.getenv, f)
			public := map[string]bool{}
			for _, k := range m.AllKeys() {
				if !k.Secret {
					public[k.Name] = true
				}
			}
			var secrets []string
			for _, k := range f.Keys() {
				if public[k] {
					continue
				}
				if v := lookup(k); v != "" {
					secrets = append(secrets, v)
				}
			}
			c := exec.Command(args[0], args[1:]...) //nolint:gosec // G204 -- the user's own command, run on purpose
			c.Env = devenv.ChildEnv(d.environ(), f.Keys(), lookup)
			c.Stdin = os.Stdin
			out := devenv.NewMaskWriter(cmd.OutOrStdout(), secrets)
			errw := devenv.NewMaskWriter(cmd.ErrOrStderr(), secrets)
			c.Stdout, c.Stderr = out, errw
			runErr := c.Run()
			_ = out.Close()
			_ = errw.Close()
			var ee *exec.ExitError
			if errors.As(runErr, &ee) {
				return exitcode.New(ee.ExitCode(), fmt.Errorf("lệnh thoát với mã %d", ee.ExitCode())) //znf:allow-lang
			}
			return runErr
		},
	}
}
