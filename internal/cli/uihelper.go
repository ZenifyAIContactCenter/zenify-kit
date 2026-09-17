package cli

import (
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/ui"
	"github.com/spf13/cobra"
)

// uiOut builds a ui.Writer bound to the command's stdout writer, so it is
// testable (a bytes.Buffer stays plain) and honors --no-color / NO_COLOR.
func uiOut(cmd *cobra.Command) *ui.Writer { return ui.New(cmd.OutOrStdout()) }

// uiErr builds a ui.Writer bound to the command's stderr writer.
func uiErr(cmd *cobra.Command) *ui.Writer { return ui.New(cmd.ErrOrStderr()) }
