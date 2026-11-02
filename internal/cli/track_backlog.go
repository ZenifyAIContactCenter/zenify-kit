package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/trackstats"
	"github.com/spf13/cobra"
)

const (
	// trackGhTimeout bounds every gh and git call of the backlog sources; it
	// matches trackstats' ghTimeout (unexported there).
	trackGhTimeout = 10 * time.Second
	backlogTTL     = 5 * time.Minute
	backlogCache   = "backlog-gh.json"
)

// trackGhSearch lists the open issues of one owner for one role (assignee or
// author); a test replaces it at the gh boundary.
var trackGhSearch = ghSearch

var (
	trackBidRe = regexp.MustCompile(`^b-\d{6}-[0-9a-f]{4}$`)
	trackSDDRe = regexp.MustCompile(`^repos/.+/progress\.md:\d+$`)
	// A multi-repo plan prefixes each ledger line with its repo, e.g. `[be] Task 3: …`.
	sddMinorRe  = regexp.MustCompile(`^(?:\[[^\]]+\] )?Task \d+: minor \(deferred\): (.+)$`)
	sddParkedRe = regexp.MustCompile(`^(?:\[[^\]]+\] )?Task \d+: parked — (.+)$`)
	ownerRe     = regexp.MustCompile(`[:/]([^/:]+)/[^/]+?(?:\.git)?/?$`)
)

func newTrackBacklogCmd(ws *string) *cobra.Command {
	c := &cobra.Command{
		Use:   "backlog",
		Short: "Thêm hoặc bỏ một việc trong backlog của pane", //znf:allow-lang
	}
	c.AddCommand(newTrackBacklogAddCmd(ws), newTrackBacklogDropCmd(ws))
	return c
}

func newTrackBacklogAddCmd(ws *string) *cobra.Command {
	var repo, est, ref string
	c := &cobra.Command{
		Use:   `add "<title>" --repo <r> [--est S|M|L] [--ref <url|memory:<name>>]`,
		Short: "Thêm một việc thủ công vào backlog và in bid của nó", //znf:allow-lang
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			wsDir, on, err := trackWorkspace(*ws)
			if err != nil {
				return err
			}
			if !on {
				_, err = fmt.Fprintln(w, "track: off")
				return err
			}
			if len(args) != 1 {
				return badTrack("expected 1 argument (the title), got %d", len(args))
			}
			if err := validTrackText("title", args[0]); err != nil {
				return err
			}
			if err := validTrackText("--repo", repo); err != nil {
				return err
			}
			if err := oneOf("est", est, "S", "M", "L", "-"); err != nil {
				return err
			}
			if ref != "-" {
				if err := validBacklogRef(ref); err != nil {
					return err
				}
			}
			bid, err := unusedBacklogID(trackLogPath(wsDir), time.Now().UTC())
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			if err := appendTrackRow(trackLogPath(wsDir), "backlog", []string{bid, repo, est, ref, args[0]}); err != nil {
				return err
			}
			_, err = fmt.Fprintf(w, "track: %s\n", bid)
			return err
		},
	}
	c.Flags().StringVar(&repo, "repo", "", "target repo name")
	c.Flags().StringVar(&est, "est", "-", "size estimate S|M|L, or - when not known yet")
	c.Flags().StringVar(&ref, "ref", "-", "issue URL or memory:<name>")
	return c
}

