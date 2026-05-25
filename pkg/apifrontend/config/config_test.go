package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
)

// validConfig returns a Config that passes Validate() for use as a base in tests.
func validConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{Port: 8443},
		Agent: config.AgentConfig{
			KABaseURL:     "http://localhost:8080",
			KAMCPEndpoint: "http://localhost:8080/api/v1/mcp/",
			DSBaseURL:     "http://localhost:9090",
		},
		MCP:       config.MCPConfig{Enabled: false},
		AgentCard: config.AgentCardConfig{URL: "https://localhost:8443"},
		Logging:   config.LoggingConfig{Level: "INFO"},
		RateLimit: config.RateLimitConfig{IPRequestsPerSec: 100, UserRequestsPerSec: 50},
		Shutdown:  config.ShutdownConfig{DrainSeconds: 15},
		Resilience: config.ResilienceConfig{
			KA:  config.DependencyConfig{CBFailureThreshold: 5},
			DS:  config.DependencyConfig{CBFailureThreshold: 3},
			K8s: config.DependencyConfig{CBFailureThreshold: 5},
		},
	}
}

var _ = Describe("Tier 1: Config Loading — Load()", func() {
	It("UT-AF-039-001 loads valid full YAML", func() {
		data, err := os.ReadFile("testdata/valid.yaml")
		Expect(err).NotTo(HaveOccurred(), "read fixture")

		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Server.Port).To(Equal(9090))
		Expect(cfg.Agent.LLM.VertexProject).To(Equal("my-project"))
		Expect(cfg.Agent.LLM.VertexLocation).To(Equal("us-east1"))
		Expect(cfg.Agent.KABaseURL).To(Equal("https://ka.example.com"))
		Expect(cfg.Agent.KAMCPEndpoint).To(Equal("https://ka.example.com/api/v1/mcp/"))
		Expect(cfg.Agent.DSBaseURL).To(Equal("https://ds.example.com"))
		Expect(cfg.MCP.Enabled).To(BeTrue())
		Expect(cfg.AgentCard.URL).To(Equal("https://af.example.com"))
	})

	It("UT-AF-039-002 applies defaults for omitted fields", func() {
		data := []byte("agent:\n  llm:\n    vertexProject: \"p\"\n")
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())

		defaults := config.DefaultConfig()
		Expect(cfg.Server.Port).To(Equal(defaults.Server.Port))
		Expect(cfg.Agent.LLM.VertexLocation).To(Equal(defaults.Agent.LLM.VertexLocation))
	})

	It("UT-AF-039-003 rejects malformed YAML", func() {
		data := []byte("server:\n  port: [invalid")
		_, err := config.Load(data)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("pars"))
	})

	It("UT-AF-039-004 accepts empty input with defaults", func() {
		cfg, err := config.Load([]byte(""))
		Expect(err).NotTo(HaveOccurred())

		defaults := config.DefaultConfig()
		Expect(cfg.Server.Port).To(Equal(defaults.Server.Port))
	})

	It("UT-AF-039-005 ignores unknown keys", func() {
		data := []byte("server:\n  port: 9090\nunknownField: \"should be ignored\"\n")
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Server.Port).To(Equal(9090))
	})

	It("UT-AF-039-006 preserves zero booleans", func() {
		data := []byte("mcp:\n  enabled: false\n")
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.MCP.Enabled).To(BeFalse())
	})

	It("UT-AF-039-007 loads port as integer", func() {
		data := []byte("server:\n  port: 3000\n")
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Server.Port).To(Equal(3000))
	})

	It("UT-AF-039-008 merges partial YAML with defaults", func() {
		data := []byte("server:\n  port: 7777\nagent:\n  llm:\n    vertexProject: \"partial\"\n")
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Server.Port).To(Equal(7777))
		Expect(cfg.Agent.LLM.VertexProject).To(Equal("partial"))

		defaults := config.DefaultConfig()
		Expect(cfg.Agent.LLM.VertexLocation).To(Equal(defaults.Agent.LLM.VertexLocation))
	})
})

