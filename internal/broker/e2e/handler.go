package e2e

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
)

const maxBody = 64 << 10

// Handler is the POST /e2e/account handler. It must sit behind auth.Require.
// Its audit line has op = role and never the email or password (FR-6.2).
type Handler struct {
	P     *Provisioner
	Audit *audit.Logger
}

// auditEnv is the env of every /e2e/account audit line: E2E accounts live on
// staging only, spelled as /query and /redis spell it.
const auditEnv = "stg"

// Validate reports wiring mistakes that would otherwise panic per request.
func (h *Handler) Validate() error {
	var errs []error
	if h.Audit == nil {
		errs = append(errs, errors.New("e2e: Audit is nil"))
	}
	switch {
	case h.P == nil:
		errs = append(errs, errors.New("e2e: Provisioner is nil"))
	case h.P.App == nil || h.P.Now == nil:
		errs = append(errs, errors.New("e2e: Provisioner needs App and Now"))
	case len(h.P.Key) < auth.MinKeyLen:
		errs = append(errs, fmt.Errorf("e2e: Provisioner Key must be at least %d bytes", auth.MinKeyLen))
	}
	return errors.Join(errs...)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	login := auth.Login(r.Context())
	status, op := http.StatusOK, ""
	defer func() {
		h.Audit.Log(audit.Entry{TS: start, Login: login, Endpoint: "/e2e/account", Env: auditEnv, Op: op,
			MS: time.Since(start).Milliseconds(), Status: status})
	}()
	fail := func(st int, code, msg string) {
		status = st
		auth.WriteError(w, st, code, msg)
	}

	var req api.E2EAccountRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
		fail(http.StatusBadRequest, api.CodeInvalidBody, "body must be {\"role\":\"...\"}")
		return
	}
	if !slices.Contains(api.E2ERoles, req.Role) {
		fail(http.StatusBadRequest, api.CodeUnknownRole, "role must be one of "+strings.Join(api.E2ERoles, ", "))
		return
	}
	op = req.Role // only an allowlisted role reaches the audit line
	resp, err := h.P.Ensure(r.Context(), login, req.Role)
	if err != nil {
		// App errors carry an operation and a status code, never a credential.
		fail(http.StatusBadGateway, api.CodeProvisionFailed, "could not provision the E2E account: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp) //nolint:gosec // G117 -- returning the E2E password to its owner is this endpoint's purpose
}
