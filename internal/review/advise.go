// Package review — the advisory-supervision part (POST seam of znf:review, M4f).
// AdviseGate MECHANICALLY decides whether to call the LLM adviser, based on
// diff risk signals + findings shape. Does NOT use an LLM; fail-open at the CLI layer.
package review

// Gate thresholds (mirrors doctrine.go: rule-data at package level).
const (
	// LargeCleanLOC: a 0-finding review on a diff larger than this threshold is considered suspicious.
	LargeCleanLOC = 200
	// ManyFindings: enough findings to look for a pattern across them.
	ManyFindings = 4
)

// AdviseFinding: one merged finding (the gate only needs dimension + severity).
type AdviseFinding struct {
	Dimension string `json:"dimension"`
	Severity  string `json:"severity"`
}

// AdviseInput: input for the gate at POST.
type AdviseInput struct {
	Shared    bool            `json:"shared"`
	Critical  bool            `json:"critical"`
	Added     int             `json:"added"`
	Findings  []AdviseFinding `json:"findings"`
	Shippable bool            `json:"shippable"`
}

// AdviseGate decides whether to call the adviser. advise = len(signals) > 0.
// Fires when any of: shared-contract · critical · clean review on a large diff ·
// enough findings to look for a pattern. Deterministic, no side effects.
func AdviseGate(in AdviseInput) (advise bool, signals []string) {
	if in.Shared {
		signals = append(signals, "shared-contract touched")
	}
	if in.Critical {
		signals = append(signals, "critical-flagged change")
	}
	if len(in.Findings) == 0 && in.Added > LargeCleanLOC {
		signals = append(signals, "clean review on large diff")
	}
	if len(in.Findings) >= ManyFindings {
		signals = append(signals, "enough findings to assess a pattern")
	}
	return len(signals) > 0, signals
}
