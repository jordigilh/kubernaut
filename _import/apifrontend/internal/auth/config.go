package auth

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ProviderConfig defines a single OIDC JWT provider.
type ProviderConfig struct {
	Issuer              IssuerConfig     `yaml:"issuer"`
	ClaimMappings       ClaimMappings    `yaml:"claimMappings"`
	UserValidationRules []ValidationRule `yaml:"userValidationRules,omitempty"`
}

// IssuerConfig holds the OIDC issuer identity and JWKS endpoint.
// URL is the issuer identifier (must match the "iss" claim in JWTs).
// JWKSURL is the HTTP(S) endpoint for fetching the JSON Web Key Set.
// When JWKSURL is empty, URL is used as both issuer and JWKS endpoint
// (legacy behavior for providers that serve JWKS at their issuer URL).
type IssuerConfig struct {
	URL       string   `yaml:"url"`
	JWKSURL   string   `yaml:"jwksURL,omitempty"`
	Audiences []string `yaml:"audiences"`
}

// ResolveJWKSURL returns the effective JWKS fetch URL.
func (c IssuerConfig) ResolveJWKSURL() string {
	if c.JWKSURL != "" {
		return c.JWKSURL
	}
	return c.URL
}

// ClaimMappings defines CEL expressions for mapping claims to user identity.
// NOTE: Currently the JWT validator uses fixed claim paths (preferred_username/sub
// for username, groups for group membership). CEL-based claim mapping is a planned
// enhancement. These fields are parsed from config for forward compatibility but
// are not yet evaluated at runtime. See buildIdentity() in jwt.go.
type ClaimMappings struct {
	Username string `yaml:"username"`
	Groups   string `yaml:"groups"`
}

// ValidationRule is a CEL expression that must evaluate to true for the user to be accepted.
type ValidationRule struct {
	Expression string `yaml:"expression"`
	Message    string `yaml:"message"`
}

// Config is the top-level authentication configuration.
type Config struct {
	JWT                  []ProviderConfig     `yaml:"jwt,omitempty"`
	Kubernetes           KubernetesAuthConfig `yaml:"kubernetes,omitempty"`
	AllowInsecureIssuers bool                 `yaml:"allowInsecureIssuers,omitempty"`
}

// KubernetesAuthConfig enables TokenReview-based authentication.
type KubernetesAuthConfig struct {
	Enabled bool `yaml:"enabled"`
}

// Validate checks the Config for structural errors.
func (c *Config) Validate() error {
	seen := make(map[string]struct{}, len(c.JWT))
	for i, p := range c.JWT {
		if p.Issuer.URL == "" {
			return fmt.Errorf("jwt[%d]: issuer URL must not be empty", i)
		}
		if err := validateIssuerURL(p.Issuer.URL, c.AllowInsecureIssuers); err != nil {
			return fmt.Errorf("jwt[%d]: %w", i, err)
		}
		if len(p.Issuer.Audiences) == 0 {
			return fmt.Errorf("jwt[%d]: audiences must not be empty (would reject all tokens)", i)
		}
		if p.Issuer.JWKSURL != "" {
			if err := validateIssuerURL(p.Issuer.JWKSURL, c.AllowInsecureIssuers); err != nil {
				return fmt.Errorf("jwt[%d]: JWKS URL: %w", i, err)
			}
		}
		for j, rule := range p.UserValidationRules {
			if rule.Expression == "" {
				return fmt.Errorf("jwt[%d].userValidationRules[%d]: expression must not be empty", i, j)
			}
		}
		if _, exists := seen[p.Issuer.URL]; exists {
			return fmt.Errorf("duplicate issuer URL: %s", p.Issuer.URL)
		}
		seen[p.Issuer.URL] = struct{}{}
	}
	return nil
}

const maxIssuerURLLen = 4096

func validateIssuerURL(rawURL string, allowInsecure bool) error {
	if len(rawURL) > maxIssuerURLLen {
		return fmt.Errorf("issuer URL exceeds %d characters", maxIssuerURLLen)
	}
	for _, b := range []byte(rawURL) {
		if b == 0 {
			return fmt.Errorf("issuer URL contains null byte")
		}
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid issuer URL %q: %w", rawURL, err)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if allowInsecure {
			return nil
		}
		return fmt.Errorf("issuer URL %q uses http; https required (set AllowInsecureIssuer for dev/test)", rawURL)
	default:
		return fmt.Errorf("issuer URL %q has unsupported scheme %q (must be https)", rawURL, u.Scheme)
	}
}

// LoadConfigFromFile reads and parses a Config from a YAML file.
func LoadConfigFromFile(path string) (Config, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return Config{}, fmt.Errorf("read auth config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse auth config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
