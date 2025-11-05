package validation

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ========================================
// RFC 7807 ERROR TYPES (TDD GREEN Phase)
// 📋 Tests Define Contract: errors_test.go
// Authority: RFC 7807 Problem Details for HTTP APIs
// ========================================
//
// This file implements RFC 7807 standardized error responses
// for the Data Storage Service validation layer.
//
// TDD DRIVEN DESIGN:
// - Tests written FIRST (errors_test.go)
// - Production code implements MINIMAL functionality to pass tests
// - Contract defined by test expectations
//
// ========================================

// ValidationError represents a validation failure with field-level details.
// BR-STORAGE-001: Input validation for audit writes
type ValidationError struct {
	Resource    string            `json:"resource"`
	Message     string            `json:"message"`
	FieldErrors map[string]string `json:"field_errors"`
}

// NewValidationError creates a new ValidationError.
func NewValidationError(resource, message string) *ValidationError {
	return &ValidationError{
		Resource:    resource,
		Message:     message,
		FieldErrors: make(map[string]string),
	}
}

// AddFieldError adds a field-specific error.
func (v *ValidationError) AddFieldError(field, message string) {
	v.FieldErrors[field] = message
}

// Error implements the error interface.
func (v *ValidationError) Error() string {
	if len(v.FieldErrors) == 0 {
		return fmt.Sprintf("validation error for %s: %s", v.Resource, v.Message)
	}
	return fmt.Sprintf("validation error for %s: %s (fields: %d errors)", v.Resource, v.Message, len(v.FieldErrors))
}

// ToRFC7807 converts ValidationError to RFC 7807 Problem format.
func (v *ValidationError) ToRFC7807() *RFC7807Problem {
	return &RFC7807Problem{
		Type:     "https://kubernaut.io/errors/validation-error",
		Title:    "Validation Error",
		Status:   http.StatusBadRequest,
		Detail:   v.Message,
		Instance: fmt.Sprintf("/audit/%s", v.Resource),
		Extensions: map[string]interface{}{
			"resource":     v.Resource,
			"field_errors": v.FieldErrors,
		},
	}
}

// RFC7807Problem represents an RFC 7807 Problem Details response.
// See: https://datatracker.ietf.org/doc/html/rfc7807
type RFC7807Problem struct {
	Type       string                 `json:"type"`
	Title      string                 `json:"title"`
	Status     int                    `json:"status"`
	Detail     string                 `json:"detail,omitempty"`
	Instance   string                 `json:"instance,omitempty"`
	Extensions map[string]interface{} `json:"-"` // Flattened into top-level JSON
}

// MarshalJSON implements custom JSON marshaling to flatten Extensions.
func (p *RFC7807Problem) MarshalJSON() ([]byte, error) {
	// Create a map with standard RFC 7807 fields
	result := make(map[string]interface{})
	result["type"] = p.Type
	result["title"] = p.Title
	result["status"] = p.Status

	// Add optional fields only if non-empty
	if p.Detail != "" {
		result["detail"] = p.Detail
	}
	if p.Instance != "" {
		result["instance"] = p.Instance
	}

	// Flatten extensions into top-level JSON
	for k, v := range p.Extensions {
		result[k] = v
	}

	return json.Marshal(result)
}

// UnmarshalJSON implements custom JSON unmarshaling to extract standard fields and extensions.
// This enables tests to use RFC7807Problem for response validation.
func (p *RFC7807Problem) UnmarshalJSON(data []byte) error {
	// Unmarshal into a temporary map
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Extract standard RFC 7807 fields
	if v, ok := raw["type"].(string); ok {
		p.Type = v
		delete(raw, "type")
	}
	if v, ok := raw["title"].(string); ok {
		p.Title = v
		delete(raw, "title")
	}
	if v, ok := raw["status"].(float64); ok {
		p.Status = int(v)
		delete(raw, "status")
	}
	if v, ok := raw["detail"].(string); ok {
		p.Detail = v
		delete(raw, "detail")
	}
	if v, ok := raw["instance"].(string); ok {
		p.Instance = v
		delete(raw, "instance")
	}

	// Remaining fields are extensions
	p.Extensions = raw

	return nil
}

