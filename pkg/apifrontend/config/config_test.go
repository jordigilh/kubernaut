package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// validConfig returns a Config that passes Validate() for use as a base in tests.
func validConfig() *Config {
	return &Config{
		Server: ServerConfig{Port: 8443},
		Agent: AgentConfig{
			KABaseURL:     "http://localhost:8080",
			KAMCPEndpoint: "http://localhost:8080/api/v1/mcp/",
			DSBaseURL:     "http://localhost:9090",
		},
		MCP:       MCPConfig{Enabled: false},
		AgentCard: AgentCardConfig{URL: "https://localhost:8443"},
		Logging:   LoggingConfig{Level: "INFO"},
		RateLimit: RateLimitConfig{IPRequestsPerSec: 100, UserRequestsPerSec: 50},
		Shutdown:  ShutdownConfig{DrainSeconds: 15},
		Resilience: ResilienceConfig{
			KA:  DependencyConfig{CBFailureThreshold: 5},
			DS:  DependencyConfig{CBFailureThreshold: 3},
			K8s: DependencyConfig{CBFailureThreshold: 5},
		},
	}
}

// --- Tier 1: Config Loading — Load() ---

func TestLoad_ValidFullYAML(t *testing.T) {
	// UT-AF-039-001
	data, err := os.ReadFile("testdata/valid.yaml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Agent.LLM.VertexProject != "my-project" {
		t.Errorf("Agent.LLM.VertexProject = %q, want %q", cfg.Agent.LLM.VertexProject, "my-project")
	}
	if cfg.Agent.LLM.VertexLocation != "us-east1" {
		t.Errorf("Agent.LLM.VertexLocation = %q, want %q", cfg.Agent.LLM.VertexLocation, "us-east1")
	}
	if cfg.Agent.KABaseURL != "https://ka.example.com" {
		t.Errorf("Agent.KABaseURL = %q, want %q", cfg.Agent.KABaseURL, "https://ka.example.com")
	}
	if cfg.Agent.KAMCPEndpoint != "https://ka.example.com/api/v1/mcp/" {
		t.Errorf("Agent.KAMCPEndpoint = %q, want %q", cfg.Agent.KAMCPEndpoint, "https://ka.example.com/api/v1/mcp/")
	}
	if cfg.Agent.DSBaseURL != "https://ds.example.com" {
		t.Errorf("Agent.DSBaseURL = %q, want %q", cfg.Agent.DSBaseURL, "https://ds.example.com")
	}
	if !cfg.MCP.Enabled {
		t.Error("MCP.Enabled = false, want true")
	}
	if cfg.AgentCard.URL != "https://af.example.com" {
		t.Errorf("AgentCard.URL = %q, want %q", cfg.AgentCard.URL, "https://af.example.com")
	}
}

func TestLoad_AppliesDefaultsForOmittedFields(t *testing.T) {
	// UT-AF-039-002
	data := []byte("agent:\n  llm:\n    vertexProject: \"p\"\n")
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	defaults := DefaultConfig()
	if cfg.Server.Port != defaults.Server.Port {
		t.Errorf("Server.Port = %d, want default %d", cfg.Server.Port, defaults.Server.Port)
	}
	if cfg.Agent.LLM.VertexLocation != defaults.Agent.LLM.VertexLocation {
		t.Errorf("Agent.LLM.VertexLocation = %q, want default %q", cfg.Agent.LLM.VertexLocation, defaults.Agent.LLM.VertexLocation)
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	// UT-AF-039-003
	data := []byte("server:\n  port: [invalid")
	_, err := Load(data)
	if err == nil {
		t.Fatal("Load() expected error for malformed YAML, got nil")
	}
	if !strings.Contains(err.Error(), "pars") {
		t.Errorf("error = %q, want to contain 'pars' (parsing context)", err.Error())
	}
}

func TestLoad_EmptyInput(t *testing.T) {
	// UT-AF-039-004
	cfg, err := Load([]byte(""))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	defaults := DefaultConfig()
	if cfg.Server.Port != defaults.Server.Port {
		t.Errorf("Server.Port = %d, want default %d", cfg.Server.Port, defaults.Server.Port)
	}
}

func TestLoad_IgnoresUnknownKeys(t *testing.T) {
	// UT-AF-039-005
	data := []byte("server:\n  port: 9090\nunknownField: \"should be ignored\"\n")
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for unknown keys", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
}

func TestLoad_PreservesZeroBooleans(t *testing.T) {
	// UT-AF-039-006
	data := []byte("mcp:\n  enabled: false\n")
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MCP.Enabled {
		t.Error("MCP.Enabled = true, want false (explicit zero-value)")
	}
}

func TestLoad_PortAsInteger(t *testing.T) {
	// UT-AF-039-007
	data := []byte("server:\n  port: 3000\n")
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("Server.Port = %d, want 3000", cfg.Server.Port)
	}
}

func TestLoad_PartialYAMLMergesWithDefaults(t *testing.T) {
	// UT-AF-039-008
	data := []byte("server:\n  port: 7777\nagent:\n  llm:\n    vertexProject: \"partial\"\n")
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != 7777 {
		t.Errorf("Server.Port = %d, want 7777", cfg.Server.Port)
	}
	if cfg.Agent.LLM.VertexProject != "partial" {
		t.Errorf("Agent.LLM.VertexProject = %q, want %q", cfg.Agent.LLM.VertexProject, "partial")
	}
	defaults := DefaultConfig()
	if cfg.Agent.LLM.VertexLocation != defaults.Agent.LLM.VertexLocation {
		t.Errorf("Agent.LLM.VertexLocation = %q, want default %q", cfg.Agent.LLM.VertexLocation, defaults.Agent.LLM.VertexLocation)
	}
}

