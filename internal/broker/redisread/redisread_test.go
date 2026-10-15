package redisread

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
)

var testKey = []byte("test-key")

// fakeDoer records every argv it is handed and answers with val / err.
type fakeDoer struct {
	mu    sync.Mutex
	got   [][]any
	val   any
	err   error
	byCmd map[string]any   // upper-case command -> reply, consulted before val
	errBy map[string]error // upper-case command -> error, consulted first
	gate  chan struct{}    // non-nil: a command not in byCmd waits on it
	enter chan struct{}    // non-nil: signalled before waiting on gate
}

func (f *fakeDoer) answer(c *redis.Cmd, args []any) {
	f.mu.Lock()
	f.got = append(f.got, args)
	name := strings.ToUpper(args[0].(string))
	e, hasErr := f.errBy[name]
	r, ok := f.byCmd[name]
	f.mu.Unlock()
	switch {
	case hasErr:
		c.SetErr(e)
	case ok:
		c.SetVal(r)
	default:
		if f.gate != nil {
			f.enter <- struct{}{}
			<-f.gate
		}
		if f.err != nil {
			c.SetErr(f.err)
		} else {
			c.SetVal(f.val)
		}
	}
}

func (f *fakeDoer) Do(ctx context.Context, args ...any) *redis.Cmd {
	c := redis.NewCmd(ctx, args...)
	f.answer(c, args)
	return c
}

// Pipelined queues fn's commands on a client that never connects, then
// answers each one as Do would, without Exec.
func (f *fakeDoer) Pipelined(ctx context.Context, fn func(redis.Pipeliner) error) ([]redis.Cmder, error) {
	cli := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer func() { _ = cli.Close() }()
	p := cli.Pipeline()
	if err := fn(p); err != nil {
		return nil, err
	}
	cmds := p.Cmds()
	for _, c := range cmds {
		f.answer(c.(*redis.Cmd), c.Args())
	}
	return cmds, nil
}

// sent reports whether cmd (upper case) reached the Doer.
func (f *fakeDoer) sent(cmd string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.got {
		if strings.EqualFold(a[0].(string), cmd) {
			return true
		}
	}
	return false
}

type fixture struct {
	stg, prd *fakeDoer
	audit    *bytes.Buffer
	ts       *httptest.Server
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{stg: &fakeDoer{val: "v"}, prd: &fakeDoer{val: "v"}, audit: &bytes.Buffer{}}
	h := &Handler{Clients: map[string]Doer{"stg": f.stg, "prd": f.prd}, Audit: &audit.Logger{W: f.audit}, MaxConcurrent: 2}
	f.ts = httptest.NewServer(auth.Require(testKey, h))
	t.Cleanup(f.ts.Close)
	return f
}

func (f *fixture) post(t *testing.T, req api.RedisRequest) (int, []byte) {
	t.Helper()
	tok, _ := auth.Issue(testKey, "alice", time.Now())
	b, _ := json.Marshal(req)
	r, _ := http.NewRequest(http.MethodPost, f.ts.URL, bytes.NewReader(b))
	r.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func argv(a []any) []string {
	out := make([]string, len(a))
	for i, v := range a {
		out[i] = v.(string)
	}
	return out
}

func TestRedis_DelRejected(t *testing.T) {
	f := newFixture(t)
	st, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: "DEL", Args: []string{"k"}})
	var e api.Error
	_ = json.Unmarshal(body, &e)
	if st != 400 || e.Error != api.CodeCommandNotAllowed {
		t.Fatalf("got %d %s", st, body)
	}
	if len(f.stg.got)+len(f.prd.got) != 0 {
		t.Fatal("Doer must not be called")
	}
	if !strings.Contains(f.audit.String(), `"status":400`) || !strings.Contains(f.audit.String(), `"endpoint":"/redis"`) {
		t.Fatalf("audit: %s", f.audit)
	}
}

