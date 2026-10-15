// Command zenify-broker is the credential broker: it holds the team's
// read-only data credentials and serves them to GitHub-org members.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/audit"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/config"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/e2e"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/mcpproxy"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/query"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/redisread"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/server"
)

// maxConcurrentPerLogin caps simultaneous /query children per developer.
const maxConcurrentPerLogin = 4

func main() {
	cfgPath := flag.String("config", "/etc/zenify-broker/broker.yaml", "path to the broker YAML config")
	flag.Parse()
	if err := run(*cfgPath); err != nil {
		fmt.Fprintln(os.Stderr, "zenify-broker:", err)
		os.Exit(1)
	}
}

func run(cfgPath string) error {
	cfg, err := config.Load(cfgPath, os.Getenv)
	if err != nil {
		return err
	}
	s := cfg.Secrets

	redisClient := func(name, u string) (*redis.Client, error) {
		o, err := redis.ParseURL(u)
		if err != nil {
			// redis's parse error can echo the URL; name only the variable.
			return nil, fmt.Errorf("%s is not a valid redis URL", name)
		}
		return redis.NewClient(o), nil
	}
	rStg, err := redisClient("REDIS_URL_STG", s.RedisStg)
	if err != nil {
		return err
	}
	rPrd, err := redisClient("REDIS_URL_PRD", s.RedisPrd)
	if err != nil {
		return err
	}

	grafanaUp, err := url.Parse("http://" + cfg.Grafana.Addr + "/mcp")
	if err != nil {
		return fmt.Errorf("grafana.addr: %w", err)
	}
	posthogUp, err := url.Parse(cfg.PostHog.Upstream)
	if err != nil {
		return fmt.Errorf("posthog.upstream: %w", err)
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() { <-sigs; cancel(nil) }()

	if _, err := mcpproxy.StartGrafana(ctx, cancel, cfg.Grafana.Bin, cfg.Grafana.URL, s.GrafanaToken, cfg.Grafana.Addr, cfg.Grafana.Args); err != nil {
		return err
	}

	log := &audit.Logger{W: os.Stdout}
	handler := server.New(server.Deps{
		JWTKey: s.JWTKey,
		Org:    cfg.Org,
		GitHub: auth.GitHub{},
		Audit:  log,
		Query: &query.Handler{
			Creds: query.Creds{MongoStg: s.MongoStg, MongoPrd: s.MongoPrd, MySQLStg: s.MySQLStg, MySQLPrd: s.MySQLPrd,
				AuditMongoPrd: s.AuditMongoPrd, MainDB: cfg.Mongo.MainDB, AuditDB: cfg.Mongo.AuditDB},
			Audit: log, Run: query.ExecRun, MaxConcurrent: maxConcurrentPerLogin,
		},
		Redis: &redisread.Handler{Clients: map[string]redisread.Doer{"stg": rStg, "prd": rPrd}, Audit: log},
		E2E: &e2e.Handler{Audit: log, P: &e2e.Provisioner{
			App: &e2e.HTTPApp{HubURL: cfg.E2E.HubURL, BEURL: cfg.E2E.BEURL, Domain: cfg.E2E.Domain,
				TenantID: cfg.E2E.TenantID, GroupID: cfg.E2E.GroupID,
				AdminEmail: s.E2EAdminEmail, AdminPassword: s.E2EAdminPassword},
			Key: s.E2EHMACKey, EmailDomain: cfg.E2E.EmailDomain, Domain: cfg.E2E.Domain, Now: time.Now,
		}},
		MCP: map[string]http.Handler{
			"grafana": mcpproxy.New("grafana", grafanaUp, nil, log),
			"posthog": mcpproxy.New("posthog", posthogUp, mcpproxy.PostHogHeaders(s.POSTHOGAPIKey, cfg.PostHog.ProjectID), log),
		},
	})

	// No WriteTimeout: it would cut MCP SSE streams.
	srv := &http.Server{Addr: cfg.Listen, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	shutCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	_ = srv.Shutdown(shutCtx)
	if cause := context.Cause(ctx); !errors.Is(cause, context.Canceled) {
		return cause
	}
	return nil
}
