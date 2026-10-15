package e2e

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"
)

var testNow = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) // period 230

// fakeApp is a stateful hub+be: a user store keyed by email.
type fakeApp struct {
	mu        sync.Mutex
	users     map[string]*fakeUser // email -> user
	loginErr  error                // non-nil: every Login fails with it
	logins    int
	badLogins int
	finds     int
	creates   int
	resets    []string // passwords
	changes   [][2]string
}

type fakeUser struct{ id, password string }

func newFakeApp() *fakeApp { return &fakeApp{users: map[string]*fakeUser{}} }

func (f *fakeApp) Login(_ context.Context, email, password string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logins++
	if f.loginErr != nil {
		return "", f.loginErr
	}
	u, ok := f.users[email]
	if !ok || u.password != password {
		f.badLogins++
		return "", ErrBadPassword
	}
	return "tok:" + email, nil
}

func (f *fakeApp) FindUser(_ context.Context, email string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finds++
	u, ok := f.users[email]
	if !ok {
		return "", false, nil
	}
	return u.id, true, nil
}

func (f *fakeApp) CreateUser(_ context.Context, _, email, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates++
	id := "id" + strconv.Itoa(len(f.users)+1)
	f.users[email] = &fakeUser{id: id, password: "random-initial"}
	return id, nil
}

func (f *fakeApp) ResetPassword(_ context.Context, id, password string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resets = append(f.resets, password)
	for _, u := range f.users {
		if u.id == id {
			u.password = password
			return nil
		}
	}
	return errors.New("no such user")
}

func (f *fakeApp) ChangePassword(_ context.Context, tok, old, new string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes = append(f.changes, [2]string{old, new})
	u := f.users[tok[len("tok:"):]]
	if u == nil || u.password != old {
		return errors.New("bad old password")
	}
	u.password = new
	return nil
}

func newProvisioner(app App) *Provisioner {
	return &Provisioner{App: app, Key: []byte("testkey"), EmailDomain: "e2e.test", Domain: "zenifytest",
		Now: func() time.Time { return testNow }}
}

const aliceAgent = "e2e-alice-agent@e2e.test"

