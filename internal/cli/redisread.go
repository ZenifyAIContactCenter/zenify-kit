package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/spf13/cobra"
)

// newRedisReadCmd builds `zenify redis-read [--prd] <cmd> [args...]`: one
// allowlisted read-only Redis command, run by the credential broker.
func newRedisReadCmd() *cobra.Command {
	var prd bool
	var manifestPath, overlayPath string
	cmd := &cobra.Command{
		Use:           "redis-read [--prd] <lệnh> [tham số...]",                                                                                                                                   //znf:allow-lang
		Short:         "Chạy một lệnh Redis chỉ đọc qua credential broker",                                                                                                                        //znf:allow-lang
		Long:          "Chạy một lệnh Redis chỉ đọc qua broker. Lệnh được phép: " + strings.Join(redisAllowedList(), " ") + ".\nCờ phải đứng trước lệnh để tham số như -1 không bị đọc thành cờ.", //znf:allow-lang
		Args:          cobra.MinimumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			argv, err := api.CheckRedis(args[0], args[1:])
			if err != nil {
				return exitcode.New(exitcode.BadArgs, fmt.Errorf("zenify redis-read: %w", err))
			}
			bc, ok, err := brokerFromFlags(manifestPath, overlayPath)
			if err != nil {
				return err
			}
			if !ok {
				return exitcode.New(exitcode.BadArgs, errors.New("redis-read cần broker: manifest không khai báo broker")) //znf:allow-lang
			}
			req := api.RedisRequest{Env: "stg", Cmd: argv[0], Args: argv[1:]}
			if prd {
				req.Env = "prd"
			}
			var resp api.RedisResponse
			if err := bc.Post(cmd.Context(), "/redis", req, &resp); err != nil {
				return brokerCallErr(err)
			}
			b, err := json.MarshalIndent(resp.Result, "", "  ")
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(b))
			if resp.Truncated {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "zenify redis-read: kết quả đã bị cắt bớt") //znf:allow-lang
			}
			return nil
		},
	}
	cmd.Flags().SetInterspersed(false)
	f := cmd.Flags()
	f.BoolVar(&prd, "prd", false, "read production Redis")
	f.StringVar(&manifestPath, "manifest", "", "path to the repos manifest (default: manifest/repos.yaml under cwd, else the embedded one)")
	f.StringVar(&overlayPath, "overlay", "", "path to personal overlay (default <workspace>/.zenify-overlay.yaml)")
	return cmd
}

func redisAllowedList() []string {
	var out []string
	for c := range api.RedisAllowed {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