func TestRedis_RangeRewrites(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		args []string
		want []string // full argv sent, nil = rejected 400
	}{
		{"lrange all", "lrange", []string{"k", "0", "-1"}, []string{"LRANGE", "k", "0", "199"}},
		{"lrange small unchanged", "LRANGE", []string{"k", "10", "20"}, []string{"LRANGE", "k", "10", "20"}},
		{"lrange big", "LRANGE", []string{"k", "0", "5000"}, []string{"LRANGE", "k", "0", "199"}},
		{"lrange offset", "LRANGE", []string{"k", "100", "-1"}, []string{"LRANGE", "k", "100", "299"}},
		{"lrange neg both", "LRANGE", []string{"k", "-10", "-1"}, nil},
		{"lrange min int start", "LRANGE", []string{"k", "-9223372036854775808", "-1"}, nil},
		{"lrange max int start", "LRANGE", []string{"k", "9223372036854775807", "-1"}, nil},
		{"lrange huge stop no overflow", "LRANGE", []string{"k", "5", "9223372036854775807"}, []string{"LRANGE", "k", "5", "204"}},
		{"lrange neg start pos stop", "LRANGE", []string{"k", "-5", "100"}, nil},
		{"lrange not int", "LRANGE", []string{"k", "a", "b"}, nil},
		{"lrange wrong arity", "LRANGE", []string{"k", "0"}, nil},
		{"zrange index", "ZRANGE", []string{"z", "0", "-1", "WITHSCORES"}, []string{"ZRANGE", "z", "0", "199", "WITHSCORES"}},
		{"zrange byscore adds limit", "ZRANGE", []string{"z", "0", "100", "BYSCORE"}, []string{"ZRANGE", "z", "0", "100", "BYSCORE", "LIMIT", "0", "200"}},
		{"zrange bylex keeps offset caps count", "ZRANGE", []string{"z", "-", "+", "bylex", "LIMIT", "5", "9999"}, []string{"ZRANGE", "z", "-", "+", "bylex", "LIMIT", "5", "200"}},
		{"zrange byscore small limit kept", "ZRANGE", []string{"z", "0", "100", "BYSCORE", "limit", "10", "50"}, []string{"ZRANGE", "z", "0", "100", "BYSCORE", "LIMIT", "10", "50"}},
		{"zrange negative count", "ZRANGE", []string{"z", "0", "100", "BYSCORE", "LIMIT", "5", "-1"}, []string{"ZRANGE", "z", "0", "100", "BYSCORE", "LIMIT", "5", "200"}},
		{"zrange last limit wins", "ZRANGE", []string{"z", "0", "100", "BYSCORE", "LIMIT", "1", "2", "WITHSCORES", "LIMIT", "3", "400"}, []string{"ZRANGE", "z", "0", "100", "BYSCORE", "WITHSCORES", "LIMIT", "3", "200"}},
		{"zrange limit not int", "ZRANGE", []string{"z", "0", "100", "BYSCORE", "LIMIT", "a", "5"}, nil},
		{"zrange limit count not int", "ZRANGE", []string{"z", "0", "100", "BYSCORE", "LIMIT", "5", "1.5"}, nil},
		{"zrange limit missing value", "ZRANGE", []string{"z", "0", "100", "BYSCORE", "LIMIT", "5"}, nil},
		{"xrange adds count", "XRANGE", []string{"s", "-", "+"}, []string{"XRANGE", "s", "-", "+", "COUNT", "200"}},
		{"xrange lowers count", "XRANGE", []string{"s", "-", "+", "COUNT", "5000"}, []string{"XRANGE", "s", "-", "+", "COUNT", "200"}},
		{"xrange keeps small count", "XRANGE", []string{"s", "-", "+", "count", "50"}, []string{"XRANGE", "s", "-", "+", "COUNT", "50"}},
		{"xrange duplicate counts last wins capped", "XRANGE", []string{"s", "-", "+", "COUNT", "5", "COUNT", "99999"}, []string{"XRANGE", "s", "-", "+", "COUNT", "200"}},
		{"xrange duplicate counts last small", "XRANGE", []string{"s", "-", "+", "COUNT", "99999", "COUNT", "5"}, []string{"XRANGE", "s", "-", "+", "COUNT", "5"}},
		{"xrange count not int", "XRANGE", []string{"s", "-", "+", "COUNT", "x"}, nil},
		{"ttl untouched", "TTL", []string{"k"}, []string{"TTL", "k"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			st, body := f.post(t, api.RedisRequest{Env: "prd", Cmd: c.cmd, Args: c.args})
			if c.want == nil {
				if st != 400 || len(f.prd.got) != 0 {
					t.Fatalf("want 400 and no call, got %d %s calls=%d", st, body, len(f.prd.got))
				}
				return
			}
			if st != 200 || len(f.prd.got) != 1 {
				t.Fatalf("got %d %s calls=%d", st, body, len(f.prd.got))
			}
			if got := strings.Join(argv(f.prd.got[0]), " "); got != strings.Join(c.want, " ") {
				t.Fatalf("sent %q, want %q", got, strings.Join(c.want, " "))
			}
		})
	}
}

