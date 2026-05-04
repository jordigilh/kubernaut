package auth

import "fmt"

// ProviderConfig defines a single OIDC JWT provider.
type ProviderConfig struct {
	Issuer              IssuerConfig
	ClaimMappings       ClaimMappings
	UserValidationRules []ValidationRule
}

// IssuerConfig holds the issuer URL and audiences.
type IssuerConfig struct {
	URL       string
	Audiences []string
}

// ClaimMappings defines CEL expressions for mapping claims to user identity.
type ClaimMappings struct {
	Username string
	Groups   string
}

// ValidationRule is a CEL expression that must evaluate to true for the user to be accepted.
type ValidationRule struct {
	Expression string
	Message    string
}

// AuthConfig is the top-level authentication configuration.
type AuthConfig struct {
	JWT        []ProviderConfig
	Kubernetes KubernetesAuthConfig
}

// KubernetesAuthConfig enables TokenReview-based authentication.
type KubernetesAuthConfig struct {
	Enabled bool
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
