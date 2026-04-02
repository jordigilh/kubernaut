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

// CredentialSanitizer scrubs credentials from tool output (G4 per DD-HAPI-005 / BR-HAPI-211).
// Wraps the shared pkg/shared/sanitization.Sanitizer with an enhanced authorization-header
// rule that captures multi-word values (e.g., "Basic <base64>").
type CredentialSanitizer struct {
	sanitizer *sharedsanitization.Sanitizer
}

// NewCredentialSanitizer creates a G4 sanitizer backed by the shared sanitization library,
// with an enhanced authorization-header rule that redacts the entire header value.
func NewCredentialSanitizer() *CredentialSanitizer {
	rules := enhancedCredentialRules()
	return &CredentialSanitizer{
		sanitizer: sharedsanitization.NewSanitizerWithRules(rules),
	}
}

// enhancedAuthPattern captures the entire authorization header value (multi-word),
// compiled once at package init to avoid per-call regex compilation overhead.
var enhancedAuthPattern = regexp.MustCompile(`(?i)(authorization)\s*:\s*(.+)`)

// enhancedCredentialRules returns the default rules with an improved authorization-header
// pattern. The shared sanitizer's default pattern only captures the first token after the
// colon, leaving multi-word values like "Basic <base64>" partially exposed.
func enhancedCredentialRules() []*sharedsanitization.Rule {
	rules := sharedsanitization.DefaultRules()
	for i, r := range rules {
		if r.Name == "authorization-header" {
			rules[i] = &sharedsanitization.Rule{
				Name:        "authorization-header",
				Pattern:     enhancedAuthPattern,
				Replacement: `${1}: ` + sharedsanitization.RedactedPlaceholder,
				Description: "Redact authorization headers (including multi-word values like Basic auth)",
			}
			break
		}
	}
	return rules
}

// Name implements Stage.
func (s *CredentialSanitizer) Name() string { return "G4" }

// Sanitize implements Stage. Scrubs credentials using all DD-005 patterns
// (passwords, API keys, tokens, database URLs, certificates, K8s secrets, etc.).
func (s *CredentialSanitizer) Sanitize(_ context.Context, input string) (string, error) {
	return s.sanitizer.Sanitize(input), nil
}
