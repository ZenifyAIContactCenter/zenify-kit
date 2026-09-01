package cli

import (
	"errors"
	"strings"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

// ClassifyExecErr maps a cobra Execute error to an exit-code-carrying error.
// An error that already carries an exit code (e.g. a flag error wrapped by the
// root's FlagErrorFunc, or a RunE returning exitcode.New) passes through. A
// cobra usage error (unknown command or flag) becomes BadArgs(2). Anything else
// is returned unchanged, so exitcode.Code reports Fail(1).
func ClassifyExecErr(err error) error {
	if err == nil {
		return nil
	}
	var coded *exitcode.Error
	if errors.As(err, &coded) {
		return err
	}
	msg := err.Error()
	if strings.HasPrefix(msg, "unknown command") ||
		strings.HasPrefix(msg, "unknown flag") ||
		strings.HasPrefix(msg, "unknown shorthand flag") {
		return exitcode.New(exitcode.BadArgs, err)
	}
	return err
}
