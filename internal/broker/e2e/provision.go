package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/api"
)

// ErrBadPassword is App.Login's answer when hub rejects the credentials.
var ErrBadPassword = errors.New("login rejected")

// ErrHubLocked is in App.Login's error when hub answers 429: too many failed
// logins for the account or for the broker's IP, which the whole team shares.
var ErrHubLocked = errors.New("hub locked logins out after too many failures, retry in 15 minutes")

// hubCredentialMessages are hub's 401 messages for a wrong password and for a
// missing user (contact-center-hub apps/auth/src/auth.service.ts). Every
// other hub login failure is also a 401, so the message is what tells them apart.
var hubCredentialMessages = map[string]bool{
	"Mật khẩu không chính xác":       true, //znf:allow-lang (hub reply text)
	"Thông tin đăng nhập không đúng": true, //znf:allow-lang (hub reply text)
}

// ErrMFARequired is App.Login's answer when the account has MFA on: hub
// returns an MFA challenge instead of a token, which the broker cannot answer.
var ErrMFARequired = errors.New("MFA is on for this account: turn it off for E2E accounts")

// App is the seam over hub and be. Errors never carry a password or token.
type App interface {
	Login(ctx context.Context, email, password string) (token string, err error)
	FindUser(ctx context.Context, email string) (id string, found bool, err error)
	CreateUser(ctx context.Context, name, email, role string) (id string, err error)
	ResetPassword(ctx context.Context, id, password string) error
	ChangePassword(ctx context.Context, userToken, oldPassword, newPassword string) error
}

const (
	// authHeader carries the raw JWT: be's checkPermission jwt.verify()s the
	// header as is, so a "Bearer " prefix would fail.
	authHeader = "Authorization"
	// Without x-app-version 2.0, be answers errors as HTTP 200 {"error":true}.
	appVersionHeader = "x-app-version"
	appVersion       = "2.0"
	deviceID         = "zenify-broker" // fixed so the broker holds one max_parallel_sessions slot
	maxReply         = 4 << 20
)

var defaultClient = &http.Client{Timeout: 30 * time.Second}

// HTTPApp is App over the real hub and be. The admin token comes from a hub
// login with AdminEmail/AdminPassword and is kept until be answers 401.
type HTTPApp struct {
	HubURL, BEURL, Domain, TenantID, GroupID, AdminEmail, AdminPassword string
	HTTP                                                                *http.Client // nil = 30s-timeout client

	mu       sync.Mutex
	adminTok string
}

// do sends body as JSON and returns the status and the (capped) reply. Its
// errors name op only: transport errors carry URLs, so none of them is echoed.
func (a *HTTPApp) do(ctx context.Context, op, method, u, token string, body any) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("%s: encoding the request failed", op)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd) //nolint:gosec // G704 -- u is the configured hub/be base URL plus a fixed route
	if err != nil {
		return 0, nil, fmt.Errorf("%s: building the request failed", op)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(appVersionHeader, appVersion)
	if token != "" {
		req.Header.Set(authHeader, token)
	}
	c := a.HTTP
	if c == nil {
		c = defaultClient
	}
	resp, err := c.Do(req) //nolint:gosec // G704 -- see NewRequestWithContext above
	if err != nil {
		return 0, nil, fmt.Errorf("%s: request failed", op)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxReply))
	if err != nil {
		return 0, nil, fmt.Errorf("%s: reading the reply failed", op)
	}
	return resp.StatusCode, b, nil
}

// envelope is the part of a hub/be reply the broker reads; error:true is a
// failure even on HTTP 200.
type envelope struct {
	Error       bool            `json:"error"`
	Token       string          `json:"token"`
	MFARequired bool            `json:"mfa_required"`
	Data        json.RawMessage `json:"data"`
}

