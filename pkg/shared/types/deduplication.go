// Package types provides shared types used across multiple Kubernaut CRDs.
// These types ensure API contract alignment between services.
package types

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DeduplicationInfo tracks duplicate signal suppression.
// Shared between RemediationRequest and SignalProcessing CRDs.
// See: RO team API Contract Alignment decision.
// DD-GATEWAY-011: Fields made optional for backward compatibility (Gateway Team Fix 2025-12-12)
type DeduplicationInfo struct {
	// True if this signal is a duplicate of an active remediation
	IsDuplicate bool `json:"isDuplicate,omitempty"`

	// Timestamp when this signal fingerprint was first seen
	// DD-GATEWAY-011: Made optional (moved to status.deduplication)
	FirstOccurrence metav1.Time `json:"firstOccurrence,omitempty"`

	// Timestamp when this signal fingerprint was last seen
	// DD-GATEWAY-011: Made optional (moved to status.deduplication)
	LastOccurrence metav1.Time `json:"lastOccurrence,omitempty"`

	// Total count of occurrences of this signal
	// DD-GATEWAY-011: Made optional (moved to status.deduplication)
	OccurrenceCount int `json:"occurrenceCount,omitempty"`

	// Optional correlation ID for grouping related signals
	CorrelationID string `json:"correlationId,omitempty"`

	// Reference to previous RemediationRequest CRD (if duplicate)
	PreviousRemediationRequestRef string `json:"previousRemediationRequestRef,omitempty"`
}