func TestRedis_ResultCapped(t *testing.T) {
	f := newFixture(t)
	f.stg.val = strings.Repeat("x", 2<<20)
	f.stg.byCmd = map[string]any{"STRLEN": int64(100)} // the guard must not hide a reply that is bigger anyway
	st, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: "GET", Args: []string{"big"}})
	var r api.RedisResponse
	if err := json.Unmarshal(body, &r); err != nil || st != 200 {
		t.Fatalf("%d %v", st, err)
	}
	if !r.Truncated || len(body) > 1<<20+256 {
		t.Fatalf("truncated=%v len=%d", r.Truncated, len(body))
	}
	if s, _ := r.Result.(string); len(s) < 1<<19 || strings.Trim(s, "x") != "" {
		t.Fatalf("result should be a long prefix of the string, len=%d", len(s))
	}
}

func TestRedis_ArrayCapped(t *testing.T) {
	f := newFixture(t)
	el := strings.Repeat("y", 1000)
	var arr []any
	for i := 0; i < 3000; i++ {
		arr = append(arr, el)
	}
	f.stg.val = arr
	st, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: "SMEMBERS", Args: []string{"s"}})
	var r api.RedisResponse
	_ = json.Unmarshal(body, &r)
	got, _ := r.Result.([]any)
	if st != 200 || !r.Truncated || len(body) > 1<<20+256 || len(got) == 0 || len(got) >= 3000 {
		t.Fatalf("st=%d truncated=%v len=%d n=%d", st, r.Truncated, len(body), len(got))
	}
}

func TestRedis_SmallResultNotTruncated(t *testing.T) {
	f := newFixture(t)
	f.stg.val = []any{"a", int64(2), nil}
	_, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: "MGET", Args: []string{"a", "b", "c"}})
	if strings.TrimSpace(string(body)) != `{"result":["a",2,null],"truncated":false}` {
		t.Fatalf("%s", body)
	}
}

func TestRedis_MapAndNil(t *testing.T) {
	f := newFixture(t)
	f.stg.val = map[any]any{"f": "1", int64(2): []any{"x"}}
	_, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: "HGETALL", Args: []string{"h"}})
	if strings.TrimSpace(string(body)) != `{"result":{"2":["x"],"f":"1"},"truncated":false}` {
		t.Fatalf("%s", body)
	}
	f.stg.val, f.stg.err = nil, redis.Nil
	st, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: "GET", Args: []string{"missing"}})
	if st != 200 || strings.TrimSpace(string(body)) != `{"result":null,"truncated":false}` {
		t.Fatalf("%d %s", st, body)
	}
}

