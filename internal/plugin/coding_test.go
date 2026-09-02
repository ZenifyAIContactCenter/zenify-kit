package plugin

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var forbiddenTokens = []string{
	"3csoft", "contact-center", "ott-gateway", "db_read", "lumi",
	"personal-zalo", "notification-hub", "models.mongo", "sendOK",
	"VITE_", "namph", "zenify",
}

// skill dir -> lowercase anchors phải xuất hiện trong SKILL.md của skill đó
var codingAnchors = map[string][]string{
	"mongo-data-safety":        {"tenant", "strict", "distinct"},
	"mongoose-modeling":        {"schema", "index", "backfill"},
	"nestjs-patterns":          {"module", "guard", "controller"},
	"express-service-patterns": {"middleware", "controller", "envelope"},
	"react-patterns":           {"component", "hook", "form"},
	"service-integration":      {"idempoten", "publish", "contract"},
}

func readCodingFile(t *testing.T, skill string) string {
	t.Helper()
	b, err := fs.ReadFile(codingAssets, codingRoot+"/"+skill+"/SKILL.md")
	if err != nil {
		t.Fatalf("read %s: %v", skill, err)
	}
	return string(b)
}

func TestCodingSkillsAreAgnostic(t *testing.T) {
	// Lặp theo CodingSkills() (nguồn thật từ embed) chứ không theo codingAnchors,
	// để một skill mới thêm vào mà quên khai anchor sẽ FAIL ở đây thay vì lọt
	// cả token-check lẫn anchor-check.
	for _, skill := range CodingSkills() {
		anchors, ok := codingAnchors[skill]
		if !ok {
			t.Errorf("skill %s chưa có entry trong codingAnchors — thêm anchor để guard kiểm được", skill)
			continue
		}
		body := readCodingFile(t, skill)
		low := strings.ToLower(body)
		for _, tok := range forbiddenTokens {
			if strings.Contains(low, strings.ToLower(tok)) {
				t.Errorf("skill %s chứa token cấm %q", skill, tok)
			}
		}
		for _, a := range anchors {
			if !strings.Contains(low, a) {
				t.Errorf("skill %s thiếu anchor %q", skill, a)
			}
		}
	}
}

func TestCodingSkillsListed(t *testing.T) {
	got := CodingSkills()
	found := false
	for _, s := range got {
		if s == "mongo-data-safety" {
			found = true
		}
	}
	if !found {
		t.Fatalf("CodingSkills() không có mongo-data-safety: %v", got)
	}
}

func TestGlobalSyncSkipsCoding(t *testing.T) {
	dest := t.TempDir()
	man := dest + "/.manifest.json"
	if _, err := Sync(dest, man); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "coding")); !os.IsNotExist(err) {
		t.Fatalf("Sync global không được materialize assets/coding (err=%v)", err)
	}
}