// --- Tier 2: Config Validation — Validate() ---

func TestValidate_AcceptsDefaultConfig(t *testing.T) {
	// UT-AF-039-009
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() on DefaultConfig() = %v, want nil", err)
	}
}

func TestValidate_PortRange(t *testing.T) {
	// UT-AF-039-010, 011, 012
	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{"port < 1", -1, true},
		{"port = 0", 0, true},
		{"port > 65535", 70000, true},
		{"port = 1 (min valid)", 1, false},
		{"port = 65535 (max valid)", 65535, false},
		{"port = 8443 (typical)", 8443, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Server.Port = tt.port
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "server.port") {
				t.Errorf("error = %q, want to contain 'server.port'", err.Error())
			}
		})
	}
}

func TestValidate_RequiredURLFields(t *testing.T) {
	// UT-AF-039-013, 015, 016
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{
			name:    "empty kaBaseURL",
			mutate:  func(c *Config) { c.Agent.KABaseURL = "" },
			wantSub: "kaBaseURL",
		},
		{
			name:    "empty kaMCPEndpoint",
			mutate:  func(c *Config) { c.Agent.KAMCPEndpoint = "" },
			wantSub: "kaMCPEndpoint",
		},
		{
			name:    "empty dsBaseURL",
			mutate:  func(c *Config) { c.Agent.DSBaseURL = "" },
			wantSub: "dsBaseURL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantSub)
			}
		})
	}
}

func TestValidate_MalformedURLs(t *testing.T) {
	// UT-AF-039-014, 017
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{
			name:    "kaBaseURL no scheme",
			mutate:  func(c *Config) { c.Agent.KABaseURL = "not-a-url" },
			wantSub: "kaBaseURL",
		},
		{
			name:    "dsBaseURL no scheme",
			mutate:  func(c *Config) { c.Agent.DSBaseURL = "://bad" },
			wantSub: "dsBaseURL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantSub)
			}
		})
	}
}

func TestValidate_AcceptsValidCompleteConfig(t *testing.T) {
	// UT-AF-039-018
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestValidate_ErrorMessageIncludesFieldName(t *testing.T) {
	// UT-AF-039-019
	cfg := validConfig()
	cfg.Server.Port = -1
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error")
	}
	if !strings.Contains(err.Error(), "server.port") {
		t.Errorf("error = %q, want to contain field path 'server.port'", err.Error())
	}
}

func TestValidate_ReturnsFirstErrorOnly(t *testing.T) {
	// UT-AF-039-020
	cfg := validConfig()
	cfg.Server.Port = -1
	cfg.Agent.KABaseURL = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error")
	}
	// Should only contain one error, not both
	errStr := err.Error()
	if strings.Contains(errStr, "server.port") && strings.Contains(errStr, "kaBaseURL") {
		t.Error("Validate() returned multiple errors; want fail-fast (first error only)")
	}
}

// --- Tier 3: Default Resolution — ResolveDefaults() ---

func TestResolveDefaults_SetsAgentCardURLFromPort(t *testing.T) {
	// UT-AF-039-021
	cfg := validConfig()
	cfg.AgentCard.URL = ""
	cfg.Server.Port = 8443
	if err := cfg.ResolveDefaults(); err != nil {
		t.Fatalf("ResolveDefaults() = %v", err)
	}
	want := "https://localhost:8443"
	if cfg.AgentCard.URL != want {
		t.Errorf("AgentCard.URL = %q, want %q", cfg.AgentCard.URL, want)
	}
}

func TestResolveDefaults_PreservesExplicitURL(t *testing.T) {
	// UT-AF-039-022
	cfg := validConfig()
	cfg.AgentCard.URL = "https://custom.example.com"
	if err := cfg.ResolveDefaults(); err != nil {
		t.Fatalf("ResolveDefaults() = %v", err)
	}
	if cfg.AgentCard.URL != "https://custom.example.com" {
		t.Errorf("AgentCard.URL = %q, want preserved value", cfg.AgentCard.URL)
	}
}

func TestResolveDefaults_Idempotent(t *testing.T) {
	// UT-AF-039-023
	cfg := validConfig()
	cfg.AgentCard.URL = ""
	cfg.Server.Port = 9000
	if err := cfg.ResolveDefaults(); err != nil {
		t.Fatalf("ResolveDefaults() = %v", err)
	}
	first := cfg.AgentCard.URL
	if err := cfg.ResolveDefaults(); err != nil {
		t.Fatalf("ResolveDefaults() second call = %v", err)
	}
	if cfg.AgentCard.URL != first {
		t.Errorf("second ResolveDefaults() changed URL: %q -> %q", first, cfg.AgentCard.URL)
	}
}

func TestResolveDefaults_CalledBeforeValidate(t *testing.T) {
	// UT-AF-039-024
	cfg := validConfig()
	cfg.AgentCard.URL = ""
	cfg.Server.Port = 8443
	if err := cfg.ResolveDefaults(); err != nil {
		t.Fatalf("ResolveDefaults() = %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() after ResolveDefaults() = %v, want nil", err)
	}
}

// --- Tier 4: Startup Integration ---

func TestLoadFromFile_ValidPath(t *testing.T) {
	// UT-AF-039-025
	path := filepath.Join("testdata", "valid.yaml")
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
}

