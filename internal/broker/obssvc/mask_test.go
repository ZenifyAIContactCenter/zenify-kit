package obssvc

import "testing"

func TestMask(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a.b@x.io", "<email>"},
		{"call 0912 345 678 now", "call <phone> now"},
		{"+84912345678", "<phone>"},
		{"0912.345.678", "<phone>"},
		{"mongodb://u:p@h:27017/db", "mongodb://<cred>@h:27017/db"},
		{"Bearer abc.def", "Bearer <token>"},
		{"eyJhbGciOi.eyJzdWIiOiIx.sig-nature_1", "<jwt>"},
		{"10.0.0.12", "<ip>"},
		{"from 10.0.0.12:8080", "from <ip>:8080"},
		{"65f1a2b3c4d5e6f708192a3b", "65f1a2b3c4d5e6f708192a3b"},
		{"1792299640000", "1792299640000"},
		{"order 12345", "order 12345"},
		{"2026-10-18 05:01:00", "2026-10-18 05:01:00"},
		{"2026/10/18 0501", "2026/10/18 0501"},
		{"id:0912345678", "id:0912345678"},
		{"x0912345678", "x0912345678"},
	}
	for _, c := range cases {
		if got := Mask(c.in); got != c.want {
			t.Errorf("Mask(%q) = %q want %q", c.in, got, c.want)
		}
	}
}
