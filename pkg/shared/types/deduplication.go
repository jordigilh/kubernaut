// Package types provides shared types used across multiple Kubernaut CRDs.
// These types ensure API contract alignment between services.
package types

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DeduplicationInfo tracks duplicate signal suppression.
// Shared between RemediationRequest and SignalProcessing CRDs.
// See: RO team API Contract Alignment decision.
type DeduplicationInfo struct {
	// True if this signal is a duplicate of an active remediation
	IsDuplicate bool `json:"isDuplicate,omitempty"`

	// Timestamp when this signal fingerprint was first seen
	FirstOccurrence metav1.Time `json:"firstOccurrence"`

	// Timestamp when this signal fingerprint was last seen
	LastOccurrence metav1.Time `json:"lastOccurrence"`

	// Total count of occurrences of this signal
	OccurrenceCount int `json:"occurrenceCount"`

	// Optional correlation ID for grouping related signals
	CorrelationID string `json:"correlationId,omitempty"`

	// Reference to previous RemediationRequest CRD (if duplicate)
	PreviousRemediationRequestRef string `json:"previousRemediationRequestRef,omitempty"`
}