func newTrackBacklogDropCmd(ws *string) *cobra.Command {
	return &cobra.Command{
		Use:   "drop <bid|sdd id>",
		Short: "Bỏ một việc khỏi backlog", //znf:allow-lang
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			wsDir, on, err := trackWorkspace(*ws)
			if err != nil {
				return err
			}
			if !on {
				_, err = fmt.Fprintln(w, "track: off")
				return err
			}
			if len(args) != 1 {
				return badTrack("expected 1 argument, got %d", len(args))
			}
			id := args[0]
			logPath := trackLogPath(wsDir)
			rows, err := readTrackRows(logPath)
			if err != nil {
				return err
			}
			switch {
			case trackBidRe.MatchString(id):
				if !rowsHave(rows, "backlog", id) {
					return badTrack("unknown backlog id %s", id)
				}
			case trackSDDRe.MatchString(id):
			default:
				return badTrack("%q is neither a backlog id nor an sdd id (repos/<...>/progress.md:<line>)", id)
			}
			if rowsHave(rows, "drop", id) {
				return badTrack("%s was already dropped", id)
			}
			if err := appendTrackRow(logPath, "drop", []string{id}); err != nil {
				return err
			}
			_, err = fmt.Fprintf(w, "track: dropped %s\n", id)
			return err
		},
	}
}

func trackLogPath(wsDir string) string { return filepath.Join(wsDir, ".zenify", "track", "log.md") }

func validBacklogRef(ref string) error {
	if strings.ContainsAny(ref, "\r\n") || strings.Contains(ref, trackSep) {
		return badTrack("invalid --ref %q", ref)
	}
	if strings.HasPrefix(ref, "http") || (strings.HasPrefix(ref, "memory:") && len(ref) > len("memory:")) {
		return nil
	}
	return badTrack("--ref must be an http URL or memory:<name>, got %q", ref)
}

func newBacklogID(now time.Time) (string, error) {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "b-" + now.Format("060102") + "-" + hex.EncodeToString(b[:]), nil
}

// unusedBacklogID draws bids until one has no backlog row in the log, so a drop or a
// start --from never hides a second item with the same bid.
func unusedBacklogID(logPath string, now time.Time) (string, error) {
	rows, err := readTrackRows(logPath)
	if err != nil {
		return "", err
	}
	for range 16 {
		bid, err := newBacklogID(now)
		if err != nil {
			return "", err
		}
		if !rowsHave(rows, "backlog", bid) {
			return bid, nil
		}
	}
	return "", fmt.Errorf("no unused backlog id left for %s after 16 draws", now.Format("060102"))
}

// readTrackRows returns the log lines split into fields; a missing log is empty.
func readTrackRows(logPath string) ([][]string, error) {
	f, err := os.Open(logPath) //nolint:gosec // G304 -- computed from the resolved workspace
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, exitcode.New(exitcode.Fail, err)
	}
	defer f.Close() //nolint:errcheck // read-only
	var rows [][]string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		rows = append(rows, strings.Split(sc.Text(), trackSep))
	}
	if err := sc.Err(); err != nil {
		return nil, exitcode.New(exitcode.Fail, err)
	}
	return rows, nil
}

func rowsHave(rows [][]string, kind, id string) bool {
	for _, p := range rows {
		if len(p) >= 3 && p[1] == kind && p[2] == id {
			return true
		}
	}
	return false
}

// checkStartFrom validates a `start --from` value against the log.
func checkStartFrom(logPath, from string) error {
	if strings.HasPrefix(from, "http") {
		if strings.ContainsAny(from, " \r\n") || strings.Contains(from, trackSep) {
			return badTrack("invalid --from %q", from)
		}
		return nil
	}
	if !trackBidRe.MatchString(from) {
		return badTrack("--from must be a backlog id or an http URL, got %q", from)
	}
	rows, err := readTrackRows(logPath)
	if err != nil {
		return err
	}
	if !rowsHave(rows, "backlog", from) {
		return badTrack("unknown backlog id %s", from)
	}
	if rowsHave(rows, "drop", from) {
		return badTrack("backlog id %s was dropped", from)
	}
	return nil
}