func TestLoadFromFile_MissingFile(t *testing.T) {
	// UT-AF-039-026
	path := filepath.Join(t.TempDir(), "nonexistent.yaml")
	_, err := os.ReadFile(filepath.Clean(path))
	if err == nil {
		t.Fatal("ReadFile() on missing file expected error, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent.yaml") {
		t.Errorf("error = %q, want to contain filename", err.Error())
	}
}

func TestLoadFromFile_ErrorIncludesPath(t *testing.T) {
	// UT-AF-039-027
	path := "/nonexistent/path/config.yaml"
	_, err := os.ReadFile(filepath.Clean(path))
	if err == nil {
		t.Fatal("ReadFile() expected error for missing path, got nil")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want to contain path %q", err.Error(), path)
	}
}

func TestLoadFromFile_InvalidContent(t *testing.T) {
	// UT-AF-039-028
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte(":::not yaml"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	_, err = Load(data)
	if err == nil {
		t.Fatal("Load() expected error for invalid YAML content, got nil")
	}
}

func TestLoadFromFile_PathCleaned(t *testing.T) {
	// UT-AF-039-029
	dir := t.TempDir()
	nested := filepath.Join(dir, "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	cfgPath := filepath.Join(nested, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("server:\n  port: 4444\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	// Use path with traversal that filepath.Clean resolves
	traversalPath := filepath.Join(dir, "sub", "..", "sub", "config.yaml")
	cleaned := filepath.Clean(traversalPath)
	data, err := os.ReadFile(cleaned)
	if err != nil {
		t.Fatalf("ReadFile(cleaned) error = %v", err)
	}
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != 4444 {
		t.Errorf("Server.Port = %d, want 4444 (via cleaned path)", cfg.Server.Port)
	}
}

func TestNoEnvVarsInCodebase(t *testing.T) {
	// UT-AF-039-030 + UT-AF-1251-001
	// Verifies the architectural constraint: no envOr/os.Getenv in production code.
	bannedFiles := map[string]string{
		filepath.Join("..", "..", "..", "cmd", "apifrontend", "main.go"): "main.go",
		"config.go": "config.go",
	}
	for path, label := range bannedFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("cannot read %s from test context: %v", label, err)
		}
		content := string(data)
		if strings.Contains(content, "os.Getenv") {
			t.Errorf("%s contains os.Getenv — env vars are banned per architectural constraint", label)
		}
		if strings.Contains(content, "envOr") {
			t.Errorf("%s contains envOr — env vars are banned per architectural constraint", label)
		}
	}
}

// --- Tier 5: Extended Config Fields (v2) ---

func TestLoad_AuthFields(t *testing.T) {
	// UT-AF-039-031
	data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  audience: "apifrontend"
`)
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Auth.IssuerURL != "https://sso.example.com/realms/kubernaut" {
		t.Errorf("Auth.IssuerURL = %q, want %q", cfg.Auth.IssuerURL, "https://sso.example.com/realms/kubernaut")
	}
	if cfg.Auth.Audience != "apifrontend" {
		t.Errorf("Auth.Audience = %q, want %q", cfg.Auth.Audience, "apifrontend")
	}
}

func TestLoad_AuthJWKSURL(t *testing.T) {
	data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  jwksURL: "https://sso.example.com/realms/kubernaut/protocol/openid-connect/certs"
  audience: "apifrontend"
`)
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Auth.JWKSURL != "https://sso.example.com/realms/kubernaut/protocol/openid-connect/certs" {
		t.Errorf("jwksURL = %q, want OIDC certs URL", cfg.Auth.JWKSURL)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestLoad_AuthJWKSURL_Invalid(t *testing.T) {
	data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  jwksURL: "not-a-url"
  audience: "apifrontend"
`)
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate should fail for malformed jwksURL")
	}
}

func TestLoad_AuthEnableReplayProtection(t *testing.T) {
	data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  audience: "apifrontend"
  enableReplayProtection: true
`)
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Auth.EnableReplayProtection {
		t.Error("enableReplayProtection should be true")
	}
}

func TestLoad_AuthAllowInsecureIssuers(t *testing.T) {
	// UT-AF-1247-001: allowInsecureIssuers field is parsed from YAML.
	data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  audience: "apifrontend"
  allowInsecureIssuers: true
`)
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Auth.AllowInsecureIssuers {
		t.Error("allowInsecureIssuers should be true")
	}
}

func TestLoad_AuthAllowInsecureIssuersDefaultsFalse(t *testing.T) {
	// UT-AF-1247-002: allowInsecureIssuers defaults to false when omitted.
	data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  audience: "apifrontend"
`)
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Auth.AllowInsecureIssuers {
		t.Error("allowInsecureIssuers should default to false")
	}
}

