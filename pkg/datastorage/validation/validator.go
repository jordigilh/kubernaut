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

package validation

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// Validator handles input validation and sanitization
// BR-STORAGE-010: Input validation
// BR-STORAGE-011: Input sanitization
type Validator struct {
	logger logr.Logger
	rules  *ValidationRules
}

// NewValidator creates a new validator with default rules
func NewValidator(logger logr.Logger) *Validator {
	return &Validator{
		logger: logger,
		rules:  DefaultRules(),
	}
}

// ValidateRemediationAudit validates a remediation audit record
// BR-STORAGE-010: Comprehensive input validation
func (v *Validator) ValidateRemediationAudit(audit *models.RemediationAudit) error {
	// Required field validation
	if audit.Name == "" {
		return fmt.Errorf("name is required")
	}
	if audit.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if audit.Phase == "" {
		return fmt.Errorf("phase is required")
	}

	// Phase validation (before other required fields to provide better error messages)
	if !v.isValidPhase(audit.Phase) {
		return fmt.Errorf("invalid phase: %s", audit.Phase)
	}

	if audit.ActionType == "" {
		return fmt.Errorf("action_type is required")
	}

	if audit.Status != "" && !v.isValidStatus(audit.Status) {
		return fmt.Errorf("invalid status: %s", audit.Status)
	}

	// Field length validation using configurable rules
	if len(audit.Name) > v.rules.MaxNameLength {
		return fmt.Errorf("name exceeds maximum length of %d", v.rules.MaxNameLength)
	}
	if len(audit.Namespace) > v.rules.MaxNamespaceLength {
		return fmt.Errorf("namespace exceeds maximum length of %d", v.rules.MaxNamespaceLength)
	}
	if len(audit.ActionType) > v.rules.MaxActionTypeLength {
		return fmt.Errorf("action_type exceeds maximum length of %d", v.rules.MaxActionTypeLength)
	}

	v.logger.V(1).Info("Validation passed",
		"name", audit.Name,
		"namespace", audit.Namespace,
		"phase", audit.Phase)

	return nil
}

// SanitizeString removes potentially malicious HTML/XSS content
// BR-STORAGE-011: XSS protection
//
// SQL Injection Prevention: Handled by parameterized queries (not string sanitization)
// All database queries use PostgreSQL parameterized queries ($1, $2, etc.) which treat
// all input as data, never as SQL code. String sanitization for SQL keywords would:
// - Remove legitimate data (e.g., "my-app-delete-jobs" → "my-app--jobs")
// - Provide false sense of security (blacklist approach incomplete)
// - Add performance overhead (regex compilation)
//
// Finding #2: Unnecessary SQL keyword removal
func (v *Validator) SanitizeString(input string) string {
	result := input

	// Remove script tags (case-insensitive, handles attributes)
	// XSS Protection: Prevents <script> tag injection
	scriptRegex := regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`)
	result = scriptRegex.ReplaceAllString(result, "")

	// Remove all HTML tags
	// XSS Protection: Prevents HTML tag injection
	htmlRegex := regexp.MustCompile(`<[^>]+>`)
	result = htmlRegex.ReplaceAllString(result, "")

	// ✅ SQL Injection Prevention: Parameterized queries (database layer)
	// ✅ XSS Prevention: HTML/script tag removal (above)
	// ✅ Data Preservation: Legitimate strings like "delete-pod", "select-namespace" preserved

	return strings.TrimSpace(result)
}

// isValidPhase checks if a phase value is valid against configured rules
func (v *Validator) isValidPhase(phase string) bool {
	for _, valid := range v.rules.ValidPhases {
		if phase == valid {
			return true
		}
	}
	return false
}

// isValidStatus checks if a status value is valid against configured rules
func (v *Validator) isValidStatus(status string) bool {
	for _, valid := range v.rules.ValidStatuses {
		if status == valid {
			return true
		}
	}
	return false
}
