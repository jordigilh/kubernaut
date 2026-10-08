/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sanitization

import (
	"context"
	"regexp"

	sharedsanitization "github.com/jordigilh/kubernaut/pkg/shared/sanitization"
)

// CredentialSanitizer scrubs credentials from tool output (G4 per DD-KA-005,
// DD-KA-2485, and BR-KA-211).
// Wraps the shared pkg/shared/sanitization.Sanitizer with an enhanced authorization-header
// rule that captures multi-word values (e.g., "Basic <base64>"). KA deliberately does not
// use the shared k8s-secret-data heuristic: Secret-shaped data is handled by SecretSanitizer,
// which has the Kubernetes object context needed to distinguish credentials from identifiers.
type CredentialSanitizer struct {
	sanitizer *sharedsanitization.Sanitizer
}

// NewCredentialSanitizer creates a G4 sanitizer backed by the shared sanitization library,
// with KA-specific same-line credential rules and an enhanced authorization-header rule that
// redacts the entire header value. Kubernetes Secret data is handled by SecretSanitizer.
func NewCredentialSanitizer() *CredentialSanitizer {
	rules := enhancedCredentialRules()
	return &CredentialSanitizer{
		sanitizer: sharedsanitization.NewSanitizerWithRules(rules),
	}
}

var (
	// enhancedAuthPattern captures the entire authorization header value (multi-word),
	// compiled once at package init to avoid per-call regex compilation overhead.
	enhancedAuthPattern = regexp.MustCompile(`(?i)(authorization)[ \t]*:[ \t]*(.+)`)

	// KA's plain-field patterns are intentionally restricted to horizontal whitespace.
	// Using \s* lets a credential regex consume the next YAML mapping key when the
	// current field has a nested object (for example, volumes[].secret).
	kaPasswordPattern   = regexp.MustCompile(`(?i)(password|passwd|pwd)[ \t]*[:=][ \t]*["']?([^\s"',}]+)["']?`)
	kaAPIKeyPattern     = regexp.MustCompile(`(?i)(api[_-]?key|apikey)[ \t]*[:=][ \t]*["']?([^\s"',}]+)["']?`)
	kaTokenPattern      = regexp.MustCompile(`(?i)\b(token|access[_-]?token)[ \t]*[:=][ \t]*["']?([^\s"',}]+)["']?`)
	kaSecretPattern     = regexp.MustCompile(`(?i)(secret|client_secret)[ \t]*[:=][ \t]*["']?([^\s"',}]+)["']?`)
	kaCredentialPattern = regexp.MustCompile(`(?i)(credential|credentials)[ \t]*[:=][ \t]*["']?([^\s"',}]+)["']?`)
)

// enhancedCredentialRules returns the shared rules with KA-specific hardening.
//
// The shared k8s-secret-data rule is intentionally excluded here. It treats every
// base64-shaped value after key/username/etc. as Secret data, even when the field is
// a taint, selector, reference, or identity. SecretSanitizer owns Kubernetes
// Secret.data/stringData redaction with object context instead.
func enhancedCredentialRules() []*sharedsanitization.Rule {
	rules := sharedsanitization.DefaultRules()
	filtered := make([]*sharedsanitization.Rule, 0, len(rules))
	for _, r := range rules {
		switch r.Name {
		case "k8s-secret-data":
			continue
		case "authorization-header":
			filtered = append(filtered, replaceRulePattern(r, enhancedAuthPattern))
		case "password-plain":
			filtered = append(filtered, replaceRulePattern(r, kaPasswordPattern))
		case "api-key-plain":
			filtered = append(filtered, replaceRulePattern(r, kaAPIKeyPattern))
		case "token-plain":
			filtered = append(filtered, replaceRulePattern(r, kaTokenPattern))
		case "secret-plain":
			filtered = append(filtered, replaceRulePattern(r, kaSecretPattern))
		default:
			filtered = append(filtered, r)
		}
	}
	filtered = append(filtered, &sharedsanitization.Rule{
		Name:        "credential-plain",
		Pattern:     kaCredentialPattern,
		Replacement: `${1}: ` + sharedsanitization.RedactedPlaceholder,
		Description: "Redact credential fields without consuming nested YAML mappings",
	})
	return filtered
}

func replaceRulePattern(rule *sharedsanitization.Rule, pattern *regexp.Regexp) *sharedsanitization.Rule {
	clonedRule := *rule
	clonedRule.Pattern = pattern
	return &clonedRule
}

// Name implements Stage.
func (s *CredentialSanitizer) Name() string { return "G4" }

// Sanitize implements Stage. Scrubs credentials using the shared DD-005 patterns
// plus KA's same-line credential rules. Kubernetes Secret data is handled by the
// separate K8S-SECRET stage so ordinary Kubernetes identifiers remain intact.
func (s *CredentialSanitizer) Sanitize(_ context.Context, input string) (string, error) {
	return s.sanitizer.Sanitize(input), nil
}