// hubMessage is the message of a hub error body, with password masked out.
// NestJS validation errors carry an array of messages.
func hubMessage(b []byte, password string) string {
	var body struct {
		Message any `json:"message"`
	}
	_ = json.Unmarshal(b, &body)
	var msg string
	switch m := body.Message.(type) {
	case string:
		msg = m
	case []any:
		parts := make([]string, 0, len(m))
		for _, p := range m {
			parts = append(parts, fmt.Sprint(p))
		}
		msg = strings.Join(parts, "; ")
	}
	if password != "" {
		msg = strings.ReplaceAll(msg, password, "***")
	}
	return msg
}

// Login is hub POST /auth/login. A 401 with one of hub's two credential
// messages is ErrBadPassword; 429 wraps ErrHubLocked; any other 4xx is a plain
// error carrying hub's message. An MFA challenge is ErrMFARequired.
func (a *HTTPApp) Login(ctx context.Context, email, password string) (string, error) {
	if a.Domain == "" {
		return "", errors.New("hub login: no tenant domain configured")
	}
	st, b, err := a.do(ctx, "hub login", http.MethodPost, a.HubURL+"/auth/login", "", map[string]string{
		"email": email, "password": password, "domain": a.Domain, "device_type": "web", "device_id": deviceID})
	switch {
	case err != nil:
		return "", err
	case st == http.StatusUnauthorized && hubCredentialMessages[hubMessage(b, "")]:
		return "", ErrBadPassword
	case st == http.StatusTooManyRequests:
		return "", fmt.Errorf("hub login: status %d: %s: %w", st, hubMessage(b, password), ErrHubLocked)
	case st/100 == 4:
		return "", fmt.Errorf("hub login: status %d: %s", st, hubMessage(b, password))
	case st/100 != 2:
		return "", fmt.Errorf("hub login: status %d", st)
	}
	var r, data envelope
	if json.Unmarshal(b, &r) != nil {
		return "", errors.New("hub login: unreadable reply")
	}
	_ = json.Unmarshal(r.Data, &data) // data may be absent or not an object
	switch {
	case r.Error || data.Error:
		return "", errors.New("hub login: error in reply")
	case r.MFARequired || data.MFARequired:
		return "", ErrMFARequired
	case r.Token != "":
		return r.Token, nil
	case data.Token != "":
		return data.Token, nil
	}
	return "", errors.New("hub login: no token in reply")
}

func (a *HTTPApp) adminToken(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.adminTok != "" {
		return a.adminTok, nil
	}
	tok, err := a.Login(ctx, a.AdminEmail, a.AdminPassword)
	if errors.Is(err, ErrBadPassword) {
		return "", errors.New("hub admin login: rejected")
	}
	if err != nil {
		return "", err
	}
	a.adminTok = tok
	return tok, nil
}

func (a *HTTPApp) dropAdminToken(tok string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.adminTok == tok {
		a.adminTok = ""
	}
}

// admin calls a be admin route and returns the envelope's data. A 401 drops
// the cached admin token and retries once with a fresh one.
func (a *HTTPApp) admin(ctx context.Context, op, method, path string, body any) (json.RawMessage, error) {
	for attempt := 0; ; attempt++ {
		tok, err := a.adminToken(ctx)
		if err != nil {
			return nil, err
		}
		st, b, err := a.do(ctx, op, method, a.BEURL+path, tok, body)
		if err != nil {
			return nil, err
		}
		if st == http.StatusUnauthorized && attempt == 0 {
			a.dropAdminToken(tok)
			continue
		}
		if st/100 != 2 {
			return nil, fmt.Errorf("%s: status %d", op, st)
		}
		var env envelope
		if json.Unmarshal(b, &env) != nil {
			return nil, fmt.Errorf("%s: unreadable reply", op)
		}
		if env.Error {
			return nil, fmt.Errorf("%s: error in reply", op)
		}
		return env.Data, nil
	}
}

func (a *HTTPApp) settingsPath(route string) string {
	return "/v2/admin/settings/" + url.PathEscape(a.TenantID) + "/" + route
}

