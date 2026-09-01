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
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitcode.Code(err))
	}
}
