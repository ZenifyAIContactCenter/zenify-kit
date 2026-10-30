package trackstats

import (
	"sort"
	"strconv"
	"time"
)

// WaitMin is the waiting time of an item in whole minutes.
type WaitMin struct {
	Ask        int `json:"ask"`
	Permission int `json:"permission"`
	Idle       int `json:"idle"`
	Total      int `json:"total"`
}

// PhaseMin is the agent and wait time spent in one phase.
type PhaseMin struct {
	AgentMin int `json:"agentMin"`
	WaitMin  int `json:"waitMin"`
}

// Tokens is the summed usage of one model.
type Tokens struct {
	In            int `json:"in"`
	Out           int `json:"out"`
	CacheRead     int `json:"cacheRead"`
	CacheCreation int `json:"cacheCreation"`
}

// Timing is the computed time and token figures of one item.
type Timing struct {
	AgentMin int
	Wait     WaitMin
	Phases   map[string]PhaseMin
	Tokens   map[string]Tokens
	Phase    string
}

// span is a half-open interval in unix milliseconds.
type span struct{ a, b int64 }

func merge(in []span) []span {
	s := append([]span(nil), in...)
	sort.Slice(s, func(i, j int) bool { return s[i].a < s[j].a })
	var out []span
	for _, v := range s {
		if v.b <= v.a {
			continue
		}
		if n := len(out); n > 0 && v.a <= out[n-1].b {
			if v.b > out[n-1].b {
				out[n-1].b = v.b
			}
			continue
		}
		out = append(out, v)
	}
	return out
}

// subtract removes the (merged) cut spans from the (merged) base spans.
func subtract(base, cut []span) []span {
	var out []span
	for _, v := range base {
		cur := v.a
		for _, c := range cut {
			if c.b <= cur || c.a >= v.b {
				continue
			}
			if c.a > cur {
				out = append(out, span{cur, c.a})
			}
			if c.b > cur {
				cur = c.b
			}
		}
		if cur < v.b {
			out = append(out, span{cur, v.b})
		}
	}
	return out
}

func clip(in []span, a, b int64) []span {
	var out []span
	for _, v := range in {
		if v.a < a {
			v.a = a
		}
		if v.b > b {
			v.b = b
		}
		if v.b > v.a {
			out = append(out, v)
		}
	}
	return out
}

func length(in []span) int64 {
	var n int64
	for _, v := range in {
		n += v.b - v.a
	}
	return n
}

func ceilMin(ms int64) int { return int((ms + 59999) / 60000) }

type phaseWin struct {
	name string
	a, b int64
}

// ComputeTiming computes the timing of item id from its events (from every
// session; events of other ids are ignored). A turn is the interval
// [at-ms, at]. Agent time is the union of the main turns minus the ask and
// permission waits, plus the sub turns. The first phase is design for a cook
// and diagnose otherwise.
func ComputeTiming(id, kind string, _ time.Time, ev []Event) Timing {
	first := "diagnose"
	if kind == "cook" {
		first = "design"
	}
	var mainT, subT, askPerm, askS, permS, allWait []span
	toks := map[string]Tokens{}
	type sw struct {
		at   int64
		name string
	}
	var switches []sw
	var totalIdleMs int64

	for _, e := range ev {
		if e.ID != id {
			continue
		}
		at := e.At.UnixMilli()
		switch e.Type {
		case "turn":
			if len(e.F) != 7 {
				continue
			}
			ms, _ := strconv.ParseInt(e.F[0], 10, 64)
			sp := span{at - ms, at}
			if e.F[1] == "sub" {
				subT = append(subT, sp)
			} else {
				mainT = append(mainT, sp)
			}
			if e.F[2] != "-" {
				t := toks[e.F[2]]
				t.In += atoiDash(e.F[3])
				t.Out += atoiDash(e.F[4])
				t.CacheRead += atoiDash(e.F[5])
				t.CacheCreation += atoiDash(e.F[6])
				toks[e.F[2]] = t
			}
		case "wait":
			if len(e.F) != 2 {
				continue
			}
			ms, _ := strconv.ParseInt(e.F[0], 10, 64)
			sp := span{at - ms, at}
			allWait = append(allWait, sp)
			switch e.F[1] {
			case "ask":
				askS = append(askS, sp)
				askPerm = append(askPerm, sp)
			case "permission":
				permS = append(permS, sp)
				askPerm = append(askPerm, sp)
			case "idle":
				totalIdleMs += ms
			}
		case "phase":
			if len(e.F) == 1 {
				switches = append(switches, sw{at, e.F[0]})
			}
		}
	}

	agent := merge(append(subtract(merge(mainT), merge(askPerm)), subT...))
	t := Timing{
		AgentMin: ceilMin(length(agent)),
		Wait: WaitMin{
			Ask:        ceilMin(length(merge(askS))),
			Permission: ceilMin(length(merge(permS))),
			Idle:       ceilMin(totalIdleMs),
		},
		Phases: map[string]PhaseMin{},
		Tokens: toks,
	}
	t.Wait.Total = t.Wait.Ask + t.Wait.Permission + t.Wait.Idle

	sort.SliceStable(switches, func(i, j int) bool { return switches[i].at < switches[j].at })
	const minT, maxT = int64(-1) << 62, int64(1) << 62
	wins := []phaseWin{{first, minT, maxT}}
	for _, s := range switches {
		wins[len(wins)-1].b = s.at
		wins = append(wins, phaseWin{s.name, s.at, maxT})
	}
	t.Phase = wins[len(wins)-1].name
	waits := merge(allWait)
	for _, w := range wins {
		p := t.Phases[w.name]
		p.AgentMin += ceilMin(length(clip(agent, w.a, w.b)))
		p.WaitMin += ceilMin(length(clip(waits, w.a, w.b)))
		t.Phases[w.name] = p
	}
	return t
}

func atoiDash(v string) int {
	n, _ := strconv.Atoi(v)
	return n
}
