package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/observe"
)

func lister(ss []observe.SessionSummary, err error) func() ([]observe.SessionSummary, error) {
	return func() ([]observe.SessionSummary, error) { return ss, err }
}

func TestReport_TableHasRowAndHumanBytes(t *testing.T) {
	reportNow = func() time.Time { return time.Unix(1000, 0) }
	defer func() { reportNow = time.Now }()
	sessions := []observe.SessionSummary{
		{ID: "sess-a", Count: 3, Calls: 5, Bytes: 2048, ModTime: time.Unix(940, 0)},
	}
	var out bytes.Buffer
	if err := runObserveReport(&out, false, lister(sessions, nil)); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"SESSION", "DISPATCH", "TOOL-OUT", "sess-a", "2KB", "1m ago"} {
		if !strings.Contains(s, want) {
			t.Fatalf("table missing %q:\n%s", want, s)
		}
	}
}

func TestReport_JSON(t *testing.T) {
	sessions := []observe.SessionSummary{{ID: "sess-a", Count: 3, Calls: 5, Bytes: 2048}}
	var out bytes.Buffer
	if err := runObserveReport(&out, true, lister(sessions, nil)); err != nil {
		t.Fatal(err)
	}
	var got []observe.SessionSummary
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out.String())
	}
	if len(got) != 1 || got[0].ID != "sess-a" || got[0].Bytes != 2048 {
		t.Fatalf("json roundtrip wrong: %+v", got)
	}
}

func TestReport_EmptyMessage(t *testing.T) {
	var out bytes.Buffer
	if err := runObserveReport(&out, false, lister(nil, nil)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No observe data yet") {
		t.Fatalf("empty report should explain, got %q", out.String())
	}
}

func TestHumanAge(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	cases := []struct {
		t    time.Time
		want string
	}{
		{time.Time{}, "—"},
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-50 * time.Hour), "2d ago"},
	}
	for _, c := range cases {
		if got := humanAge(c.t, now); got != c.want {
			t.Errorf("humanAge(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}
