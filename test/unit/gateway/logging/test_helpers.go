/*
Copyright 2025 Jordi Gil.

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

package logging

import (
	. "github.com/onsi/gomega"
)

// ========================================
// TDD REFACTOR: Test Helper Functions
// ========================================
//
// SCOPE: Eliminate duplication in sanitization tests
// PATTERN: Extract common validation logic into reusable helpers
// AUTHORITY: 00-core-development-methodology.mdc (TDD REFACTOR phase)

// AssertSecretRedacted validates that a secret was properly redacted from output.
// REFACTOR: Eliminates duplication of redaction validation (used 8+ times).
//
// Validates:
//   - Output contains [REDACTED] marker
//   - Output does not contain the original secret value
func AssertSecretRedacted(result, secretValue, context string) {
	ExpectWithOffset(1, result).To(ContainSubstring("[REDACTED]"),
		"%s - should contain redaction marker", context)
	ExpectWithOffset(1, result).NotTo(ContainSubstring(secretValue),
		"%s - should not contain secret: %s", context, secretValue)
}

// AssertSecretsRedacted validates that multiple secrets were redacted.
// REFACTOR: Eliminates duplication in multi-secret validation (used 5+ times).
//
// Validates:
//   - Output contains [REDACTED] marker
//   - Output does not contain any of the original secrets
func AssertSecretsRedacted(result string, secrets []string, context string) {
	ExpectWithOffset(1, result).To(ContainSubstring("[REDACTED]"),
		"%s - should contain redaction marker", context)

	for _, secret := range secrets {
		ExpectWithOffset(1, result).NotTo(ContainSubstring(secret),
			"%s - should not contain secret: %s", context, secret)
	}
}

// AssertContextPreserved validates that non-sensitive context was preserved.
// REFACTOR: Eliminates duplication in context preservation checks (used 5+ times).
//
// Validates:
//   - All expected context strings appear in the output
func AssertContextPreserved(result string, contextStrings []string, scenario string) {
	for _, context := range contextStrings {
		ExpectWithOffset(1, result).To(ContainSubstring(context),
			"%s - should preserve context: %s", scenario, context)
	}
}

// AssertCompleteRedaction validates full sanitization with context preservation.
// REFACTOR: Combines secret redaction + context preservation (used in table-driven tests).
//
// Validates:
//   - [REDACTED] marker present
//   - All secrets removed
//   - All context strings preserved
func AssertCompleteRedaction(result string, secrets []string, contexts []string, scenario string) {
	// Secrets must be redacted
	AssertSecretsRedacted(result, secrets, scenario)

	// Context must be preserved
	AssertContextPreserved(result, contexts, scenario)
}

// AssertFallbackBehavior validates graceful degradation behavior.
// REFACTOR: Eliminates duplication in fallback validation logic (used 3+ times).
//
// Validates:
//   - Result is not empty (degraded delivery)
//   - Critical information preserved
//   - Secrets redacted (either normal or fallback path)
func AssertFallbackBehavior(result string, err error, criticalInfo []string, secretsToRedact []string, scenario string) {
	// Degraded delivery: must have output
	ExpectWithOffset(1, result).NotTo(BeEmpty(),
		"%s - should deliver output even if sanitization fails", scenario)

	// Critical information preserved
	AssertContextPreserved(result, criticalInfo, scenario)

	// Secrets redacted (either path works)
	if err == nil {
		// Normal sanitization path
		AssertSecretsRedacted(result, secretsToRedact, scenario+" (normal path)")
	} else {
		// Fallback path - secrets still must be redacted
		AssertSecretsRedacted(result, secretsToRedact, scenario+" (fallback path)")
	}
}

// CommonGatewaySecrets returns commonly tested secret patterns for Gateway.
// REFACTOR: Centralizes test data (used in multiple test contexts).
func CommonGatewaySecrets() []string {
	return []string{
		"password=supersecret123",
		"apiKey=sk-proj-verysecretkey",
		"token=ghp_githubtoken123",
		"redis://user:redispass123@localhost:6379",
	}
}
