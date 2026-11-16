package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/uiverify"
	"github.com/spf13/cobra"
)

// realRunGit is the production uiverify.Deps.RunGit: `git -C dir <args...>`,
// returning RAW stdout (no TrimSpace — see the warning on uiverify.Deps.RunGit).
//
// Fingerprint's "hash-object --stdin" call passes the content to hash as the
// trailing arg after "--stdin" (Deps.RunGit has no separate stdin parameter),
// so that trailing arg is stripped out of argv here and piped to the git
// process's stdin instead of being passed as a CLI arg (which would be
// treated as a pathspec and produce the wrong hash).
func realRunGit(dir string, args ...string) (string, error) {
	cmdArgs := args
	var stdin *strings.Reader
	for i, a := range args {
		if a == "--stdin" && i+1 < len(args) {
			content := args[i+1]
			cmdArgs = append([]string{}, args[:i+1]...)
			stdin = strings.NewReader(content)
			break
		}
	}
	full := append([]string{"-C", dir}, cmdArgs...)
	cmd := exec.Command("git", full...) //nolint:gosec // G204 -- fixed "git"; args internally computed
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(errOut.String()))
	}
	return out.String(), nil
}

func defaultUIVerifyDeps() uiverify.Deps {
	return uiverify.Deps{RunGit: realRunGit, Now: time.Now}
}

func newUIVerifyCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "ui-verify",
		Short: "Ghi/kiểm tra bằng chứng UI-verify (layout screenshot + đo lường)", //znf:allow-lang
	}
	c.AddCommand(newUIVerifyRecordCmd())
	c.AddCommand(newUIVerifyCheckCmd())
	c.AddCommand(newUIVerifyBlockedCmd())
	c.AddCommand(newUIVerifyFilesCmd())
	return c
}

// newUIVerifyFilesCmd prints the files that require a UI verdict, one per line: the same
// set `check` gates on, so ship counts it instead of keeping its own copy of the pattern.
func newUIVerifyFilesCmd() *cobra.Command {
	var repo, base string
	var count bool
	c := &cobra.Command{
		// An error prints nothing on stdout, so a caller never reads usage text or a 0 as a count.
		SilenceUsage: true,
		Use:          "files",
		Short:        "In các file render UI đổi so với base (ship đếm để quyết định có cần ui-verifier)", //znf:allow-lang
		RunE: func(cmd *cobra.Command, _ []string) error {
			if repo == "" {
				repo, _ = os.Getwd()
			}
			if base == "" {
				return exitcode.New(exitcode.BadArgs, fmt.Errorf("need --base <base ref>"))
			}
			files, err := uiverify.RenderSet(defaultUIVerifyDeps(), repo, base)
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			if count {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), len(files))
				return err
			}
			for _, f := range files {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), f); err != nil {
					return err
				}
			}
			return nil
		},
	}
	c.Flags().StringVar(&repo, "repo", "", "repo path (mặc định cwd)")       //znf:allow-lang
	c.Flags().StringVar(&base, "base", "", "base ref để so sánh (bắt buộc)") //znf:allow-lang
	c.Flags().BoolVar(&count, "count", false, "print only the number of files (fails with no output on error)")
	return c
}

