package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
)

// WhereConfig drives the "where does the workspace live" step (FR-3). The TUI
// holds no resolution logic: cli hands in the cwd, the pointer target (if
// any), the OS default and the validator; SelectFn/InputFn are test seams
// over huh.
type WhereConfig struct {
	Cwd        string
	Existing   string // pointer target; "" when no pointer
	OSDefault  string
	Validate   func(dir string) error
	Accessible bool
	SelectFn   func(title string, opts []huh.Option[string]) (string, error)
	InputFn    func(title, placeholder string, validate func(string) error) (string, error)
}

// WhereResult is the chosen workspace and, optionally, the parent directory of
// clones the user already has (FR-3.5). SourcesDir "" means none.
type WhereResult struct {
	Workspace  string
	SourcesDir string
}

// RunWhere asks where to put the workspace (FR-3.2/3.3), validating with
// cfg.Validate (FR-3.4), then asks for a directory of existing clones (FR-3.5).
func RunWhere(cfg WhereConfig) (WhereResult, error) {
	sel, in := cfg.SelectFn, cfg.InputFn
	if sel == nil {
		sel = func(title string, opts []huh.Option[string]) (string, error) { return huhSelect(title, opts, cfg.Accessible) }
	}
	if in == nil {
		in = func(title, ph string, v func(string) error) (string, error) { return huhInput(title, ph, v, cfg.Accessible) }
	}
	cwdOK := cfg.Validate(cfg.Cwd) == nil

	var opts []huh.Option[string]
	if cfg.Existing != "" {
		opts = append(opts, huh.NewOption("Dùng workspace đang có: "+cfg.Existing, "existing")) //znf:allow-lang
		if cwdOK {
			opts = append(opts, huh.NewOption("Tạo workspace mới tại đây: "+cfg.Cwd, "cwd")) //znf:allow-lang
		}
	} else {
		if cwdOK {
			opts = append(opts, huh.NewOption("Thư mục hiện tại: "+cfg.Cwd, "cwd")) //znf:allow-lang
		}
		opts = append(opts,
			huh.NewOption("Mặc định: "+cfg.OSDefault, "default"), //znf:allow-lang
			huh.NewOption("Đường dẫn khác…", "custom"),           //znf:allow-lang
		)
	}
	choice, err := sel("Đặt workspace Zenify ở đâu?", opts) //znf:allow-lang
	if err != nil {
		return WhereResult{}, err
	}
	var ws string
	switch choice {
	case "existing":
		ws = cfg.Existing
	case "cwd":
		ws = cfg.Cwd
	case "default":
		ws = cfg.OSDefault
	case "custom":
		ws, err = in("Đường dẫn workspace", cfg.OSDefault, cfg.Validate) //znf:allow-lang
		if err != nil {
			return WhereResult{}, err
		}
	default:
		return WhereResult{}, fmt.Errorf("unknown choice %q", choice)
	}
	if choice != "existing" {
		if err := cfg.Validate(ws); err != nil {
			return WhereResult{}, err
		}
	}
	src, err := in("Bạn đã clone repo Zenify nào sẵn chưa? Nhập thư mục cha (bỏ trống nếu chưa)", "", func(string) error { return nil }) //znf:allow-lang
	if err != nil {
		return WhereResult{}, err
	}
	return WhereResult{Workspace: ws, SourcesDir: strings.TrimSpace(src)}, nil
}

func huhSelect(title string, opts []huh.Option[string], accessible bool) (string, error) {
	if len(opts) == 0 {
		return "", errors.New("no options")
	}
	v := opts[0].Value
	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title(title).Options(opts...).Value(&v),
	)).WithAccessible(accessible).Run()
	return v, err
}

func huhInput(title, placeholder string, validate func(string) error, accessible bool) (string, error) {
	var v string
	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title(title).Placeholder(placeholder).Value(&v).Validate(func(s string) error {
			if strings.TrimSpace(s) == "" && placeholder == "" {
				return nil // optional field
			}
			return validate(strings.TrimSpace(s))
		}),
	)).WithAccessible(accessible).Run()
	return v, err
}