var _ = Describe("Tier 2: Config Validation — Validate()", func() {
	It("UT-AF-039-009 accepts DefaultConfig", func() {
		cfg := config.DefaultConfig()
		Expect(cfg.Validate()).To(Succeed())
	})

	DescribeTable("UT-AF-039-010, 011, 012 port range validation",
		func(port int, wantErr bool) {
			cfg := validConfig()
			cfg.Server.Port = port
			err := cfg.Validate()
			if wantErr {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("server.port"))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("port < 1", -1, true),
		Entry("port = 0", 0, true),
		Entry("port > 65535", 70000, true),
		Entry("port = 1 (privileged)", 1, true),
		Entry("port = 80 (privileged)", 80, true),
		Entry("port = 1023 (privileged)", 1023, true),
		Entry("port = 1024 (min valid)", 1024, false),
		Entry("port = 65535 (max valid)", 65535, false),
		Entry("port = 8443 (typical)", 8443, false),
	)

	DescribeTable("UT-AF-039-013, 015, 016 required URL fields",
		func(mutate func(*config.Config), wantSub string) {
			cfg := validConfig()
			mutate(cfg)
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(wantSub))
		},
		Entry("empty kaBaseURL", func(c *config.Config) { c.Agent.KABaseURL = "" }, "kaBaseURL"),
		Entry("empty kaMCPEndpoint", func(c *config.Config) { c.Agent.KAMCPEndpoint = "" }, "kaMCPEndpoint"),
		Entry("empty dsBaseURL", func(c *config.Config) { c.Agent.DSBaseURL = "" }, "dsBaseURL"),
	)

	DescribeTable("UT-AF-039-014, 017 malformed URLs",
		func(mutate func(*config.Config), wantSub string) {
			cfg := validConfig()
			mutate(cfg)
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(wantSub))
		},
		Entry("kaBaseURL no scheme", func(c *config.Config) { c.Agent.KABaseURL = "not-a-url" }, "kaBaseURL"),
		Entry("dsBaseURL no scheme", func(c *config.Config) { c.Agent.DSBaseURL = "://bad" }, "dsBaseURL"),
	)

	It("UT-AF-039-018 accepts valid complete config", func() {
		cfg := validConfig()
		Expect(cfg.Validate()).To(Succeed())
	})

	It("UT-AF-039-019 error message includes field name", func() {
		cfg := validConfig()
		cfg.Server.Port = -1
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("server.port"))
	})

	It("UT-AF-039-020 returns first error only", func() {
		cfg := validConfig()
		cfg.Server.Port = -1
		cfg.Agent.KABaseURL = ""
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		errStr := err.Error()
		Expect(strings.Contains(errStr, "server.port") && strings.Contains(errStr, "kaBaseURL")).To(BeFalse())
	})
})

var _ = Describe("Tier 3: Default Resolution — ResolveDefaults()", func() {
	It("UT-AF-039-021 sets AgentCard URL from port", func() {
		cfg := validConfig()
		cfg.AgentCard.URL = ""
		cfg.Server.Port = 8443
		Expect(cfg.ResolveDefaults()).To(Succeed())
		Expect(cfg.AgentCard.URL).To(Equal("https://localhost:8443"))
	})

	It("UT-AF-039-022 preserves explicit URL", func() {
		cfg := validConfig()
		cfg.AgentCard.URL = "https://custom.example.com"
		Expect(cfg.ResolveDefaults()).To(Succeed())
		Expect(cfg.AgentCard.URL).To(Equal("https://custom.example.com"))
	})

	It("UT-AF-039-023 is idempotent", func() {
		cfg := validConfig()
		cfg.AgentCard.URL = ""
		cfg.Server.Port = 9000
		Expect(cfg.ResolveDefaults()).To(Succeed())
		first := cfg.AgentCard.URL
		Expect(cfg.ResolveDefaults()).To(Succeed())
		Expect(cfg.AgentCard.URL).To(Equal(first))
	})

	It("UT-AF-039-024 resolves defaults before validate", func() {
		cfg := validConfig()
		cfg.AgentCard.URL = ""
		cfg.Server.Port = 8443
		Expect(cfg.ResolveDefaults()).To(Succeed())
		Expect(cfg.Validate()).To(Succeed())
	})
})