func TestRedis_UnknownEnv400(t *testing.T) {
	f := newFixture(t)
	for _, env := range []string{"", "dev", "PRD"} {
		st, body := f.post(t, api.RedisRequest{Env: env, Cmd: "GET", Args: []string{"k"}})
		var e api.Error
		_ = json.Unmarshal(body, &e)
		if st != 400 || e.Error != api.CodeInvalidTarget {
			t.Fatalf("env %q: %d %s", env, st, body)
		}
	}
	if len(f.stg.got)+len(f.prd.got) != 0 {
		t.Fatal("Doer must not be called")
	}
}

func TestRedis_ExecErrorIs502WithoutAddress(t *testing.T) {
	f := newFixture(t)
	f.stg.err = errors.New("dial tcp 10.9.8.7:6379: connection refused")
	st, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: "GET", Args: []string{"k"}})
	var e api.Error
	_ = json.Unmarshal(body, &e)
	if st != 502 || e.Error != api.CodeQueryFailed || strings.Contains(string(body), "10.9.8.7") {
		t.Fatalf("%d %s", st, body)
	}
}

func TestRedis_AuditLine(t *testing.T) {
	f := newFixture(t)
	f.stg.val = "secret-value-123"
	f.post(t, api.RedisRequest{Env: "stg", Cmd: "get", Args: []string{"user:1"}})
	lines := strings.Split(strings.TrimSpace(f.audit.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("want exactly 1 audit line, got %d", len(lines))
	}
	var e map[string]any
	_ = json.Unmarshal([]byte(lines[0]), &e)
	if e["login"] != "alice" || e["endpoint"] != "/redis" || e["env"] != "stg" || e["op"] != "GET" || e["status"] != float64(200) {
		t.Fatalf("%s", lines[0])
	}
	if strings.Contains(lines[0], "secret-value-123") || !strings.Contains(lines[0], "user:1") {
		t.Fatalf("args must hold the key and never the result: %s", lines[0])
	}
}

func TestRedis_BadBody400(t *testing.T) {
	f := newFixture(t)
	tok, _ := auth.Issue(testKey, "alice", time.Now())
	r, _ := http.NewRequest(http.MethodPost, f.ts.URL, strings.NewReader("{nope"))
	r.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 || strings.Count(f.audit.String(), "\n") != 1 {
		t.Fatalf("%d %s", resp.StatusCode, f.audit)
	}
}

func TestRedis_BigGetUsesGetRange(t *testing.T) {
	f := newFixture(t)
	f.stg.byCmd = map[string]any{"STRLEN": int64(5 << 20), "GETRANGE": strings.Repeat("z", 1<<20)}
	st, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: "GET", Args: []string{"big"}})
	var r api.RedisResponse
	_ = json.Unmarshal(body, &r)
	if st != 200 || !r.Truncated || len(body) > 1<<20+256 {
		t.Fatalf("st=%d truncated=%v len=%d", st, r.Truncated, len(body))
	}
	var sent []string
	for _, a := range f.stg.got {
		sent = append(sent, strings.Join(argv(a), " "))
	}
	if strings.Join(sent, "|") != "STRLEN big|GETRANGE big 0 1048575" {
		t.Fatalf("sent %v", sent)
	}
}

func TestRedis_SmallGetIsPlainGet(t *testing.T) {
	f := newFixture(t)
	f.stg.byCmd = map[string]any{"STRLEN": int64(3)}
	_, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: "GET", Args: []string{"k"}})
	if !strings.Contains(string(body), `"truncated":false`) || len(f.stg.got) != 2 || argv(f.stg.got[1])[0] != "GET" {
		t.Fatalf("%s %v", body, f.stg.got)
	}
}