func TestLoad_AuthAllFieldsCombined(t *testing.T) {
	// UT-AF-1247-003: All three #1247 fields parsed together.
	data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  jwksURL: "https://sso.example.com/realms/kubernaut/protocol/openid-connect/certs"
  audience: "apifrontend"
  oidcCaFile: "/etc/apifrontend/ingress-ca/ca.crt"
  allowInsecureIssuers: false
`)
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Auth.JWKSURL != "https://sso.example.com/realms/kubernaut/protocol/openid-connect/certs" {
		t.Errorf("jwksURL = %q", cfg.Auth.JWKSURL)
	}
	if cfg.Auth.OIDCCaFile != "/etc/apifrontend/ingress-ca/ca.crt" {
		t.Errorf("oidcCaFile = %q", cfg.Auth.OIDCCaFile)
	}
	if cfg.Auth.AllowInsecureIssuers {
		t.Error("allowInsecureIssuers should be false")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestLoad_LoggingLevel(t *testing.T) {
	// UT-AF-039-032
	for _, level := range []string{"debug", "DEBUG", "info", "INFO", "warn", "WARN", "error", "ERROR"} {
		data := []byte("logging:\n  level: " + level + "\n")
		cfg, err := Load(data)
		if err != nil {
			t.Fatalf("Load(level=%s) error = %v", level, err)
		}
		if !strings.EqualFold(cfg.Logging.Level, level) {
			t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, level)
		}
	}
}

func TestLoad_RateLimitFields(t *testing.T) {
	// UT-AF-039-033
	data := []byte(`
rateLimit:
  ipRequestsPerSec: 200
  userRequestsPerSec: 75
`)
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.RateLimit.IPRequestsPerSec != 200 {
		t.Errorf("RateLimit.IPRequestsPerSec = %d, want 200", cfg.RateLimit.IPRequestsPerSec)
	}
	if cfg.RateLimit.UserRequestsPerSec != 75 {
		t.Errorf("RateLimit.UserRequestsPerSec = %d, want 75", cfg.RateLimit.UserRequestsPerSec)
	}
}

func TestLoad_ShutdownDrainSeconds(t *testing.T) {
	// UT-AF-039-034
	data := []byte("shutdown:\n  drainSeconds: 30\n")
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Shutdown.DrainSeconds != 30 {
		t.Errorf("Shutdown.DrainSeconds = %d, want 30", cfg.Shutdown.DrainSeconds)
	}
}

func TestDefaultConfig_ExtendedDefaults(t *testing.T) {
	// UT-AF-039-035
	cfg := DefaultConfig()
	if cfg.Logging.Level != "INFO" {
		t.Errorf("DefaultConfig().Logging.Level = %q, want %q", cfg.Logging.Level, "INFO")
	}
	if cfg.Shutdown.DrainSeconds != 15 {
		t.Errorf("DefaultConfig().Shutdown.DrainSeconds = %d, want 15", cfg.Shutdown.DrainSeconds)
	}
	if cfg.RateLimit.IPRequestsPerSec != 100 {
		t.Errorf("DefaultConfig().RateLimit.IPRequestsPerSec = %d, want 100", cfg.RateLimit.IPRequestsPerSec)
	}
	if cfg.RateLimit.UserRequestsPerSec != 50 {
		t.Errorf("DefaultConfig().RateLimit.UserRequestsPerSec = %d, want 50", cfg.RateLimit.UserRequestsPerSec)
	}
}

// --- TC-P2C-03: DefaultConfig field assertions (BAC-11) ---

func TestDefaultConfig_Port(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if cfg.Server.Port != 8443 {
		t.Errorf("TC-P2C-03a: DefaultConfig().Server.Port = %d, want 8443", cfg.Server.Port)
	}
}

func TestDefaultConfig_DrainSeconds(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if cfg.Shutdown.DrainSeconds != 15 {
		t.Errorf("TC-P2C-03b: DefaultConfig().Shutdown.DrainSeconds = %d, want 15", cfg.Shutdown.DrainSeconds)
	}
}

func TestDefaultConfig_ResilienceNonZero(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if cfg.Resilience.KA.CBFailureThreshold <= 0 {
		t.Errorf("TC-P2C-03c: KA CB threshold should be > 0, got %d", cfg.Resilience.KA.CBFailureThreshold)
	}
	if cfg.Resilience.DS.CBFailureThreshold <= 0 {
		t.Errorf("TC-P2C-03c: DS CB threshold should be > 0, got %d", cfg.Resilience.DS.CBFailureThreshold)
	}
	if cfg.Resilience.K8s.CBFailureThreshold <= 0 {
		t.Errorf("TC-P2C-03c: K8s CB threshold should be > 0, got %d", cfg.Resilience.K8s.CBFailureThreshold)
	}
}

// --- Tier 6: Extended Validation (v2) ---

func TestValidate_AuthIssuerURLNoScheme(t *testing.T) {
	// UT-AF-039-036
	cfg := validConfig()
	cfg.Auth.IssuerURL = "sso.example.com/realms/kubernaut"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for auth.issuerURL without scheme")
	}
	if !strings.Contains(err.Error(), "auth.issuerURL") {
		t.Errorf("error = %q, want to contain 'auth.issuerURL'", err.Error())
	}
}

func TestValidate_AuthEmptyIsOptional(t *testing.T) {
	// UT-AF-039-037
	cfg := validConfig()
	cfg.Auth.IssuerURL = ""
	cfg.Auth.Audience = ""
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for empty auth (optional in dev), got: %v", err)
	}
}

func TestValidate_InvalidLoggingLevel(t *testing.T) {
	// UT-AF-039-038
	cfg := validConfig()
	cfg.Logging.Level = "TRACE"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid logging.level")
	}
	if !strings.Contains(err.Error(), "logging.level") {
		t.Errorf("error = %q, want to contain 'logging.level'", err.Error())
	}
}

func TestValidate_ValidLoggingLevels(t *testing.T) {
	// UT-AF-039-039
	for _, level := range []string{"DEBUG", "INFO", "WARN", "ERROR", "debug", "info", "warn", "error"} {
		cfg := validConfig()
		cfg.Logging.Level = level
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate(level=%s) unexpected error: %v", level, err)
		}
	}
}

func TestValidate_ZeroIPRequestsPerSec(t *testing.T) {
	// UT-AF-039-040
	cfg := validConfig()
	cfg.RateLimit.IPRequestsPerSec = 0
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for zero ipRequestsPerSec")
	}
	if !strings.Contains(err.Error(), "rateLimit") {
		t.Errorf("error = %q, want to contain 'rateLimit'", err.Error())
	}
}

func TestValidate_NegativeUserRequestsPerSec(t *testing.T) {
	// UT-AF-039-041
	cfg := validConfig()
	cfg.RateLimit.UserRequestsPerSec = -1
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative userRequestsPerSec")
	}
	if !strings.Contains(err.Error(), "rateLimit") {
		t.Errorf("error = %q, want to contain 'rateLimit'", err.Error())
	}
}

func TestValidate_ZeroDrainSeconds(t *testing.T) {
	// UT-AF-039-042
	cfg := validConfig()
	cfg.Shutdown.DrainSeconds = 0
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for zero shutdown.drainSeconds")
	}
	if !strings.Contains(err.Error(), "shutdown.drainSeconds") {
		t.Errorf("error = %q, want to contain 'shutdown.drainSeconds'", err.Error())
	}
}

// --- Tier 7: Resilience Config (Issue #38) ---

func TestLoad_ResilienceConfig(t *testing.T) {
	// UT-AF-038-001
	data := []byte(`
resilience:
  ka:
    connectTimeout: 5s
    requestTimeout: 30s
    cbMaxRequests: 3
    cbInterval: 10s
    cbTimeout: 30s
    cbFailureThreshold: 5
    retryMax: 2
    retryInitBackoff: 500ms
    retryMaxBackoff: 5s
    retryableStatuses: [502, 503, 504]
  ds:
    connectTimeout: 3s
    requestTimeout: 10s
    cbMaxRequests: 3
    cbInterval: 10s
    cbTimeout: 15s
    cbFailureThreshold: 3
    retryMax: 3
    retryInitBackoff: 200ms
    retryMaxBackoff: 3s
    retryableStatuses: [502, 503, 504]
  k8s:
    connectTimeout: 5s
    requestTimeout: 30s
    cbMaxRequests: 3
    cbInterval: 10s
    cbTimeout: 30s
    cbFailureThreshold: 5
    retryMax: 0
    retryInitBackoff: 0s
    retryMaxBackoff: 0s
    retryableStatuses: []
`)
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Resilience.KA.ConnectTimeout.String() != "5s" {
		t.Errorf("Resilience.KA.ConnectTimeout = %v, want 5s", cfg.Resilience.KA.ConnectTimeout)
	}
	if cfg.Resilience.KA.RequestTimeout.String() != "30s" {
		t.Errorf("Resilience.KA.RequestTimeout = %v, want 30s", cfg.Resilience.KA.RequestTimeout)
	}
	if cfg.Resilience.KA.CBMaxRequests != 3 {
		t.Errorf("Resilience.KA.CBMaxRequests = %d, want 3", cfg.Resilience.KA.CBMaxRequests)
	}
	if cfg.Resilience.KA.CBFailureThreshold != 5 {
		t.Errorf("Resilience.KA.CBFailureThreshold = %d, want 5", cfg.Resilience.KA.CBFailureThreshold)
	}
	if cfg.Resilience.KA.RetryMax != 2 {
		t.Errorf("Resilience.KA.RetryMax = %d, want 2", cfg.Resilience.KA.RetryMax)
	}
	if cfg.Resilience.DS.ConnectTimeout.String() != "3s" {
		t.Errorf("Resilience.DS.ConnectTimeout = %v, want 3s", cfg.Resilience.DS.ConnectTimeout)
	}
	if cfg.Resilience.DS.CBFailureThreshold != 3 {
		t.Errorf("Resilience.DS.CBFailureThreshold = %d, want 3", cfg.Resilience.DS.CBFailureThreshold)
	}
	if cfg.Resilience.DS.RetryMax != 3 {
		t.Errorf("Resilience.DS.RetryMax = %d, want 3", cfg.Resilience.DS.RetryMax)
	}
	if len(cfg.Resilience.KA.RetryableStatuses) != 3 {
		t.Errorf("Resilience.KA.RetryableStatuses len = %d, want 3", len(cfg.Resilience.KA.RetryableStatuses))
	}
}

func TestValidate_ResilienceNegativeConnectTimeout(t *testing.T) {
	// UT-AF-038-002
	cfg := validConfig()
	cfg.Resilience.KA.ConnectTimeout = -1
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative connectTimeout")
	}
	if !strings.Contains(err.Error(), "connectTimeout") {
		t.Errorf("error = %q, want to contain 'connectTimeout'", err.Error())
	}
}

func TestValidate_ResilienceNegativeRequestTimeout(t *testing.T) {
	// UT-AF-038-003
	cfg := validConfig()
	cfg.Resilience.DS.RequestTimeout = -1
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative requestTimeout")
	}
	if !strings.Contains(err.Error(), "requestTimeout") {
		t.Errorf("error = %q, want to contain 'requestTimeout'", err.Error())
	}
}

func TestValidate_ResilienceCBFailureThresholdTooHigh(t *testing.T) {
	// UT-AF-038-004
	cfg := validConfig()
	cfg.Resilience.KA.CBFailureThreshold = 101
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for cbFailureThreshold > 100")
	}
	if !strings.Contains(err.Error(), "cbFailureThreshold") {
		t.Errorf("error = %q, want to contain 'cbFailureThreshold'", err.Error())
	}
}

func TestValidate_ResilienceCBFailureThresholdZero(t *testing.T) {
	// P3 SEC-2: 0 should be rejected (would open CB on first failure)
	cfg := validConfig()
	cfg.Resilience.KA.CBFailureThreshold = 0
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for cbFailureThreshold == 0")
	}
	if !strings.Contains(err.Error(), "cbFailureThreshold") {
		t.Errorf("error = %q, want to contain 'cbFailureThreshold'", err.Error())
	}
}

func TestValidate_ResilienceRetryMaxTooHigh(t *testing.T) {
	// UT-AF-038-005
	cfg := validConfig()
	cfg.Resilience.DS.RetryMax = 11
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for retryMax > 10")
	}
	if !strings.Contains(err.Error(), "retryMax") {
		t.Errorf("error = %q, want to contain 'retryMax'", err.Error())
	}
}

func TestValidate_ResilienceRetryableStatusesOutOfRange(t *testing.T) {
	// UT-AF-038-006
	cfg := validConfig()
	cfg.Resilience.KA.RetryableStatuses = []int{200, 503}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for retryableStatuses containing 200")
	}
	if !strings.Contains(err.Error(), "retryableStatuses") {
		t.Errorf("error = %q, want to contain 'retryableStatuses'", err.Error())
	}
}

func TestLoad_ResilienceDefaultsWhenOmitted(t *testing.T) {
	// UT-AF-038-007
	data := []byte("server:\n  port: 8443\n")
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	// When resilience section is omitted, defaults should apply
	defaults := DefaultConfig()
	if cfg.Resilience.KA.RequestTimeout != defaults.Resilience.KA.RequestTimeout {
		t.Errorf("Resilience.KA.RequestTimeout = %v, want default %v",
			cfg.Resilience.KA.RequestTimeout, defaults.Resilience.KA.RequestTimeout)
	}
	if cfg.Resilience.DS.CBFailureThreshold != defaults.Resilience.DS.CBFailureThreshold {
		t.Errorf("Resilience.DS.CBFailureThreshold = %d, want default %d",
			cfg.Resilience.DS.CBFailureThreshold, defaults.Resilience.DS.CBFailureThreshold)
	}
}

func TestValidate_ResilienceRequestTimeoutLessThanConnect(t *testing.T) {
	// UT-AF-038-008
	cfg := validConfig()
	cfg.Resilience.KA.ConnectTimeout = 10000000000 // 10s
	cfg.Resilience.KA.RequestTimeout = 5000000000  // 5s (less than connect)
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when requestTimeout < connectTimeout")
	}
	if !strings.Contains(err.Error(), "requestTimeout") {
		t.Errorf("error = %q, want to contain 'requestTimeout'", err.Error())
	}
}

func TestValidate_SeverityTriageEnabledRequiresPrometheusURL(t *testing.T) {
	cfg := validConfig()
	cfg.SeverityTriage.Enabled = true
	cfg.SeverityTriage.PrometheusURL = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when triage enabled without PrometheusURL")
	}
	if !strings.Contains(err.Error(), "prometheusURL") {
		t.Errorf("error = %q, want to contain 'prometheusURL'", err.Error())
	}
}

func TestValidate_SeverityTriageDisabledSkipsValidation(t *testing.T) {
	cfg := validConfig()
	cfg.SeverityTriage.Enabled = false
	cfg.SeverityTriage.PrometheusURL = ""
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_SeverityTriageLLMConfidenceOutOfRange(t *testing.T) {
	cfg := validConfig()
	cfg.SeverityTriage.Enabled = true
	cfg.SeverityTriage.PrometheusURL = "http://prometheus:9090"
	cfg.SeverityTriage.LLMConfidence = 1.5
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when LLMConfidence > 1.0")
	}
	if !strings.Contains(err.Error(), "llmConfidence") {
		t.Errorf("error = %q, want to contain 'llmConfidence'", err.Error())
	}
}

func TestValidate_SeverityTriageValidConfig(t *testing.T) {
	cfg := validConfig()
	cfg.SeverityTriage.Enabled = true
	cfg.SeverityTriage.PrometheusURL = "http://prometheus:9090"
	cfg.SeverityTriage.LLMConfidence = 0.7
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// UseOIDCDirect tests removed — OIDC-direct mode deprecated per ADR-022.

func TestValidate_SessionNamespaceRequiredWhenTTLsSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		namespace string
		disconn   time.Duration
		retention time.Duration
		wantErr   bool
	}{
		{
			name:      "empty namespace with disconnectTTL rejects",
			namespace: "",
			disconn:   5 * time.Minute,
			wantErr:   true,
		},
		{
			name:      "empty namespace with retentionTTL rejects",
			namespace: "",
			retention: 720 * time.Hour,
			wantErr:   true,
		},
		{
			name:      "empty namespace with both TTLs rejects",
			namespace: "",
			disconn:   10 * time.Minute,
			retention: 720 * time.Hour,
			wantErr:   true,
		},
		{
			name:      "set namespace with TTLs passes",
			namespace: "kubernaut-system",
			disconn:   10 * time.Minute,
			retention: 720 * time.Hour,
			wantErr:   false,
		},
		{
			name:      "empty namespace with zero TTLs passes",
			namespace: "",
			disconn:   0,
			retention: 0,
			wantErr:   false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := validConfig()
			cfg.Session.Namespace = tc.namespace
			cfg.Session.DisconnectTTL = tc.disconn
			cfg.Session.RetentionTTL = tc.retention
			err := cfg.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr && err != nil && !strings.Contains(err.Error(), "session.namespace") {
				t.Errorf("error = %q, want to contain 'session.namespace'", err.Error())
			}
		})
	}
}

// --- Tier 8: OIDC CA File (Issue #1245) ---

func TestLoad_AuthOIDCCaFile(t *testing.T) {
	// UT-AF-1245-010: oidcCaFile field is parsed from YAML.
	data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  audience: "apifrontend"
  oidcCaFile: "/etc/apifrontend/ingress-ca/ca.crt"
`)
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Auth.OIDCCaFile != "/etc/apifrontend/ingress-ca/ca.crt" {
		t.Errorf("Auth.OIDCCaFile = %q, want %q", cfg.Auth.OIDCCaFile, "/etc/apifrontend/ingress-ca/ca.crt")
	}
}

