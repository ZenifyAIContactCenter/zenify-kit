// Package creds serves POST /creds: it vends connection strings by env, mode
// and role, and audits every call without ever logging a value.
package creds

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"slices"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
)

const maxBody = 64 << 10

// Handler is the POST /creds handler. It must sit behind auth.Require.
// Read, Write and Lumi map env (stg|prd) -> fixed variable name -> value;
// empty values must not be in the maps.
type Handler struct {
	Read, Write, Lumi map[string]map[string]string
	Audit             *audit.Logger
}

// Validate reports wiring mistakes that would otherwise panic per request.
func (h *Handler) Validate() error {
	if h.Audit == nil {
		return errors.New("creds: Audit is nil")
	}
	return nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	login, role := auth.Login(r.Context()), auth.Role(r.Context())
	status, env, mode := http.StatusOK, "", ""
	var keys []string
	lumi := false
	defer func() {
		// Args hold only variable names and the lumi flag, never a value (FR-2.5).
		h.Audit.Log(audit.Entry{TS: start, Login: login, Role: role, Endpoint: "/creds", Env: env, Op: mode,
			Args: map[string]any{"keys": keys, "lumi": lumi}, MS: time.Since(start).Milliseconds(), Status: status})
	}()
	fail := func(st int, code, msg string) {
		status = st
		auth.WriteError(w, st, code, msg)
	}

	var req api.CredsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil ||
		(req.Env != "stg" && req.Env != "prd") ||
		(req.Mode != api.CredsModeRead && req.Mode != api.CredsModeWrite) {
		fail(http.StatusBadRequest, api.CodeInvalidBody, "body must be {\"env\":\"stg|prd\",\"mode\":\"read|write\",\"lumi\":bool}")
		return
	}
	env, mode, lumi = req.Env, req.Mode, req.Lumi // allowlisted, safe to audit
	if env == "prd" && role != api.RoleLead {
		fail(http.StatusForbidden, api.CodeLeadOnly, "prd credentials are lead-only")
		return
	}
	src := h.Read[env]
	if mode == api.CredsModeWrite {
		src = h.Write[env]
		if len(src) == 0 {
			fail(http.StatusInternalServerError, api.CodeBrokerMisconfigured, "no write credentials configured for "+env)
			return
		}
	}
	vals := maps.Clone(src)
	if vals == nil {
		vals = map[string]string{}
	}
	if lumi {
		maps.Copy(vals, h.Lumi[env])
	}
	keys = slices.Sorted(maps.Keys(vals))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.CredsResponse{Values: vals}) //nolint:gosec // G117 -- vending credentials is this endpoint's purpose
}
