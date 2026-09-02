package observe

import "fmt"

// State is the per-session persisted counter.
type State struct {
	Count              int `json:"count"`
	LastWarnedMultiple int `json:"last_warned_multiple"`
}

// Decision is the outcome of one Task dispatch.
type Decision struct {
	Warn    bool
	Message string
}

// Evaluate applies one dispatch to st and reports whether to warn. It warns
// once per multiple of softCap (softCap, 2*softCap, ...), never on intermediate counts.
// softCap must be > 0; the caller disables the feature by short-circuiting on
// softCap <= 0 before reaching here.
func Evaluate(st State, softCap int) (State, Decision) {
	st.Count++
	if st.Count/softCap > st.LastWarnedMultiple {
		st.LastWarnedMultiple = st.Count / softCap
		return st, Decision{Warn: true, Message: warnMessage(st.Count, softCap)}
	}
	return st, Decision{}
}

func warnMessage(count, softCap int) string {
	return fmt.Sprintf(
		"⚠️ [observe] This session has dispatched %d subagents (soft cap %d). "+
			"House rule #10: fan-out should be bounded — single fact-find = 1, "+
			"comparison = 2-4, complex = 3-5+, rarely 10+. Consider whether more "+
			"parallel agents are warranted before dispatching this one.",
		count, softCap)
}
