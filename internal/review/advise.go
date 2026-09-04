// Package review — phần advisory-supervision (seam POST của znf:review, M4f).
// AdviseGate quyết định CƠ HỌC có nên gọi adviser LLM không, dựa trên tín hiệu
// rủi ro của diff + hình dạng findings. KHÔNG dùng LLM; fail-open ở tầng CLI.
package review

// Ngưỡng gate (mold doctrine.go: rule-data ở package-level).
const (
	// LargeCleanLOC: review 0-finding trên diff lớn hơn ngưỡng này bị coi là đáng ngờ.
	LargeCleanLOC = 200
	// ManyFindings: đủ số finding để soi pattern xuyên findings.
	ManyFindings = 4
)

// AdviseFinding: một finding đã gộp (gate chỉ cần dimension + severity).
type AdviseFinding struct {
	Dimension string `json:"dimension"`
	Severity  string `json:"severity"`
}

// AdviseInput: input cho gate ở POST.
type AdviseInput struct {
	Shared    bool            `json:"shared"`
	Critical  bool            `json:"critical"`
	Added     int             `json:"added"`
	Findings  []AdviseFinding `json:"findings"`
	Shippable bool            `json:"shippable"`
}

// AdviseGate quyết định có gọi adviser không. advise = len(signals) > 0.
// Bật khi bất kỳ: shared-contract · critical · review sạch trên diff lớn ·
// đủ nhiều finding để soi pattern. Deterministic, không side-effect.
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
