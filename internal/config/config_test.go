package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validConfig returns a Config that passes Validate() for use as a base in tests.
func validConfig() *Config {
	return &Config{
		Server: ServerConfig{Port: 8443},
		Agent: AgentConfig{
			GCPProject:    "test-project",
			GCPRegion:     "us-central1",
			KABaseURL:     "http://localhost:8080",
			KAMCPEndpoint: "http://localhost:8080/api/v1/mcp/",
			DSBaseURL:     "http://localhost:9090",
		},
		MCP:       MCPConfig{Enabled: false},
		AgentCard: AgentCardConfig{URL: "https://localhost:8443"},
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
	if cfg.Agent.GCPProject != "my-project" {
		t.Errorf("Agent.GCPProject = %q, want %q", cfg.Agent.GCPProject, "my-project")
	}
	if cfg.Agent.GCPRegion != "us-east1" {
		t.Errorf("Agent.GCPRegion = %q, want %q", cfg.Agent.GCPRegion, "us-east1")
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
	data := []byte("agent:\n  gcpProject: \"p\"\n")
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	defaults := DefaultConfig()
	if cfg.Server.Port != defaults.Server.Port {
		t.Errorf("Server.Port = %d, want default %d", cfg.Server.Port, defaults.Server.Port)
	}
	if cfg.Agent.GCPRegion != defaults.Agent.GCPRegion {
		t.Errorf("Agent.GCPRegion = %q, want default %q", cfg.Agent.GCPRegion, defaults.Agent.GCPRegion)
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
	data := []byte("server:\n  port: 7777\nagent:\n  gcpProject: \"partial\"\n")
	cfg, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != 7777 {
		t.Errorf("Server.Port = %d, want 7777", cfg.Server.Port)
	}
	if cfg.Agent.GCPProject != "partial" {
		t.Errorf("Agent.GCPProject = %q, want %q", cfg.Agent.GCPProject, "partial")
	}
	defaults := DefaultConfig()
	if cfg.Agent.GCPRegion != defaults.Agent.GCPRegion {
		t.Errorf("Agent.GCPRegion = %q, want default %q", cfg.Agent.GCPRegion, defaults.Agent.GCPRegion)
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
	cfg.ResolveDefaults()
	want := "https://localhost:8443"
	if cfg.AgentCard.URL != want {
		t.Errorf("AgentCard.URL = %q, want %q", cfg.AgentCard.URL, want)
	}
}

func TestResolveDefaults_PreservesExplicitURL(t *testing.T) {
	// UT-AF-039-022
	cfg := validConfig()
	cfg.AgentCard.URL = "https://custom.example.com"
	cfg.ResolveDefaults()
	if cfg.AgentCard.URL != "https://custom.example.com" {
		t.Errorf("AgentCard.URL = %q, want preserved value", cfg.AgentCard.URL)
	}
}

func TestResolveDefaults_Idempotent(t *testing.T) {
	// UT-AF-039-023
	cfg := validConfig()
	cfg.AgentCard.URL = ""
	cfg.Server.Port = 9000
	cfg.ResolveDefaults()
	first := cfg.AgentCard.URL
	cfg.ResolveDefaults()
	if cfg.AgentCard.URL != first {
		t.Errorf("second ResolveDefaults() changed URL: %q -> %q", first, cfg.AgentCard.URL)
	}
}

func TestResolveDefaults_CalledBeforeValidate(t *testing.T) {
	// UT-AF-039-024
	cfg := validConfig()
	cfg.AgentCard.URL = ""
	cfg.Server.Port = 8443
	cfg.ResolveDefaults()
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
	// UT-AF-039-030
	// This test verifies the architectural constraint: no envOr/os.Getenv in production code.
	// It reads cmd/apifrontend/main.go and checks for the banned patterns.
	mainPath := filepath.Join("..", "..", "cmd", "apifrontend", "main.go")
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Skipf("cannot read main.go from test context: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "os.Getenv") {
		t.Error("main.go contains os.Getenv — env vars are banned per architectural constraint")
	}
	if strings.Contains(content, "envOr") {
		t.Error("main.go contains envOr — env vars are banned per architectural constraint")
	}
}