var _ = Describe("Tier 4: Startup Integration", func() {
	It("UT-AF-039-025 loads from valid file path", func() {
		path := filepath.Join("testdata", "valid.yaml")
		data, err := os.ReadFile(filepath.Clean(path))
		Expect(err).NotTo(HaveOccurred())

		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Server.Port).To(Equal(9090))
	})

	It("UT-AF-039-026 reports error for missing file", func() {
		path := filepath.Join(GinkgoT().TempDir(), "nonexistent.yaml")
		_, err := os.ReadFile(filepath.Clean(path))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("nonexistent.yaml"))
	})

	It("UT-AF-039-027 error includes path", func() {
		path := "/nonexistent/path/config.yaml"
		_, err := os.ReadFile(filepath.Clean(path))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(path))
	})

	It("UT-AF-039-028 rejects invalid file content", func() {
		dir := GinkgoT().TempDir()
		path := filepath.Join(dir, "bad.yaml")
		Expect(os.WriteFile(path, []byte(":::not yaml"), 0o644)).To(Succeed())

		data, err := os.ReadFile(filepath.Clean(path))
		Expect(err).NotTo(HaveOccurred())

		_, err = config.Load(data)
		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-039-029 loads via cleaned path", func() {
		dir := GinkgoT().TempDir()
		nested := filepath.Join(dir, "sub")
		Expect(os.MkdirAll(nested, 0o755)).To(Succeed())

		cfgPath := filepath.Join(nested, "config.yaml")
		Expect(os.WriteFile(cfgPath, []byte("server:\n  port: 4444\n"), 0o644)).To(Succeed())

		traversalPath := filepath.Join(dir, "sub", "..", "sub", "config.yaml")
		cleaned := filepath.Clean(traversalPath)
		data, err := os.ReadFile(cleaned)
		Expect(err).NotTo(HaveOccurred())

		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Server.Port).To(Equal(4444))
	})

	It("UT-AF-039-030 + UT-AF-1251-001 forbids os.Getenv in production code", func() {
		bannedFiles := map[string]string{
			filepath.Join("..", "..", "..", "cmd", "apifrontend", "main.go"): "main.go",
			"config.go": "config.go",
		}

		allowedGetenvCounts := map[string]int{
			"config.go": 1,
			"main.go":   1,
		}

		for path, label := range bannedFiles {
			data, err := os.ReadFile(path)
			if err != nil {
				Skip("cannot read " + label + " from test context: " + err.Error())
			}
			content := string(data)
			count := strings.Count(content, "os.Getenv")
			allowed := allowedGetenvCounts[label]
			Expect(count).To(BeNumerically("<=", allowed),
				"%s contains %d os.Getenv calls (allowed %d) — env vars are banned per architectural constraint (exception: BR-PLATFORM-1262 PORT override)",
				label, count, allowed)
			Expect(content).NotTo(ContainSubstring("envOr"),
				"%s contains envOr — env vars are banned per architectural constraint", label)
		}
	})
})

