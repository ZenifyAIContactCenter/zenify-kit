// internal/cli/hookactions.go  (Task 3 stub; Task 4 fills the bodies)
package cli

import (
	"fmt"
	"io"
)

func runSessionStart(wsRoot string, w io.Writer) int      { fmt.Fprintln(w, "{}"); return 0 }
func runDocsSyncHook(wsRoot string, w io.Writer) int      { fmt.Fprintln(w, "{}"); return 0 }
func runObserveHook(wsRoot, kind string, w io.Writer) int { fmt.Fprintln(w, "{}"); return 0 }