func newUIVerifyRecordCmd() *cobra.Command {
	var repo, screen, screenshot, verdict string
	var childRight, containerRight, paddingRight float64
	c := &cobra.Command{
		Use:   "record",
		Short: "Ghi lại artifact UI-verify cho fingerprint hiện tại", //znf:allow-lang
		RunE: func(cmd *cobra.Command, _ []string) error {
			if repo == "" {
				repo, _ = os.Getwd()
			}
			s := uiverify.Screen{
				Screen: screen,
				Measurement: uiverify.Measurement{
					ChildRight:     childRight,
					ContainerRight: containerRight,
					PaddingRight:   paddingRight,
				},
				Verdict: verdict,
			}
			return uiverify.Record(defaultUIVerifyDeps(), repo, s, screenshot)
		},
	}
	c.Flags().StringVar(&repo, "repo", "", "repo path (mặc định cwd)")                                    //znf:allow-lang
	c.Flags().StringVar(&screen, "screen", "", "tên screen/route (bắt buộc)")                             //znf:allow-lang
	c.Flags().StringVar(&screenshot, "screenshot", "", "đường dẫn ảnh chụp màn hình (bắt buộc)")          //znf:allow-lang
	c.Flags().Float64Var(&childRight, "child-right", 0, "child.right của element đã đổi (bắt buộc)")      //znf:allow-lang
	c.Flags().Float64Var(&containerRight, "container-right", 0, "right của container chứa nó (bắt buộc)") //znf:allow-lang
	c.Flags().Float64Var(&paddingRight, "padding-right", 0, "padding-right của container (bắt buộc)")     //znf:allow-lang
	c.Flags().StringVar(&verdict, "verdict", "pass", "pass|fail (fail never satisfies check)")            //znf:allow-lang
	_ = c.MarkFlagRequired("screen")
	_ = c.MarkFlagRequired("screenshot")
	_ = c.MarkFlagRequired("child-right")
	_ = c.MarkFlagRequired("container-right")
	_ = c.MarkFlagRequired("padding-right")
	return c
}

func newUIVerifyCheckCmd() *cobra.Command {
	var repo, base string
	c := &cobra.Command{
		Use:   "check",
		Short: "Kiểm tra artifact UI-verify còn khớp HEAD hay không", //znf:allow-lang
		RunE: func(cmd *cobra.Command, _ []string) error {
			if repo == "" {
				repo, _ = os.Getwd()
			}
			if base == "" {
				return exitcode.New(exitcode.BadArgs, fmt.Errorf("need --base <base ref>"))
			}
			state, msg, err := uiverify.Check(defaultUIVerifyDeps(), repo, base)
			uiOut(cmd).KV([][2]string{
				{"state", string(state)},
				{"msg", msg},
			})
			// The host reads ZNF_UI_STATUS_FILE as "UI unchecked"; once the repo
			// that wrote it resolves to any other state, that report is stale.
			if path := os.Getenv("ZNF_UI_STATUS_FILE"); path != "" && state != "" && !strings.HasPrefix(state, "infra-blocked: ") {
				if rerr := clearUIStatusFile(path, repo); rerr != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: cannot remove ZNF_UI_STATUS_FILE: %v\n", rerr)
				}
			}
			// Autonomous runs have no person to dispatch a ui-verifier, so a
			// missing verdict stops at a needs-human await (FR-08.4).
			if state == "required" && os.Getenv("ZNF_AUTONOMOUS") == "1" {
				id, aerr := awaitNow("needs-human", "cần kiểm UI: "+msg, "", "", "", nil) //znf:allow-lang
				if aerr != nil {
					return aerr
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), id)
				return exitcode.New(exitcode.Awaiting, fmt.Errorf("awaiting human: needs-human (%s)", id))
			}
			// The sandbox itself is broken: ship opens the PR with a warning
			// line instead of stopping (FR-4.2).
			if strings.HasPrefix(state, "infra-blocked: ") {
				if os.Getenv("ZNF_AUTONOMOUS") == "1" {
					return nil
				}
				return exitcode.New(exitcode.Fail, fmt.Errorf("%s", msg))
			}
			return err
		},
	}
	c.Flags().StringVar(&repo, "repo", "", "repo path (mặc định cwd)")       //znf:allow-lang
	c.Flags().StringVar(&base, "base", "", "base ref để so sánh (bắt buộc)") //znf:allow-lang
	return c
}

// blockedReasonRe is the reason shape the zenie host accepts (UI_BLOCKED_RE in
// its src/agent/results.ts); a reason outside it would be dropped there silently.
var blockedReasonRe = regexp.MustCompile(`^[a-z_]{1,40}(?::[A-Za-z0-9._-]{1,60})?$`)

