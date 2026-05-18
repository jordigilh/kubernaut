package main

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jordigilh/kubernaut-apifrontend/internal/config"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ratelimit"
)

func TestReloadCallback_LogLevel(t *testing.T) {
	level := zap.NewAtomicLevelAt(zap.InfoLevel)

	newContent := []byte(`
server:
  port: 8443
agent:
  kaBaseURL: "http://localhost:8080"
  kaMCPEndpoint: "http://localhost:8080/api/v1/mcp/"
  dsBaseURL: "http://localhost:9090"
logging:
  level: "DEBUG"
rateLimit:
  ipRequestsPerSec: 100
  userRequestsPerSec: 50
`)

	cfg, err := config.Load(newContent)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	cfg.ResolveDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config.Validate: %v", err)
	}

	if newLevel, err := parseLogLevel(cfg.Logging.Level); err == nil {
		level.SetLevel(newLevel)
	}

	if level.Level() != zapcore.DebugLevel {
		t.Errorf("expected DebugLevel, got %v", level.Level())
	}
}

func TestReloadCallback_RateLimits(t *testing.T) {
	ipLimiter := ratelimit.NewIPLimiter(ratelimit.PerIPConfig{
		RequestsPerSecond: 10,
		Burst:             20,
	})
	defer ipLimiter.Stop()

	userLimiter := ratelimit.NewUserLimiter(ratelimit.PerUserConfig{
		RequestsPerMinute:     30,
		MaxConcurrentSessions: 5,
		ToolCallsPerMinute:    60,
	})

	ipLimiter.UpdateLimits(200, 400)
	userLimiter.UpdateRequestRate(120)
	userLimiter.UpdateToolRate(120)

	// After updating to 200 RPS with burst 400, a burst of requests should succeed
	for i := 0; i < 50; i++ {
		if !ipLimiter.Allow("10.0.0.1") {
			t.Fatalf("expected Allow=true after UpdateLimits at request %d", i)
		}
	}

	// After updating to 120 RPM (2/sec burst 120), many rapid requests should succeed
	for i := 0; i < 50; i++ {
		if !userLimiter.AllowRequest("testuser") {
			t.Fatalf("expected AllowRequest=true after UpdateRequestRate at request %d", i)
		}
	}

	for i := 0; i < 50; i++ {
		if !userLimiter.AllowToolCall("testuser") {
			t.Fatalf("expected AllowToolCall=true after UpdateToolRate at request %d", i)
		}
	}
}