func TestRedis_MGetKeyLimit(t *testing.T) {
	f := newFixture(t)
	keys := make([]string, 101)
	for i := range keys {
		keys[i] = "k"
	}
	st, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: "MGET", Args: keys})
	if st != 400 || !strings.Contains(string(body), api.CodeCommandNotAllowed) || !strings.Contains(string(body), "100") || len(f.stg.got) != 0 {
		t.Fatalf("%d %s", st, body)
	}
	if st, _ = f.post(t, api.RedisRequest{Env: "stg", Cmd: "MGET", Args: keys[:100]}); st != 200 {
		t.Fatalf("100 keys: %d", st)
	}
}

func TestRedis_CollectionSizePrecheck(t *testing.T) {
	for cmd, count := range map[string]string{"HGETALL": "HLEN", "HKEYS": "HLEN", "SMEMBERS": "SCARD"} {
		f := newFixture(t)
		f.stg.byCmd = map[string]any{count: int64(10001)}
		st, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: cmd, Args: []string{"h"}})
		if st != 400 || !strings.Contains(string(body), api.CodeCommandNotAllowed) || len(f.stg.got) != 1 {
			t.Fatalf("%s: %d %s calls=%d", cmd, st, body, len(f.stg.got))
		}
		f.stg.byCmd = map[string]any{count: int64(10000)}
		if st, _ = f.post(t, api.RedisRequest{Env: "stg", Cmd: cmd, Args: []string{"h"}}); st != 200 {
			t.Fatalf("%s at limit: %d", cmd, st)
		}
	}
}

func TestRedis_NaNReplyEncoded(t *testing.T) {
	f := newFixture(t)
	f.stg.val = []any{math.NaN(), math.Inf(1), 1.5}
	st, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: "MGET", Args: []string{"a"}})
	if st != 200 || strings.TrimSpace(string(body)) != `{"result":["NaN","+Inf",1.5],"truncated":false}` {
		t.Fatalf("%d %s", st, body)
	}
}

func TestRedis_ServerError502KeepsMessage(t *testing.T) {
	f := newFixture(t)
	f.stg.err = redisErr("WRONGTYPE Operation against a key holding the wrong kind of value")
	st, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: "GET", Args: []string{"k"}})
	var e api.Error
	_ = json.Unmarshal(body, &e)
	if st != 502 || e.Error != api.CodeQueryFailed || !strings.HasPrefix(e.Message, "WRONGTYPE") {
		t.Fatalf("%d %s", st, body)
	}
}

type redisErr string

func (e redisErr) Error() string { return string(e) }
func (redisErr) RedisError()     {}

func TestRedis_MultiByteStringTruncatedOnBoundary(t *testing.T) {
	f := newFixture(t)
	f.stg.val = strings.Repeat("\u00e9", 700000) // 2 bytes each, 1.4 MB
	_, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: "HGET", Args: []string{"h", "f"}})
	var r api.RedisResponse
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatal(err)
	}
	s, _ := r.Result.(string)
	if !r.Truncated || len(body) > 1<<20+256 || len(s) == 0 || strings.Trim(s, "\u00e9") != "" || strings.ContainsRune(s, '\uFFFD') {
		t.Fatalf("truncated=%v len=%d", r.Truncated, len(body))
	}
}

func TestRedis_MGetOverByteBudgetRefusedWithoutRead(t *testing.T) {
	f := newFixture(t)
	f.stg.byCmd = map[string]any{"STRLEN": int64(600 << 10)}
	st, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: "MGET", Args: []string{"a", "b"}})
	if st != 400 || !strings.Contains(string(body), api.CodeCommandNotAllowed) || !strings.Contains(string(body), "GET one key") {
		t.Fatalf("%d %s", st, body)
	}
	if f.stg.sent("MGET") || len(f.stg.got) != 2 {
		t.Fatalf("want two STRLEN and no MGET, got %v", f.stg.got)
	}
	// Under budget, and a non-string key (WRONGTYPE) counts 0: MGET runs.
	f.stg.byCmd = map[string]any{"STRLEN": int64(10)}
	if st, body = f.post(t, api.RedisRequest{Env: "stg", Cmd: "MGET", Args: []string{"a", "b"}}); st != 200 || !f.stg.sent("MGET") {
		t.Fatalf("under budget: %d %s", st, body)
	}
	g := newFixture(t)
	g.stg.errBy = map[string]error{"STRLEN": redisErr("WRONGTYPE Operation against a key holding the wrong kind of value")}
	if st, body = g.post(t, api.RedisRequest{Env: "stg", Cmd: "MGET", Args: []string{"h"}}); st != 200 || !g.stg.sent("MGET") {
		t.Fatalf("wrongtype: %d %s", st, body)
	}
}

