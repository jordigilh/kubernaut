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

// Package builders provides test object builders for creating test fixtures.
// Reference: REFACTOR-RO-007
package builders

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// ========================================
// REMEDIATION REQUEST BUILDER (REFACTOR-RO-007)
// ========================================
//
// RemediationRequestBuilder provides a fluent API for building RemediationRequest test fixtures.
//
// WHY Test Builders?
// - ✅ Reduce test boilerplate (DRY principle)
// - ✅ Fluent API improves test readability
// - ✅ Default values eliminate repetitive setup
// - ✅ Easy to extend with new fields
// - ✅ Type-safe construction
//
// Usage:
//   rr := builders.NewRemediationRequest("test-rr", "default").
//       WithSignalFingerprint("abc123...").
//       WithSeverity("high").
//       WithPhase(remediationv1.PhaseProcessing).
//       Build()
//
// Reference: REFACTOR-RO-007

// RemediationRequestBuilder builds RemediationRequest objects for tests.
type RemediationRequestBuilder struct {
	rr *remediationv1.RemediationRequest
}

// NewRemediationRequest creates a new builder with default values.
func NewRemediationRequest(name, namespace string) *RemediationRequestBuilder {
	return &RemediationRequestBuilder{
		rr: &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "0000000000000000000000000000000000000000000000000000000000000000", // Default 64-char hex
				SignalName:        "default-signal",
				Severity:          "medium",
				TargetType:        "pod",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "default-resource",
					Namespace: "default",
				},
				FiringTime:   metav1.Now(),
				ReceivedTime: metav1.Now(),
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhasePending,
			},
		},
	}
}

// WithSignalFingerprint sets the signal fingerprint.
func (b *RemediationRequestBuilder) WithSignalFingerprint(fingerprint string) *RemediationRequestBuilder {
	b.rr.Spec.SignalFingerprint = fingerprint
	return b
}

// WithSignalName sets the signal name.
func (b *RemediationRequestBuilder) WithSignalName(name string) *RemediationRequestBuilder {
	b.rr.Spec.SignalName = name
	return b
}

// WithSeverity sets the severity.
func (b *RemediationRequestBuilder) WithSeverity(severity string) *RemediationRequestBuilder {
	b.rr.Spec.Severity = severity
	return b
}

// WithTargetType sets the target type.
func (b *RemediationRequestBuilder) WithTargetType(targetType string) *RemediationRequestBuilder {
	b.rr.Spec.TargetType = targetType
	return b
}

// WithTargetResource sets the target resource identifier.
func (b *RemediationRequestBuilder) WithTargetResource(resource remediationv1.ResourceIdentifier) *RemediationRequestBuilder {
	b.rr.Spec.TargetResource = resource
	return b
}

// WithPhase sets the overall phase.
func (b *RemediationRequestBuilder) WithPhase(phase remediationv1.RemediationPhase) *RemediationRequestBuilder {
	b.rr.Status.OverallPhase = phase
	return b
}

// WithMessage sets the status message.
func (b *RemediationRequestBuilder) WithMessage(message string) *RemediationRequestBuilder {
	b.rr.Status.Message = message
	return b
}

// WithSkipReason sets the skip reason.
func (b *RemediationRequestBuilder) WithSkipReason(reason string) *RemediationRequestBuilder {
	b.rr.Status.SkipReason = reason
	return b
}

// WithDuplicateOf sets the parent RR for duplicates.
func (b *RemediationRequestBuilder) WithDuplicateOf(parent string) *RemediationRequestBuilder {
	b.rr.Status.DuplicateOf = parent
	return b
}

// WithRequiresManualReview sets whether manual review is required.
func (b *RemediationRequestBuilder) WithRequiresManualReview(required bool) *RemediationRequestBuilder {
	b.rr.Status.RequiresManualReview = required
	return b
}

// WithConsecutiveFailureCount sets the consecutive failure count.
func (b *RemediationRequestBuilder) WithConsecutiveFailureCount(count int32) *RemediationRequestBuilder {
	b.rr.Status.ConsecutiveFailureCount = count
	return b
}

// WithBlockReason sets the block reason.
func (b *RemediationRequestBuilder) WithBlockReason(reason string) *RemediationRequestBuilder {
	b.rr.Status.BlockReason = reason
	return b
}

// WithBlockedUntil sets the blocked until timestamp.
func (b *RemediationRequestBuilder) WithBlockedUntil(until metav1.Time) *RemediationRequestBuilder {
	b.rr.Status.BlockedUntil = &until
	return b
}

// WithLabels sets custom labels.
func (b *RemediationRequestBuilder) WithLabels(labels map[string]string) *RemediationRequestBuilder {
	if b.rr.Labels == nil {
		b.rr.Labels = make(map[string]string)
	}
	for k, v := range labels {
		b.rr.Labels[k] = v
	}
	return b
}

// WithAnnotations sets custom annotations.
func (b *RemediationRequestBuilder) WithAnnotations(annotations map[string]string) *RemediationRequestBuilder {
	if b.rr.Annotations == nil {
		b.rr.Annotations = make(map[string]string)
	}
	for k, v := range annotations {
		b.rr.Annotations[k] = v
	}
	return b
}

// Build returns the constructed RemediationRequest.
func (b *RemediationRequestBuilder) Build() *remediationv1.RemediationRequest {
	return b.rr
}
