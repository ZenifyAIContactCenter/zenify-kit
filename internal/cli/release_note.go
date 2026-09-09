package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/gitx"
	"github.com/spf13/cobra"
)

// buildNoteMessage builds the note-commit message (pure, testable). Tags MIRROR the format
// read by internal/release/speclink.go + internal/analyze. Empty → safe default.
func buildNoteMessage(slug, note, blast, db, rollback, spec string) string {
	if blast == "" {
		blast = "unknown"
	}
	if db == "" {
		db = "N/A"
	}
	if rollback == "" {
		rollback = "revert PR"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "chore(release): note %s\n\n", slug)
	fmt.Fprintf(&b, "_Release-Slug: %s\n", slug)
	fmt.Fprintf(&b, "_Release-Note: %s\n", note)
	fmt.Fprintf(&b, "_Blast-radius: %s\n", blast)
	fmt.Fprintf(&b, "_DB: %s\n", db)
	fmt.Fprintf(&b, "_Rollback: %s\n", rollback)
	if spec != "" {
		fmt.Fprintf(&b, "Spec: %s\n", spec)
	}
	return b.String()
}

// runReleaseNote creates an empty note-commit carrying the trailer. FAIL-OPEN: any error → note to stderr, return nil.
func runReleaseNote(dir, slug, note, blast, db, rollback, spec string, r gitx.Runner, stdout, stderr io.Writer) error {
	if slug == "" {
		fmt.Fprintln(stderr, "release-note: thiếu --slug (fail-open, bỏ qua)") //znf:allow-lang
		return nil
	}
	msg := buildNoteMessage(slug, note, blast, db, rollback, spec)
	if _, err := r.Run(dir, "commit", "--allow-empty", "-m", msg); err != nil {
		fmt.Fprintf(stderr, "release-note: commit lỗi: %v (fail-open)\n", err) //znf:allow-lang
		return nil
	}
	fmt.Fprintf(stdout, "release-note: đã ghi note-commit cho %q\n", slug) //znf:allow-lang
	return nil
}

func newReleaseNoteCmd() *cobra.Command {
	var dir, slug, note, blast, db, rollback, spec string
	cmd := &cobra.Command{
		Use:   "release-note",
		Short: "ghi note-commit release (trailer risk-metadata) — dùng bởi /ship", //znf:allow-lang
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				dir, _ = os.Getwd()
			}
			return runReleaseNote(dir, slug, note, blast, db, rollback, spec,
				gitx.ExecRunner(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "repo dir (mặc định cwd)")    //znf:allow-lang
	cmd.Flags().StringVar(&slug, "slug", "", "slug thay đổi (bắt buộc)") //znf:allow-lang
	cmd.Flags().StringVar(&note, "note", "", "mô tả một dòng")           //znf:allow-lang
	cmd.Flags().StringVar(&blast, "blast", "", "_Blast-radius (default 'unknown')")
	cmd.Flags().StringVar(&db, "db", "", "_DB (default 'N/A')")
	cmd.Flags().StringVar(&rollback, "rollback", "", "_Rollback (default 'revert PR')")
	cmd.Flags().StringVar(&spec, "spec", "", "path spec (tuỳ chọn)") //znf:allow-lang
	return cmd
}
