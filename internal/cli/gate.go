package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	"github.com/spf13/cobra"
)

type participant struct {
	Name           string   `json:"name"`
	AccessPatterns []string `json:"accessPatterns"`
	DBAccessor     string   `json:"dbAccessor"`
}

// gateParticipants scan subdir trực tiếp của workspace; repo nào có worktree.json
// với gate.sharedStore=true thì là participant. Repo Load lỗi (không có worktree.json)
// hoặc sharedStore=false → bỏ qua.
func gateParticipants(workspace string) ([]participant, error) {
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return nil, err
	}
	var ps []participant
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		c, err := wt.Load(filepath.Join(workspace, e.Name()))
		if err != nil || !c.GateSharedStore {
			continue
		}
		ps = append(ps, participant{Name: e.Name(), AccessPatterns: c.GateAccessPatterns, DBAccessor: c.GateDBAccessor})
	}
	return ps, nil
}

func newGateCmd() *cobra.Command {
	var workspace string
	var asJSON bool
	cmd := &cobra.Command{Use: "gate", Short: "trợ giúp gate (contract sweep)"}
	participants := &cobra.Command{
		Use:   "participants",
		Short: "list repo chia sẻ shared store (gate.sharedStore=true) trong workspace",
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
	participants.Flags().BoolVar(&asJSON, "json", false, "xuất JSON")
	cmd.AddCommand(participants)
	return cmd
}
