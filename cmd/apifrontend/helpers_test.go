package main

import (
	"testing"

	"go.uber.org/zap/zapcore"

	"github.com/jordigilh/kubernaut-apifrontend/internal/config"
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
}
