package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/wt"
	"github.com/spf13/cobra"
)

func newWtCacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Ghi/đọc cache node_modules dùng chung slot với wt new", //znf:allow-lang
	}
	cmd.AddCommand(newWtCachePutCmd(), newWtCacheGetCmd())
	return cmd
}

// cacheTarget resolves <dir> to an absolute path and the cache repo name
// (--name, default the dir's base name).
func cacheTarget(args []string, name string) (dir, repoName string, err error) {
	dir, err = filepath.Abs(args[0])
	if err != nil {
		return "", "", err
	}
	if name == "" {
		name = filepath.Base(dir)
	}
	return dir, name, nil
}

func newWtCachePutCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "put <dir>",
		Short: "Ghi node_modules của <dir> vào cache deps (từ chối cây lệch lock)", //znf:allow-lang
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, repoName, err := cacheTarget(args, name)
			if err != nil {
				return err
			}
			if err := wt.CachePut(dir, repoName, os.LookupEnv); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "cached %s as %s\n", dir, repoName)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "tên repo trong cache (mặc định: tên thư mục của <dir>)") //znf:allow-lang
	return cmd
}

func newWtCacheGetCmd() *cobra.Command {
	var name string
	var check bool
	cmd := &cobra.Command{
		Use:   "get <dir>",
		Short: "Chép node_modules từ cache vào <dir>; thoát 0 khi hit, 1 khi miss", //znf:allow-lang
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, repoName, err := cacheTarget(args, name)
			if err != nil {
				return err
			}
			miss := exitcode.New(exitcode.Fail, errors.New("wt cache: miss"))
			if check {
				if !wt.CacheHas(dir, repoName, os.LookupEnv) {
					return miss
				}
				return nil
			}
			hit, err := wt.CacheGet(dir, repoName, os.LookupEnv)
			if err != nil {
				return err
			}
			if !hit {
				return miss
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "restored %s/node_modules from cache %s\n", dir, repoName)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "tên repo trong cache (mặc định: tên thư mục của <dir>)") //znf:allow-lang
	cmd.Flags().BoolVar(&check, "check", false, "chỉ kiểm tra có hit không, không chép gì")            //znf:allow-lang
	return cmd
}
