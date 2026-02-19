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

// Package aianalysis contains unit tests for PostRCAContext population
// in the ResponseProcessor.
//
// ADR-056: DetectedLabels are computed by HAPI's LabelDetector and returned
// in the response. The ResponseProcessor extracts them into PostRCAContext
// on AIAnalysisStatus for Rego policy input and immutability enforcement.
//
// Business Requirements:
//   - BR-AI-056: DetectedLabels in AIAnalysis CRD status (PostRCAContext)
//   - BR-AI-082: Recovery flow support (PostRCAContext for recovery responses)
package aianalysis

import (
	"context"

	"github.com/go-faster/jx"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	client "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

var _ = Describe("ResponseProcessor PostRCAContext Population (ADR-056)", func() {
	var (
		processor *handlers.ResponseProcessor
		analysis  *aianalysisv1.AIAnalysis
		ctx       context.Context
		m         *metrics.Metrics
	)

	BeforeEach(func() {
		m = metrics.NewMetrics()
		processor = handlers.NewResponseProcessor(logr.Discard(), m, &noopAuditClient{})
		ctx = context.Background()
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-056-003: ProcessIncidentResponse populates PostRCAContext
	// ADR-056: DetectedLabels from HAPI response → PostRCAContext
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-056-003: should populate PostRCAContext.DetectedLabels from incident response", func() {
		// GIVEN: An AIAnalysis in Investigating phase with no PostRCAContext
		analysis = createAnalysisForPostRCA()
		Expect(analysis.Status.PostRCAContext).To(BeNil(), "PostRCAContext must be nil initially")

		// AND: A successful HAPI incident response with detected_labels
		hapiResp := buildIncidentResponseWithDetectedLabels(map[string]jx.Raw{
			"gitOpsManaged":   jx.Raw(`true`),
			"pdbProtected":    jx.Raw(`true`),
			"hpaEnabled":      jx.Raw(`false`),
			"stateful":        jx.Raw(`true`),
			"helmManaged":     jx.Raw(`false`),
			"networkIsolated": jx.Raw(`true`),
			"serviceMesh":     jx.Raw(`"istio"`),
			"gitOpsTool":      jx.Raw(`"argocd"`),
		})

		// WHEN: Processing the incident response
		_, err := processor.ProcessIncidentResponse(ctx, analysis, hapiResp)

		// THEN: PostRCAContext should be populated
		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.PostRCAContext).ToNot(BeNil(),
			"ADR-056: PostRCAContext must be populated when detected_labels present")
		Expect(analysis.Status.PostRCAContext.DetectedLabels).ToNot(BeNil(),
			"ADR-056: DetectedLabels must be set within PostRCAContext")

		// AND: Individual label values must match HAPI response
		dl := analysis.Status.PostRCAContext.DetectedLabels
		Expect(dl.GitOpsManaged).To(BeTrue(), "gitOpsManaged must be true")
		Expect(dl.PDBProtected).To(BeTrue(), "pdbProtected must be true")
		Expect(dl.HPAEnabled).To(BeFalse(), "hpaEnabled must be false")
		Expect(dl.Stateful).To(BeTrue(), "stateful must be true")
		Expect(dl.HelmManaged).To(BeFalse(), "helmManaged must be false")
		Expect(dl.NetworkIsolated).To(BeTrue(), "networkIsolated must be true")
		Expect(dl.ServiceMesh).To(Equal("istio"), "serviceMesh must be istio")
		Expect(dl.GitOpsTool).To(Equal("argocd"), "gitOpsTool must be argocd")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-056-004: ProcessRecoveryResponse populates PostRCAContext
	// ADR-056: Same extraction logic for recovery flow
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-056-004: should populate PostRCAContext.DetectedLabels from recovery response", func() {
		// GIVEN: An AIAnalysis in Investigating phase with no PostRCAContext
		analysis = createAnalysisForPostRCA()
		Expect(analysis.Status.PostRCAContext).To(BeNil(), "PostRCAContext must be nil initially")

		// AND: A successful HAPI recovery response with detected_labels
		recoveryResp := buildRecoveryResponseWithDetectedLabels(map[string]jx.Raw{
			"gitOpsManaged":   jx.Raw(`false`),
			"pdbProtected":    jx.Raw(`false`),
			"hpaEnabled":      jx.Raw(`true`),
			"stateful":        jx.Raw(`false`),
			"helmManaged":     jx.Raw(`true`),
			"networkIsolated": jx.Raw(`false`),
			"serviceMesh":     jx.Raw(`""`),
		})

		// WHEN: Processing the recovery response
		_, err := processor.ProcessRecoveryResponse(ctx, analysis, recoveryResp)

		// THEN: PostRCAContext should be populated
		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.PostRCAContext).ToNot(BeNil(),
			"ADR-056: PostRCAContext must be populated from recovery response")
		Expect(analysis.Status.PostRCAContext.DetectedLabels).ToNot(BeNil(),
			"ADR-056: DetectedLabels must be set from recovery response")

		// AND: Values must match
		dl := analysis.Status.PostRCAContext.DetectedLabels
		Expect(dl.HPAEnabled).To(BeTrue(), "hpaEnabled must be true")
		Expect(dl.HelmManaged).To(BeTrue(), "helmManaged must be true")
		Expect(dl.Stateful).To(BeFalse(), "stateful must be false")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-056-005: SetAt timestamp is populated for immutability guard
	// ADR-056 + CEL: PostRCAContext becomes immutable once SetAt is non-nil
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-056-005: should set PostRCAContext.SetAt timestamp when detected_labels present", func() {
		// GIVEN: An AIAnalysis with no PostRCAContext
		analysis = createAnalysisForPostRCA()

		// AND: A HAPI incident response with detected_labels
		hapiResp := buildIncidentResponseWithDetectedLabels(map[string]jx.Raw{
			"stateful": jx.Raw(`true`),
		})

		// WHEN: Processing the incident response
		_, err := processor.ProcessIncidentResponse(ctx, analysis, hapiResp)

		// THEN: SetAt must be non-nil (immutability guard)
		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.PostRCAContext).ToNot(BeNil())
		Expect(analysis.Status.PostRCAContext.SetAt).ToNot(BeNil(),
			"ADR-056: SetAt must be populated for CEL immutability guard")
		Expect(analysis.Status.PostRCAContext.SetAt.Time).ToNot(BeZero(),
			"SetAt timestamp must be a valid time")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-056-006: PostRCAContext is nil when detected_labels absent
	// ADR-056: No labels in HAPI response means PostRCAContext stays nil
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-056-006: should leave PostRCAContext nil when detected_labels absent from response", func() {
		// GIVEN: An AIAnalysis with no PostRCAContext
		analysis = createAnalysisForPostRCA()

		// AND: A successful HAPI incident response WITHOUT detected_labels
		hapiResp := &client.IncidentResponse{
			IncidentID:       "test-no-labels-001",
			Analysis:         "Test analysis",
			NeedsHumanReview: client.NewOptBool(false),
			Confidence:       0.85,
			Timestamp:        "2026-02-17T12:00:00Z",
			SelectedWorkflow: client.OptNilIncidentResponseSelectedWorkflow{
				Value: client.IncidentResponseSelectedWorkflow{
					"workflow_id":      jx.Raw(`"restart-pod-v1"`),
					"execution_bundle": jx.Raw(`"ghcr.io/kubernaut/restart-pod:v1.0"`),
					"confidence":       jx.Raw(`0.85`),
				},
				Set: true,
			},
		}

		// WHEN: Processing the incident response
		_, err := processor.ProcessIncidentResponse(ctx, analysis, hapiResp)

		// THEN: PostRCAContext should remain nil (no labels to extract)
		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.PostRCAContext).To(BeNil(),
			"PostRCAContext must remain nil when no detected_labels in HAPI response")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-056-007: PostRCAContext handles failedDetections array
	// DD-WORKFLOW-001 v2.2: Detection failure tracking via failedDetections
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-056-007: should propagate failedDetections from HAPI response to PostRCAContext", func() {
		// GIVEN: An AIAnalysis with no PostRCAContext
		analysis = createAnalysisForPostRCA()

		// AND: A HAPI response where some detections failed (RBAC denied)
		hapiResp := buildIncidentResponseWithDetectedLabels(map[string]jx.Raw{
			"gitOpsManaged":    jx.Raw(`false`),
			"pdbProtected":     jx.Raw(`false`),
			"hpaEnabled":       jx.Raw(`false`),
			"stateful":         jx.Raw(`true`),
			"helmManaged":      jx.Raw(`false`),
			"networkIsolated":  jx.Raw(`false`),
			"failedDetections": jx.Raw(`["pdbProtected","hpaEnabled"]`),
		})

		// WHEN: Processing the incident response
		_, err := processor.ProcessIncidentResponse(ctx, analysis, hapiResp)

		// THEN: PostRCAContext should be populated with failedDetections
		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.PostRCAContext).ToNot(BeNil())
		Expect(analysis.Status.PostRCAContext.DetectedLabels).ToNot(BeNil())

		dl := analysis.Status.PostRCAContext.DetectedLabels
		Expect(dl.FailedDetections).To(ConsistOf("pdbProtected", "hpaEnabled"),
			"failedDetections must be propagated from HAPI response")
		Expect(dl.PDBProtected).To(BeFalse(),
			"pdbProtected should be false (but in failedDetections, so value is unreliable)")
		Expect(dl.Stateful).To(BeTrue(),
			"stateful should be true (reliable, not in failedDetections)")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-056-008: Malformed detected_labels handled gracefully
	// ADR-056: When detected_labels is not a valid object, skip extraction, no panic
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-056-008: should handle malformed detected_labels gracefully without panic", func() {
		// GIVEN: An AIAnalysis with no PostRCAContext
		analysis = createAnalysisForPostRCA()

		// AND: A HAPI incident response with detected_labels Set but Value is nil/invalid
		// (simulates API returning non-object e.g. string "not_a_dict" - client would pass nil)
		hapiResp := &client.IncidentResponse{
			IncidentID:       "test-malformed-labels-001",
			Analysis:         "Test analysis",
			NeedsHumanReview: client.NewOptBool(false),
			Confidence:       0.85,
			Timestamp:        "2026-02-17T12:00:00Z",
			SelectedWorkflow: client.OptNilIncidentResponseSelectedWorkflow{
				Value: client.IncidentResponseSelectedWorkflow{
					"workflow_id":      jx.Raw(`"restart-pod-v1"`),
					"execution_bundle": jx.Raw(`"ghcr.io/kubernaut/restart-pod:v1.0"`),
					"confidence":       jx.Raw(`0.85`),
				},
				Set: true,
			},
			DetectedLabels: client.OptNilIncidentResponseDetectedLabels{
				Value: nil, // malformed - not a valid map (e.g. API returned string)
				Set:   true,
				Null:  false,
			},
		}

		// WHEN: Processing the incident response (must not panic)
		_, err := processor.ProcessIncidentResponse(ctx, analysis, hapiResp)

		// THEN: No error, PostRCAContext remains nil (malformed data skipped)
		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.PostRCAContext).To(BeNil(),
			"PostRCAContext must remain nil when detected_labels is malformed")
	})
})

