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
		// a ':' or '/' counts as part of a word only when a digit sits beyond it
		{"id:0912345678", "id:<phone>"},
		{"/api/contacts/0912345678", "/api/contacts/<phone>"},
		{"phone:0912345678", "phone:<phone>"},
		{"tel:0912345678", "tel:<phone>"},
		{"at 12:34 on 2026/10/18", "at 12:34 on 2026/10/18"},
		{"2026-10-18T05:01:00.123456789Z", "2026-10-18T05:01:00.123456789Z"},
		{"redis://:s3cret@10.0.0.5:6379", "redis://<cred>@<ip>:6379"},
		{"mongodb://u:p@ss@h/db", "mongodb://<cred>@h/db"},
		{"amqp://u:p%2Fq@h", "amqp://<cred>@h"},
		{"http://h:8080/a@b", "http://h:8080/a@b"},
		{"Authorization: Basic dXNlcjpwYXNz", "Authorization: Basic <token>"},
		{"basic QWxhZGRpbjpvcGVu==", "Basic <token>"},
		{"Basic abc", "Basic abc"},
		{"password=hunter2 next", "password=<secret> next"},
		{"PWD: hunter2", "PWD: <secret>"},
		{"passwd=x&user=y", "passwd=<secret>&user=y"},
		{"secret=s token=t", "secret=<secret> token=<secret>"},
		{`{"access_token":"a b c","refresh_token": "r"}`, `{"access_token":"<secret>","refresh_token": "<secret>"}`},
		{`api_key="k1" apikey: k2 x-api-key=k3`, `api_key="<secret>" apikey: <secret> x-api-key=<secret>`},
		{"authorization: Bearer abc", "authorization: Bearer <token>"},
		{"token_count=5", "token_count=5"},
		{"the password is wrong", "the password is wrong"},
		{"password", "password"},
		{"order 12345 0912345678", "order 12345 <phone>"},
		{"2026-10-18 0912345678", "2026-10-18 <phone>"},
		{"bearer x", "Bearer <token>"},
		{"0912345678 12", "<phone> 12"},
		// greedy 11-digit match "0912 345 678 9" is standalone and single-separator: whole run masked
		{"call 0912 345 678 9", "call <phone>"},
		{"x0912345678", "x0912345678"},
	}
	for _, c := range cases {
		if got := Mask(c.in); got != c.want {
			t.Errorf("Mask(%q) = %q want %q", c.in, got, c.want)
		}
	}
}
