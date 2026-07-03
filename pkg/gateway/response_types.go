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

package gateway

import (
	"fmt"
	"time"

	// BR-GATEWAY-093: Circuit breaker detection

	// DD-AUDIT-003: Audit integration
	// Ogen generated audit types
	// BR-AUDIT-005 Gap #7: Standardized error details
	// BR-GATEWAY-036/037: Shared auth middleware
	// ADR-052 Addendum 001: Exponential backoff with jitter
	// Issue #753: Dedicated health server
	// Issue #756: FileWatcher for cert rotation
	// Issue #493/#678: Conditional TLS

	// BR-GATEWAY-190: Lease resources for distributed locking

	// BR-GATEWAY-036/037: K8s clientset for TokenReview/SAR

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1" // ADR-068: Federated scope checking factory
	// BR-109: Request ID middleware
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	// BR-HTTP-015: Shared CORS library
	// DD-005: Shared sanitization library
	// BR-SCOPE-002: Resource scope management
)

// ProcessingResponse represents the result of signal processing
//
// Note: Environment and Priority fields removed from response (2025-12-06)
// These classifications are now owned by Signal Processing service per DD-CATEGORIZATION-001.
// AlertManager/webhook callers don't need this information - they only need to know
// if the alert was accepted (HTTP status code).
type ProcessingResponse struct {
	Status                      string                            `json:"status"` // "created", "duplicate", or "rejected"
	Message                     string                            `json:"message"`
	Fingerprint                 string                            `json:"fingerprint"`
	Duplicate                   bool                              `json:"duplicate"`
	RemediationRequestName      string                            `json:"remediationRequestName,omitempty"`
	RemediationRequestNamespace string                            `json:"remediationRequestNamespace,omitempty"`
	Metadata                    *processing.DeduplicationMetadata `json:"metadata,omitempty"`
	// BR-SCOPE-002: Rejection details for unmanaged resource signals
	Rejection *RejectionResponse `json:"rejection,omitempty"`
}

// Processing status constants (HTTP response body status field)
// Aligned with OpenAPI enum values for consistency (no backwards compatibility needed)
const (
	StatusCreated      = "created"   // RemediationRequest CRD created
	StatusDeduplicated = "duplicate" // Signal deduplicated to existing RR (matches OpenAPI enum)
)

// BatchProcessingResponse is the JSON response for adapters that implement BatchParser.
// Returned with HTTP 207 Multi-Status to indicate per-alert independent outcomes.
// NOTE: This is a JSON-encoded body (Content-Type: application/json), not the
// RFC 4918 (WebDAV) XML multi-status format.
type BatchProcessingResponse struct {
	Results []ProcessingResult `json:"results"`
	Summary BatchSummary       `json:"summary"`
}

// ProcessingResult represents the outcome of processing a single signal within a batch.
type ProcessingResult struct {
	Status      string `json:"status"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Message     string `json:"message,omitempty"`
	Error       string `json:"error,omitempty"`
}

// BatchSummary provides aggregate counts for a batch processing response.
type BatchSummary struct {
	Total        int `json:"total"`
	Created      int `json:"created"`
	Deduplicated int `json:"deduplicated"`
	Rejected     int `json:"rejected"`
	Failed       int `json:"failed"`
}

// BatchSignalOutcome classifies the result of processing one signal within a
// multi-signal batch (#1036), used to increment the right BatchSummary counter.
type BatchSignalOutcome int

const (
	BatchOutcomeFailed BatchSignalOutcome = iota
	BatchOutcomeRejected
	BatchOutcomeCreated
	BatchOutcomeDeduplicated
)

// record increments the counter matching outcome. Extracted from
// processMultiSignalBatch's per-signal loop (funlen).
func (s *BatchSummary) record(outcome BatchSignalOutcome) {
	switch outcome {
	case BatchOutcomeFailed:
		s.Failed++
	case BatchOutcomeRejected:
		s.Rejected++
	case BatchOutcomeCreated:
		s.Created++
	case BatchOutcomeDeduplicated:
		s.Deduplicated++
	}
}

// NewDuplicateResponseFromRR creates a ProcessingResponse for duplicate signals using K8s RR data
// DD-GATEWAY-011: Status-based deduplication (Redis deprecation)
// BR-GATEWAY-185: All dedup state from K8s status, not Redis
func NewDuplicateResponseFromRR(fingerprint string, rr *remediationv1alpha1.RemediationRequest) *ProcessingResponse {
	// Build metadata from RR status (DD-GATEWAY-011: status-based tracking)
	var occurrenceCount int
	var firstOccurrence, lastOccurrence string

	if rr.Status.Deduplication != nil {
		occurrenceCount = int(rr.Status.Deduplication.OccurrenceCount)
		if rr.Status.Deduplication.FirstSeenAt != nil {
			firstOccurrence = rr.Status.Deduplication.FirstSeenAt.Format(time.RFC3339)
		}
		if rr.Status.Deduplication.LastSeenAt != nil {
			lastOccurrence = rr.Status.Deduplication.LastSeenAt.Format(time.RFC3339)
		}
	}

	return &ProcessingResponse{
		Status:                      StatusDeduplicated,
		Message:                     "Duplicate signal (K8s status-based deduplication)",
		Fingerprint:                 fingerprint,
		Duplicate:                   true,
		RemediationRequestName:      rr.Name,
		RemediationRequestNamespace: rr.Namespace,
		Metadata: &processing.DeduplicationMetadata{
			Count:                 occurrenceCount,
			FirstOccurrence:       firstOccurrence,
			LastOccurrence:        lastOccurrence,
			RemediationRequestRef: fmt.Sprintf("%s/%s", rr.Namespace, rr.Name),
		},
	}
}

// NewCRDCreatedResponse creates a ProcessingResponse for successful CRD creation
// TDD REFACTOR: Extracted factory function for CRD creation response pattern
// Business Outcome: Consistent CRD creation handling (BR-004)
//
// Note: Environment, Priority, and RemediationPath parameters removed (2025-12-06)
// Classification and path decision now owned by Signal Processing service
// per DD-CATEGORIZATION-001 and DD-WORKFLOW-001 (risk_tolerance in CustomLabels)
func NewCRDCreatedResponse(fingerprint, crdName, crdNamespace string) *ProcessingResponse {
	return &ProcessingResponse{
		Status:                      StatusCreated,
		Message:                     "RemediationRequest CRD created successfully",
		Fingerprint:                 fingerprint,
		Duplicate:                   false,
		RemediationRequestName:      crdName,
		RemediationRequestNamespace: crdNamespace,
	}
}
