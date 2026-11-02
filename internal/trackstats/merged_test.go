package trackstats

import "testing"

func TestParsePRTitle(t *testing.T) {
	got, err := parsePRTitle([]byte(`{"title":"feat(track): kitchen"}`))
	if err != nil || got != "feat(track): kitchen" {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err := parsePRTitle([]byte(`not json`)); err == nil {
		t.Fatal("bad json: want an error")
	}
}