// ghSearch runs `gh search issues` for one owner and role.
func ghSearch(owner, role string) ([]trackstats.Issue, error) {
	ctx, cancel := context.WithTimeout(context.Background(), trackGhTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "search", "issues", "--state", "open", "--owner", owner, "--"+role, "@me", //nolint:gosec // G204 -- owner comes from a git remote and is one argument
		"--json", "url,title,repository,createdAt", "--limit", "100")
	cmd.WaitDelay = time.Second
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("timed out after %s", trackGhTimeout)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var rows []struct {
		URL        string    `json:"url"`
		Title      string    `json:"title"`
		CreatedAt  time.Time `json:"createdAt"`
		Repository struct {
			Name string `json:"name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("parse gh output: %w", err)
	}
	issues := make([]trackstats.Issue, 0, len(rows))
	for _, r := range rows {
		issues = append(issues, trackstats.Issue{URL: r.URL, Repo: r.Repository.Name, Title: r.Title, CreatedAt: r.CreatedAt})
	}
	return issues, nil
}

// backlogCacheFile is <dir>/backlog-gh.json: one TTL for the owner list, the gh
// results and the SDD scan.
type backlogCacheFile struct {
	FetchedAt time.Time            `json:"fetchedAt"`
	Owners    []string             `json:"owners"`
	Issues    []trackstats.Issue   `json:"issues"`
	SDD       []trackstats.SDDItem `json:"sdd"`
}

// backlogSources prepares Input.GitHub and Input.SDD. The gh searches start at
// once and run beside the merged-PR lookups; the returned func waits for them.
func backlogSources(dir, wsDir string, force bool, now time.Time) (func() ([]trackstats.Issue, error), []trackstats.SDDItem) {
	cachePath := filepath.Join(dir, backlogCache)
	var notes, errs []string // notes (an unreadable cache) never stop the cache from being rewritten
	if !force {
		c, err := readBacklogCache(cachePath)
		switch {
		case err != nil:
			notes = append(notes, err.Error())
		case c != nil && now.Sub(c.FetchedAt) < backlogTTL && now.Sub(c.FetchedAt) >= 0:
			issues := c.Issues
			return func() ([]trackstats.Issue, error) { return issues, nil }, c.SDD
		}
	}
	sdd, sddErrs := scanSDD(wsDir)
	errs = append(errs, sddErrs...)
	var (
		wg     sync.WaitGroup
		issues []trackstats.Issue
		owners []string
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var searchErrs []string
		owners, searchErrs = repoOwners(wsDir)
		errs = append(errs, searchErrs...)
		issues, searchErrs = searchOwners(owners)
		errs = append(errs, searchErrs...)
		if len(errs) == 0 {
			c := backlogCacheFile{FetchedAt: now.UTC(), Owners: owners, Issues: issues, SDD: sdd}
			if err := writeBacklogCache(cachePath, c); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}()
	return func() ([]trackstats.Issue, error) {
		wg.Wait()
		if all := append(notes, errs...); len(all) > 0 {
			return issues, errors.New(strings.Join(all, "; "))
		}
		return issues, nil
	}, sdd
}

func searchOwners(owners []string) ([]trackstats.Issue, []string) {
	type res struct {
		issues []trackstats.Issue
		err    error
	}
	roles := []string{"assignee", "author"}
	out := make([]res, len(owners)*len(roles))
	var wg sync.WaitGroup
	for i, o := range owners {
		for j, r := range roles {
			wg.Add(1)
			go func() {
				defer wg.Done()
				is, err := trackGhSearch(o, r)
				out[i*len(roles)+j] = res{is, err}
			}()
		}
	}
	wg.Wait()
	var all []trackstats.Issue
	var errs []string
	for i, r := range out {
		if r.err != nil {
			errs = append(errs, fmt.Sprintf("gh search issues owner=%s role=%s: %v", owners[i/len(roles)], roles[i%len(roles)], r.err))
			continue
		}
		all = append(all, r.issues...)
	}
	return all, errs
}

func readBacklogCache(path string) (*backlogCacheFile, error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- computed from the resolved workspace
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var c backlogCacheFile
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &c, nil
}

// writeBacklogCache writes a temp file in the same directory, then renames it.
func writeBacklogCache(path string, c backlogCacheFile) (err error) {
	b, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), backlogCache+".*.tmp")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// repoOwners returns the distinct owners of the origin remotes of <ws>/repos/*.
func repoOwners(wsDir string) ([]string, []string) {
	entries, err := os.ReadDir(filepath.Join(wsDir, "repos"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []string{fmt.Sprintf("list repos: %v", err)}
	}
	seen := map[string]bool{}
	var owners, errs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		o, err := originOwner(filepath.Join(wsDir, "repos", e.Name()))
		if errors.Is(err, errNoOrigin) {
			continue // a local-only repo has no owner to search
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("owner of repos/%s: %v", e.Name(), err))
			continue
		}
		if !seen[o] {
			seen[o] = true
			owners = append(owners, o)
		}
	}
	sort.Strings(owners)
	return owners, errs
}

var errNoOrigin = errors.New("no origin remote")

func originOwner(repoDir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), trackGhTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repoDir, "remote", "get-url", "origin") //nolint:gosec // G204 -- repoDir is a directory under the workspace
	cmd.WaitDelay = time.Second
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("git remote get-url timed out after %s", trackGhTimeout)
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 2 && strings.Contains(stderr.String(), "No such remote") {
		return "", errNoOrigin
	}
	if err != nil {
		return "", fmt.Errorf("git remote get-url origin: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	url := strings.TrimSpace(string(out))
	m := ownerRe.FindStringSubmatch(url)
	if m == nil {
		return "", fmt.Errorf("cannot read an owner from remote %q", url)
	}
	return m[1], nil
}

// scanSDD lists the deferred and parked findings of every SDD ledger under the
// workspace; it only reads.
func scanSDD(wsDir string) ([]trackstats.SDDItem, []string) {
	var files []string
	var errs []string
	for _, pat := range []string{
		filepath.Join(wsDir, "repos", "*", ".worktrees", "*", ".znf", "sdd", "*", "progress.md"),
		filepath.Join(wsDir, "repos", "*", ".znf", "sdd", "*", "progress.md"),
	} {
		m, err := filepath.Glob(pat)
		if err != nil {
			errs = append(errs, fmt.Sprintf("sdd scan %s: %v", pat, err))
			continue
		}
		files = append(files, m...)
	}
	sort.Strings(files)
	items := []trackstats.SDDItem{}
	for _, f := range files {
		rel, err := filepath.Rel(wsDir, f)
		if err != nil {
			errs = append(errs, fmt.Sprintf("sdd scan %s: %v", f, err))
			continue
		}
		rel = filepath.ToSlash(rel)
		repo := strings.Split(rel, "/")[1]
		st, err := os.Stat(f)
		if err != nil {
			errs = append(errs, fmt.Sprintf("sdd scan %s: %v", rel, err))
			continue
		}
		got, err := readLedgerFindings(f)
		if err != nil {
			errs = append(errs, fmt.Sprintf("sdd scan %s: %v", rel, err))
			continue
		}
		for _, g := range got {
			items = append(items, trackstats.SDDItem{ID: fmt.Sprintf("%s:%d", rel, g.line), Repo: repo, Title: g.title, ModTime: st.ModTime().UTC()})
		}
	}
	return items, errs
}

type ledgerFinding struct {
	line  int
	title string
}

func readLedgerFindings(path string) ([]ledgerFinding, error) {
	f, err := os.Open(path) //nolint:gosec // G304 -- listed by glob under the workspace
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only
	return scanLedger(f)
}

func scanLedger(r io.Reader) ([]ledgerFinding, error) {
	var out []ledgerFinding
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for n := 1; sc.Scan(); n++ {
		l := strings.TrimSpace(sc.Text())
		for _, re := range []*regexp.Regexp{sddMinorRe, sddParkedRe} {
			if m := re.FindStringSubmatch(l); m != nil {
				out = append(out, ledgerFinding{n, strings.TrimSpace(m[1])})
				break
			}
		}
	}
	return out, sc.Err()
}
