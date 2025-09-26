package validation

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var (
	// kubernetesNameRegex validates Kubernetes resource names
	kubernetesNameRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

	// kubernetesNamespaceRegex validates Kubernetes namespace names
	kubernetesNamespaceRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

	// sqlInjectionPatterns contains common SQL injection patterns
	sqlInjectionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(union|select|insert|update|delete|drop|create|alter|exec|execute)\b`),
		regexp.MustCompile(`(?i)(script|javascript|vbscript|onload|onerror|onclick)`),
		regexp.MustCompile(`[';\"\\]`), // Removed hyphen to allow underscores and hyphens in action types
		regexp.MustCompile(`\/\*.*\*\/`),
		regexp.MustCompile(`--\s`), // Only match SQL comments with space after --
	}
)

// ValidationError and ValidationErrors aliases removed - use sharedtypes directly

// NewValidationError creates a validation error using the shared type
func NewValidationError(field, message string) sharedtypes.ValidationResult {
	return sharedtypes.NewValidationError(field, message)
}

// ValidateResourceReference validates a Kubernetes resource reference
func ValidateResourceReference(ref actionhistory.ResourceReference) error {
	var errors sharedtypes.ValidationErrors

	// Validate namespace
	if ref.Namespace == "" {
		errors = append(errors, NewValidationError("namespace", "namespace is required"))
	} else if len(ref.Namespace) > 63 {
		errors = append(errors, NewValidationError("namespace", "namespace must be 63 characters or less"))
	} else if !kubernetesNamespaceRegex.MatchString(ref.Namespace) {
		errors = append(errors, NewValidationError("namespace", "namespace must be a valid Kubernetes namespace name"))
	}

	// Validate kind
	if ref.Kind == "" {
		errors = append(errors, NewValidationError("kind", "kind is required"))
	} else if len(ref.Kind) > 100 {
		errors = append(errors, NewValidationError("kind", "kind must be 100 characters or less"))
	} else if !isValidKubernetesKind(ref.Kind) {
		errors = append(errors, NewValidationError("kind", "kind must be a valid Kubernetes resource kind"))
	}

	// Validate name
	if ref.Name == "" {
		errors = append(errors, NewValidationError("name", "name is required"))
	} else if len(ref.Name) > 253 {
		errors = append(errors, NewValidationError("name", "name must be 253 characters or less"))
	} else if !kubernetesNameRegex.MatchString(ref.Name) {
		errors = append(errors, NewValidationError("name", "name must be a valid Kubernetes resource name"))
	}

	if len(errors) > 0 {
		return errors
	}

	return nil
}

// ValidateStringInput validates a string input for safety
func ValidateStringInput(field, value string, maxLength int) error {
	if len(value) > maxLength {
		return NewValidationError(field, fmt.Sprintf("must be %d characters or less", maxLength))
	}

	// Check for SQL injection patterns
	for _, pattern := range sqlInjectionPatterns {
		if pattern.MatchString(value) {
			return NewValidationError(field, "contains potentially unsafe characters")
		}
	}

	// Check for control characters
	for _, r := range value {
		if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' {
			return NewValidationError(field, "contains invalid control characters")
		}
	}

	return nil
}

// ValidateActionType validates an action type
func ValidateActionType(actionType string) error {
	if err := ValidateStringInput("actionType", actionType, 50); err != nil {
		return err
	}

	// List of allowed action types
	allowedActions := map[string]bool{
		"scale_deployment":      true,
		"increase_resources":    true,
		"restart_deployment":    true,
		"rollback_deployment":   true,
		"create_hpa":            true,
		"update_hpa":            true,
		"create_pdb":            true,
		"scale_statefulset":     true,
		"increase_pvc_size":     true,
		"add_node_affinity":     true,
		"update_tolerations":    true,
		"create_network_policy": true,
	}

	if !allowedActions[actionType] {
		return NewValidationError("actionType", fmt.Sprintf("'%s' is not a recognized action type", actionType))
	}

	return nil
}

// ValidateTimeRange validates a time range parameter
func ValidateTimeRange(timeRange string) error {
	if err := ValidateStringInput("timeRange", timeRange, 10); err != nil {
		return err
	}

	// Valid time range formats
	validFormats := regexp.MustCompile(`^[0-9]+[hdm]$`)
	if !validFormats.MatchString(timeRange) {
		return NewValidationError("timeRange", "must be in format like '24h', '7d', or '30m'")
	}

	return nil
}

// ValidateWindowMinutes validates a window minutes parameter
func ValidateWindowMinutes(windowMinutes int) error {
	if windowMinutes < 1 {
		return NewValidationError("windowMinutes", "must be greater than 0")
	}

	if windowMinutes > 10080 { // 7 days in minutes
		return NewValidationError("windowMinutes", "must be 7 days (10080 minutes) or less")
	}

	return nil
}

// ValidateLimit validates a limit parameter
func ValidateLimit(limit int) error {
	if limit < 1 {
		return NewValidationError("limit", "must be greater than 0")
	}

	if limit > 10000 {
		return NewValidationError("limit", "must be 10000 or less")
	}

	return nil
}

// isValidKubernetesKind checks if a string is a valid Kubernetes resource kind
func isValidKubernetesKind(kind string) bool {
	// Must start with uppercase letter
	if len(kind) == 0 || !unicode.IsUpper(rune(kind[0])) {
		return false
	}

	// Can contain letters and numbers only
	for _, r := range kind {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) {
			return false
		}
	}

	return true
}

// SanitizeForLogging sanitizes a string for safe logging
func SanitizeForLogging(input string) string {
	// Remove control characters except standard whitespace
	var result strings.Builder
	for _, r := range input {
		if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' {
			result.WriteString("?")
		} else {
			result.WriteRune(r)
		}
	}

	output := result.String()

	// Truncate if too long
	if len(output) > 200 {
		output = output[:197] + "..."
	}

	return output
}