// newUIVerifyBlockedCmd records that the sandbox infra is broken so no UI
// verdict is possible; `check` then reports infra-blocked instead of required.
func newUIVerifyBlockedCmd() *cobra.Command {
	var repo, reason string
	c := &cobra.Command{
		Use:   "blocked",
		Short: "Ghi nhận hạ tầng sandbox hỏng nên không kiểm UI được (chỉ autonomous)", //znf:allow-lang
		RunE: func(cmd *cobra.Command, _ []string) error {
			if os.Getenv("ZNF_AUTONOMOUS") != "1" {
				return exitcode.New(exitcode.Fail, fmt.Errorf("ui-verify blocked is only allowed under ZNF_AUTONOMOUS=1"))
			}
			reason = strings.TrimSpace(reason)
			if reason == "" {
				return exitcode.New(exitcode.BadArgs, fmt.Errorf("need --reason <code>"))
			}
			if !blockedReasonRe.MatchString(reason) {
				return exitcode.New(exitcode.BadArgs, fmt.Errorf("--reason %q: want <code> or <code>:<detail>, code [a-z_]{1,40}, detail [A-Za-z0-9._-]{1,60} (a route /deals/:id becomes deals-id)", reason))
			}
			if repo == "" {
				repo, _ = os.Getwd()
			}
			fp, err := uiverify.RecordBlocked(defaultUIVerifyDeps(), repo, reason)
			if err != nil {
				return err
			}
			if path := os.Getenv("ZNF_UI_STATUS_FILE"); path != "" {
				if werr := writeUIStatusFile(path, reason, repo, fp); werr != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: cannot write ZNF_UI_STATUS_FILE: %v\n", werr)
				}
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), fp)
			return nil
		},
	}
	c.Flags().StringVar(&repo, "repo", "", "repo path (mặc định cwd)")                            //znf:allow-lang
	c.Flags().StringVar(&reason, "reason", "", "mã lý do, vd deps_auth_failed:<repo> (bắt buộc)") //znf:allow-lang
	return c
}

// uiStatus is the ZNF_UI_STATUS_FILE JSON. The host reads state and reason;
// repo (absolute, clean) and fp say which tree it is about, since one run may
// check several repos.
type uiStatus struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
	Repo   string `json:"repo"`
	FP     string `json:"fp"`
}

// statusRepo is the form of a repo path stored in and compared against uiStatus.Repo.
func statusRepo(repo string) string {
	if abs, err := filepath.Abs(repo); err == nil {
		return abs
	}
	return filepath.Clean(repo)
}

// clearUIStatusFile removes path only when it parses and names repo; a file
// about another repo, an unparseable one or a missing one is left as is.
func clearUIStatusFile(path, repo string) error {
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- path is the operator-set ZNF_UI_STATUS_FILE
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var st uiStatus
	if json.Unmarshal(b, &st) != nil || st.Repo == "" || st.Repo != statusRepo(repo) {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) { //nolint:gosec // G703 -- path is ZNF_UI_STATUS_FILE, set by the host that runs this agent; the kit never derives it from repo content
		return err
	}
	return nil
}

// writeUIStatusFile writes the host-readable status JSON via a temp file in the
// same directory, then renames it so a reader never sees a partial file.
func writeUIStatusFile(path, reason, repo, fp string) error {
	b, err := json.Marshal(uiStatus{State: "infra-blocked", Reason: reason, Repo: statusRepo(repo), FP: fp})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ui-status-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name()) //nolint:gosec // G703 -- path is ZNF_UI_STATUS_FILE, set by the host that runs this agent; the kit never derives it from repo content
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name()) //nolint:gosec // G703 -- path is ZNF_UI_STATUS_FILE, set by the host that runs this agent; the kit never derives it from repo content
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil { //nolint:gosec // G703 -- path is ZNF_UI_STATUS_FILE, set by the host that runs this agent; the kit never derives it from repo content
		_ = os.Remove(tmp.Name()) //nolint:gosec // G703 -- path is ZNF_UI_STATUS_FILE, set by the host that runs this agent; the kit never derives it from repo content
		return err
	}
	return nil
}
