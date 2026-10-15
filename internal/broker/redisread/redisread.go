// Package redisread serves POST /redis: one allowlisted read-only command run
// with the broker's own Redis credentials, with bounded range sizes and result.
package redisread

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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

	maxMGetKeys   = 100   // MGET key cap
	maxCollection = 10000 // HGETALL/HKEYS/SMEMBERS entry cap
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
	// run sends one command; on failure it has already answered the request.
	run := func(args ...string) (any, bool) {
		anyArgs := make([]any, len(args))
		for i, a := range args {
			anyArgs[i] = a
		}
		val, err := cli.Do(ctx, anyArgs...).Result()
		var rerr redis.Error
		switch {
		case errors.Is(err, redis.Nil):
			return nil, true
		case errors.As(err, &rerr):
			fail(http.StatusBadGateway, api.CodeQueryFailed, rerr.Error())
			return nil, false
		case err != nil:
			// Connection errors carry the server address, so none of err is echoed.
			fail(http.StatusBadGateway, api.CodeQueryFailed, "redis command failed")
			return nil, false
		}
		return val, true
	}

	// Guard the reply size before go-redis buffers it.
	truncated := false
	switch argv[0] {
	case "MGET":
		if len(argv)-1 > maxMGetKeys {
			fail(http.StatusBadRequest, api.CodeCommandNotAllowed, fmt.Sprintf("MGET is limited to %d keys", maxMGetKeys))
			return
		}
	case "HGETALL", "HKEYS", "SMEMBERS":
		count := map[string]string{"HGETALL": "HLEN", "HKEYS": "HLEN", "SMEMBERS": "SCARD"}[argv[0]]
		if len(argv) == 2 {
			n, ok := run(count, argv[1])
			if !ok {
				return
			}
			if c, _ := n.(int64); c > maxCollection {
				fail(http.StatusBadRequest, api.CodeCommandNotAllowed, fmt.Sprintf(
					"%s on %d entries exceeds the %d limit: use SCAN (HSCAN/SSCAN are not allowed, so narrow the read) instead", argv[0], c, maxCollection))
				return
			}
		}
	case "GET":
		if len(argv) == 2 {
			n, ok := run("STRLEN", argv[1])
			if !ok {
				return
			}
			if l, _ := n.(int64); l > maxResult {
				argv, truncated = []string{"GETRANGE", argv[1], "0", strconv.Itoa(maxResult - 1)}, true
			}
		}
	}

	val, ok := run(argv...)
	if !ok {
		return
	}
	res, cut := capResult(jsonSafe(val), maxResult)
	b, err := json.Marshal(api.RedisResponse{Result: res, Truncated: truncated || cut})
	if err != nil {
		fail(http.StatusBadGateway, api.CodeQueryFailed, "redis reply could not be encoded")
		return
	}
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
// A negative start counts from the end of the list, so it is refused.
func capIndexes(argv []string, i int) ([]string, error) {
	start, err1 := strconv.ParseInt(argv[i], 10, 64)
	stop, err2 := strconv.ParseInt(argv[i+1], 10, 64)
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("%s start and stop must be integers", argv[0])
	}
	if start < 0 || start > math.MaxInt64-maxElems {
		return nil, fmt.Errorf("%s needs a non-negative start (and at most %d)", argv[0], int64(math.MaxInt64-maxElems))
	}
	out := append([]string(nil), argv...)
	if stop < 0 || stop-start >= maxElems {
		out[i+1] = strconv.FormatInt(start+maxElems-1, 10)
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

// capCount rewrites an XRANGE to exactly one COUNT: the last one given, at
// most maxElems (Redis reads the last COUNT), or maxElems when none was given.
func capCount(argv []string) ([]string, error) {
	out := append([]string(nil), argv[:min(4, len(argv))]...)
	n := int64(maxElems)
	rest := argv[min(4, len(argv)):]
	for i := 0; i < len(rest); i++ {
		if !strings.EqualFold(rest[i], "COUNT") {
			out = append(out, rest[i])
			continue
		}
		if i+1 >= len(rest) {
			return nil, errors.New("XRANGE COUNT is missing its value")
		}
		v, err := strconv.ParseInt(rest[i+1], 10, 64)
		if err != nil {
			return nil, errors.New("XRANGE COUNT must be an integer")
		}
		n = min(v, maxElems)
		i++
	}
	return append(out, "COUNT", strconv.FormatInt(n, 10)), nil
}

// jsonSafe converts a go-redis reply to values encoding/json can marshal:
// RESP3 maps get string keys, unknown types are printed.
func jsonSafe(v any) any {
	switch t := v.(type) {
	case nil, string, bool, int64:
		return t
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return fmt.Sprint(t)
		}
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

// capResult returns v unchanged when its JSON fits in limit bytes; otherwise a
// shortened v (string prefix, leading elements or map keys) and true. It sums
// sizes while walking and stops at the budget, so it never marshals more than
// about limit bytes at once.
func capResult(v any, limit int) (any, bool) {
	out, _, cut := capValue(v, limit)
	return out, cut
}

// capValue returns v cut to fit budget bytes of JSON, that size, and whether it was cut.
func capValue(v any, budget int) (any, int, bool) {
	budget = max(budget, 0)
	switch t := v.(type) {
	case string:
		cut := false
		if len(t) > budget {
			t, cut = validPrefix(t, budget), true
		}
		// Escapes and invalid UTF-8 can still grow a prefix; t is within budget bytes here.
		if n := size(t); n > budget {
			k := sort.Search(len(t), func(k int) bool { return size(validPrefix(t, k+1)) > budget })
			t, cut = validPrefix(t, k), true
		}
		return t, size(t), cut
	case []any:
		out, used, cut := make([]any, 0, min(len(t), 1024)), 2, false
		for i, e := range t {
			sep := 0
			if i > 0 {
				sep = 1
			}
			ev, n, ecut := capValue(e, budget-used-sep)
			if ecut || used+sep+n > budget {
				if i == 0 && ecut {
					out, used = append(out, ev), used+n
				}
				cut = true
				break
			}
			out, used = append(out, ev), used+sep+n
		}
		return out, used, cut
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out, used, cut := make(map[string]any), 2, false
		for i, k := range keys {
			kn := size(k) + 1 // key and colon
			if len(k) > budget {
				kn = budget + 1
			}
			sep := 0
			if i > 0 {
				sep = 1
			}
			if used+sep+kn > budget {
				cut = true
				break
			}
			ev, n, ecut := capValue(t[k], budget-used-sep-kn)
			if ecut || used+sep+kn+n > budget {
				cut = true
				break
			}
			out[k], used = ev, used+sep+kn+n
		}
		return out, used, cut
	}
	n := size(v)
	if n > budget {
		return nil, 2, true
	}
	return v, n, false
}

// validPrefix is s[:n] backed off to a UTF-8 boundary.
func validPrefix(s string, n int) string {
	for n > 0 && n < len(s) && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}
