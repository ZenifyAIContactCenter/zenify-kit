package cli

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func newPlaneCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "plane",
		Short: "Làm việc với Plane bằng PAT của chính dev", //znf:allow-lang
	}
	c.AddCommand(newPlaneCreateCmd())
	return c
}

func newPlaneCreateCmd() *cobra.Command {
	var project, description, ws string
	c := &cobra.Command{
		Use:           `create "<title>" [--project ZEN] [--description <text>]`,
		Short:         "Tạo work item Plane giao cho chính mình và in key kèm URL", //znf:allow-lang
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				return badTrack("expected 1 argument (the title), got %d", len(args))
			}
			cwd, err := os.Getwd()
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			wsDir, _, ok := resolveWorkspace(cwd, ws, os.Getenv, os.UserHomeDir)
			if !ok {
				return exitcode.New(exitcode.BadArgs, errors.New("no workspace found; pass --workspace"))
			}
			conn, err := planeConnect(wsDir)
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			if conn == nil {
				return exitcode.New(exitcode.BadArgs, errors.New("Plane is not set up: the manifest has no plane or "+planePATKey+" is empty; run `zenify up --apply`"))
			}
			ctx, cancel := context.WithTimeout(context.Background(), trackGhTimeout)
			defer cancel()
			out, err := planeCreate(conn.api(ctx), conn, project, args[0], description)
			if err != nil {
				return exitcode.New(exitcode.Fail, err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	}
	c.Flags().StringVar(&project, "project", "ZEN", "Plane project identifier")
	c.Flags().StringVar(&description, "description", "", "work item description (plain text)")
	c.Flags().StringVar(&ws, "workspace", "", "workspace root (default: detected)")
	return c
}

// planeCreate creates a work item assigned to the PAT's owner and returns
// `plane: <KEY>-<n> <browse url>`.
func planeCreate(p planeAPI, c *planeConn, project, title, description string) (string, error) {
	var me struct {
		ID string `json:"id"`
	}
	if err := p.do(http.MethodGet, "/users/me/", nil, &me); err != nil {
		return "", err
	}
	if me.ID == "" {
		return "", errors.New("/users/me/ returned no id")
	}
	wsPath := "/workspaces/" + url.PathEscape(c.ws) + "/projects/"
	projects, err := planeList[struct {
		ID         string `json:"id"`
		Identifier string `json:"identifier"`
	}](p, wsPath)
	if err != nil {
		return "", err
	}
	var id, ident string
	for _, pr := range projects {
		if strings.EqualFold(pr.Identifier, project) {
			id, ident = pr.ID, pr.Identifier
			break
		}
	}
	if id == "" {
		return "", fmt.Errorf("no Plane project with identifier %q in workspace %s", project, c.ws)
	}
	body := map[string]any{"name": title, "assignees": []string{me.ID}}
	if description != "" {
		body["description_html"] = "<p>" + html.EscapeString(description) + "</p>"
	}
	var wi struct {
		SequenceID int `json:"sequence_id"`
	}
	if err := p.do(http.MethodPost, wsPath+url.PathEscape(id)+"/work-items/", body, &wi); err != nil {
		return "", err
	}
	if wi.SequenceID == 0 {
		return "", errors.New("Plane created the work item but returned no sequence_id")
	}
	key := fmt.Sprintf("%s-%d", ident, wi.SequenceID)
	return fmt.Sprintf("plane: %s %s/%s/browse/%s/", key, c.browse, c.ws, key), nil
}