func TestEnsure_FreshUser(t *testing.T) { // SC-7
	app := newFakeApp()
	p := newProvisioner(app)
	r1, err := p.Ensure(context.Background(), "Alice", "agent")
	if err != nil {
		t.Fatal(err)
	}
	r2, err := p.Ensure(context.Background(), "Alice", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if r1 != r2 {
		t.Fatalf("responses differ: %+v vs %+v", r1, r2)
	}
	want := Password([]byte("testkey"), "alice", "agent", 230)
	if r1.Email != aliceAgent || r1.Password != want || r1.Role != "agent" || r1.Domain != "zenifytest" {
		t.Fatalf("got %+v", r1)
	}
	if app.creates != 1 {
		t.Fatalf("CreateUser called %d times, want 1", app.creates)
	}
	if app.badLogins > 2 {
		t.Fatalf("%d failed logins, want <= 2", app.badLogins)
	}
	if _, err := app.Login(context.Background(), r1.Email, r1.Password); err != nil {
		t.Fatalf("returned password does not log in: %v", err)
	}
}

func TestEnsure_CacheExpiresAfterAnHour(t *testing.T) {
	app := newFakeApp()
	now := testNow
	p := newProvisioner(app)
	p.Now = func() time.Time { return now }
	if _, err := p.Ensure(context.Background(), "alice", "agent"); err != nil {
		t.Fatal(err)
	}
	n := app.logins
	now = now.Add(59 * time.Minute)
	_, _ = p.Ensure(context.Background(), "alice", "agent")
	if app.logins != n {
		t.Fatal("cached account was re-verified within the hour")
	}
	now = now.Add(2 * time.Minute)
	_, _ = p.Ensure(context.Background(), "alice", "agent")
	if app.logins != n+1 {
		t.Fatalf("after an hour want exactly one verifying login, got %d", app.logins-n)
	}
}

func TestEnsure_PrevPeriodRotates(t *testing.T) { // SC-17
	app := newFakeApp()
	prev := Password([]byte("testkey"), "alice", "agent", 229)
	cur := Password([]byte("testkey"), "alice", "agent", 230)
	app.users[aliceAgent] = &fakeUser{id: "u1", password: prev}
	r, err := newProvisioner(app).Ensure(context.Background(), "alice", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if len(app.changes) != 1 || app.changes[0] != [2]string{prev, cur} {
		t.Fatalf("changes %v", app.changes)
	}
	if r.Password != cur || app.creates != 0 || len(app.resets) != 0 {
		t.Fatalf("got %+v creates=%d resets=%d", r, app.creates, len(app.resets))
	}
}

func TestEnsure_DriftedPasswordResets(t *testing.T) { // SC-13
	app := newFakeApp()
	app.users[aliceAgent] = &fakeUser{id: "u1", password: "someone-changed-it"}
	r, err := newProvisioner(app).Ensure(context.Background(), "alice", "agent")
	if err != nil {
		t.Fatal(err)
	}
	// The brief defines the temporary password as Password(key, login, role+":tmp", period).
	tmp := Password([]byte("testkey"), "alice", "agent:tmp", 230)
	cur := Password([]byte("testkey"), "alice", "agent", 230)
	if len(app.resets) != 1 || app.resets[0] != tmp {
		t.Fatalf("resets %v", app.resets)
	}
	if len(app.changes) != 1 || app.changes[0] != [2]string{tmp, cur} {
		t.Fatalf("changes %v", app.changes)
	}
	if app.creates != 0 || app.badLogins > 2 {
		t.Fatalf("creates=%d badLogins=%d", app.creates, app.badLogins)
	}
	if _, err := app.Login(context.Background(), r.Email, r.Password); err != nil {
		t.Fatalf("returned password does not log in: %v", err)
	}
}

func TestEnsure_HubDownDoesNotReset(t *testing.T) {
	app := newFakeApp()
	app.loginErr = errors.New("hub login: status 503")
	if _, err := newProvisioner(app).Ensure(context.Background(), "alice", "agent"); err == nil {
		t.Fatal("want error")
	}
	if app.logins != 1 || app.finds != 0 || app.creates != 0 || len(app.resets) != 0 {
		t.Fatalf("logins=%d finds=%d creates=%d resets=%d", app.logins, app.finds, app.creates, len(app.resets))
	}
	// A failure is not cached: the next call tries again.
	app.loginErr = nil
	if _, err := newProvisioner(app).Ensure(context.Background(), "alice", "agent"); err != nil {
		t.Fatal(err)
	}
}

func TestEnsure_ConcurrentCreatesOnce(t *testing.T) {
	app := newFakeApp()
	p := newProvisioner(app)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Ensure(context.Background(), "alice", "agent"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if app.creates != 1 {
		t.Fatalf("CreateUser called %d times", app.creates)
	}
}

func TestEnsure_FailureCachedTenMinutes(t *testing.T) {
	app := newFakeApp()
	app.loginErr = errors.New("hub login: status 401: Domain không tồn tại")
	now := testNow
	p := newProvisioner(app)
	p.Now = func() time.Time { return now }
	_, err1 := p.Ensure(context.Background(), "alice", "agent")
	if err1 == nil {
		t.Fatal("want error")
	}
	now = now.Add(9 * time.Minute)
	_, err2 := p.Ensure(context.Background(), "alice", "agent")
	if err2 == nil || err2.Error() != err1.Error() || app.logins != 1 {
		t.Fatalf("within 10 min: err=%v logins=%d, want the cached error and no upstream call", err2, app.logins)
	}
	now = now.Add(2 * time.Minute)
	app.loginErr = nil
	if _, err := p.Ensure(context.Background(), "alice", "agent"); err != nil || app.logins < 2 {
		t.Fatalf("after 10 min: err=%v logins=%d", err, app.logins)
	}
}
