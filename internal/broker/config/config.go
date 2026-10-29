// Package config loads zenify-broker's settings: non-secret values from a YAML
// file, secrets from the host environment.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ZenifyAIContactCenter/zenify-kit/internal/broker/auth"
)

// Config is the broker's whole configuration.
type Config struct {
	Org    string `yaml:"org"`
	Listen string `yaml:"listen"`
	// LeadTeam / DevTeam are the org team slugs that grant role lead / dev.
	LeadTeam string `yaml:"lead_team"`
	DevTeam  string `yaml:"dev_team"`
	// DenyLogins are GitHub logins refused a token (shared accounts), matched
	// case-insensitively.
	DenyLogins []string `yaml:"deny_logins"`
	// AgentIDs are GitHub databaseIds of App installations' bot users that get
	// role agent from an installation token (ghs_).
	AgentIDs []int64 `yaml:"agent_ids"`
	// E2EWorkflows are the exact GitHub Actions workflow_ref values whose OIDC
	// token gets role e2e; empty = no OIDC token is accepted.
	E2EWorkflows []string `yaml:"e2e_workflows"`
	Grafana      struct {
		URL  string   `yaml:"url"`
		Bin  string   `yaml:"bin"`
		Addr string   `yaml:"addr"`
		Args []string `yaml:"args"`
	} `yaml:"grafana"`
	PostHog struct {
		Upstream  string `yaml:"upstream"`
		ProjectID string `yaml:"project_id"`
	} `yaml:"posthog"`
	E2E struct {
		HubURL      string `yaml:"hub_url"`
		BEURL       string `yaml:"be_url"`
		Domain      string `yaml:"domain"`
		TenantID    string `yaml:"tenant_id"`
		GroupID     string `yaml:"group_id"`
		EmailDomain string `yaml:"email_domain"`
	} `yaml:"e2e"`
	Mongo struct {
		MainDB  string `yaml:"main_db"`
		AuditDB string `yaml:"audit_db"`
		// MsghubDB is the database read by db-read --target msghub.
		MsghubDB string `yaml:"msghub_db"`
	} `yaml:"mongo"`
	Lumi struct {
		AppURLStg string `yaml:"app_url_stg"`
	} `yaml:"lumi"`

	// Catalog is optional; when set the broker serves the service catalog.
	Catalog *Catalog `yaml:"catalog"`

	Secrets Secrets `yaml:"-"`
}

// Catalog locates the catalog file in a GitHub repo.
type Catalog struct {
	Repo string `yaml:"repo"`
	Path string `yaml:"path"`
	Ref  string `yaml:"ref"`
}

// Secrets come from the host environment only.
type Secrets struct {
	JWTKey, E2EHMACKey                                    []byte
	MongoStg, MongoPrd, MySQLStg, MySQLPrd, AuditMongoPrd string
	RedisStg, RedisPrd, GrafanaToken, POSTHOGAPIKey       string
	E2EAdminEmail, E2EAdminPassword                       string
	// Optional: empty when the env var is unset; never an error.
	MongoWriteStg, MongoWritePrd, MySQLWriteStg, MySQLWritePrd string
	RedisWriteStg, RedisWritePrd, LumiMySQLStg, LumiMySQLPrd   string
	MsghubMongoStg, MsghubMongoPrd                             string
	QdrantStg, QdrantPrd                                       string
	ElasticStg, ElasticPrd                                     string
	// Required only when Config.Catalog is set.
	CatalogGitHubToken string
}

// DefaultGrafanaArgs is the flag set the team runs mcp-grafana with.
var DefaultGrafanaArgs = []string{
	"--enabled-tools=search,datasource,prometheus,loki,tempo,alerting,dashboard,folder,annotations,navigation",
	"--loki-guardrail-mode=enforce", "--max-loki-log-limit=100", "--loki-guardrail-max-range=24h", "--usage-stats=disabled",
}

