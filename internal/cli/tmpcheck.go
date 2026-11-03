// internal/cli/tmpcheck.go
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// A per-user temp dir flooded with leaked test dirs can hang a Mac at boot: launchd's
// dirhelper cleans it before login and stalls on ~1M entries. A healthy one holds a few
// hundred. tmp-check warns at SessionStart, before the reboot that would hang.
const (
	tmpFloodLimit     = 50000
	tmpSizePrefilter  = 1 << 20 // bytes; a dir smaller than this cannot hold tmpFloodLimit entries
	tmpReadTimeout    = 3 * time.Second
	tmpUptimeWarnDays = 7
)

// tmpCountCapped counts entries in dir, stopping at limit. A read that has not finished
// within timeout returns limit: a dir that slow to read is the flood itself. The reading
// goroutine may stay blocked in readdir; the hook process exits right after.
func tmpCountCapped(dir string, limit int, timeout time.Duration) int {
	done := make(chan int, 1)
	go func() {
		f, err := os.Open(dir)
		if err != nil {
			done <- 0
			return
		}
		defer f.Close()
		n := 0
		for n < limit {
			names, err := f.Readdirnames(1024)
			n += len(names)
			if err != nil {
				break
			}
		}
		done <- n
	}()
	select {
	case n := <-done:
		return n
	case <-time.After(timeout):
		return limit
	}
}

// tmpFloodWarning returns a warning line when dir is flooded, else "". st_size of a
// directory is O(1) and only pre-filters: on APFS it is a high-water mark.
func tmpFloodWarning(label, dir string, limit int, prefilter int64, timeout time.Duration) string {
	if dir == "" {
		return ""
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() || st.Size() < prefilter {
		return ""
	}
	if tmpCountCapped(dir, limit, timeout) < limit {
		return ""
	}
	return fmt.Sprintf("%s %s holds >= %d entries (or is too slow to read). Leaked test temp dirs; a reboot in this state can hang macOS at boot. Find the leaking prefix and stop it before rebooting.", label, dir, limit)
}

// userTempDir is the dir the OS cleans at boot. On macOS that is DARWIN_USER_TEMP_DIR,
// read from getconf because $TMPDIR is exactly what tests redirect.
func userTempDir() string {
	if runtime.GOOS == "darwin" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "getconf", "DARWIN_USER_TEMP_DIR").Output(); err == nil {
			if d := strings.TrimSpace(string(out)); d != "" {
				return d
			}
		}
	}
	return os.TempDir()
}

var bootSecRe = regexp.MustCompile(`\{ sec = (\d+),`)

// bootTime returns the last boot time, or zero when it cannot be read.
func bootTime() time.Time {
	switch runtime.GOOS {
	case "darwin":
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "sysctl", "-n", "kern.boottime").Output()
		if err != nil {
			return time.Time{}
		}
		if m := bootSecRe.FindSubmatch(out); m != nil {
			if s, err := strconv.ParseInt(string(m[1]), 10, 64); err == nil {
				return time.Unix(s, 0)
			}
		}
	case "linux":
		raw, err := os.ReadFile("/proc/uptime")
		if err != nil {
			return time.Time{}
		}
		f := strings.Fields(string(raw))
		if len(f) > 0 {
			if s, err := strconv.ParseFloat(f[0], 64); err == nil {
				return time.Now().Add(-time.Duration(s * float64(time.Second)))
			}
		}
	}
	return time.Time{}
}

// tmpUptimeWarning returns a warning when the machine has been up >= warnDays.
func tmpUptimeWarning(boot, now time.Time, warnDays int) string {
	if boot.IsZero() {
		return ""
	}
	days := int(now.Sub(boot).Hours() / 24)
	if days < warnDays {
		return ""
	}
	return fmt.Sprintf("Uptime %dd. Restart soon: the user temp dir is only cleaned at boot, so a long uptime lets leaked test dirs pile up.", days)
}

// runTmpCheckHook prints the warnings, or nothing. Never blocks, never fails a session.
func runTmpCheckHook(w io.Writer) int {
	var wrote bool
	defer failOpen(io.Discard, &wrote)
	var lines []string
	user := userTempDir()
	for _, d := range []struct{ label, dir string }{{"user temp dir", user}, {"/tmp", "/tmp"}} {
		if d.label == "/tmp" && filepath.Clean(user) == "/tmp" {
			continue // Linux: the user temp dir is /tmp; count it once
		}
		if s := tmpFloodWarning(d.label, d.dir, tmpFloodLimit, tmpSizePrefilter, tmpReadTimeout); s != "" {
			lines = append(lines, s)
		}
	}
	if s := tmpUptimeWarning(bootTime(), time.Now(), tmpUptimeWarnDays); s != "" {
		lines = append(lines, s)
	}
	if len(lines) > 0 {
		fmt.Fprintln(w, "<tmp-check>\n"+strings.Join(lines, "\n")+"\n</tmp-check>")
	}
	wrote = true
	return 0
}
