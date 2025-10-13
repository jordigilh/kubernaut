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

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"go.uber.org/zap"
)

// Validator handles input validation and sanitization
// BR-STORAGE-010: Input validation
// BR-STORAGE-011: Input sanitization
type Validator struct {
	logger *zap.Logger
}

// NewValidator creates a new validator
func NewValidator(logger *zap.Logger) *Validator {
	return &Validator{
		logger: logger,
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
	if audit.ActionType == "" {
		return fmt.Errorf("action_type is required")
	}

	// Phase validation
	if !v.isValidPhase(audit.Phase) {
		return fmt.Errorf("invalid phase: %s", audit.Phase)
	}

	// Field length validation
	if len(audit.Name) > 255 {
		return fmt.Errorf("name exceeds maximum length of 255")
	}
	if len(audit.Namespace) > 255 {
		return fmt.Errorf("namespace exceeds maximum length of 255")
	}
	if len(audit.ActionType) > 100 {
		return fmt.Errorf("action_type exceeds maximum length of 100")
	}

	v.logger.Debug("Validation passed",
		zap.String("name", audit.Name),
		zap.String("namespace", audit.Namespace),
		zap.String("phase", audit.Phase))

	return nil
}

// SanitizeString removes potentially malicious content
// BR-STORAGE-011: XSS and SQL injection protection
func (v *Validator) SanitizeString(input string) string {
	result := input

	// Remove script tags (case-insensitive, handles attributes)
	scriptRegex := regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`)
	result = scriptRegex.ReplaceAllString(result, "")

	// Remove all HTML tags
	htmlRegex := regexp.MustCompile(`<[^>]+>`)
	result = htmlRegex.ReplaceAllString(result, "")

	// Escape SQL special characters (remove semicolons)
	result = strings.ReplaceAll(result, ";", "")

	return result
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

