package uiverify

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/exitcode"
)

func lay(child Box, ox, oy string) Layout {
	return Layout{
		InnerWidth: 100,
		Child:      child,
		Container:  Box{0, 0, 100, 100},
		Padding:    Box{4, 4, 4, 4},
		OverflowX:  ox,
		OverflowY:  oy,
	}
}

func TestOverflows(t *testing.T) {
	tests := []struct {
		name string
		in   Layout
		want []Overflow
	}{
		{"inside", lay(Box{10, 10, 90, 90}, "visible", "visible"), nil},
		{"right visible", lay(Box{10, 10, 101, 90}, "visible", "visible"), []Overflow{{"right", 5}}},
		{"right auto exempt", lay(Box{10, 10, 101, 90}, "auto", "visible"), nil},
		{"right scroll exempt", lay(Box{10, 10, 101, 90}, "scroll", "visible"), nil},
		{"within tolerance", lay(Box{10, 10, 96.6, 90}, "visible", "visible"), nil},
		{"left hidden", lay(Box{2, 10, 90, 90}, "hidden", "visible"), []Overflow{{"left", 2}}},
		{"left clip", lay(Box{2, 10, 90, 90}, "clip", "visible"), []Overflow{{"left", 2}}},
		{"bottom, x scroll does not exempt y", lay(Box{10, 10, 90, 105}, "auto", "visible"), []Overflow{{"bottom", 9}}},
		{"bottom y auto exempt", lay(Box{10, 10, 90, 105}, "visible", "auto"), nil},
		{"two edges ordered", lay(Box{2, 10, 90, 105}, "visible", "visible"), []Overflow{{"left", 2}, {"bottom", 9}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Overflows(tc.in)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

const validLayout = `{"innerWidth":100,"child":{"left":10,"top":10,"right":90,"bottom":90},` +
	`"container":{"left":0,"top":0,"right":100,"bottom":100},` +
	`"padding":{"left":4,"top":4,"right":4,"bottom":4},"overflowX":"visible","overflowY":"auto"}`

func TestParseLayout(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string // empty = ok
	}{
		{"ok", validLayout, ""},
		{"missing container.bottom", strings.Replace(validLayout, `,"bottom":100}`, `}`, 1), "container.bottom"},
		{"unknown field", strings.Replace(validLayout, `"innerWidth":100,`, `"innerWidth":100,"foo":1,`, 1), "foo"},
		{"negative padding", strings.Replace(validLayout, `"left":4`, `"left":-1`, 1), "padding.left"},
		{"empty overflowX", strings.Replace(validLayout, `"overflowX":"visible"`, `"overflowX":""`, 1), "overflowX"},
		{"not json", "nope", "layout"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l, err := ParseLayout(tc.raw)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				if l.Child.Right != 90 || l.OverflowY != "auto" || l.Padding.Top != 4 {
					t.Fatalf("bad parse: %+v", l)
				}
				return
			}
			var ee *exitcode.Error
			if !errors.As(err, &ee) || ee.Code != exitcode.BadArgs {
				t.Fatalf("want BadArgs, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q should name %q", err, tc.wantErr)
			}
		})
	}
}
