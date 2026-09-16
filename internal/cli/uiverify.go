package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
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
	c.Flags().StringVar(&verdict, "verdict", "pass", "pass|fail")                                         //znf:allow-lang
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
			out := cmd.OutOrStdout()
			state, msg, err := uiverify.Check(defaultUIVerifyDeps(), repo, base)
			fmt.Fprintf(out, "state: %s\n", state)
			fmt.Fprintf(out, "msg: %s\n", msg)
			return err
		},
	}
	c.Flags().StringVar(&repo, "repo", "", "repo path (mặc định cwd)")       //znf:allow-lang
	c.Flags().StringVar(&base, "base", "", "base ref để so sánh (bắt buộc)") //znf:allow-lang
	return c
}