var _ = Describe("Tier 5: Extended Config Fields (v2)", func() {
	It("UT-AF-039-031 loads auth fields", func() {
		data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  audience: "apifrontend"
`)
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Auth.IssuerURL).To(Equal("https://sso.example.com/realms/kubernaut"))
		Expect(cfg.Auth.Audience).To(Equal("apifrontend"))
	})

	It("loads auth JWKS URL", func() {
		data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  jwksURL: "https://sso.example.com/realms/kubernaut/protocol/openid-connect/certs"
  audience: "apifrontend"
`)
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Auth.JWKSURL).To(Equal("https://sso.example.com/realms/kubernaut/protocol/openid-connect/certs"))
		Expect(cfg.Validate()).To(Succeed())
	})

	It("rejects invalid auth JWKS URL on validate", func() {
		data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  jwksURL: "not-a-url"
  audience: "apifrontend"
`)
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Validate()).NotTo(Succeed())
	})

	It("loads auth enableReplayProtection", func() {
		data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  audience: "apifrontend"
  enableReplayProtection: true
`)
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Auth.EnableReplayProtection).To(BeTrue())
	})

	It("UT-AF-1247-001 loads allowInsecureIssuers field", func() {
		data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  audience: "apifrontend"
  allowInsecureIssuers: true
`)
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Auth.AllowInsecureIssuers).To(BeTrue())
	})

	It("UT-AF-1247-002 defaults allowInsecureIssuers to false", func() {
		data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  audience: "apifrontend"
`)
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Auth.AllowInsecureIssuers).To(BeFalse())
	})

	It("UT-AF-1247-003 loads all #1247 auth fields together", func() {
		data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  jwksURL: "https://sso.example.com/realms/kubernaut/protocol/openid-connect/certs"
  audience: "apifrontend"
  oidcCaFile: "/etc/apifrontend/ingress-ca/ca.crt"
  allowInsecureIssuers: false
`)
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Auth.JWKSURL).To(Equal("https://sso.example.com/realms/kubernaut/protocol/openid-connect/certs"))
		Expect(cfg.Auth.OIDCCaFile).To(Equal("/etc/apifrontend/ingress-ca/ca.crt"))
		Expect(cfg.Auth.AllowInsecureIssuers).To(BeFalse())
		Expect(cfg.Validate()).To(Succeed())
	})

	DescribeTable("UT-AF-039-032 loads logging level",
		func(level string) {
			data := []byte("logging:\n  level: " + level + "\n")
			cfg, err := config.Load(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.EqualFold(cfg.Logging.Level, level)).To(BeTrue())
		},
		Entry("debug lowercase", "debug"),
		Entry("DEBUG uppercase", "DEBUG"),
		Entry("info lowercase", "info"),
		Entry("INFO uppercase", "INFO"),
		Entry("warn lowercase", "warn"),
		Entry("WARN uppercase", "WARN"),
		Entry("error lowercase", "error"),
		Entry("ERROR uppercase", "ERROR"),
	)

	It("UT-AF-039-033 loads rate limit fields", func() {
		data := []byte(`
rateLimit:
  ipRequestsPerSec: 200
  userRequestsPerSec: 75
`)
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.RateLimit.IPRequestsPerSec).To(Equal(200))
		Expect(cfg.RateLimit.UserRequestsPerSec).To(Equal(75))
	})

	It("UT-AF-039-034 loads shutdown drainSeconds", func() {
		data := []byte("shutdown:\n  drainSeconds: 30\n")
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Shutdown.DrainSeconds).To(Equal(30))
	})

	It("UT-AF-039-035 DefaultConfig has extended defaults", func() {
		cfg := config.DefaultConfig()
		Expect(cfg.Logging.Level).To(Equal("INFO"))
		Expect(cfg.Shutdown.DrainSeconds).To(Equal(15))
		Expect(cfg.RateLimit.IPRequestsPerSec).To(Equal(100))
		Expect(cfg.RateLimit.UserRequestsPerSec).To(Equal(50))
	})
})

var _ = Describe("TC-P2C-03: DefaultConfig field assertions", func() {
	It("TC-P2C-03a DefaultConfig port is 8443", func() {
		cfg := config.DefaultConfig()
		Expect(cfg.Server.Port).To(Equal(8443))
	})

	It("TC-P2C-03b DefaultConfig drainSeconds is 15", func() {
		cfg := config.DefaultConfig()
		Expect(cfg.Shutdown.DrainSeconds).To(Equal(15))
	})

	It("TC-P2C-03c DefaultConfig resilience thresholds are non-zero", func() {
		cfg := config.DefaultConfig()
		Expect(cfg.Resilience.KA.CBFailureThreshold).To(BeNumerically(">", 0))
		Expect(cfg.Resilience.DS.CBFailureThreshold).To(BeNumerically(">", 0))
		Expect(cfg.Resilience.K8s.CBFailureThreshold).To(BeNumerically(">", 0))
	})
})

var _ = Describe("Tier 6: Extended Validation (v2)", func() {
	It("UT-AF-039-036 rejects auth issuerURL without scheme", func() {
		cfg := validConfig()
		cfg.Auth.IssuerURL = "sso.example.com/realms/kubernaut"
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("auth.issuerURL"))
	})

	It("UT-AF-039-037 accepts empty auth as optional", func() {
		cfg := validConfig()
		cfg.Auth.IssuerURL = ""
		cfg.Auth.Audience = ""
		Expect(cfg.Validate()).To(Succeed())
	})

	It("UT-AF-039-038 rejects invalid logging level", func() {
		cfg := validConfig()
		cfg.Logging.Level = "TRACE"
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("logging.level"))
	})

	DescribeTable("UT-AF-039-039 accepts valid logging levels",
		func(level string) {
			cfg := validConfig()
			cfg.Logging.Level = level
			Expect(cfg.Validate()).To(Succeed())
		},
		Entry("DEBUG", "DEBUG"),
		Entry("INFO", "INFO"),
		Entry("WARN", "WARN"),
		Entry("ERROR", "ERROR"),
		Entry("debug", "debug"),
		Entry("info", "info"),
		Entry("warn", "warn"),
		Entry("error", "error"),
	)

	It("UT-AF-039-040 rejects zero ipRequestsPerSec", func() {
		cfg := validConfig()
		cfg.RateLimit.IPRequestsPerSec = 0
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("rateLimit"))
	})

	It("UT-AF-039-041 rejects negative userRequestsPerSec", func() {
		cfg := validConfig()
		cfg.RateLimit.UserRequestsPerSec = -1
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("rateLimit"))
	})

	It("UT-AF-039-042 rejects zero drainSeconds", func() {
		cfg := validConfig()
		cfg.Shutdown.DrainSeconds = 0
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("shutdown.drainSeconds"))
	})
})

var _ = Describe("Tier 7: Resilience Config (Issue #38)", func() {
	It("UT-AF-038-001 loads resilience config", func() {
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
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Resilience.KA.ConnectTimeout.String()).To(Equal("5s"))
		Expect(cfg.Resilience.KA.RequestTimeout.String()).To(Equal("30s"))
		Expect(cfg.Resilience.KA.CBMaxRequests).To(Equal(uint32(3)))
		Expect(cfg.Resilience.KA.CBFailureThreshold).To(Equal(uint32(5)))
		Expect(cfg.Resilience.KA.RetryMax).To(Equal(2))
		Expect(cfg.Resilience.DS.ConnectTimeout.String()).To(Equal("3s"))
		Expect(cfg.Resilience.DS.CBFailureThreshold).To(Equal(uint32(3)))
		Expect(cfg.Resilience.DS.RetryMax).To(Equal(3))
		Expect(cfg.Resilience.KA.RetryableStatuses).To(HaveLen(3))
	})

	It("UT-AF-038-002 rejects negative connectTimeout", func() {
		cfg := validConfig()
		cfg.Resilience.KA.ConnectTimeout = -1
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("connectTimeout"))
	})

	It("UT-AF-038-003 rejects negative requestTimeout", func() {
		cfg := validConfig()
		cfg.Resilience.DS.RequestTimeout = -1
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("requestTimeout"))
	})

	It("UT-AF-038-004 rejects cbFailureThreshold > 100", func() {
		cfg := validConfig()
		cfg.Resilience.KA.CBFailureThreshold = 101
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cbFailureThreshold"))
	})

	It("rejects cbFailureThreshold == 0", func() {
		cfg := validConfig()
		cfg.Resilience.KA.CBFailureThreshold = 0
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cbFailureThreshold"))
	})

	It("UT-AF-038-005 rejects retryMax > 10", func() {
		cfg := validConfig()
		cfg.Resilience.DS.RetryMax = 11
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("retryMax"))
	})

	It("UT-AF-038-006 rejects retryableStatuses out of range", func() {
		cfg := validConfig()
		cfg.Resilience.KA.RetryableStatuses = []int{200, 503}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("retryableStatuses"))
	})

	It("UT-AF-038-007 applies resilience defaults when omitted", func() {
		data := []byte("server:\n  port: 8443\n")
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())

		defaults := config.DefaultConfig()
		Expect(cfg.Resilience.KA.RequestTimeout).To(Equal(defaults.Resilience.KA.RequestTimeout))
		Expect(cfg.Resilience.DS.CBFailureThreshold).To(Equal(defaults.Resilience.DS.CBFailureThreshold))
	})

	It("UT-AF-038-008 rejects requestTimeout less than connectTimeout", func() {
		cfg := validConfig()
		cfg.Resilience.KA.ConnectTimeout = 10000000000 // 10s
		cfg.Resilience.KA.RequestTimeout = 5000000000  // 5s
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("requestTimeout"))
	})

	It("requires PrometheusURL when severity triage enabled", func() {
		cfg := validConfig()
		cfg.SeverityTriage.Enabled = true
		cfg.SeverityTriage.PrometheusURL = ""
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("prometheusURL"))
	})

	It("skips severity triage validation when disabled", func() {
		cfg := validConfig()
		cfg.SeverityTriage.Enabled = false
		cfg.SeverityTriage.PrometheusURL = ""
		Expect(cfg.Validate()).To(Succeed())
	})

	It("rejects severity triage LLMConfidence out of range", func() {
		cfg := validConfig()
		cfg.SeverityTriage.Enabled = true
		cfg.SeverityTriage.PrometheusURL = "http://prometheus:9090"
		cfg.SeverityTriage.LLMConfidence = 1.5
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("llmConfidence"))
	})

	It("accepts valid severity triage config", func() {
		cfg := validConfig()
		cfg.SeverityTriage.Enabled = true
		cfg.SeverityTriage.PrometheusURL = "http://prometheus:9090"
		cfg.SeverityTriage.LLMConfidence = 0.7
		Expect(cfg.Validate()).To(Succeed())
	})

	DescribeTable("requires session namespace when TTLs are set",
		func(namespace string, disconn, retention time.Duration, wantErr bool) {
			cfg := validConfig()
			cfg.Session.Namespace = namespace
			cfg.Session.DisconnectTTL = disconn
			cfg.Session.RetentionTTL = retention
			err := cfg.Validate()
			if wantErr {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("session.namespace"))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("empty namespace with disconnectTTL rejects", "", 5*time.Minute, time.Duration(0), true),
		Entry("empty namespace with retentionTTL rejects", "", time.Duration(0), 720*time.Hour, true),
		Entry("empty namespace with both TTLs rejects", "", 10*time.Minute, 720*time.Hour, true),
		Entry("set namespace with TTLs passes", "kubernaut-system", 10*time.Minute, 720*time.Hour, false),
		Entry("empty namespace with zero TTLs passes", "", time.Duration(0), time.Duration(0), false),
	)
})

var _ = Describe("Tier 7b: Retention TTL Floor (AU-11)", func() {
	DescribeTable("UT-AF-1272-011 session.retentionTTL must be >= 30d",
		func(ttl time.Duration, wantErr bool) {
			cfg := validConfig()
			cfg.Session.Namespace = "kubernaut-system"
			cfg.Session.RetentionTTL = ttl
			err := cfg.Validate()
			if wantErr {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("retentionTTL"))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("zero TTL passes (not configured)", time.Duration(0), false),
		Entry("29 days rejects", 29*24*time.Hour, true),
		Entry("30 days passes", 30*24*time.Hour, false),
		Entry("90 days passes", 90*24*time.Hour, false),
	)
})

var _ = Describe("Tier 8: OIDC CA File (Issue #1245)", func() {
	It("UT-AF-1245-010 loads oidcCaFile field", func() {
		data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  audience: "apifrontend"
  oidcCaFile: "/etc/apifrontend/ingress-ca/ca.crt"
`)
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Auth.OIDCCaFile).To(Equal("/etc/apifrontend/ingress-ca/ca.crt"))
	})

	It("UT-AF-1245-011 defaults oidcCaFile to empty when omitted", func() {
		data := []byte(`
auth:
  issuerURL: "https://sso.example.com/realms/kubernaut"
  audience: "apifrontend"
`)
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Auth.OIDCCaFile).To(BeEmpty())
	})

	It("UT-AF-1245-012 rejects relative oidcCaFile path", func() {
		cfg := validConfig()
		cfg.Auth.IssuerURL = "https://sso.example.com/realms/kubernaut"
		cfg.Auth.OIDCCaFile = "relative/path/ca.crt"
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("oidcCaFile"))
	})

	It("UT-AF-1245-013 accepts absolute oidcCaFile path", func() {
		cfg := validConfig()
		cfg.Auth.IssuerURL = "https://sso.example.com/realms/kubernaut"
		cfg.Auth.OIDCCaFile = "/etc/apifrontend/ingress-ca/ca.crt"
		Expect(cfg.Validate()).To(Succeed())
	})

	It("UT-AF-1245-014 accepts empty oidcCaFile as optional", func() {
		cfg := validConfig()
		cfg.Auth.IssuerURL = "https://sso.example.com/realms/kubernaut"
		cfg.Auth.OIDCCaFile = ""
		Expect(cfg.Validate()).To(Succeed())
	})
})

