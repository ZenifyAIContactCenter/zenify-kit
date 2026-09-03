package review

import "testing"

func fakeFS(files map[string]string) func(string) ([]byte, error) {
	return func(name string) ([]byte, error) {
		c, ok := files[name]
		if !ok {
			return nil, errNotFound
		}
		return []byte(c), nil
	}
}

var errNotFound = &fsErr{}

type fsErr struct{}

func (*fsErr) Error() string { return "not found" }

func TestVerify_KeepExactMatch(t *testing.T) {
	fs := fakeFS(map[string]string{"a.go": "line1\nfoo := bar()\nline3\n"})
	res := Verify([]Finding{{File: "a.go", Line: "2", Evidence: "foo := bar()"}}, fs)
	if res.Kept != 1 || res.Refuted != 0 {
		t.Fatalf("kept=%d refuted=%d, want 1/0", res.Kept, res.Refuted)
	}
}

func TestVerify_KeepWithinDrift(t *testing.T) {
	// evidence ở dòng 5, finding khai line 3 → trong cửa sổ ±3
	fs := fakeFS(map[string]string{"a.go": "1\n2\n3\n4\nneedle()\n6\n"})
	res := Verify([]Finding{{File: "a.go", Line: "3", Evidence: "needle()"}}, fs)
	if res.Kept != 1 {
		t.Fatalf("kept=%d, want 1 (drift trong ±3)", res.Kept)
	}
}

func TestVerify_RefuteNotFound(t *testing.T) {
	fs := fakeFS(map[string]string{"a.go": "1\n2\n3\n"})
	res := Verify([]Finding{{File: "a.go", Line: "2", Evidence: "khong-ton-tai"}}, fs)
	if res.Kept != 0 || res.Refuted != 1 {
		t.Fatalf("kept=%d refuted=%d, want 0/1", res.Kept, res.Refuted)
	}
}

func TestVerify_RefuteMissingFile(t *testing.T) {
	res := Verify([]Finding{{File: "khong.go", Line: "2", Evidence: "x"}}, fakeFS(nil))
	if res.Refuted != 1 {
		t.Fatalf("refuted=%d, want 1", res.Refuted)
	}
}

func TestVerify_KeepNoEvidence(t *testing.T) {
	fs := fakeFS(map[string]string{"a.go": "x\n"})
	res := Verify([]Finding{{File: "a.go", Line: "1"}}, fs)
	if res.Kept != 1 || res.Refuted != 0 {
		t.Fatalf("kept=%d refuted=%d, want 1/0", res.Kept, res.Refuted)
	}
	if res.Findings[0].Reason != "unverified: no evidence" {
		t.Errorf("reason=%q, want 'unverified: no evidence'", res.Findings[0].Reason)
	}
}

func TestVerify_KeepNoLocation(t *testing.T) {
	res := Verify([]Finding{{Title: "kiến trúc tổng thể", Issue: "x"}}, fakeFS(nil))
	if res.Kept != 1 {
		t.Fatalf("kept=%d, want 1 (không file+line → skip verify)", res.Kept)
	}
}

func TestVerify_RangeLineParses(t *testing.T) {
	fs := fakeFS(map[string]string{"a.go": "1\n2\nhit()\n4\n"})
	res := Verify([]Finding{{File: "a.go", Line: "3-5", Evidence: "hit()"}}, fs)
	if res.Kept != 1 {
		t.Fatalf("kept=%d, want 1 (line '3-5' parse ->3)", res.Kept)
	}
}

func TestVerify_WhitespaceInsensitive(t *testing.T) {
	fs := fakeFS(map[string]string{"a.go": "1\n\t if  ok  {\n3\n"})
	res := Verify([]Finding{{File: "a.go", Line: "2", Evidence: "if ok {"}}, fs)
	if res.Kept != 1 {
		t.Fatalf("kept=%d, want 1 (whitespace-insensitive)", res.Kept)
	}
}

func TestVerify_KeepDiffPrefixedEvidence(t *testing.T) {
	rf := func(string) ([]byte, error) { return []byte("line1\n\tfoo()\nline3\n"), nil }
	res := Verify([]Finding{{File: "x.go", Line: "2", Evidence: "+\tfoo()"}}, rf)
	if res.Kept != 1 || res.Refuted != 0 {
		t.Fatalf("kept=%d refuted=%d, want 1/0 (dấu + của diff phải bị bỏ)", res.Kept, res.Refuted)
	}
}
