package trackstats

import (
	"errors"
	"testing"
)

func TestIsFullID(t *testing.T) {
	for ref, want := range map[string]bool{
		"b-261111-b81e":                      true,
		"cook-261111-ab12":                   true,
		"https://github.com/o/r/issues/5":    true,
		"repos/kit/.znf/sdd/p/progress.md:3": true,
		"b81e":                               false,
		"#5":                                 false,
		"infrastructure#5":                   false,
		"ZEN-288":                            false,
	} {
		if got := IsFullID(ref); got != want {
			t.Errorf("IsFullID(%q) = %v, want %v", ref, got, want)
		}
	}
}

func TestResolve(t *testing.T) {
	const (
		bid   = "b-261111-b81e"
		track = "cook-261111-c0de"
		gh581 = "https://github.com/Z/zenify-kit/issues/581"
		infra = "https://github.com/Z/infrastructure/issues/5"
		dash  = "https://github.com/Z/zenify-dashboard/issues/5"
		plane = "https://plane.example.com/browse/ZEN-288/"
	)
	cands := []Candidate{
		{ID: bid, Code: "b81e", Repo: "kit", Title: "manual"},
		{ID: track, Code: "c0de", Repo: "hub", Title: "cook hub"},
		{ID: gh581, Code: "#581", Repo: "zenify-kit", Title: "kit issue"},
		{ID: infra, Code: "#5", Repo: "infrastructure", Title: "infra issue"},
		{ID: dash, Code: "#5", Repo: "zenify-dashboard", Title: "dash issue"},
		{ID: plane, Code: "ZEN-288", Repo: "", Title: "plane item"},
		// The same id seen twice (plan and backlog) is one match, not two.
		{ID: gh581, Code: "#581", Repo: "zenify-kit", Title: "kit issue"},
	}
	for ref, want := range map[string]string{
		"b81e":             bid,
		"c0de":             track,
		"#581":             gh581,
		"ZEN-288":          plane,
		"zen-288":          plane,
		"infrastructure#5": infra,
	} {
		got, many, err := Resolve(ref, cands)
		if err != nil || got.ID != want || many != nil {
			t.Errorf("Resolve(%q) = %q %v %v, want %q", ref, got.ID, many, err, want)
		}
	}
	for _, ref := range []string{"zzzz", "ffff", "#6", "kit#581", "ZEN-289", "nonsense"} {
		if _, _, err := Resolve(ref, cands); !errors.Is(err, ErrNoMatch) {
			t.Errorf("Resolve(%q) err = %v, want ErrNoMatch", ref, err)
		}
	}
	got, many, err := Resolve("#5", cands)
	if !errors.Is(err, ErrAmbiguous) || got.ID != "" || len(many) != 2 || many[0].ID != infra || many[1].ID != dash {
		t.Fatalf("Resolve(#5) = %+v %+v %v", got, many, err)
	}
}