func TestRedis_CollectionOverByteBudgetRefused(t *testing.T) {
	for cmd, count := range map[string]string{"HGETALL": "HLEN", "HKEYS": "HLEN", "SMEMBERS": "SCARD"} {
		f := newFixture(t)
		f.stg.byCmd = map[string]any{count: int64(3), "MEMORY": int64(2 << 20)}
		st, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: cmd, Args: []string{"h"}})
		if st != 400 || !strings.Contains(string(body), api.CodeCommandNotAllowed) || f.stg.sent(cmd) {
			t.Fatalf("%s: %d %s %v", cmd, st, body, f.stg.got)
		}
		if strings.Contains(string(body), "SCAN") {
			t.Fatalf("%s: message points at SCAN: %s", cmd, body)
		}
		if got := strings.Join(argv(f.stg.got[1]), " "); got != "MEMORY USAGE h SAMPLES 0" {
			t.Fatalf("%s: second command %q", cmd, got)
		}
		// A nil MEMORY USAGE (key absent) lets the read through.
		f.stg.byCmd = map[string]any{count: int64(0), "MEMORY": nil}
		if st, body = f.post(t, api.RedisRequest{Env: "stg", Cmd: cmd, Args: []string{"h"}}); st != 200 || !f.stg.sent(cmd) {
			t.Fatalf("%s absent key: %d %s", cmd, st, body)
		}
	}
}

func TestRedis_HGetOverByteBudgetRefused(t *testing.T) {
	f := newFixture(t)
	f.stg.byCmd = map[string]any{"HSTRLEN": int64(2 << 20)}
	st, body := f.post(t, api.RedisRequest{Env: "stg", Cmd: "HGET", Args: []string{"h", "f"}})
	if st != 400 || !strings.Contains(string(body), api.CodeCommandNotAllowed) || f.stg.sent("HGET") {
		t.Fatalf("%d %s %v", st, body, f.stg.got)
	}
	f.stg.byCmd = map[string]any{"HSTRLEN": int64(5)}
	if st, _ = f.post(t, api.RedisRequest{Env: "stg", Cmd: "HGET", Args: []string{"h", "f"}}); st != 200 || !f.stg.sent("HGET") {
		t.Fatalf("small field: %d", st)
	}
}

func TestRedis_ConcurrentLimit(t *testing.T) {
	f := newFixture(t)
	f.stg.gate, f.stg.enter = make(chan struct{}), make(chan struct{}, 4)
	req := api.RedisRequest{Env: "stg", Cmd: "TTL", Args: []string{"k"}}
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() { st, _ := f.post(t, req); codes <- st }()
	}
	<-f.stg.enter
	<-f.stg.enter
	st, body := f.post(t, req)
	var e api.Error
	_ = json.Unmarshal(body, &e)
	if st != 429 || e.Error != api.CodeTooManyConcurrent {
		t.Fatalf("3rd call: %d %s", st, body)
	}
	close(f.stg.gate)
	for i := 0; i < 2; i++ {
		if st := <-codes; st != 200 {
			t.Fatalf("held call: %d", st)
		}
	}
	if st, _ := f.post(t, req); st != 200 {
		t.Fatalf("after release: %d", st)
	}
}
