// Package redisread serves POST /redis: one allowlisted read-only command run
// with the broker's own Redis credentials, with bounded range sizes and result.
package redisread

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
)

const (
	maxBody    = 64 << 10
	maxElems   = 200     // LRANGE / ZRANGE / XRANGE element cap
	maxResult  = 1 << 20 // serialized result cap
	cmdTimeout = 10 * time.Second
)

// Doer is the seam over *redis.Client.
type Doer interface {
	Do(ctx context.Context, args ...any) *redis.Cmd
}

// Handler is the POST /redis handler. It must sit behind auth.Require.
// Clients is keyed by env: "stg" and "prd".
type Handler struct {
	Clients map[string]Doer
	Audit   *audit.Logger
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	login := auth.Login(r.Context())
	var req api.RedisRequest
	status, nbytes := http.StatusOK, 0
	defer func() {
		op := strings.ToUpper(strings.TrimSpace(req.Cmd))
		h.Audit.Log(audit.Entry{TS: start, Login: login, Endpoint: "/redis", Env: req.Env, Op: op,
			Args: req.Args, Bytes: nbytes, MS: time.Since(start).Milliseconds(), Status: status})
	}()
	fail := func(st int, code, msg string) {
		status = st
		auth.WriteError(w, st, code, msg)
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
		fail(http.StatusBadRequest, api.CodeInvalidQuery, "body must be {\"env\":\"stg|prd\",\"cmd\":\"...\",\"args\":[...]}")
		return
	}
	cli := h.Clients[req.Env]
	if (req.Env != "stg" && req.Env != "prd") || cli == nil {
		fail(http.StatusBadRequest, api.CodeInvalidTarget, "env must be stg or prd")
		return
	}
	argv, err := api.CheckRedis(req.Cmd, req.Args)
	if err == nil {
		argv, err = capRange(argv)
	}
	if err != nil {
		fail(http.StatusBadRequest, api.CodeCommandNotAllowed, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), cmdTimeout)
	defer cancel()
	anyArgs := make([]any, len(argv))
	for i, a := range argv {
		anyArgs[i] = a
	}
	val, err := cli.Do(ctx, anyArgs...).Result()
	var rerr redis.Error
	switch {
	case errors.Is(err, redis.Nil):
		val = nil
	case errors.As(err, &rerr):
		fail(http.StatusBadGateway, api.CodeQueryFailed, rerr.Error())
		return
	case err != nil:
		// Connection errors carry the server address, so none of err is echoed.
		fail(http.StatusBadGateway, api.CodeQueryFailed, "redis command failed")
		return
	}

	res, truncated := capResult(jsonSafe(val), maxResult)
	b, _ := json.Marshal(api.RedisResponse{Result: res, Truncated: truncated})
	nbytes = len(b)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(append(b, '\n'))
}

// capRange rewrites argv (command first, upper case) so LRANGE, ZRANGE and
// XRANGE return at most maxElems elements.
func capRange(argv []string) ([]string, error) {
	switch argv[0] {
	case "LRANGE":
		if len(argv) != 4 {
			return nil, errors.New("LRANGE takes key start stop")
		}
		return capIndexes(argv, 2)
	case "ZRANGE":
		if len(argv) < 4 {
			return nil, errors.New("ZRANGE takes key start stop [options]")
		}
		for _, o := range argv[4:] {
			if u := strings.ToUpper(o); u == "BYSCORE" || u == "BYLEX" {
				return withLimit(argv), nil
			}
		}
		return capIndexes(argv, 2)
	case "XRANGE":
		return capCount(argv)
	}
	return argv, nil
}

// capIndexes caps the start/stop pair at argv[i], argv[i+1] to maxElems entries.
func capIndexes(argv []string, i int) ([]string, error) {
	start, err1 := strconv.Atoi(argv[i])
	stop, err2 := strconv.Atoi(argv[i+1])
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("%s start and stop must be integers", argv[0])
	}
	out := append([]string(nil), argv...)
	switch {
	case start >= 0:
		if stop < 0 || stop > start+maxElems-1 {
			out[i+1] = strconv.Itoa(start + maxElems - 1)
		}
	case stop < 0:
		// Both counted from the end: the span is stop-start+1 elements.
		if stop-start+1 > maxElems {
			out[i+1] = strconv.Itoa(start + maxElems - 1)
		}
	default:
		return nil, fmt.Errorf("%s with a negative start needs a negative stop, or use a non-negative start", argv[0])
	}
	return out, nil
}

// withLimit makes a BYSCORE/BYLEX ZRANGE end in LIMIT 0 maxElems, dropping any LIMIT the caller gave.
func withLimit(argv []string) []string {
	out := append([]string(nil), argv[:4]...)
	rest := argv[4:]
	for i := 0; i < len(rest); i++ {
		if strings.EqualFold(rest[i], "LIMIT") {
			i += 2
			continue
		}
		out = append(out, rest[i])
	}
	return append(out, "LIMIT", "0", strconv.Itoa(maxElems))
}

// capCount adds COUNT maxElems to an XRANGE, or lowers a larger COUNT to it.
func capCount(argv []string) ([]string, error) {
	out := append([]string(nil), argv...)
	for i := 4; i < len(out); i++ {
		if !strings.EqualFold(out[i], "COUNT") {
			continue
		}
		if i+1 >= len(out) {
			return nil, errors.New("XRANGE COUNT is missing its value")
		}
		n, err := strconv.Atoi(out[i+1])
		if err != nil {
			return nil, errors.New("XRANGE COUNT must be an integer")
		}
		if n > maxElems {
			out[i+1] = strconv.Itoa(maxElems)
		}
		return out, nil
	}
	return append(out, "COUNT", strconv.Itoa(maxElems)), nil
}

// jsonSafe converts a go-redis reply to values encoding/json can marshal:
// RESP3 maps get string keys, unknown types are printed.
func jsonSafe(v any) any {
	switch t := v.(type) {
	case nil, string, bool, int64, float64:
		return t
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = jsonSafe(e)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[fmt.Sprint(k)] = jsonSafe(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = jsonSafe(e)
		}
		return out
	}
	return fmt.Sprint(v)
}

func size(v any) int {
	b, _ := json.Marshal(v)
	return len(b)
}

// capResult returns v unchanged when its JSON is within limit bytes; otherwise
// a shortened v (string prefix, leading array elements or map keys) and true.
func capResult(v any, limit int) (any, bool) {
	if size(v) <= limit {
		return v, false
	}
	switch t := v.(type) {
	case string:
		n := sort.Search(len(t), func(n int) bool { return size(validPrefix(t, n+1)) > limit })
		return validPrefix(t, n), true
	case []any:
		n := sort.Search(len(t), func(n int) bool { return size(t[:n+1]) > limit })
		if n == 0 && len(t) > 0 {
			first, _ := capResult(t[0], limit-2)
			return []any{first}, true
		}
		return t[:n], true
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pick := func(n int) map[string]any {
			m := make(map[string]any, n)
			for _, k := range keys[:n] {
				m[k] = t[k]
			}
			return m
		}
		n := sort.Search(len(keys), func(n int) bool { return size(pick(n+1)) > limit })
		return pick(n), true
	}
	return nil, true
}

// validPrefix is s[:n] backed off to a UTF-8 boundary.
func validPrefix(s string, n int) string {
	for n > 0 && n < len(s) && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}
