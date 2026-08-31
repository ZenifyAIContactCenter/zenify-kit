package managed

import "testing"

func TestDecideRefresh(t *testing.T) {
	m := &Manifest{Entries: map[string]Entry{
		"a.txt": {Path: "a.txt", SHA256: Fingerprint([]byte("orig"))},
	}}
	cases := []struct {
		name   string
		path   string
		onDisk []byte
		want   Decision
	}{
		{"unchanged -> update", "a.txt", []byte("orig"), DecisionUpdate},
		{"modified -> keep", "a.txt", []byte("edited"), DecisionKeepModified},
		{"user-added -> keep", "b.txt", []byte("whatever"), DecisionKeepUserAdded},
	}
	for _, c := range cases {
		if got := m.DecideRefresh(c.path, c.onDisk); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