// Error implements the error interface.
func (p *RFC7807Problem) Error() string {
	return fmt.Sprintf("%s (status %d): %s", p.Title, p.Status, p.Detail)
}

// ========================================
// RFC 7807 HELPER FUNCTIONS
// ========================================

// NewValidationErrorProblem creates an RFC 7807 problem for validation errors.
func NewValidationErrorProblem(resource string, fieldErrors map[string]string) *RFC7807Problem {
	return &RFC7807Problem{
		Type:     "https://kubernaut.io/errors/validation-error",
		Title:    "Validation Error",
		Status:   http.StatusBadRequest,
		Detail:   fmt.Sprintf("Validation failed for %s", resource),
		Instance: fmt.Sprintf("/audit/%s", resource),
		Extensions: map[string]interface{}{
			"resource":     resource,
			"field_errors": fieldErrors,
		},
	}
}

// NewNotFoundProblem creates an RFC 7807 problem for resource not found.
func NewNotFoundProblem(resource, id string) *RFC7807Problem {
	return &RFC7807Problem{
		Type:     "https://kubernaut.io/errors/not-found",
		Title:    "Resource Not Found",
		Status:   http.StatusNotFound,
		Detail:   fmt.Sprintf("Resource %s with ID '%s' not found", resource, id),
		Instance: fmt.Sprintf("/audit/%s/%s", resource, id),
		Extensions: map[string]interface{}{
			"resource": resource,
			"id":       id,
		},
	}
}

// NewInternalErrorProblem creates an RFC 7807 problem for internal errors.
func NewInternalErrorProblem(detail string) *RFC7807Problem {
	return &RFC7807Problem{
		Type:   "https://kubernaut.io/errors/internal-error",
		Title:  "Internal Server Error",
		Status: http.StatusInternalServerError,
		Detail: detail,
		Extensions: map[string]interface{}{
			"retry": true,
		},
	}
}

// NewServiceUnavailableProblem creates an RFC 7807 problem for service unavailable.
func NewServiceUnavailableProblem(detail string) *RFC7807Problem {
	return &RFC7807Problem{
		Type:   "https://kubernaut.io/errors/service-unavailable",
		Title:  "Service Unavailable",
		Status: http.StatusServiceUnavailable,
		Detail: detail,
		Extensions: map[string]interface{}{
			"retry": true,
		},
	}
}

// NewConflictProblem creates an RFC 7807 problem for resource conflicts.
func NewConflictProblem(resource, field, value string) *RFC7807Problem {
	return &RFC7807Problem{
		Type:     "https://kubernaut.io/errors/conflict",
		Title:    "Resource Conflict",
		Status:   http.StatusConflict,
		Detail:   fmt.Sprintf("Resource %s with %s='%s' already exists", resource, field, value),
		Instance: fmt.Sprintf("/audit/%s", resource),
		Extensions: map[string]interface{}{
			"resource": resource,
			"field":    field,
			"value":    value,
		},
	}
}


		Detail: detail,
		Extensions: map[string]interface{}{
			"retry": true,
		},
	}
}

// NewServiceUnavailableProblem creates an RFC 7807 problem for service unavailable.
func NewServiceUnavailableProblem(detail string) *RFC7807Problem {
	return &RFC7807Problem{
		Type:   "https://kubernaut.io/errors/service-unavailable",
		Title:  "Service Unavailable",
		Status: http.StatusServiceUnavailable,
		Detail: detail,
		Extensions: map[string]interface{}{
			"retry": true,
		},
	}
}

// NewConflictProblem creates an RFC 7807 problem for resource conflicts.
func NewConflictProblem(resource, field, value string) *RFC7807Problem {
	return &RFC7807Problem{
		Type:     "https://kubernaut.io/errors/conflict",
		Title:    "Resource Conflict",
		Status:   http.StatusConflict,
		Detail:   fmt.Sprintf("Resource %s with %s='%s' already exists", resource, field, value),
		Instance: fmt.Sprintf("/audit/%s", resource),
		Extensions: map[string]interface{}{
			"resource": resource,
			"field":    field,
			"value":    value,
		},
	}
}

