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
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/catalogsvc"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/config"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/creds"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/e2e"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/mcpproxy"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/obssvc"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/query"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/redisread"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/server"
	"github.com/ZenifyAIContactCenter/zenify-kit/internal/version"
)

// maxConcurrentPerLogin caps simultaneous /query children and /redis commands per developer.
const maxConcurrentPerLogin = 2

func main() {
	cfgPath := flag.String("config", "/etc/zenify-broker/broker.yaml", "path to the broker YAML config")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("zenify-broker", version.Version)
		return
	}
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

	log := &audit.Logger{W: os.Stdout}
	queryH := &query.Handler{
		Creds: query.Creds{MongoStg: s.MongoStg, MongoPrd: s.MongoPrd, MySQLStg: s.MySQLStg, MySQLPrd: s.MySQLPrd,
			AuditMongoPrd: s.AuditMongoPrd, MainDB: cfg.Mongo.MainDB, AuditDB: cfg.Mongo.AuditDB,
			LumiMySQLStg: s.LumiMySQLStg, LumiMySQLPrd: s.LumiMySQLPrd,
			MsghubMongoStg: s.MsghubMongoStg, MsghubMongoPrd: s.MsghubMongoPrd, MsghubDB: cfg.Mongo.MsghubDB},
		Audit: log, Run: query.ExecRun, MaxConcurrent: maxConcurrentPerLogin,
	}
	redisH := &redisread.Handler{Clients: map[string]redisread.Doer{"stg": rStg, "prd": rPrd}, Audit: log,
		MaxConcurrent: maxConcurrentPerLogin}
	e2eH := &e2e.Handler{Audit: log, P: &e2e.Provisioner{
		App: &e2e.HTTPApp{HubURL: cfg.E2E.HubURL, BEURL: cfg.E2E.BEURL, Domain: cfg.E2E.Domain,
			TenantID: cfg.E2E.TenantID, GroupID: cfg.E2E.GroupID,
			AdminEmail: s.E2EAdminEmail, AdminPassword: s.E2EAdminPassword},
		Key: s.E2EHMACKey, EmailDomain: cfg.E2E.EmailDomain, Domain: cfg.E2E.Domain, Now: time.Now,
	}}
	credsH := &creds.Handler{Audit: log,
		Read: map[string]map[string]string{
			"stg": nonEmpty("MONGO_URL", s.MongoStg, "MYSQL_URL", s.MySQLStg, "REDIS_URL", s.RedisStg,
				"ELASTIC_URL", s.ElasticStg),
			"prd": nonEmpty("MONGO_URL", s.MongoPrd, "MYSQL_URL", s.MySQLPrd, "REDIS_URL", s.RedisPrd,
				"ELASTIC_URL", s.ElasticPrd)},
		Write: map[string]map[string]string{
			"stg": nonEmpty("MONGO_URL", s.MongoWriteStg, "MYSQL_URL", s.MySQLWriteStg, "REDIS_URL", s.RedisWriteStg),
			"prd": nonEmpty("MONGO_URL", s.MongoWritePrd, "MYSQL_URL", s.MySQLWritePrd, "REDIS_URL", s.RedisWritePrd)},
		Lumi: map[string]map[string]string{
			"stg": nonEmpty("LUMI_MYSQL_URL", s.LumiMySQLStg, "QDRANT_URL", s.QdrantStg, "LUMI_APP_URL", cfg.Lumi.AppURLStg),
			"prd": nonEmpty("LUMI_MYSQL_URL", s.LumiMySQLPrd, "QDRANT_URL", s.QdrantPrd)},
	}
	// A mis-wired handler fails here, at startup, not on every request.
	if err := errors.Join(queryH.Validate(), redisH.Validate(), e2eH.Validate(), credsH.Validate()); err != nil {
		return err
	}

	// grafana.url and GRAFANA_SERVICE_ACCOUNT_TOKEN are required by config.Load,
	// so drift is served whenever the catalog is.
	var catalogH, driftH, obsRouteH, obsTraceH http.Handler
	if cfg.Catalog != nil {
		ch := catalogsvc.New(*cfg.Catalog, s.CatalogGitHubToken, catalogsvc.WithAudit(log))
		catalogH = ch
		driftH = catalogsvc.NewDrift(ch, cfg.Grafana.URL, s.GrafanaToken, catalogsvc.WithAudit(log), catalogsvc.WithSlots(1))
		oh := obssvc.New(ch, cfg.Grafana.URL, s.GrafanaToken, obssvc.WithAudit(log))
		obsRouteH, obsTraceH = oh.Route(), oh.Trace()
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go watchSignals(sigs, cancel, os.Exit)

	if _, err := mcpproxy.StartGrafana(ctx, cancel, cfg.Grafana.Bin, cfg.Grafana.URL, s.GrafanaToken, cfg.Grafana.Addr, cfg.Grafana.Args); err != nil {
		return err
	}

	handler := server.New(server.Deps{
		JWTKey:       s.JWTKey,
		Org:          cfg.Org,
		LeadTeam:     cfg.LeadTeam,
		DevTeam:      cfg.DevTeam,
		DenyLogins:   cfg.DenyLogins,
		AgentIDs:     cfg.AgentIDs,
		Actions:      &auth.ActionsVerifier{},
		E2EWorkflows: cfg.E2EWorkflows,
		GitHub:       auth.GitHub{},
		Audit:        log,
		Query:        queryH,
		Redis:        redisH,
		E2E:          e2eH,
		Creds:        credsH,
		Catalog:      catalogH,
		CatalogDrift: driftH,
		ObsRoute:     obsRouteH,
		ObsTrace:     obsTraceH,
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

// watchSignals starts a graceful shutdown on the first signal and exits at
// once on the second, so an operator can cut a stuck shutdown short.
func watchSignals(sigs <-chan os.Signal, cancel context.CancelCauseFunc, exit func(int)) {
	<-sigs
	cancel(nil)
	<-sigs
	exit(1)
}

// nonEmpty builds a name -> value map from name, value pairs, skipping empty values.
func nonEmpty(pairs ...string) map[string]string {
	m := map[string]string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i+1] != "" {
			m[pairs[i]] = pairs[i+1]
		}
	}
	return m
}
