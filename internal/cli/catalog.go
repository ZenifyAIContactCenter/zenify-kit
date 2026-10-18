package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/brokerclient"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/catalog"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const noCatalogMsg = "no catalog configured"

// newCatalogCmd builds `zenify catalog [id] [--prd|--stg]` and its `drift`
// subcommand. The broker serves the whole catalog; filtering happens here.
func newCatalogCmd() *cobra.Command {
	var prd, stg bool
	var manifestPath, overlayPath string
	envFlag := func() (string, error) {
		switch {
		case prd && stg:
			return "", exitcode.New(exitcode.BadArgs, errors.New("zenify catalog: --prd and --stg are mutually exclusive"))
		case prd:
			return "prd", nil
		case stg:
			return "stg", nil
		}
		return "", nil
	}
	// call posts to the broker and maps "no catalog" to its exit code.
	call := func(cmd *cobra.Command, path string, in, out any) error {
		bc, ok, err := brokerFromFlags(manifestPath, overlayPath)
		if err != nil {
			return err
		}
		if !ok {
			return exitcode.New(exitcode.NoCatalog, errors.New(noCatalogMsg))
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), brokerCallTimeout)
		defer cancel()
		if err := bc.Post(ctx, path, in, out); err != nil {
			var ae *brokerclient.APIError
			if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
				return exitcode.New(exitcode.NoCatalog, errors.New(noCatalogMsg))
			}
			return brokerCallErr(err)
		}
		return nil
	}

	cmd := &cobra.Command{
		Use:           "catalog [id]",
		Short:         "In danh mục dịch vụ giám sát (datasource Grafana, tên log/trace/metric) dạng YAML", //znf:allow-lang
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := envFlag()
			if err != nil {
				return err
			}
			var resp api.CatalogResponse
			if err := call(cmd, "/catalog", struct{}{}, &resp); err != nil {
				return err
			}
			if resp.Catalog == nil {
				return exitcode.New(exitcode.NoCatalog, errors.New(noCatalogMsg))
			}
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			out, err := resp.Catalog.Filter(id, env)
			if err != nil {
				var ue *catalog.UnknownIDError
				if errors.As(err, &ue) || strings.HasPrefix(err.Error(), "unknown env") {
					return exitcode.New(exitcode.BadArgs, fmt.Errorf("zenify catalog: %w", err))
				}
				return err
			}
			b, err := yaml.Marshal(out)
			if err != nil {
				return err
			}
			if resp.StaleAgeSeconds > 0 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "catalog is %ds old (broker could not refresh it)\n", resp.StaleAgeSeconds)
			}
			_, _ = cmd.OutOrStdout().Write(b)
			return nil
		},
	}
	pf := cmd.PersistentFlags()
	pf.BoolVar(&prd, "prd", false, "production only")
	pf.BoolVar(&stg, "stg", false, "staging only")
	pf.StringVar(&manifestPath, "manifest", "", brokerManifestUsage)
	pf.StringVar(&overlayPath, "overlay", "", brokerOverlayUsage)

	cmd.AddCommand(&cobra.Command{
		Use:           "drift",
		Short:         "So danh mục với tên đang có thật trong Loki, Tempo, Prometheus (24 giờ qua)", //znf:allow-lang
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := envFlag()
			if err != nil {
				return err
			}
			var resp api.DriftResponse
			if err := call(cmd, "/catalog/drift", api.DriftRequest{Env: env}, &resp); err != nil {
				return err
			}
			if printDrift(cmd, resp) {
				return exitcode.New(exitcode.Drift, errors.New("catalog drift found"))
			}
			return nil
		},
	})
	return cmd
}

// printDrift writes the drift table and reports whether any source drifted.
func printDrift(cmd *cobra.Command, resp api.DriftResponse) (drift bool) {
	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ENV\tSOURCE\tUNLISTED\tMISSING\tERROR")
	dash := func(s []string) string {
		if len(s) == 0 {
			return "-"
		}
		return strings.Join(s, ",")
	}
	for _, env := range driftKeys(resp.Envs) {
		for _, src := range driftKeys(resp.Envs[env]) {
			r := resp.Envs[env][src]
			if len(r.Unlisted) > 0 || len(r.Missing) > 0 || r.Error != "" {
				drift = true
			}
			errCol := r.Error
			if errCol == "" {
				errCol = "-"
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", env, src, dash(r.Unlisted), dash(r.Missing), errCol)
		}
	}
	_ = tw.Flush()
	return drift
}

func driftKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
