package cli

import (
	"encoding/json"
	"fmt"
	"os"

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

// gateParticipants finds repos in workspaceDir down to workspace.DefaultMaxDepth
// (not just direct children); a repo whose worktree.json has gate.sharedStore=true
// is a participant. A repo whose Load fails (no worktree.json) or sharedStore=false is skipped.
func gateParticipants(workspaceDir string) ([]participant, error) {
	var ps []participant
	for _, repo := range workspace.Discover(workspaceDir, workspace.DefaultMaxDepth, os.ReadDir) {
		c, err := wt.Load(repo.Path)
		if err != nil || !c.GateSharedStore {
			continue
		}
		ps = append(ps, participant{Name: repo.Name, AccessPatterns: c.GateAccessPatterns, DBAccessor: c.GateDBAccessor})
	}
	return ps, nil
}

func newGateCmd() *cobra.Command {
	var workspace string
	var asJSON bool
	cmd := &cobra.Command{Use: "gate", Short: "trợ giúp gate (contract sweep)"} //znf:allow-lang
	participants := &cobra.Command{
		Use:   "participants",
		Short: "list repo chia sẻ shared store (gate.sharedStore=true) trong workspace", //znf:allow-lang
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