var _ = Describe("Tier 9: LLM Config Validation (Issue #1252)", func() {
	It("accepts empty LLM provider as optional", func() {
		cfg := validConfig()
		cfg.Agent.LLM.Provider = ""
		Expect(cfg.Validate()).To(Succeed())
	})

	It("rejects unknown LLM provider", func() {
		cfg := validConfig()
		cfg.Agent.LLM.Provider = "openai"
		cfg.Agent.LLM.Model = "gpt-4"
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("agent.llm.provider"))
	})

	It("requires model when provider is set", func() {
		cfg := validConfig()
		cfg.Agent.LLM.Provider = config.LLMProviderGemini
		cfg.Agent.LLM.Model = ""
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("agent.llm.model"))
	})

	It("requires vertexProject for vertex_ai provider", func() {
		cfg := validConfig()
		cfg.Agent.LLM.Provider = config.LLMProviderVertexAI
		cfg.Agent.LLM.Model = "claude-sonnet-4-20250514"
		cfg.Agent.LLM.VertexProject = ""
		cfg.Agent.LLM.VertexLocation = "us-central1"
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("vertexProject"))
	})

	It("requires vertexLocation for vertex_ai provider", func() {
		cfg := validConfig()
		cfg.Agent.LLM.Provider = config.LLMProviderVertexAI
		cfg.Agent.LLM.Model = "claude-sonnet-4-20250514"
		cfg.Agent.LLM.VertexProject = "my-project"
		cfg.Agent.LLM.VertexLocation = ""
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("vertexLocation"))
	})

	It("accepts gemini provider", func() {
		cfg := validConfig()
		cfg.Agent.LLM.Provider = config.LLMProviderGemini
		cfg.Agent.LLM.Model = "gemini-2.0-flash"
		cfg.Agent.LLM.APIKeyFile = "/etc/secrets/llm-key"
		Expect(cfg.Validate()).To(Succeed())
	})

	It("accepts anthropic provider", func() {
		cfg := validConfig()
		cfg.Agent.LLM.Provider = config.LLMProviderAnthropic
		cfg.Agent.LLM.Model = "claude-sonnet-4-20250514"
		cfg.Agent.LLM.APIKeyFile = "/etc/secrets/llm-key"
		Expect(cfg.Validate()).To(Succeed())
	})

	It("requires credentials for gemini provider", func() {
		cfg := validConfig()
		cfg.Agent.LLM.Provider = config.LLMProviderGemini
		cfg.Agent.LLM.Model = "gemini-2.0-flash"
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("apiKeyFile"))
	})

	It("rejects relative apiKeyFile path", func() {
		cfg := validConfig()
		cfg.Agent.LLM.Provider = config.LLMProviderGemini
		cfg.Agent.LLM.Model = "gemini-2.0-flash"
		cfg.Agent.LLM.APIKeyFile = "relative/path/key"
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("apiKeyFile"))
	})

	It("requires tokenURL when oauth2 enabled", func() {
		cfg := validConfig()
		cfg.Agent.LLM.Provider = config.LLMProviderGemini
		cfg.Agent.LLM.Model = "gemini-2.0-flash"
		cfg.Agent.LLM.OAuth2.Enabled = true
		cfg.Agent.LLM.OAuth2.TokenURL = ""
		cfg.Agent.LLM.OAuth2.CredentialsDir = "/etc/creds"
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("tokenURL"))
	})

	It("requires credentialsDir when oauth2 enabled", func() {
		cfg := validConfig()
		cfg.Agent.LLM.Provider = config.LLMProviderGemini
		cfg.Agent.LLM.Model = "gemini-2.0-flash"
		cfg.Agent.LLM.OAuth2.Enabled = true
		cfg.Agent.LLM.OAuth2.TokenURL = "https://auth.example.com/token"
		cfg.Agent.LLM.OAuth2.CredentialsDir = ""
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("credentialsDir"))
	})

	It("ResolveDefaults loads LLM API key from file", func() {
		dir := GinkgoT().TempDir()
		keyFile := filepath.Join(dir, "llm-api-key")
		Expect(os.WriteFile(keyFile, []byte("  secret-key-123  \n"), 0o600)).To(Succeed())

		cfg := validConfig()
		cfg.Agent.LLM.APIKeyFile = keyFile
		Expect(cfg.ResolveDefaults()).To(Succeed())
		Expect(cfg.Agent.LLM.APIKey).To(Equal("secret-key-123"))
	})

	It("ResolveDefaults errors when apiKeyFile not found", func() {
		cfg := validConfig()
		cfg.Agent.LLM.APIKeyFile = "/nonexistent/path/key"
		err := cfg.ResolveDefaults()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("/nonexistent/path/key"))
	})

	It("ResolveDefaults errors when apiKeyFile is empty", func() {
		dir := GinkgoT().TempDir()
		keyFile := filepath.Join(dir, "llm-api-key")
		Expect(os.WriteFile(keyFile, []byte("   \n"), 0o600)).To(Succeed())

		cfg := validConfig()
		cfg.Agent.LLM.APIKeyFile = keyFile
		err := cfg.ResolveDefaults()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("empty"))
	})

	It("loads LLM config from YAML", func() {
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
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Agent.LLM.Provider).To(Equal(config.LLMProviderAnthropic))
		Expect(cfg.Agent.LLM.Model).To(Equal("claude-sonnet-4-20250514"))
		Expect(cfg.Agent.LLM.Endpoint).To(Equal("https://anthropic.example.com"))
		Expect(cfg.Agent.LLM.OAuth2.Enabled).To(BeTrue())
		Expect(cfg.Agent.LLM.OAuth2.TokenURL).To(Equal("https://auth.example.com/token"))
		Expect(cfg.Agent.LLM.OAuth2.Scopes).To(Equal([]string{"llm:invoke"}))
		Expect(cfg.Agent.LLM.CircuitBreaker.Enabled).To(BeTrue())
		Expect(cfg.Agent.LLM.CircuitBreaker.MaxRequests).To(Equal(uint32(5)))
		Expect(cfg.Agent.LLM.CustomHeaders).To(HaveLen(1))
		Expect(cfg.Agent.LLM.CustomHeaders[0].Name).To(Equal("X-Tenant-ID"))
	})
})