// FindUser is be GET list-agents; search is a regex over several fields, so
// only an exact, case-insensitive email match counts.
func (a *HTTPApp) FindUser(ctx context.Context, email string) (string, bool, error) {
	q := url.Values{"search": {email}, "limit": {"10"}}
	data, err := a.admin(ctx, "be list-agents", http.MethodGet, a.settingsPath("list-agents?"+q.Encode()), nil)
	if err != nil {
		return "", false, err
	}
	var d struct {
		List []struct {
			ID    string `json:"_id"`
			Email string `json:"email"`
		} `json:"list"`
	}
	if json.Unmarshal(data, &d) != nil {
		return "", false, errors.New("be list-agents: unreadable reply")
	}
	for _, u := range d.List {
		if strings.EqualFold(u.Email, email) {
			return u.ID, true, nil
		}
	}
	return "", false, nil
}

// CreateUser is be POST create-user. The id is read from data._id; when the
// reply lacks it, the new user is looked up by email.
func (a *HTTPApp) CreateUser(ctx context.Context, name, email, role string) (string, error) {
	data, err := a.admin(ctx, "be create-user", http.MethodPost, a.settingsPath("create-user"),
		map[string]string{"name": name, "email": email, "role": role, "group_id": a.GroupID})
	if err != nil {
		return "", err
	}
	var d struct {
		ID string `json:"_id"`
	}
	if json.Unmarshal(data, &d) == nil && d.ID != "" {
		return d.ID, nil
	}
	id, found, err := a.FindUser(ctx, email)
	if err != nil {
		return "", err
	}
	if !found {
		return "", errors.New("be create-user: created user not found")
	}
	return id, nil
}

// ResetPassword is be POST reset-password/:id; be then sets must_change_password.
func (a *HTTPApp) ResetPassword(ctx context.Context, id, password string) error {
	_, err := a.admin(ctx, "be reset-password", http.MethodPost, a.settingsPath("reset-password/"+url.PathEscape(id)),
		map[string]string{"password": password})
	return err
}

// ChangePassword is be POST /v2/change-password as the user.
func (a *HTTPApp) ChangePassword(ctx context.Context, userToken, oldPassword, newPassword string) error {
	st, b, err := a.do(ctx, "be change-password", http.MethodPost, a.BEURL+"/v2/change-password", userToken,
		map[string]string{"password": oldPassword, "new_password": newPassword, "c_password": newPassword})
	if err != nil {
		return err
	}
	if st/100 != 2 {
		return fmt.Errorf("be change-password: status %d", st)
	}
	var env envelope
	if json.Unmarshal(b, &env) != nil {
		return errors.New("be change-password: unreadable reply")
	}
	if env.Error {
		return errors.New("be change-password: error in reply")
	}
	return nil
}

// verifiedTTL is how long a confirmed (login, role, period) skips the login check (FR-5.3).
const verifiedTTL = time.Hour

// failedTTL is how long a failed Ensure is answered from memory, so retries
// cannot add up failed hub logins toward the lockout.
const failedTTL = 10 * time.Minute

// lockedTTL keeps an ErrHubLocked failure for hub's 15-minute lockout window.
const lockedTTL = 15 * time.Minute

// provisionTimeout bounds one provisioning run, which outlives its request.
const provisionTimeout = 2 * time.Minute

type failure struct {
	err   error
	until time.Time
}

type cacheKey struct {
	login, role string
	period      int64
}

// Provisioner makes sure an E2E account exists and accepts P(period).
type Provisioner struct {
	App         App
	Key         []byte
	EmailDomain string
	Domain      string // hub login domain, returned to the caller
	Now         func() time.Time

	// mu guards the maps below and is never held across a hub/be call.
	mu       sync.Mutex
	verified map[cacheKey]time.Time // -> expiry
	failed   map[cacheKey]failure
	// keyLocks serialise provisioning per cacheKey (1-slot semaphores), so
	// parallel calls cannot create a user twice or add up failed logins past
	// the lockout, while other developers' accounts proceed.
	keyLocks map[cacheKey]chan struct{}
}

