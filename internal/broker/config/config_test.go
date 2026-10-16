package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleYAML = `org: TestOrg
listen: ":9090"
grafana:
  url: https://grafana.example.test
posthog:
  upstream: https://mcp.example.test/mcp
  project_id: "123"
e2e:
  hub_url: https://hub.example.test
  be_url: https://be.example.test
  domain: acme
  tenant_id: t1
  group_id: g1
  email_domain: e2e.example.test
`

var secretNames = []string{"BROKER_JWT_KEY", "BROKER_E2E_HMAC_KEY", "MONGO_URL_STG", "MONGO_URL_PRD",
	"MYSQL_URL_STG", "MYSQL_URL_PRD", "AUDIT_MONGO_URL_PRD", "REDIS_URL_STG", "REDIS_URL_PRD",
	"GRAFANA_SERVICE_ACCOUNT_TOKEN", "POSTHOG_API_KEY", "E2E_ADMIN_EMAIL", "E2E_ADMIN_PASSWORD"}

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "broker.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func fullEnv() map[string]string {
	m := map[string]string{}
	for _, n := range secretNames {
		m[n] = "val-of-" + strings.ToLower(n)
	}
	m["BROKER_JWT_KEY"] = strings.Repeat("j", 32)
	m["BROKER_E2E_HMAC_KEY"] = strings.Repeat("h", 40)
	return m
}

func TestLoad_MissingSecretsNamed(t *testing.T) {
	env := fullEnv()
	delete(env, "POSTHOG_API_KEY")
	env["REDIS_URL_PRD"] = ""
	_, err := Load(writeCfg(t, sampleYAML), envOf(env))
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"POSTHOG_API_KEY", "REDIS_URL_PRD"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should name %s", err, want)
		}
	}
	for _, n := range secretNames {
		if v := env[n]; v != "" && strings.Contains(err.Error(), v) {
			t.Errorf("error leaks the value of %s", n)
		}
	}
}

func TestLoad_ShortJWTKey(t *testing.T) {
	for _, k := range []string{"BROKER_JWT_KEY", "BROKER_E2E_HMAC_KEY"} {
		env := fullEnv()
		env[k] = strings.Repeat("s", 31)
		_, err := Load(writeCfg(t, sampleYAML), envOf(env))
		if err == nil || !strings.Contains(err.Error(), k) {
			t.Errorf("%s of 31 bytes: want error naming it, got %v", k, err)
		}
		if err != nil && strings.Contains(err.Error(), strings.Repeat("s", 31)) {
			t.Errorf("error leaks the key")
		}
	}
}

func TestLoad_OK(t *testing.T) {
	c, err := Load(writeCfg(t, sampleYAML), envOf(fullEnv()))
	if err != nil {
		t.Fatal(err)
	}
	if c.Org != "TestOrg" || c.Listen != ":9090" || c.Grafana.URL != "https://grafana.example.test" {
		t.Errorf("yaml fields not loaded: %+v", c)
	}
	if c.Mongo.MainDB != "3csoft" || c.Mongo.AuditDB != "audit_logs" {
		t.Errorf("mongo defaults: %+v", c.Mongo)
	}
	if c.Grafana.Bin != "mcp-grafana" || c.Grafana.Addr != "127.0.0.1:8000" {
		t.Errorf("grafana defaults: %+v", c.Grafana)
	}
	wantArgs := []string{"--enabled-tools=search,datasource,prometheus,loki,tempo,alerting,dashboard,folder,annotations,navigation",
		"--loki-guardrail-mode=enforce", "--max-loki-log-limit=100", "--loki-guardrail-max-range=24h", "--usage-stats=disabled"}
	if strings.Join(c.Grafana.Args, "|") != strings.Join(wantArgs, "|") {
		t.Errorf("default grafana.args = %v", c.Grafana.Args)
	}
	if c.Secrets.POSTHOGAPIKey != "val-of-posthog_api_key" || len(c.Secrets.JWTKey) != 32 {
		t.Errorf("secrets not loaded")
	}
}

func TestLoad_OverridesAndUnknownKey(t *testing.T) {
	over := sampleYAML + "mongo:\n  main_db: prodmain\ngrafana2: x\n"
	if _, err := Load(writeCfg(t, over), envOf(fullEnv())); err == nil {
		t.Error("unknown key should be rejected")
	}
	over = strings.Replace(sampleYAML, "listen", "mongo:\n  main_db: prodmain\nlisten", 1)
	c, err := Load(writeCfg(t, over), envOf(fullEnv()))
	if err != nil || c.Mongo.MainDB != "prodmain" || c.Mongo.AuditDB != "audit_logs" {
		t.Errorf("override: %+v %v", c.Mongo, err)
	}
}

func TestLoad_MissingRequiredYAML(t *testing.T) {
	_, err := Load(writeCfg(t, strings.Replace(sampleYAML, "org: TestOrg\n", "", 1)), envOf(fullEnv()))
	if err == nil || !strings.Contains(err.Error(), "org") {
		t.Errorf("want error naming org, got %v", err)
	}
}

func TestLoad_YAMLAndEnvErrorsTogether(t *testing.T) {
	env := fullEnv()
	delete(env, "POSTHOG_API_KEY")
	_, err := Load(writeCfg(t, strings.Replace(sampleYAML, "org: TestOrg\n", "", 1)), envOf(env))
	if err == nil || !strings.Contains(err.Error(), "missing org") || !strings.Contains(err.Error(), "POSTHOG_API_KEY") {
		t.Errorf("want one error naming both org and POSTHOG_API_KEY, got %v", err)
	}
}

func TestLoad_TeamDefaultsAndDenyLogins(t *testing.T) {
	c, err := Load(writeCfg(t, sampleYAML), envOf(fullEnv()))
	if err != nil {
		t.Fatal(err)
	}
	if c.LeadTeam != "leaders" || c.DevTeam != "zenify-team" || len(c.DenyLogins) != 0 {
		t.Errorf("defaults = %q %q %v", c.LeadTeam, c.DevTeam, c.DenyLogins)
	}
	c, err = Load(writeCfg(t, sampleYAML+"lead_team: core\ndev_team: devs\ndeny_logins: [zenifydevteam, zenifydeveloper]\n"), envOf(fullEnv()))
	if err != nil {
		t.Fatal(err)
	}
	if c.LeadTeam != "core" || c.DevTeam != "devs" || strings.Join(c.DenyLogins, ",") != "zenifydevteam,zenifydeveloper" {
		t.Errorf("set = %q %q %v", c.LeadTeam, c.DevTeam, c.DenyLogins)
	}
}
