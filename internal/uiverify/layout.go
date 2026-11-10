package uiverify

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

// OverflowTolerancePx absorbs sub-pixel rounding from getBoundingClientRect.
const OverflowTolerancePx = 1.0

type Box struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
}

// Layout is the in-page measurement of one child against its container.
type Layout struct {
	InnerWidth float64 `json:"innerWidth"`
	Child      Box     `json:"child"`
	Container  Box     `json:"container"`
	Padding    Box     `json:"padding"`
	OverflowX  string  `json:"overflowX"`
	OverflowY  string  `json:"overflowY"`
}

type Overflow struct {
	Edge string  `json:"edge"`
	Px   float64 `json:"px"`
}

func badLayout(format string, a ...any) error {
	return exitcode.New(exitcode.BadArgs, fmt.Errorf(format, a...))
}

// ParseLayout decodes strictly: unknown and missing fields are errors, so a
// drifting measurement script fails loudly instead of reading as "no overflow".
func ParseLayout(raw string) (Layout, error) {
	var l Layout
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return Layout{}, badLayout("layout measurement: %v", err)
	}
	if err := requireFields(raw); err != nil {
		return Layout{}, err
	}
	for name, v := range map[string]float64{
		"padding.left": l.Padding.Left, "padding.top": l.Padding.Top,
		"padding.right": l.Padding.Right, "padding.bottom": l.Padding.Bottom,
	} {
		if v < 0 {
			return Layout{}, badLayout("layout measurement: %s must be >= 0, got %v", name, v)
		}
	}
	if l.OverflowX == "" {
		return Layout{}, badLayout("layout measurement: overflowX is empty")
	}
	if l.OverflowY == "" {
		return Layout{}, badLayout("layout measurement: overflowY is empty")
	}
	return l, nil
}

// requireFields names the first absent field; json.Decode leaves it zero.
func requireFields(raw string) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &top); err != nil {
		return badLayout("layout measurement: %v", err)
	}
	for _, k := range []string{"innerWidth", "overflowX", "overflowY"} {
		if _, ok := top[k]; !ok {
			return badLayout("layout measurement: missing field %s", k)
		}
	}
	for _, k := range []string{"child", "container", "padding"} {
		sub, ok := top[k]
		if !ok {
			return badLayout("layout measurement: missing field %s", k)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(sub, &m); err != nil {
			return badLayout("layout measurement: %s: %v", k, err)
		}
		for _, e := range []string{"left", "top", "right", "bottom"} {
			if _, ok := m[e]; !ok {
				return badLayout("layout measurement: missing field %s.%s", k, e)
			}
		}
	}
	return nil
}

func scrolls(overflow string) bool { return overflow == "auto" || overflow == "scroll" }

// Overflows returns the child edges that pass the container's content box by
// more than OverflowTolerancePx, in left, top, right, bottom order. An axis
// whose overflow is auto/scroll is exempt: the user can scroll to it.
func Overflows(l Layout) []Overflow {
	content := Box{
		Left:   l.Container.Left + l.Padding.Left,
		Top:    l.Container.Top + l.Padding.Top,
		Right:  l.Container.Right - l.Padding.Right,
		Bottom: l.Container.Bottom - l.Padding.Bottom,
	}
	cands := []struct {
		edge   string
		px     float64
		exempt bool
	}{
		{"left", content.Left - l.Child.Left, scrolls(l.OverflowX)},
		{"top", content.Top - l.Child.Top, scrolls(l.OverflowY)},
		{"right", l.Child.Right - content.Right, scrolls(l.OverflowX)},
		{"bottom", l.Child.Bottom - content.Bottom, scrolls(l.OverflowY)},
	}
	var out []Overflow
	for _, c := range cands {
		if !c.exempt && c.px > OverflowTolerancePx {
			out = append(out, Overflow{c.edge, c.px})
		}
	}
	return out
}
