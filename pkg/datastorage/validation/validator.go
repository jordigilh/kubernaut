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

	"github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/go-logr/logr"
)

// Validator handles input validation and sanitization
// BR-STORAGE-010: Input validation
// BR-STORAGE-011: Input sanitization
type Validator struct {
	logger logr.Logger
}

// NewValidator creates a new validator
func NewValidator(logger logr.Logger) *Validator {
	return &Validator{
		logger: logger,
	}
}

// ValidateRemediationAudit validates a remediation audit record
// BR-STORAGE-010: Comprehensive input validation
// BR-STORAGE-019: Validation failure tracking with Prometheus metrics
func (v *Validator) ValidateRemediationAudit(audit *models.RemediationAudit) error {
	// Required field validation
	if audit.Name == "" {
		metrics.ValidationFailures.WithLabelValues("name", metrics.ValidationReasonRequired).Inc()
		return fmt.Errorf("name is required")
	}
	if audit.Namespace == "" {
		metrics.ValidationFailures.WithLabelValues("namespace", metrics.ValidationReasonRequired).Inc()
		return fmt.Errorf("namespace is required")
	}
	if audit.Phase == "" {
		metrics.ValidationFailures.WithLabelValues("phase", metrics.ValidationReasonRequired).Inc()
		return fmt.Errorf("phase is required")
	}

	// Phase validation (before other required fields to provide better error messages)
	if !v.isValidPhase(audit.Phase) {
		metrics.ValidationFailures.WithLabelValues("phase", metrics.ValidationReasonInvalid).Inc()
		return fmt.Errorf("invalid phase: %s", audit.Phase)
	}

	if audit.ActionType == "" {
		metrics.ValidationFailures.WithLabelValues("action_type", metrics.ValidationReasonRequired).Inc()
		return fmt.Errorf("action_type is required")
	}

	// Field length validation
	if len(audit.Name) > 255 {
		metrics.ValidationFailures.WithLabelValues("name", metrics.ValidationReasonLengthExceeded).Inc()
		return fmt.Errorf("name exceeds maximum length of 255")
	}
	if len(audit.Namespace) > 255 {
		metrics.ValidationFailures.WithLabelValues("namespace", metrics.ValidationReasonLengthExceeded).Inc()
		return fmt.Errorf("namespace exceeds maximum length of 255")
	}
	if len(audit.ActionType) > 100 {
		metrics.ValidationFailures.WithLabelValues("action_type", metrics.ValidationReasonLengthExceeded).Inc()
		return fmt.Errorf("action_type exceeds maximum length of 100")
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
// See: docs/services/stateless/data-storage/implementation/DATA-STORAGE-CODE-TRIAGE.md
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

// isValidPhase checks if a phase value is valid
// Valid phases: pending, processing, completed, failed
func (v *Validator) isValidPhase(phase string) bool {
	validPhases := []string{"pending", "processing", "completed", "failed"}
	for _, valid := range validPhases {
		if phase == valid {
			return true
		}
	}
	return false
}