// lookup answers k from the verified or failed cache.
func (p *Provisioner) lookup(k cacheKey, now time.Time) (hit bool, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if exp, ok := p.verified[k]; ok && now.Before(exp) {
		return true, nil
	}
	if f, ok := p.failed[k]; ok && now.Before(f.until) {
		return true, f.err
	}
	return false, nil
}

func (p *Provisioner) keyLock(k cacheKey) chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.keyLocks == nil {
		p.keyLocks = map[cacheKey]chan struct{}{}
	}
	if p.keyLocks[k] == nil {
		p.keyLocks[k] = make(chan struct{}, 1)
	}
	return p.keyLocks[k]
}

// Ensure returns the account of login for role, following FR-5.2: P(period),
// then P(period-1) rotated to P(period), then create-if-missing, admin reset
// to a temporary password and self-change to P(period).
func (p *Provisioner) Ensure(ctx context.Context, login, role string) (api.E2EAccountResponse, error) {
	login = strings.ToLower(login)
	now := p.Now()
	period := Period(now)
	resp := api.E2EAccountResponse{Domain: p.Domain, Email: "e2e-" + login + "-" + role + "@" + p.EmailDomain,
		Password: Password(p.Key, login, role, period), Role: role}
	k := cacheKey{login, role, period}

	result := func(err error) (api.E2EAccountResponse, error) {
		if err != nil {
			return api.E2EAccountResponse{}, err
		}
		return resp, nil
	}
	if hit, err := p.lookup(k, now); hit {
		return result(err)
	}
	sem := p.keyLock(k)
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return api.E2EAccountResponse{}, ctx.Err()
	}
	defer func() { <-sem }()
	// Whoever held the lock may have settled k meanwhile.
	if hit, err := p.lookup(k, now); hit {
		return result(err)
	}

	// A client that disconnects after the reset must not leave the account
	// on its temporary password: the run ignores ctx's cancellation.
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), provisionTimeout)
	defer cancel()
	err := p.provision(pctx, login, role, period, resp.Email)
	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		if p.failed == nil {
			p.failed = map[cacheKey]failure{}
		}
		ttl := failedTTL
		if errors.Is(err, ErrHubLocked) {
			ttl = lockedTTL
		}
		// From when provisioning ended, which can be minutes after now.
		p.failed[k] = failure{err, p.Now().Add(ttl)}
		return api.E2EAccountResponse{}, err
	}
	if p.verified == nil {
		p.verified = map[cacheKey]time.Time{}
	}
	p.verified[k] = now.Add(verifiedTTL)
	return resp, nil
}

// provision runs FR-5.2. Only ErrBadPassword moves on to the next step, so a
// hub outage never triggers a reset; at most two probe logins fail per call.
func (p *Provisioner) provision(ctx context.Context, login, role string, period int64, email string) error {
	cur := Password(p.Key, login, role, period)
	if _, err := p.App.Login(ctx, email, cur); !errors.Is(err, ErrBadPassword) {
		return err
	}
	prev := Password(p.Key, login, role, period-1)
	tok, err := p.App.Login(ctx, email, prev)
	if err == nil {
		return p.App.ChangePassword(ctx, tok, prev, cur)
	}
	if !errors.Is(err, ErrBadPassword) {
		return err
	}

	id, found, err := p.App.FindUser(ctx, email)
	if err != nil {
		return err
	}
	if !found {
		if id, err = p.App.CreateUser(ctx, "E2E "+login+" "+role, email, role); err != nil {
			return err
		}
	}
	tmp := Password(p.Key, login, role+":tmp", period)
	if err := p.App.ResetPassword(ctx, id, tmp); err != nil {
		return err
	}
	if tok, err = p.App.Login(ctx, email, tmp); err != nil {
		return fmt.Errorf("login after reset: %w", err)
	}
	return p.App.ChangePassword(ctx, tok, tmp, cur)
}
