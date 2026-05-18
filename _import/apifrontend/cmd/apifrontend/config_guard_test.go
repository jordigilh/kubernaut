package main

import (
	"testing"

	"github.com/jordigilh/kubernaut-apifrontend/internal/config"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ratelimit"
)

// ---------------------------------------------------------------------------
// TC-C-03: CONFIG-01 — Startup Guard: Empty Issuer + TLS Required
// ---------------------------------------------------------------------------

func TestStartupGuard_EmptyIssuer_TLSRequired(t *testing.T) {
	// TC-C-03a: empty issuerURL + tls.required:true → startup error
	cfg := &config.Config{}
	cfg.Auth.IssuerURL = ""
	cfg.Server.TLS.Required = true

	// This should be caught by config validation.
	// Currently the config.Validate() may not check this.
	err := cfg.Validate()
	if err == nil {
		t.Errorf("TC-C-03a: expected startup error when issuerURL empty and TLS required, got nil")
	}
}

func TestStartupGuard_EmptyIssuer_TLSNotRequired(t *testing.T) {
	// TC-C-03b: empty issuerURL + tls.required:false → succeeds (dev mode)
	cfg := config.DefaultConfig()
	cfg.Auth.IssuerURL = ""
	cfg.Server.TLS.Required = false
	cfg.Server.Port = 8443

	err := cfg.Validate()
	if err != nil {
		t.Errorf("TC-C-03b: expected no error in dev mode, got %v", err)
	}
}

func TestStartupGuard_ValidIssuer_TLSRequired(t *testing.T) {
	// TC-C-03c: valid issuer + tls.required:true → succeeds
	cfg := config.DefaultConfig()
	cfg.Auth.IssuerURL = "https://dex.example.com"
	cfg.Auth.Audience = "test"
	cfg.Server.TLS.Required = true
	cfg.Server.Port = 8443

	err := cfg.Validate()
	if err != nil {
		t.Errorf("TC-C-03c: expected no error for valid issuer + TLS, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// TC-C-08: WIRE-16 — Limiter and Cache Stop() on shutdown
// ---------------------------------------------------------------------------

func TestShutdownStopsLimiters(t *testing.T) {
	// TC-C-08a/c: Verify Stop() is idempotent (double-stop doesn't panic).
	il := ratelimit.NewIPLimiter(ratelimit.PerIPConfig{
		RequestsPerSecond: 10, Burst: 20,
	})
	ul := ratelimit.NewUserLimiter(ratelimit.PerUserConfig{
		MaxConcurrentSessions: 5, ToolCallsPerMinute: 100,
	})

	il.Stop()
	il.Stop()

	ul.Stop()
	ul.Stop()
}
