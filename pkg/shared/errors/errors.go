<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package errors

import (
	"fmt"
	"strings"
)

// This file maintains backward compatibility with existing error handling
// while delegating to the new enhanced error system in enhanced_errors.go
// All functionality has been consolidated into the EnhancedError system.

// Backward compatibility removed - use EnhancedError directly

// All functions are now available through enhanced_errors.go
// The following functions maintain their original signatures:
// - FailedTo(action string, cause error) error
// - FailedToWithDetails(action, component, resource string, cause error) error
// - DatabaseError(operation string, cause error) error
// - NetworkError(operation string, endpoint string, cause error) error

// For new code, prefer using the enhanced error system directly:
// - New(category ErrorCategory, message string) *EnhancedError
// - Wrap(err error, category ErrorCategory, message string) *EnhancedError
// - Wrapf(err error, category ErrorCategory, format string, args ...interface{}) *EnhancedError

// Convenience functions for backward compatibility with tests

// ValidationError creates a validation error
func ValidationError(field, message string) error {
	return New(ErrorCategoryValidation, fmt.Sprintf("validation failed for field %s: %s", field, message))
}

// ConfigurationError creates a configuration error
func ConfigurationError(setting, message string) error {
	return New(ErrorCategoryConfiguration, fmt.Sprintf("configuration error for %s: %s", setting, message))
}

// TimeoutError creates a timeout error
func TimeoutError(operation, duration string) error {
	return New(ErrorCategoryTimeout, fmt.Sprintf("timeout while %s after %s", operation, duration))
}

// AuthenticationError creates an authentication error
func AuthenticationError(message string) error {
	return New(ErrorCategoryAuth, fmt.Sprintf("authentication failed: %s", message))
}

// AuthorizationError creates an authorization error
func AuthorizationError(operation, resource string) error {
	return New(ErrorCategoryAuth, fmt.Sprintf("authorization failed: insufficient permissions to %s %s", operation, resource))
}

// ParseError creates a parse error
func ParseError(source, format string, cause error) error {
	return Wrap(cause, ErrorCategoryValidation, fmt.Sprintf("failed to parse %s as %s", source, format))
}

// IsRetryable determines if an error is retryable
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	errorStr := strings.ToLower(err.Error())
	retryablePatterns := []string{
		"timeout", "connection refused", "temporary failure",
		"service unavailable", "too many requests", "rate limit",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errorStr, pattern) {
			return true
		}
	}

	return false
}

// Chain combines multiple errors into a single error
func Chain(errors ...error) error {
	var validErrors []error
	for _, err := range errors {
		if err != nil {
			validErrors = append(validErrors, err)
		}
	}

	if len(validErrors) == 0 {
		return nil
	}
	if len(validErrors) == 1 {
		return validErrors[0]
	}

	var messages []string
	for _, err := range validErrors {
		messages = append(messages, err.Error())
	}

	return fmt.Errorf("multiple errors: %s", strings.Join(messages, "; "))
}