// ═══════════════════════════════════════════════════════════════════════
// Test Helpers
// ═══════════════════════════════════════════════════════════════════════

func createAnalysisForPostRCA() *aianalysisv1.AIAnalysis {
	return &aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-post-rca",
			Namespace: "default",
			UID:       types.UID("test-uid-prc-001"),
		},
		Spec: aianalysisv1.AIAnalysisSpec{
			RemediationID: "test-rr-prc-001",
		},
		Status: aianalysisv1.AIAnalysisStatus{
			Phase: aianalysis.PhaseInvestigating,
		},
	}
}

func buildIncidentResponseWithDetectedLabels(labels map[string]jx.Raw) *client.IncidentResponse {
	return &client.IncidentResponse{
		IncidentID:       "test-with-labels-001",
		Analysis:         "Root cause: memory pressure",
		NeedsHumanReview: client.NewOptBool(false),
		Confidence:       0.90,
		Timestamp:        "2026-02-17T12:00:00Z",
		SelectedWorkflow: client.OptNilIncidentResponseSelectedWorkflow{
			Value: client.IncidentResponseSelectedWorkflow{
				"workflow_id":      jx.Raw(`"restart-pod-v1"`),
				"execution_bundle": jx.Raw(`"ghcr.io/kubernaut/restart-pod:v1.0"`),
				"confidence":       jx.Raw(`0.90`),
			},
			Set: true,
		},
		DetectedLabels: client.NewOptNilIncidentResponseDetectedLabels(
			client.IncidentResponseDetectedLabels(labels),
		),
	}
}

func buildRecoveryResponseWithDetectedLabels(labels map[string]jx.Raw) *client.RecoveryResponse {
	return &client.RecoveryResponse{
		IncidentID:         "test-recovery-with-labels-001",
		CanRecover:         true,
		AnalysisConfidence: 0.85,
		SelectedWorkflow: client.OptNilRecoveryResponseSelectedWorkflow{
			Value: client.RecoveryResponseSelectedWorkflow{
				"workflow_id":      jx.Raw(`"scale-up-v1"`),
				"execution_bundle": jx.Raw(`"ghcr.io/kubernaut/scale-up:v1.0"`),
				"confidence":       jx.Raw(`0.85`),
			},
			Set: true,
		},
		DetectedLabels: client.NewOptNilRecoveryResponseDetectedLabels(
			client.RecoveryResponseDetectedLabels(labels),
		),
	}
}
