package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/workspace"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	"github.com/spf13/cobra"
)

type participant struct {
	Name           string   `json:"name"`
	AccessPatterns []string `json:"accessPatterns"`
	DBAccessor     string   `json:"dbAccessor"`
}

// gateParticipantsFile is the store-side participant list, distributed with
// the rest of <store>/.config (W0 FR-04). It is the primary source; a repo's
// worktree.json gate.sharedStore=true adds to it without duplicating a name.
const gateParticipantsFile = "gate-participants.json"

// gateParticipants returns the repos whose changes must go through the
// contract gate: the store file first (in file order), then any repo under
// workspaceDir (to workspace.DefaultMaxDepth) whose worktree.json declares
// gate.sharedStore=true and whose name is not already listed. Never returns a
// nil slice so --json prints [] for an empty set.
func gateParticipants(workspaceDir string) ([]participant, error) {
	ps := storeParticipants(workspaceDir, os.Stderr)
	seen := make(map[string]bool, len(ps))
	for _, p := range ps {
		seen[p.Name] = true
	}
	for _, repo := range workspace.Discover(workspaceDir, workspace.DefaultMaxDepth, os.ReadDir) {
		if seen[repo.Name] {
			continue
		}
		c, err := wt.Load(repo.Path)
		if err != nil || !c.GateSharedStore {
			continue
		}
		ps = append(ps, participant{Name: repo.Name, AccessPatterns: c.GateAccessPatterns, DBAccessor: c.GateDBAccessor})
		seen[repo.Name] = true
	}
	if ps == nil {
		ps = []participant{}
	}
	return ps, nil
}

// storeParticipants reads <store>/.config/gate-participants.json. A missing
// file is an empty list; a corrupt file is one stderr line and an empty list.
func storeParticipants(workspaceDir string, stderr io.Writer) []participant {
	dir := filepath.Join(resolveDocsStore(workspaceDir, os.Getenv, os.UserHomeDir, os.Stat, os.ReadDir), defaultConfigSub)
	path := filepath.Join(dir, gateParticipantsFile)
	b, err := os.ReadFile(path) //nolint:gosec // G304 -- path is inside the trusted store config dir, not user input
	if err != nil {
		return nil
	}
	var ps []participant
	if err := json.Unmarshal(b, &ps); err != nil {
		fmt.Fprintf(stderr, "gate: %s không đọc được: %v\n", path, err) //znf:allow-lang
		return nil
	}
	return ps
}

func newGateCmd() *cobra.Command {
	var workspace string
	var asJSON bool
	cmd := &cobra.Command{Use: "gate", Short: "trợ giúp gate (contract sweep)"} //znf:allow-lang
	participants := &cobra.Command{
		Use:   "participants",
		Short: "list repo tham gia contract gate (store gate-participants.json + worktree.json gate.sharedStore=true)", //znf:allow-lang
		RunE: func(cmd *cobra.Command, _ []string) error {
			ps, err := gateParticipants(workspace)
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			w := cmd.OutOrStdout()
			if asJSON {
				return json.NewEncoder(w).Encode(ps)
			}
			for _, p := range ps {
				fmt.Fprintf(w, "%s\taccessor=%s\tpatterns=%v\n", p.Name, p.DBAccessor, p.AccessPatterns)
			}
			return nil
		},
	}
	participants.Flags().StringVar(&workspace, "workspace", ".", "workspace root")
	participants.Flags().BoolVar(&asJSON, "json", false, "xuất JSON") //znf:allow-lang
	cmd.AddCommand(participants)
	return cmd
}