func TestLoad_AuthOIDCCaFileOmitted(t *testing.T) {
	// UT-AF-1245-011: oidcCaFile defaults to empty when omitted.
	data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  audience: "apifrontend"
`)
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Auth.OIDCCaFile != "" {
		t.Errorf("Auth.OIDCCaFile = %q, want empty", cfg.Auth.OIDCCaFile)
	}
}

func TestValidate_AuthOIDCCaFileRelativePath(t *testing.T) {
	// UT-AF-1245-012: Relative oidcCaFile path is rejected.
	cfg := validConfig()
	cfg.Auth.IssuerURL = "https://sso.example.com/realms/kubernaut"
	cfg.Auth.OIDCCaFile = "relative/path/ca.crt"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for relative oidcCaFile path")
	}
	if !strings.Contains(err.Error(), "oidcCaFile") {
		t.Errorf("error = %q, want to contain 'oidcCaFile'", err.Error())
	}
}

func TestValidate_AuthOIDCCaFileAbsolutePath(t *testing.T) {
	// UT-AF-1245-013: Absolute oidcCaFile path passes validation.
	cfg := validConfig()
	cfg.Auth.IssuerURL = "https://sso.example.com/realms/kubernaut"
	cfg.Auth.OIDCCaFile = "/etc/apifrontend/ingress-ca/ca.crt"
	err := cfg.Validate()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_AuthOIDCCaFileEmptyIsOptional(t *testing.T) {
	// UT-AF-1245-014: Empty oidcCaFile is valid (use system CAs).
	cfg := validConfig()
	cfg.Auth.IssuerURL = "https://sso.example.com/realms/kubernaut"
	cfg.Auth.OIDCCaFile = ""
	err := cfg.Validate()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Tier 9: LLM Config Validation (Issue #1252) ---

func TestValidate_LLMEmptyProviderIsOptional(t *testing.T) {
	cfg := validConfig()
	cfg.Agent.LLM.Provider = ""
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for empty LLM provider (A2A disabled), got: %v", err)
	}
}

func TestValidate_LLMUnknownProviderRejected(t *testing.T) {
	cfg := validConfig()
	cfg.Agent.LLM.Provider = "openai"
	cfg.Agent.LLM.Model = "gpt-4"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "agent.llm.provider") {
		t.Errorf("error = %q, want to contain 'agent.llm.provider'", err.Error())
	}
}

func TestValidate_LLMModelRequiredWhenProviderSet(t *testing.T) {
	cfg := validConfig()
	cfg.Agent.LLM.Provider = LLMProviderGemini
	cfg.Agent.LLM.Model = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty model with provider set")
	}
	if !strings.Contains(err.Error(), "agent.llm.model") {
		t.Errorf("error = %q, want to contain 'agent.llm.model'", err.Error())
	}
}

func TestValidate_LLMVertexAIRequiresProject(t *testing.T) {
	cfg := validConfig()
	cfg.Agent.LLM.Provider = LLMProviderVertexAI
	cfg.Agent.LLM.Model = "claude-sonnet-4-20250514"
	cfg.Agent.LLM.VertexProject = ""
	cfg.Agent.LLM.VertexLocation = "us-central1"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for vertex_ai without project")
	}
	if !strings.Contains(err.Error(), "vertexProject") {
		t.Errorf("error = %q, want to contain 'vertexProject'", err.Error())
	}
}

func TestValidate_LLMVertexAIRequiresLocation(t *testing.T) {
	cfg := validConfig()
	cfg.Agent.LLM.Provider = LLMProviderVertexAI
	cfg.Agent.LLM.Model = "claude-sonnet-4-20250514"
	cfg.Agent.LLM.VertexProject = "my-project"
	cfg.Agent.LLM.VertexLocation = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for vertex_ai without location")
	}
	if !strings.Contains(err.Error(), "vertexLocation") {
		t.Errorf("error = %q, want to contain 'vertexLocation'", err.Error())
	}
}

func TestValidate_LLMGeminiAccepted(t *testing.T) {
	cfg := validConfig()
	cfg.Agent.LLM.Provider = LLMProviderGemini
	cfg.Agent.LLM.Model = "gemini-2.0-flash"
	cfg.Agent.LLM.APIKeyFile = "/etc/secrets/llm-key"
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_LLMAnthropicAccepted(t *testing.T) {
	cfg := validConfig()
	cfg.Agent.LLM.Provider = LLMProviderAnthropic
	cfg.Agent.LLM.Model = "claude-sonnet-4-20250514"
	cfg.Agent.LLM.APIKeyFile = "/etc/secrets/llm-key"
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_LLMGeminiRequiresCredentials(t *testing.T) {
	cfg := validConfig()
	cfg.Agent.LLM.Provider = LLMProviderGemini
	cfg.Agent.LLM.Model = "gemini-2.0-flash"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for gemini without apiKeyFile or oauth2")
	}
	if !strings.Contains(err.Error(), "apiKeyFile") {
		t.Errorf("error = %q, want to contain 'apiKeyFile'", err.Error())
	}
}

func TestValidate_LLMAPIKeyFileRelativeRejected(t *testing.T) {
	cfg := validConfig()
	cfg.Agent.LLM.Provider = LLMProviderGemini
	cfg.Agent.LLM.Model = "gemini-2.0-flash"
	cfg.Agent.LLM.APIKeyFile = "relative/path/key"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for relative apiKeyFile")
	}
	if !strings.Contains(err.Error(), "apiKeyFile") {
		t.Errorf("error = %q, want to contain 'apiKeyFile'", err.Error())
	}
}

func TestValidate_LLMOAuth2RequiresTokenURL(t *testing.T) {
	cfg := validConfig()
	cfg.Agent.LLM.Provider = LLMProviderGemini
	cfg.Agent.LLM.Model = "gemini-2.0-flash"
	cfg.Agent.LLM.OAuth2.Enabled = true
	cfg.Agent.LLM.OAuth2.TokenURL = ""
	cfg.Agent.LLM.OAuth2.CredentialsDir = "/etc/creds"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for oauth2 without tokenURL")
	}
	if !strings.Contains(err.Error(), "tokenURL") {
		t.Errorf("error = %q, want to contain 'tokenURL'", err.Error())
	}
}

func TestValidate_LLMOAuth2RequiresCredentialsDir(t *testing.T) {
	cfg := validConfig()
	cfg.Agent.LLM.Provider = LLMProviderGemini
	cfg.Agent.LLM.Model = "gemini-2.0-flash"
	cfg.Agent.LLM.OAuth2.Enabled = true
	cfg.Agent.LLM.OAuth2.TokenURL = "https://auth.example.com/token"
	cfg.Agent.LLM.OAuth2.CredentialsDir = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for oauth2 without credentialsDir")
	}
	if !strings.Contains(err.Error(), "credentialsDir") {
		t.Errorf("error = %q, want to contain 'credentialsDir'", err.Error())
	}
}

func TestResolveDefaults_LLMApiKeyFromFile(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "llm-api-key")
	if err := os.WriteFile(keyFile, []byte("  secret-key-123  \n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := validConfig()
	cfg.Agent.LLM.APIKeyFile = keyFile
	if err := cfg.ResolveDefaults(); err != nil {
		t.Fatalf("ResolveDefaults() = %v", err)
	}
	if cfg.Agent.LLM.APIKey != "secret-key-123" {
		t.Errorf("APIKey = %q, want %q", cfg.Agent.LLM.APIKey, "secret-key-123")
	}
}

func TestResolveDefaults_LLMAPIKeyFileNotFound(t *testing.T) {
	cfg := validConfig()
	cfg.Agent.LLM.APIKeyFile = "/nonexistent/path/key"
	err := cfg.ResolveDefaults()
	if err == nil {
		t.Fatalf("ResolveDefaults() = nil, want error for missing apiKeyFile")
	}
	if !strings.Contains(err.Error(), "/nonexistent/path/key") {
		t.Errorf("error should reference path, got: %v", err)
	}
}

func TestResolveDefaults_LLMAPIKeyFileEmpty(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "llm-api-key")
	if err := os.WriteFile(keyFile, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := validConfig()
	cfg.Agent.LLM.APIKeyFile = keyFile
	err := cfg.ResolveDefaults()
	if err == nil {
		t.Fatalf("ResolveDefaults() = nil, want error for empty key file")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention 'empty', got: %v", err)
	}
}

func TestLoad_LLMConfigFromYAML(t *testing.T) {
	data := []byte(`
agent:
  llm:
    provider: anthropic
    model: claude-sonnet-4-20250514
    endpoint: "https://anthropic.example.com"
    apiKeyFile: "/etc/secrets/anthropic-key"
    tlsCaFile: "/etc/ca/custom.pem"
    oauth2:
      enabled: true
      tokenURL: "https://auth.example.com/token"
      scopes: ["llm:invoke"]
      credentialsDir: "/etc/oauth2"
    circuitBreaker:
      enabled: true
      maxRequests: 5
      interval: 15s
      timeout: 45s
      failureThreshold: 3
    customHeaders:
      - name: X-Tenant-ID
        value: "kubernaut"
`)
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Agent.LLM.Provider != LLMProviderAnthropic {
		t.Errorf("Provider = %q, want %q", cfg.Agent.LLM.Provider, LLMProviderAnthropic)
	}
	if cfg.Agent.LLM.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q", cfg.Agent.LLM.Model)
	}
	if cfg.Agent.LLM.Endpoint != "https://anthropic.example.com" {
		t.Errorf("Endpoint = %q", cfg.Agent.LLM.Endpoint)
	}
	if !cfg.Agent.LLM.OAuth2.Enabled {
		t.Error("OAuth2.Enabled should be true")
	}
	if cfg.Agent.LLM.OAuth2.TokenURL != "https://auth.example.com/token" {
		t.Errorf("OAuth2.TokenURL = %q", cfg.Agent.LLM.OAuth2.TokenURL)
	}
	if len(cfg.Agent.LLM.OAuth2.Scopes) != 1 || cfg.Agent.LLM.OAuth2.Scopes[0] != "llm:invoke" {
		t.Errorf("OAuth2.Scopes = %v", cfg.Agent.LLM.OAuth2.Scopes)
	}
	if !cfg.Agent.LLM.CircuitBreaker.Enabled {
		t.Error("CircuitBreaker.Enabled should be true")
	}
	if cfg.Agent.LLM.CircuitBreaker.MaxRequests != 5 {
		t.Errorf("CircuitBreaker.MaxRequests = %d", cfg.Agent.LLM.CircuitBreaker.MaxRequests)
	}
	if len(cfg.Agent.LLM.CustomHeaders) != 1 {
		t.Fatalf("CustomHeaders len = %d, want 1", len(cfg.Agent.LLM.CustomHeaders))
	}
	if cfg.Agent.LLM.CustomHeaders[0].Name != "X-Tenant-ID" {
		t.Errorf("CustomHeaders[0].Name = %q", cfg.Agent.LLM.CustomHeaders[0].Name)
	}
}