// Load reads the YAML at path and the secrets via getenv. Errors name keys,
// never values; missing YAML keys and env problems are reported together.
func Load(path string, getenv func(string) string) (Config, error) {
	var c Config
	raw, err := os.ReadFile(path) //nolint:gosec // G304 -- path is the operator's --config flag
	if err != nil {
		return c, fmt.Errorf("read config: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("parse %s: %w", path, err)
	}
	def := func(p *string, v string) {
		if *p == "" {
			*p = v
		}
	}
	def(&c.Listen, ":8080")
	def(&c.LeadTeam, "leaders")
	def(&c.DevTeam, "zenify-team")
	def(&c.Grafana.Bin, "mcp-grafana")
	def(&c.Grafana.Addr, "127.0.0.1:8000")
	def(&c.Mongo.MainDB, "3csoft")
	def(&c.Mongo.AuditDB, "audit_logs")
	if c.Grafana.Args == nil {
		c.Grafana.Args = append([]string(nil), DefaultGrafanaArgs...)
	}

	var errs []error
	var missing []string
	for k, v := range map[string]string{"org": c.Org, "grafana.url": c.Grafana.URL,
		"posthog.upstream": c.PostHog.Upstream, "posthog.project_id": c.PostHog.ProjectID,
		"e2e.hub_url": c.E2E.HubURL, "e2e.be_url": c.E2E.BEURL, "e2e.domain": c.E2E.Domain,
		"e2e.tenant_id": c.E2E.TenantID, "e2e.group_id": c.E2E.GroupID, "e2e.email_domain": c.E2E.EmailDomain} {
		if v == "" {
			missing = append(missing, k)
		}
	}
	if c.Catalog != nil {
		def(&c.Catalog.Ref, "main")
		if c.Catalog.Repo == "" {
			missing = append(missing, "catalog.repo")
		}
		if c.Catalog.Path == "" {
			missing = append(missing, "catalog.path")
		}
	}
	if len(missing) > 0 {
		errs = append(errs, fmt.Errorf("config %s: missing %s", path, strings.Join(sorted(missing), ", ")))
	}

	var noEnv []string
	get := func(name string) string {
		v := getenv(name)
		if v == "" {
			noEnv = append(noEnv, name)
		}
		return v
	}
	s := &c.Secrets
	jwt, hmac := get("BROKER_JWT_KEY"), get("BROKER_E2E_HMAC_KEY")
	s.JWTKey, s.E2EHMACKey = []byte(jwt), []byte(hmac)
	s.MongoStg, s.MongoPrd = get("MONGO_URL_STG"), get("MONGO_URL_PRD")
	s.MySQLStg, s.MySQLPrd = get("MYSQL_URL_STG"), get("MYSQL_URL_PRD")
	s.AuditMongoPrd = get("AUDIT_MONGO_URL_PRD")
	s.RedisStg, s.RedisPrd = get("REDIS_URL_STG"), get("REDIS_URL_PRD")
	s.GrafanaToken, s.POSTHOGAPIKey = get("GRAFANA_SERVICE_ACCOUNT_TOKEN"), get("POSTHOG_API_KEY")
	s.E2EAdminEmail, s.E2EAdminPassword = get("E2E_ADMIN_EMAIL"), get("E2E_ADMIN_PASSWORD")
	s.MongoWriteStg, s.MongoWritePrd = getenv("MONGO_WRITE_URL_STG"), getenv("MONGO_WRITE_URL_PRD")
	s.MySQLWriteStg, s.MySQLWritePrd = getenv("MYSQL_WRITE_URL_STG"), getenv("MYSQL_WRITE_URL_PRD")
	s.RedisWriteStg, s.RedisWritePrd = getenv("REDIS_WRITE_URL_STG"), getenv("REDIS_WRITE_URL_PRD")
	s.LumiMySQLStg, s.LumiMySQLPrd = getenv("LUMI_MYSQL_URL_STG"), getenv("LUMI_MYSQL_URL_PRD")
	s.MsghubMongoStg, s.MsghubMongoPrd = getenv("MSGHUB_MONGO_URL_STG"), getenv("MSGHUB_MONGO_URL_PRD")
	s.QdrantStg, s.QdrantPrd = getenv("QDRANT_URL_STG"), getenv("QDRANT_URL_PRD")
	s.ElasticStg, s.ElasticPrd = getenv("ELASTIC_URL_STG"), getenv("ELASTIC_URL_PRD")
	if c.Catalog != nil {
		s.CatalogGitHubToken = get("CATALOG_GITHUB_TOKEN")
	}
	if (s.MsghubMongoStg != "" || s.MsghubMongoPrd != "") && c.Mongo.MsghubDB == "" {
		errs = append(errs, fmt.Errorf("config %s: missing mongo.msghub_db (required when MSGHUB_MONGO_URL_* is set)", path))
	}
	if len(noEnv) > 0 {
		errs = append(errs, fmt.Errorf("missing required env: %s", strings.Join(sorted(noEnv), ", ")))
	}
	for name, v := range map[string]string{"BROKER_JWT_KEY": jwt, "BROKER_E2E_HMAC_KEY": hmac} {
		if v != "" && len(v) < auth.MinKeyLen {
			errs = append(errs, fmt.Errorf("%s must be at least %d bytes", name, auth.MinKeyLen))
		}
	}
	return c, errors.Join(errs...)
}

func sorted(s []string) []string {
	sort.Strings(s)
	return s
}
