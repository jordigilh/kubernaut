package main

import (
	"testing"

	"go.uber.org/zap/zapcore"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input   string
		want    zapcore.Level
		wantErr bool
	}{
		{"debug", zapcore.DebugLevel, false},
		{"info", zapcore.InfoLevel, false},
		{"warn", zapcore.WarnLevel, false},
		{"error", zapcore.ErrorLevel, false},
		{"INFO", zapcore.InfoLevel, false},
		{"", zapcore.InfoLevel, false},
		{"INVALID", zapcore.InfoLevel, true},
		{"trace", zapcore.InfoLevel, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseLogLevel(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseLogLevel(%q) = %v, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("parseLogLevel(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildAuthConfig(t *testing.T) {
	t.Run("empty issuer returns empty config", func(t *testing.T) {
		cfg := &config.Config{Auth: config.AuthConfig{IssuerURL: "", Audience: "test"}}
		result := buildAuthConfig(cfg)
		if len(result.JWT) != 0 {
			t.Errorf("expected empty JWT slice, got %d providers", len(result.JWT))
		}
	})

	t.Run("valid issuer returns configured provider", func(t *testing.T) {
		cfg := &config.Config{Auth: config.AuthConfig{
			IssuerURL: "https://keycloak.example.com/realms/kubernaut",
			Audience:  "apifrontend",
		}}
		result := buildAuthConfig(cfg)
		if len(result.JWT) != 1 {
			t.Fatalf("expected 1 JWT provider, got %d", len(result.JWT))
		}
		if result.JWT[0].Issuer.URL != cfg.Auth.IssuerURL {
			t.Errorf("issuer URL = %q, want %q", result.JWT[0].Issuer.URL, cfg.Auth.IssuerURL)
		}
		if len(result.JWT[0].Issuer.Audiences) != 1 || result.JWT[0].Issuer.Audiences[0] != "apifrontend" {
			t.Errorf("audiences = %v, want [apifrontend]", result.JWT[0].Issuer.Audiences)
		}
	})

	t.Run("UT-AF-1309-022: issuerURL set returns populated JWT slice", func(t *testing.T) {
		cfg := &config.Config{Auth: config.AuthConfig{
			IssuerURL: "https://dex.example.com",
			Audience:  "af",
		}}
		result := buildAuthConfig(cfg)
		if len(result.JWT) == 0 {
			t.Fatal("UT-AF-1309-022: expected non-empty JWT slice when issuerURL is set")
		}
	})

	t.Run("UT-AF-1309-023: empty issuerURL returns empty JWT slice", func(t *testing.T) {
		cfg := &config.Config{Auth: config.AuthConfig{IssuerURL: ""}}
		result := buildAuthConfig(cfg)
		if len(result.JWT) != 0 {
			t.Errorf("UT-AF-1309-023: expected empty JWT slice, got %d", len(result.JWT))
		}
	})
}

func TestBuildAuthConfigMultiProvider(t *testing.T) {
	t.Run("UT-AF-1436-010: jwtProviders with two providers returns both", func(t *testing.T) {
		cfg := &config.Config{Auth: config.AuthConfig{
			JWTProviders: []config.JWTProviderConfig{
				{
					Name:      "keycloak",
					IssuerURL: "https://keycloak.example.com/realms/kubernaut",
					JWKSURL:   "https://keycloak.example.com/realms/kubernaut/certs",
					Audiences: []string{"kubernaut-apifrontend"},
				},
				{
					Name:      "spire",
					IssuerURL: "https://oidc-discovery.example.com",
					JWKSURL:   "https://spire-oidc.example.com/keys",
					Audiences: []string{"spiffe://trust-domain/ns/kubernaut/sa/af"},
				},
			},
		}}
		result := buildAuthConfig(cfg)
		if len(result.JWT) != 2 {
			t.Fatalf("expected 2 JWT providers, got %d", len(result.JWT))
		}
		if result.JWT[0].Issuer.URL != "https://keycloak.example.com/realms/kubernaut" {
			t.Errorf("provider[0] issuer URL = %q, want keycloak URL", result.JWT[0].Issuer.URL)
		}
		if result.JWT[1].Issuer.URL != "https://oidc-discovery.example.com" {
			t.Errorf("provider[1] issuer URL = %q, want spire URL", result.JWT[1].Issuer.URL)
		}
		if result.JWT[0].Issuer.JWKSURL != "https://keycloak.example.com/realms/kubernaut/certs" {
			t.Errorf("provider[0] JWKS URL = %q", result.JWT[0].Issuer.JWKSURL)
		}
		if len(result.JWT[1].Issuer.Audiences) != 1 || result.JWT[1].Issuer.Audiences[0] != "spiffe://trust-domain/ns/kubernaut/sa/af" {
			t.Errorf("provider[1] audiences = %v", result.JWT[1].Issuer.Audiences)
		}
	})

	t.Run("UT-AF-1436-011: legacy issuerURL returns single-element slice", func(t *testing.T) {
		cfg := &config.Config{Auth: config.AuthConfig{
			IssuerURL: "https://keycloak.example.com/realms/kubernaut",
			Audience:  "apifrontend",
		}}
		result := buildAuthConfig(cfg)
		if len(result.JWT) != 1 {
			t.Fatalf("expected 1 JWT provider, got %d", len(result.JWT))
		}
		if result.JWT[0].Issuer.URL != cfg.Auth.IssuerURL {
			t.Errorf("issuer URL = %q, want %q", result.JWT[0].Issuer.URL, cfg.Auth.IssuerURL)
		}
	})

	t.Run("UT-AF-1436-012: both set -- jwtProviders wins", func(t *testing.T) {
		cfg := &config.Config{Auth: config.AuthConfig{
			IssuerURL: "https://legacy.example.com",
			Audience:  "old-audience",
			JWTProviders: []config.JWTProviderConfig{
				{
					Name:      "keycloak",
					IssuerURL: "https://keycloak.example.com",
					Audiences: []string{"new-audience"},
				},
			},
		}}
		result := buildAuthConfig(cfg)
		if len(result.JWT) != 1 {
			t.Fatalf("expected 1 JWT provider from jwtProviders, got %d", len(result.JWT))
		}
		if result.JWT[0].Issuer.URL != "https://keycloak.example.com" {
			t.Errorf("jwtProviders should take precedence, got URL = %q", result.JWT[0].Issuer.URL)
		}
	})

	t.Run("UT-AF-1436-013: neither set returns empty slice", func(t *testing.T) {
		cfg := &config.Config{Auth: config.AuthConfig{IssuerURL: "", JWTProviders: nil}}
		result := buildAuthConfig(cfg)
		if len(result.JWT) != 0 {
			t.Errorf("expected empty JWT slice, got %d", len(result.JWT))
		}
	})

	t.Run("UT-AF-1436-014: claimMappings propagated to auth.ProviderConfig", func(t *testing.T) {
		cfg := &config.Config{Auth: config.AuthConfig{
			JWTProviders: []config.JWTProviderConfig{
				{
					Name:      "with-claims",
					IssuerURL: "https://issuer.example.com",
					Audiences: []string{"aud"},
					ClaimMappings: config.ConfigClaimMappings{
						Username: "email",
						Groups:   "team_membership",
					},
				},
			},
		}}
		result := buildAuthConfig(cfg)
		if len(result.JWT) != 1 {
			t.Fatalf("expected 1 provider, got %d", len(result.JWT))
		}
		if result.JWT[0].ClaimMappings.Username != "email" {
			t.Errorf("ClaimMappings.Username = %q, want %q", result.JWT[0].ClaimMappings.Username, "email")
		}
		if result.JWT[0].ClaimMappings.Groups != "team_membership" {
			t.Errorf("ClaimMappings.Groups = %q, want %q", result.JWT[0].ClaimMappings.Groups, "team_membership")
		}
	})

	t.Run("UT-AF-1436-020: jwtProviders non-empty triggers OIDC mode", func(t *testing.T) {
		cfg := &config.Config{Auth: config.AuthConfig{
			JWTProviders: []config.JWTProviderConfig{
				{
					Name:      "keycloak",
					IssuerURL: "https://keycloak.example.com",
					Audiences: []string{"aud"},
				},
			},
		}}
		result := buildAuthConfig(cfg)
		if len(result.JWT) == 0 {
			t.Error("jwtProviders non-empty should trigger OIDC mode (non-empty JWT slice)")
		}
	})

	t.Run("UT-AF-1436-021: SPIRE OIDC discovery URL accepted", func(t *testing.T) {
		cfg := &config.Config{Auth: config.AuthConfig{
			JWTProviders: []config.JWTProviderConfig{
				{
					Name:      "spire",
					IssuerURL: "https://oidc-discovery.zero-trust.svc:443",
					JWKSURL:   "https://spire-oidc.zero-trust.svc:443/keys",
					Audiences: []string{"spiffe://trust-domain/ns/kubernaut/sa/af"},
				},
			},
		}}
		result := buildAuthConfig(cfg)
		if len(result.JWT) != 1 {
			t.Fatalf("expected 1 provider, got %d", len(result.JWT))
		}
		if result.JWT[0].Issuer.URL != "https://oidc-discovery.zero-trust.svc:443" {
			t.Errorf("SPIRE URL = %q", result.JWT[0].Issuer.URL)
		}
	})
}

// Ensure buildAuthConfig returns auth.ProviderConfig with ClaimMappings.
var _ auth.ClaimMappings = auth.ClaimMappings{}