var _ = Describe("AF SA Token Config (#1287)", func() {
	It("UT-AF-1287-001: KABearerTokenFile parsed from YAML", func() {
		data := []byte(`
server:
  port: 8443
agent:
  kaBaseURL: "http://ka:8080"
  kaMCPEndpoint: "http://ka:8080/api/v1/mcp/"
  dsBaseURL: "http://ds:9090"
  kaBearerTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token"
logging:
  level: INFO
rateLimit:
  ipRequestsPerSec: 100
  userRequestsPerSec: 50
shutdown:
  drainSeconds: 15
resilience:
  ka:
    cbFailureThreshold: 5
  ds:
    cbFailureThreshold: 3
  k8s:
    cbFailureThreshold: 5
`)
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Agent.KABearerTokenFile).To(Equal("/var/run/secrets/kubernetes.io/serviceaccount/token"))
	})

	It("UT-AF-1287-008: Validate rejects inaccessible KABearerTokenFile (IA-5)", func() {
		data := []byte(`
server:
  port: 8443
agent:
  kaBaseURL: "http://ka:8080"
  kaMCPEndpoint: "http://ka:8080/api/v1/mcp/"
  dsBaseURL: "http://ds:9090"
  kaBearerTokenFile: "/nonexistent/path/to/token"
logging:
  level: INFO
rateLimit:
  ipRequestsPerSec: 100
  userRequestsPerSec: 50
shutdown:
  drainSeconds: 15
resilience:
  ka:
    cbFailureThreshold: 5
  ds:
    cbFailureThreshold: 3
  k8s:
    cbFailureThreshold: 5
`)
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		err = cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("kaBearerTokenFile"))
		Expect(err.Error()).To(ContainSubstring("not accessible"))
	})

	It("UT-AF-1287-003: missing KABearerTokenFile means empty (no auth)", func() {
		data := []byte(`
server:
  port: 8443
agent:
  kaBaseURL: "http://ka:8080"
  kaMCPEndpoint: "http://ka:8080/api/v1/mcp/"
  dsBaseURL: "http://ds:9090"
logging:
  level: INFO
rateLimit:
  ipRequestsPerSec: 100
  userRequestsPerSec: 50
shutdown:
  drainSeconds: 15
resilience:
  ka:
    cbFailureThreshold: 5
  ds:
    cbFailureThreshold: 3
  k8s:
    cbFailureThreshold: 5
`)
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Agent.KABearerTokenFile).To(BeEmpty())
	})
})
