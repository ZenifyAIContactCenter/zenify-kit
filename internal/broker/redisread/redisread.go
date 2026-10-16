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
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/perlogin"
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
	Pipelined(ctx context.Context, fn func(redis.Pipeliner) error) ([]redis.Cmder, error)
}

// Handler is the POST /redis handler. It must sit behind auth.Require.
// Clients is keyed by env: "stg" and "prd".
type Handler struct {
	Clients       map[string]Doer
	Audit         *audit.Logger
	MaxConcurrent int // per login; Validate refuses < 1

	slots perlogin.Slots
}

// Validate reports wiring mistakes that would otherwise fail every request
// (MaxConcurrent < 1, no client for an env) or panic (nil Audit).
func (h *Handler) Validate() error {
	var errs []error
	if h.Audit == nil {
		errs = append(errs, errors.New("redis: Audit is nil"))
	}
	if h.MaxConcurrent < 1 {
		errs = append(errs, errors.New("redis: MaxConcurrent must be at least 1"))
	}
	for _, env := range []string{"stg", "prd"} {
		if h.Clients[env] == nil {
			errs = append(errs, fmt.Errorf("redis: no client for env %s", env))
		}
	}
	return errors.Join(errs...)
}

// readHint names what a caller can read instead of a collection that is too big.
var readHint = map[string]string{
	"HGETALL":  "read specific fields with HGET",
	"HKEYS":    "read specific fields with HGET",
	"SMEMBERS": "SCARD gives the set's size; its members cannot be read in one call",
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	login, role := auth.Login(r.Context()), auth.Role(r.Context())
	var req api.RedisRequest
	status, nbytes := http.StatusOK, 0
	defer func() {
		op := strings.ToUpper(strings.TrimSpace(req.Cmd))
		h.Audit.Log(audit.Entry{TS: start, Login: login, Role: role, Endpoint: "/redis", Env: req.Env, Op: op,
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
	if role != api.RoleLead && req.Env == "prd" && !api.DevRedisAllowed(req.Cmd) {
		fail(http.StatusForbidden, api.CodeLeadOnly, "production values are lead-only; dev on prd may only use "+api.DevPrdSummary())
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

	if !h.slots.Acquire(login, h.MaxConcurrent) {
		fail(http.StatusTooManyRequests, api.CodeTooManyConcurrent, "too many concurrent redis commands for this login")
		return
	}
	defer h.slots.Release(login)

	ctx, cancel := context.WithTimeout(r.Context(), cmdTimeout)
	defer cancel()
	// check maps a reply error; on failure it has already answered the request.
	check := func(err error) bool {
		var rerr redis.Error
		switch {
		case err == nil:
			return true
		case errors.As(err, &rerr) && role != api.RoleLead && req.Env == "prd":
			fail(http.StatusBadGateway, api.CodeQueryFailed, api.DevPrdErrorWithheld)
			return false
		case errors.As(err, &rerr):
			fail(http.StatusBadGateway, api.CodeQueryFailed, rerr.Error())
			return false
		}
		// Connection errors carry the server address, so none of err is echoed.
		fail(http.StatusBadGateway, api.CodeQueryFailed, "redis command failed")
		return false
	}
	// run sends one command; on failure it has already answered the request.
	run := func(args ...string) (any, bool) {
		anyArgs := make([]any, len(args))
		for i, a := range args {
			anyArgs[i] = a
		}
		val, err := cli.Do(ctx, anyArgs...).Result()
		if errors.Is(err, redis.Nil) {
			return nil, true
		}
		if !check(err) {
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
		total, ok := mgetBytes(ctx, cli, argv[1:], check)
		if !ok {
			return
		}
		if total > maxResult {
			fail(http.StatusBadRequest, api.CodeCommandNotAllowed, fmt.Sprintf(
				"MGET of these %d keys is %d bytes, over the %d-byte result cap: ask for fewer keys, or GET one key (a big value comes back truncated)",
				len(argv)-1, total, maxResult))
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
					"%s on %d entries exceeds the %d-entry limit: %s", argv[0], c, maxCollection, readHint[argv[0]]))
				return
			}
			// HKEYS returns names only, so the entry cap bounds it; MEMORY USAGE
			// counts values too. Only after the count check: SAMPLES 0 is O(entries).
			if argv[0] == "HKEYS" {
				break
			}
			n, ok = run("MEMORY", "USAGE", argv[1], "SAMPLES", "0")
			if !ok {
				return
			}
			if b, _ := n.(int64); b > maxResult {
				fail(http.StatusBadRequest, api.CodeCommandNotAllowed, fmt.Sprintf(
					"%s on this key is about %d bytes, over the %d-byte result cap: %s", argv[0], b, maxResult, readHint[argv[0]]))
				return
			}
		}
	case "HGET":
		if len(argv) == 3 {
			n, ok := run("HSTRLEN", argv[1], argv[2])
			if !ok {
				return
			}
			if l, _ := n.(int64); l > maxResult {
				fail(http.StatusBadRequest, api.CodeCommandNotAllowed, fmt.Sprintf(
					"HGET of this field is %d bytes, over the %d-byte result cap: a hash field cannot be read in part", l, maxResult))
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

// mgetBytes sums STRLEN of keys in one pipeline. A key that is not a string
// counts 0, as MGET returns nil for it. On failure check has answered.
func mgetBytes(ctx context.Context, cli Doer, keys []string, check func(error) bool) (int64, bool) {
	cmds, _ := cli.Pipelined(ctx, func(p redis.Pipeliner) error {
		for _, k := range keys {
			p.Do(ctx, "STRLEN", k)
		}
		return nil
	})
	var total int64
	for _, c := range cmds {
		cmd, ok := c.(*redis.Cmd)
		if !ok {
			continue
		}
		val, err := cmd.Result()
		var rerr redis.Error
		if errors.As(err, &rerr) && strings.HasPrefix(rerr.Error(), "WRONGTYPE") {
			continue
		}
		if !check(err) {
			return 0, false
		}
		n, _ := val.(int64)
		total += n
	}
	return total, true
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
				return withLimit(argv)
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

// withLimit makes a BYSCORE/BYLEX ZRANGE end in exactly one LIMIT: the
// caller's offset (the last LIMIT given) and a count of at most maxElems; a
// missing or negative count is maxElems, LIMIT 0 maxElems when none was given.
func withLimit(argv []string) ([]string, error) {
	out := append([]string(nil), argv[:4]...)
	offset, count := int64(0), int64(maxElems)
	rest := argv[4:]
	for i := 0; i < len(rest); i++ {
		if !strings.EqualFold(rest[i], "LIMIT") {
			out = append(out, rest[i])
			continue
		}
		if i+2 >= len(rest) {
			return nil, errors.New("ZRANGE LIMIT takes offset count")
		}
		o, err1 := strconv.ParseInt(rest[i+1], 10, 64)
		c, err2 := strconv.ParseInt(rest[i+2], 10, 64)
		if err1 != nil || err2 != nil {
			return nil, errors.New("ZRANGE LIMIT offset and count must be integers")
		}
		offset, count = o, c
		if count < 0 || count > maxElems {
			count = maxElems
		}
		i += 2
	}
	return append(out, "LIMIT", strconv.FormatInt(offset, 10), strconv.FormatInt(count, 10)), nil
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
			// A key longer than the budget cannot fit; skip marshalling it.
			kn := budget + 1
			if len(k) <= budget {
				kn = size(k) + 1 // key and colon
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
				// As for arrays: a cut first value is kept, so one oversized
				// field still returns a prefix rather than {}.
				if i == 0 && ecut {
					out[k], used = ev, used+kn+n
				}
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
