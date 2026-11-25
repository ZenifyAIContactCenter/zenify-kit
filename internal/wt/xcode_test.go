package wt

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func writeDerivedData(t *testing.T, root, name, workspace string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>LastAccessedDate</key>
	<date>2026-11-20T07:54:56Z</date>
	<key>WorkspacePath</key>
	<string>` + workspace + `</string>
</dict>
</plist>
`
	if err := os.WriteFile(filepath.Join(dir, "info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestClearXcodeDerivedData(t *testing.T) {
	root := t.TempDir()
	orig := derivedDataRoot
	derivedDataRoot = func() string { return root }
	t.Cleanup(func() { derivedDataRoot = orig })

	wt := "/repo/.worktrees/foo"
	hit := writeDerivedData(t, root, "App-aaa", wt+"/ios/App.xcworkspace")
	sibling := writeDerivedData(t, root, "App-bbb", "/repo/.worktrees/foobar/ios/App.xcworkspace")
	main := writeDerivedData(t, root, "App-ccc", "/repo/ios/App.xcworkspace")
	noPlist := filepath.Join(root, "ModuleCache.noindex")
	if err := os.MkdirAll(noPlist, 0o755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	clearXcodeDerivedData(&buf, wt)

	if _, err := os.Stat(hit); !os.IsNotExist(err) {
		t.Errorf("DerivedData of the removed worktree should be deleted")
	}
	for _, keep := range []string{sibling, main, noPlist} {
		if _, err := os.Stat(keep); err != nil {
			t.Errorf("%s should be kept: %v", keep, err)
		}
	}
	if !bytes.Contains(buf.Bytes(), []byte("App-aaa")) {
		t.Errorf("expected deletion to be reported, got %q", buf.String())
	}
}

func TestClearXcodeDerivedDataMissingRoot(t *testing.T) {
	orig := derivedDataRoot
	derivedDataRoot = func() string { return filepath.Join(t.TempDir(), "nope") }
	t.Cleanup(func() { derivedDataRoot = orig })

	var buf bytes.Buffer
	clearXcodeDerivedData(&buf, "/repo/.worktrees/foo")
	if buf.Len() != 0 {
		t.Errorf("missing DerivedData dir should be silent, got %q", buf.String())
	}
}
