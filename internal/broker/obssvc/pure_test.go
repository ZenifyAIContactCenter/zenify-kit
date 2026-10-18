package obssvc

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestWindows(t *testing.T) {
	end := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		since time.Duration
		n     int
		last  time.Duration
		err   error
	}{
		{24 * time.Hour, 12, 2 * time.Hour, nil},
		{48 * time.Hour, 24, 2 * time.Hour, nil},
		{5 * time.Hour, 3, time.Hour, nil},
		{49 * time.Hour, 0, 0, ErrBadSince},
		{0, 0, 0, ErrBadSince},
	}
	for _, c := range cases {
		w, err := Windows(c.since, end)
		if !errors.Is(err, c.err) {
			t.Fatalf("since %v: err %v want %v", c.since, err, c.err)
		}
		if err != nil {
			continue
		}
		if len(w) != c.n {
			t.Fatalf("since %v: %d windows want %d", c.since, len(w), c.n)
		}
		if !w[0].Start.Equal(end.Add(-c.since)) || !w[len(w)-1].End.Equal(end) {
			t.Fatalf("since %v: bounds %v..%v", c.since, w[0].Start, w[len(w)-1].End)
		}
		for i := 1; i < len(w); i++ {
			if !w[i].Start.Equal(w[i-1].End) {
				t.Fatalf("not contiguous at %d", i)
			}
		}
		if got := w[len(w)-1].End.Sub(w[len(w)-1].Start); got != c.last {
			t.Fatalf("last window %v want %v", got, c.last)
		}
	}
}

func TestPadTraceID(t *testing.T) {
	ok := []struct{ in, want string }{
		{"632dc9d174626f37496dc76c4c031", "000632dc9d174626f37496dc76c4c031"},
		{"632dc9d174626f37496dc76c4c031abc", "632dc9d174626f37496dc76c4c031abc"},
		{"ABC", "00000000000000000000000000000abc"},
	}
	for _, c := range ok {
		p, o, err := PadTraceID(c.in)
		if err != nil || p != c.want || o != c.in {
			t.Errorf("%q: got %q,%q,%v want %q", c.in, p, o, err, c.want)
		}
	}
	for _, in := range []string{strings.Repeat("a", 33), "0x12", "xyz", ""} {
		if _, _, err := PadTraceID(in); !errors.Is(err, ErrBadTraceID) {
			t.Errorf("%q: err %v want ErrBadTraceID", in, err)
		}
	}
}

func TestQuantile(t *testing.T) {
	h := Hist{0.033554432: 50, 0.067108864: 50}
	if got := Quantile(h, 0.5); got != 0.033554432 {
		t.Errorf("p50 = %v", got)
	}
	if got := Quantile(h, 0.95); got <= 0.0335 || got > 0.0672 {
		t.Errorf("p95 = %v", got)
	}
	if got := Quantile(Hist{}, 0.5); got != 0 {
		t.Errorf("empty = %v", got)
	}
}

func TestTruncate(t *testing.T) {
	s := strings.Repeat("Việt Nam ", 67) // > 600 B
	if len(s) < 600 {
		t.Fatalf("fixture only %d B", len(s))
	}
	got := Truncate(s, 500)
	if len(got) > 500 || !utf8.ValidString(got) {
		t.Errorf("len %d valid %v", len(got), utf8.ValidString(got))
	}
	if Truncate("abc", 500) != "abc" {
		t.Error("short string changed")
	}
}

func TestAllowAttr(t *testing.T) {
	for _, k := range []string{"http.route", "db.statement", "exception.message", "otel.status_code"} {
		if !AllowAttr(k, nil) {
			t.Errorf("%s should be allowed", k)
		}
	}
	for _, k := range []string{"db.connection_string", "user.id", "enduser.id", "client.address", "http.user_agent", "net.peer.ip", "user_agent.original"} {
		if AllowAttr(k, nil) {
			t.Errorf("%s should be denied", k)
		}
	}
	if AllowAttr("tenant.id", nil) {
		t.Error("tenant.id denied without extra")
	}
	if !AllowAttr("tenant.id", map[string]string{"tenant.id": "x"}) {
		t.Error("tenant.id allowed via extra")
	}
}

type fakeCap struct {
	Logs  []string `json:"logs"`
	Errs  []string `json:"errors"`
	Grps  []string `json:"groups"`
	order []string
}

func (f *fakeCap) Drop() (string, bool) {
	switch {
	case len(f.Logs) > 0:
		f.Logs = f.Logs[:len(f.Logs)-1]
		f.order = append(f.order, "logs")
		return "logs", true
	case len(f.Errs) > 0:
		f.Errs = f.Errs[:len(f.Errs)-1]
		f.order = append(f.order, "errors")
		return "errors", true
	case len(f.Grps) > 0:
		f.Grps = f.Grps[:len(f.Grps)-1]
		f.order = append(f.order, "groups")
		return "groups", true
	}
	return "", false
}

func TestCapJSON(t *testing.T) {
	item := strings.Repeat("x", 1000)
	f := &fakeCap{}
	for i := 0; i < 40; i++ {
		f.Logs = append(f.Logs, item)
		f.Errs = append(f.Errs, item)
		f.Grps = append(f.Grps, item)
	}
	if b, _ := json.Marshal(f); len(b) < 100000 {
		t.Fatalf("fixture only %d B", len(b))
	}
	out, err := CapJSON(f, 32768)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) > 32768 {
		t.Errorf("output %d B", len(out))
	}
	if len(f.order) == 0 || f.order[0] != "logs" || f.order[len(f.order)-1] == "logs" {
		t.Errorf("drop order %v", f.order)
	}
	seenErr := false
	for _, k := range f.order {
		if k == "errors" {
			seenErr = true
		}
		if k == "logs" && seenErr {
			t.Fatalf("logs dropped after errors: %v", f.order)
		}
	}
}

func TestCapJSONExhausted(t *testing.T) {
	f := &fakeCap{Logs: []string{strings.Repeat("x", 100)}}
	out, err := CapJSON(f, 10)
	if err != nil || len(out) == 0 {
		t.Fatalf("out %q err %v", out, err)
	}
}
