package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/brokerclient"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const obsNotAvailableMsg = "obs not available on this broker"

// obsCallTimeout is longer than brokerCallTimeout: a 48h route summary fans
// out up to 28 Grafana calls.
const obsCallTimeout = 60 * time.Second

// newObsCmd builds `zenify obs <id> --route "<METHOD path>"` and
// `zenify obs --trace <id>`: a bounded, PII-masked summary from the broker.
func newObsCmd() *cobra.Command {
	var route, trace, since string
	var prd, stg, asJSON bool
	var manifestPath, overlayPath string

	badArgs := func(msg string) error {
		return exitcode.New(exitcode.BadArgs, errors.New("zenify obs: "+msg))
	}

	cmd := &cobra.Command{
		Use:           "obs [id]",
		Short:         "Tóm tắt sức khỏe một route hoặc một trace qua broker (đã che PII, có giới hạn kích thước)", //znf:allow-lang
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if (route == "") == (trace == "") {
				return badArgs("exactly one of --route and --trace is required")
			}
			if prd == stg {
				return badArgs("exactly one of --prd and --stg is required")
			}
			env := "stg"
			if prd {
				env = "prd"
			}
			path := "/obs/trace"
			var in, out any
			var resp interface {
				truncation() (bool, map[string]int)
			}
			if route != "" {
				if len(args) != 1 {
					return badArgs("--route needs a service <id> argument")
				}
				r := &obsRoute{}
				path, in, out, resp = "/obs/route", api.ObsRouteRequest{Service: args[0], Route: route, Since: since, Env: env}, &r.ObsRouteResponse, r
			} else {
				if len(args) != 0 {
					return badArgs("--trace takes no <id> argument")
				}
				t := &obsTrace{}
				in, out, resp = api.ObsTraceRequest{TraceID: trace, Env: env}, &t.ObsTraceResponse, t
			}

			bc, ok, err := brokerFromFlags(manifestPath, overlayPath)
			if err != nil {
				return err
			}
			if !ok {
				return exitcode.New(exitcode.NoCatalog, errors.New(obsNotAvailableMsg))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), obsCallTimeout)
			defer cancel()
			if err := bc.Post(ctx, path, in, out); err != nil {
				var ae *brokerclient.APIError
				if errors.As(err, &ae) && ae.Status == http.StatusNotFound && ae.Code != api.CodeTraceNotFound {
					return exitcode.New(exitcode.NoCatalog, errors.New(obsNotAvailableMsg))
				}
				return brokerCallErr(err)
			}

			b, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				return err
			}
			if !asJSON {
				var generic any
				if err := json.Unmarshal(b, &generic); err != nil {
					return err
				}
				if b, err = yaml.Marshal(generic); err != nil {
					return err
				}
			} else {
				b = append(b, '\n')
			}
			if trunc, omitted := resp.truncation(); trunc {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), truncationLine(omitted))
			}
			_, _ = cmd.OutOrStdout().Write(b)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&route, "route", "", `route to summarise, "<METHOD path>"; needs the service <id> argument`)
	f.StringVar(&trace, "trace", "", "trace id (1-32 hex characters) to summarise")
	f.StringVar(&since, "since", "24h", "look-back for --route, a Go duration, at most 48h")
	f.BoolVar(&prd, "prd", false, "production")
	f.BoolVar(&stg, "stg", false, "staging")
	f.BoolVar(&asJSON, "json", false, "print the broker response as JSON instead of YAML")
	f.StringVar(&manifestPath, "manifest", "", brokerManifestUsage)
	f.StringVar(&overlayPath, "overlay", "", brokerOverlayUsage)
	return cmd
}

type obsRoute struct{ api.ObsRouteResponse }

func (r *obsRoute) truncation() (bool, map[string]int) { return r.Truncated, r.Omitted }

type obsTrace struct{ api.ObsTraceResponse }

func (t *obsTrace) truncation() (bool, map[string]int) { return t.Truncated, t.Omitted }

// truncationLine names what the broker cut: "obs: response truncated (logs=2, spans=4)".
func truncationLine(omitted map[string]int) string {
	if len(omitted) == 0 {
		return "obs: response truncated"
	}
	kinds := make([]string, 0, len(omitted))
	for k := range omitted {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	parts := make([]string, len(kinds))
	for i, k := range kinds {
		parts[i] = fmt.Sprintf("%s=%d", k, omitted[k])
	}
	return "obs: response truncated (" + strings.Join(parts, ", ") + ")"
}
