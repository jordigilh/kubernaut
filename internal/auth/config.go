package auth

import (
	"fmt"
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
	JWT        []ProviderConfig     `yaml:"jwt,omitempty"`
	Kubernetes KubernetesAuthConfig `yaml:"kubernetes,omitempty"`
}

// KubernetesAuthConfig enables TokenReview-based authentication.
type KubernetesAuthConfig struct {
	Enabled bool `yaml:"enabled"`
}

// Validate checks the Config for structural errors.
func (c *Config) Validate() error {
	seen := make(map[string]struct{}, len(c.JWT))
	for _, p := range c.JWT {
		if _, exists := seen[p.Issuer.URL]; exists {
			return fmt.Errorf("duplicate issuer URL: %s", p.Issuer.URL)
		}
		seen[p.Issuer.URL] = struct{}{}
	}
	return nil
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
