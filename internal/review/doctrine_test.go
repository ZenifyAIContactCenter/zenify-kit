package review

import (
	"reflect"
	"testing"
)

func TestSanitizeVerified_StripsPureVerdict(t *testing.T) {
	clean, stripped := SanitizeVerified("✅ Behaviour verified")
	if clean != "" {
		t.Fatalf("clean=%q, want empty", clean)
	}
	if !reflect.DeepEqual(stripped, []string{"✅ Behaviour verified"}) {
		t.Fatalf("stripped=%v", stripped)
	}
}

func TestSanitizeVerified_KeepsFactLineWithVerdictWord(t *testing.T) {
	in := "12/12 pass — behaviour verified via pm test src/auth"
	clean, stripped := SanitizeVerified(in)
	if clean != in {
		t.Fatalf("clean=%q, want unchanged", clean)
	}
	if len(stripped) != 0 {
		t.Fatalf("stripped=%v", stripped)
	}
}

func TestSanitizeVerified_KeepsPlainFactsAndBlanks(t *testing.T) {
	in := "Ran: pm test src/modules/auth\n\n5 spec files touched\nlint clean"
	clean, stripped := SanitizeVerified(in)
	if clean != in {
		t.Fatalf("clean=%q, want unchanged", clean)
	}
	if len(stripped) != 0 {
		t.Fatalf("stripped=%v", stripped)
	}
}

func TestSanitizeVerified_KeepsUntestedGapLineWithVerdictWord(t *testing.T) {
	// dòng "no test" là dòng giá trị nhất — KHÔNG được strip dù có verdict-word.
	in := "not verified: no test covers this branch"
	clean, stripped := SanitizeVerified(in)
	if clean != in {
		t.Fatalf("gap line stripped: clean=%q stripped=%v", clean, stripped)
	}
}

func TestSanitizeVerified_FailOpenEmpty(t *testing.T) {
	clean, stripped := SanitizeVerified("")
	if clean != "" || stripped != nil {
		t.Fatalf("clean=%q stripped=%v, want empty/nil", clean, stripped)
	}
}
