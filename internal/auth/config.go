package auth

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ProviderConfig defines a single OIDC JWT provider.
type ProviderConfig struct {
	Issuer              IssuerConfig     `yaml:"issuer"`
	ClaimMappings       ClaimMappings    `yaml:"claimMappings"`
	UserValidationRules []ValidationRule `yaml:"userValidationRules,omitempty"`
}

// IssuerConfig holds the issuer URL and audiences.
// URL is the JWKS endpoint, not the OIDC discovery URL. This is intentional:
// the operator provides the JWKS URL explicitly in the configuration rather than
// relying on .well-known/openid-configuration discovery, which avoids an extra
// network round-trip at startup and supports non-standard OIDC providers.
// If OIDC discovery is needed in future, add a DiscoveryURL field and resolve
// the JWKS URL from the discovery document at startup.
type IssuerConfig struct {
	URL       string   `yaml:"url"`
	Audiences []string `yaml:"audiences"`
}

// ClaimMappings defines CEL expressions for mapping claims to user identity.
type ClaimMappings struct {
	Username string `yaml:"username"`
	Groups   string `yaml:"groups"`
}

// ValidationRule is a CEL expression that must evaluate to true for the user to be accepted.
type ValidationRule struct {
	Expression string `yaml:"expression"`
	Message    string `yaml:"message"`
}

// AuthConfig is the top-level authentication configuration.
type AuthConfig struct {
	JWT        []ProviderConfig     `yaml:"jwt,omitempty"`
	Kubernetes KubernetesAuthConfig `yaml:"kubernetes,omitempty"`
}

// KubernetesAuthConfig enables TokenReview-based authentication.
type KubernetesAuthConfig struct {
	Enabled bool `yaml:"enabled"`
}

// Validate checks the AuthConfig for structural errors.
func (c *AuthConfig) Validate() error {
	seen := make(map[string]struct{}, len(c.JWT))
	for _, p := range c.JWT {
		if _, exists := seen[p.Issuer.URL]; exists {
			return fmt.Errorf("duplicate issuer URL: %s", p.Issuer.URL)
		}
		seen[p.Issuer.URL] = struct{}{}
	}
	return nil
}

// LoadAuthConfigFromFile reads and parses an AuthConfig from a YAML file.
func LoadAuthConfigFromFile(path string) (AuthConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AuthConfig{}, fmt.Errorf("read auth config: %w", err)
	}
	var cfg AuthConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return AuthConfig{}, fmt.Errorf("parse auth config: %w", err)
	}
	return cfg, cfg.Validate()
}
