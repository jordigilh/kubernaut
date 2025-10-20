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

const (
	// MinLimit is the minimum allowed LIMIT value
	// BR-CONTEXT-007: Pagination boundary - at least one result
	MinLimit = 1

	// MaxLimit is the maximum allowed LIMIT value
	// BR-CONTEXT-007: Pagination boundary - prevent database overload
	MaxLimit = 1000

	// DefaultLimit is the default LIMIT value when not specified
	// BR-CONTEXT-007: Safe default for pagination
	DefaultLimit = 100

	// MinOffset is the minimum allowed OFFSET value
	// BR-CONTEXT-007: Pagination boundary - start of results
	MinOffset = 0
)

// ValidateLimit checks if limit is within valid range (1-1000)
// BR-CONTEXT-007: Limit boundary validation
//
// Rules:
//   - Minimum: 1 (at least one result)
//   - Maximum: 1000 (prevent database overload)
//
// Returns ValidationError if out of range.
func ValidateLimit(limit int) error {
	if limit < MinLimit || limit > MaxLimit {
		return NewLimitError(limit)
	}
	return nil
}

// ValidateOffset checks if offset is non-negative
// BR-CONTEXT-007: Offset boundary validation
//
// Rules:
//   - Minimum: 0 (start of results)
//   - Maximum: No upper limit (but consider performance implications)
//
// Returns ValidationError if negative.
func ValidateOffset(offset int) error {
	if offset < MinOffset {
		return NewOffsetError(offset)
	}
	return nil
}

