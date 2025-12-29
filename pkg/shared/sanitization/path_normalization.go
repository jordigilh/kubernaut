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

package sanitization

import (
	"regexp"
	"strings"
)

// Path placeholders for normalized segments
const (
	PathPlaceholderID   = ":id"   // For UUIDs and numeric IDs
	PathPlaceholderUUID = ":uuid" // Specifically for UUIDs
	PathPlaceholderNum  = ":num"  // Specifically for numeric IDs
)

var (
	// uuidPattern matches standard UUID format
	uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	// numericPattern matches pure numeric IDs
	numericPattern = regexp.MustCompile(`^\d+$`)

	// hexPattern matches hex strings (like short Git hashes)
	hexPattern = regexp.MustCompile(`^[0-9a-fA-F]{6,40}$`)
)

// NormalizePath replaces dynamic path segments with placeholders.
// This prevents high-cardinality metrics from overwhelming Prometheus.
//
// DD-005 Compliance: Per lines 689-710, path normalization is MANDATORY
// for all services exposing HTTP metrics.
//
// Example:
//
//	NormalizePath("/api/v1/users/123/orders/abc-def-123")
//	// Returns: "/api/v1/users/:id/orders/:id"
//
//	NormalizePath("/api/v1/events/550e8400-e29b-41d4-a716-446655440000")
//	// Returns: "/api/v1/events/:id"
func NormalizePath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = normalizeSegment(segment)
	}
	return strings.Join(segments, "/")
}

// NormalizePathWithPlaceholder normalizes using specific placeholder types.
// Use this when you need to distinguish between UUIDs and numeric IDs.
//
// Example:
//
//	NormalizePathWithPlaceholder("/users/123")
//	// Returns: "/users/:num"
//
//	NormalizePathWithPlaceholder("/users/550e8400-e29b-41d4-a716-446655440000")
//	// Returns: "/users/:uuid"
func NormalizePathWithPlaceholder(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = normalizeSegmentDetailed(segment)
	}
	return strings.Join(segments, "/")
}

// normalizeSegment replaces dynamic segments with generic :id placeholder.
func normalizeSegment(segment string) string {
	if segment == "" {
		return segment
	}

	// Check for UUID
	if uuidPattern.MatchString(segment) {
		return PathPlaceholderID
	}

	// Check for numeric ID
	if numericPattern.MatchString(segment) {
		return PathPlaceholderID
	}

	// Check for hex string (e.g., git commit hash)
	if hexPattern.MatchString(segment) && len(segment) >= 8 {
		return PathPlaceholderID
	}

	return segment
}

// normalizeSegmentDetailed replaces dynamic segments with specific placeholders.
func normalizeSegmentDetailed(segment string) string {
	if segment == "" {
		return segment
	}

	// Check for UUID
	if uuidPattern.MatchString(segment) {
		return PathPlaceholderUUID
	}

	// Check for numeric ID
	if numericPattern.MatchString(segment) {
		return PathPlaceholderNum
	}

	// Check for hex string
	if hexPattern.MatchString(segment) && len(segment) >= 8 {
		return PathPlaceholderID
	}

	return segment
}

// IsIDLikeSegment checks if a path segment looks like a dynamic ID.
// Useful for custom normalization logic.
func IsIDLikeSegment(segment string) bool {
	if segment == "" {
		return false
	}

	return uuidPattern.MatchString(segment) ||
		numericPattern.MatchString(segment) ||
		(hexPattern.MatchString(segment) && len(segment) >= 8)
}
