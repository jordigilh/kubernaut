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

package processing

// DD-GATEWAY-012: DeduplicationMetadata is used for HTTP response metadata
// Extracted from deleted deduplication.go (Redis-based deduplication removed)
// This type is still needed for API response compatibility

// DeduplicationMetadata contains deduplication tracking information for HTTP responses
// DD-GATEWAY-011: This data now comes from RR status.deduplication instead of Redis
type DeduplicationMetadata struct {
	// Fingerprint is the SHA256 hash of the alert
	Fingerprint string `json:"fingerprint,omitempty"`

	// Count is the number of times this alert has been received (including current)
	// Example: First duplicate has count=2 (original + 1 duplicate)
	Count int `json:"count"`

	// RemediationRequestRef is the name of the RemediationRequest CRD
	// Used in HTTP 202 response to inform caller which CRD was updated
	RemediationRequestRef string `json:"remediationRequestRef,omitempty"`

	// FirstOccurrence is when the alert first appeared (ISO 8601 timestamp)
	// Example: "2025-10-09T10:00:00Z"
	FirstOccurrence string `json:"firstOccurrence,omitempty"`

	// LastOccurrence is the most recent occurrence (ISO 8601 timestamp)
	// Updated every time a duplicate is detected
	// Example: "2025-10-09T10:04:30Z"
	LastOccurrence string `json:"lastOccurrence,omitempty"`
}

