package main

import (
	"fmt"
	"os"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/cli"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		// root sets SilenceErrors, so print here, then translate to an exit code.
		err = cli.ClassifyExecErr(err)
		if exitcode.Code(err) == exitcode.Awaiting {
			fmt.Fprintln(os.Stderr, err) // a planned stop, not a failure: no "error:" prefix
		} else {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(exitcode.Code(err))
	}
}
