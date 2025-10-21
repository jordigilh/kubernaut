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

package sqlbuilder

import "fmt"

// ValidationError represents a query builder validation error
// BR-CONTEXT-007: Input validation error handling
type ValidationError struct {
	Field   string // The field that failed validation (e.g., "limit", "offset")
	Value   int    // The invalid value provided
	Message string // Human-readable error message
}

// Error implements error interface
func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s validation error: %s (value: %d)", e.Field, e.Message, e.Value)
}

// NewLimitError creates a validation error for invalid limit
func NewLimitError(value int) *ValidationError {
	return &ValidationError{
		Field:   "limit",
		Value:   value,
		Message: "limit must be between 1 and 1000",
	}
}

// NewOffsetError creates a validation error for invalid offset
func NewOffsetError(value int) *ValidationError {
	return &ValidationError{
		Field:   "offset",
		Value:   value,
		Message: "offset must be >= 0",
	}
}
