package auth

import (
	"testing"
)

func TestConfig_Validate_RejectsEmptyIssuerURL(t *testing.T) {
	cfg := &Config{
		JWT: []ProviderConfig{
			{
				Issuer: IssuerConfig{
					URL:       "",
					Audiences: []string{"kubernaut-agent"},
				},
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty issuer URL")
	}
}

func TestConfig_Validate_RejectsEmptyAudiences(t *testing.T) {
	cfg := &Config{
		JWT: []ProviderConfig{
			{
				Issuer: IssuerConfig{
					URL:       "https://issuer.example.com",
					Audiences: []string{},
				},
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty audiences (would reject all tokens)")
	}
}

func TestConfig_Validate_RejectsNilAudiences(t *testing.T) {
	cfg := &Config{
		JWT: []ProviderConfig{
			{
				Issuer: IssuerConfig{
					URL:       "https://issuer.example.com",
					Audiences: nil,
				},
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for nil audiences")
	}
}

func TestConfig_Validate_RejectsEmptyValidationExpression(t *testing.T) {
	cfg := &Config{
		JWT: []ProviderConfig{
			{
				Issuer: IssuerConfig{
					URL:       "https://issuer.example.com",
					Audiences: []string{"kubernaut-agent"},
				},
				UserValidationRules: []ValidationRule{
					{Expression: "", Message: "rule with no expression"},
				},
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty expression in validation rule")
	}
}

func TestConfig_Validate_RejectsMalformedJWKSURL(t *testing.T) {
	cfg := &Config{
		JWT: []ProviderConfig{
			{
				Issuer: IssuerConfig{
					URL:       "https://issuer.example.com",
					JWKSURL:   "not-a-valid-url",
					Audiences: []string{"kubernaut-agent"},
				},
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for malformed JWKS URL")
	}
}

func TestConfig_Validate_AcceptsValidJWKSURL(t *testing.T) {
	cfg := &Config{
		JWT: []ProviderConfig{
			{
				Issuer: IssuerConfig{
					URL:       "https://issuer.example.com",
					JWKSURL:   "https://issuer.example.com/.well-known/jwks.json",
					Audiences: []string{"kubernaut-agent"},
				},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error for valid config: %v", err)
	}
}

func TestConfig_Validate_AcceptsEmptyJWKSURL(t *testing.T) {
	cfg := &Config{
		JWT: []ProviderConfig{
			{
				Issuer: IssuerConfig{
					URL:       "https://issuer.example.com",
					JWKSURL:   "",
					Audiences: []string{"kubernaut-agent"},
				},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error when JWKSURL is empty (should fall back to issuer URL): %v", err)
	}
}

func TestConfig_Validate_AcceptsValidConfigWithRules(t *testing.T) {
	cfg := &Config{
		JWT: []ProviderConfig{
			{
				Issuer: IssuerConfig{
					URL:       "https://issuer.example.com",
					Audiences: []string{"kubernaut-agent"},
				},
				UserValidationRules: []ValidationRule{
					{Expression: "!user.username.startsWith('system:')", Message: "no system users"},
				},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error for valid config with rules: %v", err)
	}
}
