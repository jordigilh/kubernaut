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
// ADR-056: DetectedLabels are computed by KA's LabelDetector and returned
// in the response. The ResponseProcessor extracts them into PostRCAContext
// on AIAnalysisStatus for Rego policy input and immutability enforcement.
//
// Business Requirements:
//   - BR-AI-056: DetectedLabels in AIAnalysis CRD status (PostRCAContext)
package aianalysis_test

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
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
	// UT-AA-056-003: ProcessAgentSessionResult populates PostRCAContext
	// ADR-056: DetectedLabels from KA response → PostRCAContext
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-056-003: should populate PostRCAContext.DetectedLabels from the AgentSessionResult", func() {
		// GIVEN: An AIAnalysis in Investigating phase with no PostRCAContext
		analysis = createAnalysisForPostRCA()
		Expect(analysis.Status.PostRCAContext).To(BeNil(), "PostRCAContext must be nil initially")

		// AND: A successful KA result with detected_labels
		res := buildResultWithDetectedLabels(map[string]interface{}{
			"gitOpsManaged":   true,
			"pdbProtected":    true,
			"hpaEnabled":      false,
			"stateful":        true,
			"helmManaged":     false,
			"networkIsolated": true,
			"serviceMesh":     "istio",
			"gitOpsTool":      "argocd",
		})

		// WHEN: Processing the result
		_, err := processor.ProcessAgentSessionResult(ctx, analysis, res)

		// THEN: PostRCAContext should be populated
		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.PostRCAContext.DetectedLabels.GitOpsManaged).To(BeTrue(),
			"ADR-056: PostRCAContext.DetectedLabels must be populated when detected_labels present")

		// AND: Individual label values must match KA response
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
	// UT-AA-056-005: SetAt timestamp is populated for immutability guard
	// ADR-056 + CEL: PostRCAContext becomes immutable once SetAt is non-nil
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-056-005: should set PostRCAContext.SetAt timestamp when detected_labels present", func() {
		// GIVEN: An AIAnalysis with no PostRCAContext
		analysis = createAnalysisForPostRCA()

		// AND: A KA result with detected_labels
		res := buildResultWithDetectedLabels(map[string]interface{}{
			"stateful": true,
		})

		// WHEN: Processing the result
		_, err := processor.ProcessAgentSessionResult(ctx, analysis, res)

		// THEN: SetAt must be non-nil (immutability guard)
		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.PostRCAContext.SetAt).ToNot(BeNil(),
			"ADR-056: SetAt must be populated for CEL immutability guard")
		Expect(analysis.Status.PostRCAContext.SetAt.Time).ToNot(BeZero(),
			"SetAt timestamp must be a valid time")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-056-006: PostRCAContext is nil when detected_labels absent
	// ADR-056: No labels in KA response means PostRCAContext stays nil
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-056-006: should leave PostRCAContext nil when detected_labels absent from response", func() {
		// GIVEN: An AIAnalysis with no PostRCAContext
		analysis = createAnalysisForPostRCA()

		// AND: A successful KA result WITHOUT detected_labels
		res := &agentsessionv1.AgentSessionResult{
			IncidentID:       "test-no-labels-001",
			Analysis:         "Test analysis",
			NeedsHumanReview: false,
			Confidence:       0.85,
			Timestamp:        "2026-02-17T12:00:00Z",
			SelectedWorkflow: rawJSONPostRCA(map[string]interface{}{
				"workflow_id":      "restart-pod-v1",
				"execution_bundle": "ghcr.io/kubernaut/restart-pod:v1.0",
				"confidence":       0.85,
			}),
		}

		// WHEN: Processing the result
		_, err := processor.ProcessAgentSessionResult(ctx, analysis, res)

		// THEN: PostRCAContext should remain nil (no labels to extract)
		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.PostRCAContext).To(BeNil(),
			"PostRCAContext must remain nil when no detected_labels in KA response")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-056-007: PostRCAContext handles failedDetections array
	// DD-WORKFLOW-001 v2.2: Detection failure tracking via failedDetections
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-056-007: should propagate failedDetections from KA response to PostRCAContext", func() {
		// GIVEN: An AIAnalysis with no PostRCAContext
		analysis = createAnalysisForPostRCA()

		// AND: A KA response where some detections failed (RBAC denied)
		res := buildResultWithDetectedLabels(map[string]interface{}{
			"gitOpsManaged":    false,
			"pdbProtected":     false,
			"hpaEnabled":       false,
			"stateful":         true,
			"helmManaged":      false,
			"networkIsolated":  false,
			"failedDetections": []string{"pdbProtected", "hpaEnabled"},
		})

		// WHEN: Processing the result
		_, err := processor.ProcessAgentSessionResult(ctx, analysis, res)

		// THEN: PostRCAContext should be populated with failedDetections
		Expect(err).ToNot(HaveOccurred())
		dl := analysis.Status.PostRCAContext.DetectedLabels
		Expect(dl.FailedDetections).To(ConsistOf("pdbProtected", "hpaEnabled"),
			"failedDetections must be propagated from KA response")
		Expect(dl.PDBProtected).To(BeFalse(),
			"pdbProtected should be false (but in failedDetections, so value is unreliable)")
		Expect(dl.Stateful).To(BeTrue(),
			"stateful should be true (reliable, not in failedDetections)")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-366-001: ResourceQuotaConstrained + FailedDetections round-trip
	// #366: Detect namespace ResourceQuota and surface to LLM
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-366-001: should round-trip ResourceQuotaConstrained and failedDetections from KA response", func() {
		// GIVEN: An AIAnalysis in Investigating phase with no PostRCAContext
		analysis = createAnalysisForPostRCA()
		Expect(analysis.Status.PostRCAContext).To(BeNil(), "PostRCAContext must be nil initially")

		// AND: A KA result with resourceQuotaConstrained=true and failedDetections including it
		res := buildResultWithDetectedLabels(map[string]interface{}{
			"gitOpsManaged":            false,
			"pdbProtected":             false,
			"hpaEnabled":               false,
			"stateful":                 false,
			"helmManaged":              false,
			"networkIsolated":          false,
			"serviceMesh":              "",
			"resourceQuotaConstrained": true,
			"failedDetections":         []string{"resourceQuotaConstrained"},
		})

		// WHEN: Processing the result
		_, err := processor.ProcessAgentSessionResult(ctx, analysis, res)

		// THEN: PostRCAContext should be populated with the new field
		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.PostRCAContext).ToNot(BeNil(),
			"PostRCAContext must be populated when detected_labels present")
		dl := analysis.Status.PostRCAContext.DetectedLabels
		Expect(dl.ResourceQuotaConstrained).To(BeTrue(),
			"#366: ResourceQuotaConstrained must round-trip as true")
		Expect(dl.FailedDetections).To(ContainElement("resourceQuotaConstrained"),
			"#366: failedDetections must include resourceQuotaConstrained")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// IT-AA-RESP-PARAMS-001: SelectedWorkflow.Parameters mapped from KA response
	// Verifies that parameters in KA's selected_workflow JSON (including
	// TARGET_RESOURCE_*) propagate into AIAnalysis.Status.SelectedWorkflow.Parameters.
	// ═══════════════════════════════════════════════════════════════════════

	It("IT-AA-RESP-PARAMS-001: should map parameters from KA selected_workflow to AIAnalysis status", func() {
		analysis = createAnalysisForPostRCA()

		res := &agentsessionv1.AgentSessionResult{
			IncidentID:       "test-params-001",
			Analysis:         "Root cause: memory pressure on api-server",
			NeedsHumanReview: false,
			Confidence:       0.92,
			Timestamp:        "2026-05-30T12:00:00Z",
			SelectedWorkflow: rawJSONPostRCA(map[string]interface{}{
				"workflow_id":      "increase-memory-v1",
				"execution_bundle": "ghcr.io/kubernaut/increase-memory:v1.0",
				"confidence":       0.92,
				"parameters": map[string]interface{}{
					"TARGET_RESOURCE_NAME":      "memory-eater",
					"TARGET_RESOURCE_KIND":      "Deployment",
					"TARGET_RESOURCE_NAMESPACE": "production",
					"MEMORY_LIMIT_NEW":          "512Mi",
				},
			}),
		}

		_, err := processor.ProcessAgentSessionResult(ctx, analysis, res)
		Expect(err).ToNot(HaveOccurred())

		Expect(analysis.Status.GetRCAResult().SelectedWorkflow).NotTo(BeNil(),
			"SelectedWorkflow must be populated after successful processing")
		Expect(analysis.Status.GetRCAResult().SelectedWorkflow.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_NAME", "memory-eater"),
			"TARGET_RESOURCE_NAME must round-trip from KA response to AIAnalysis status")
		Expect(analysis.Status.GetRCAResult().SelectedWorkflow.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_KIND", "Deployment"),
			"TARGET_RESOURCE_KIND must round-trip from KA response to AIAnalysis status")
		Expect(analysis.Status.GetRCAResult().SelectedWorkflow.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_NAMESPACE", "production"),
			"TARGET_RESOURCE_NAMESPACE must round-trip from KA response to AIAnalysis status")
		Expect(analysis.Status.GetRCAResult().SelectedWorkflow.Parameters).To(HaveKeyWithValue("MEMORY_LIMIT_NEW", "512Mi"),
			"LLM-provided MEMORY_LIMIT_NEW must round-trip from KA response to AIAnalysis status")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-056-008: Malformed detected_labels handled gracefully
	// ADR-056: When detected_labels is not a valid object, skip extraction, no panic
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-056-008: should handle malformed detected_labels gracefully without panic", func() {
		// GIVEN: An AIAnalysis with no PostRCAContext
		analysis = createAnalysisForPostRCA()

		// AND: A KA result with DetectedLabels set but containing invalid JSON
		// (simulates the API returning a non-object, e.g. a bare string)
		res := &agentsessionv1.AgentSessionResult{
			IncidentID:       "test-malformed-labels-001",
			Analysis:         "Test analysis",
			NeedsHumanReview: false,
			Confidence:       0.85,
			Timestamp:        "2026-02-17T12:00:00Z",
			SelectedWorkflow: rawJSONPostRCA(map[string]interface{}{
				"workflow_id":      "restart-pod-v1",
				"execution_bundle": "ghcr.io/kubernaut/restart-pod:v1.0",
				"confidence":       0.85,
			}),
			DetectedLabels: &apiextensionsv1.JSON{Raw: []byte(`"not_a_dict"`)}, // malformed - not a valid map
		}

		// WHEN: Processing the result (must not panic)
		_, err := processor.ProcessAgentSessionResult(ctx, analysis, res)

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

// rawJSONPostRCA marshals v to *apiextensionsv1.JSON, panicking on failure (test-only helper).
func rawJSONPostRCA(v interface{}) *apiextensionsv1.JSON {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return &apiextensionsv1.JSON{Raw: raw}
}

func buildResultWithDetectedLabels(labels map[string]interface{}) *agentsessionv1.AgentSessionResult {
	return &agentsessionv1.AgentSessionResult{
		IncidentID:       "test-with-labels-001",
		Analysis:         "Root cause: memory pressure",
		NeedsHumanReview: false,
		Confidence:       0.90,
		Timestamp:        "2026-02-17T12:00:00Z",
		SelectedWorkflow: rawJSONPostRCA(map[string]interface{}{
			"workflow_id":      "restart-pod-v1",
			"execution_bundle": "ghcr.io/kubernaut/restart-pod:v1.0",
			"confidence":       0.90,
		}),
		DetectedLabels: rawJSONPostRCA(labels),
	}
}

// ═══════════════════════════════════════════════════════════════════════
// #1400: CNV Label Extraction Tests
// ═══════════════════════════════════════════════════════════════════════

var _ = Describe("CNV Label Extraction — #1400", func() {
	var (
		processor *handlers.ResponseProcessor
		analysis  *aianalysisv1.AIAnalysis
		ctx       context.Context
	)

	BeforeEach(func() {
		m := metrics.NewMetrics()
		processor = handlers.NewResponseProcessor(logr.Discard(), m, &noopAuditClient{})
		ctx = context.Background()
	})

	It("UT-AA-1400-001: extractDetectedLabels maps all 4 CNV fields from KA response", func() {
		analysis = createAnalysisForPostRCA()

		res := buildResultWithDetectedLabels(map[string]interface{}{
			"gitOpsManaged":  false,
			"stateful":       true,
			"virtualMachine": true,
			"liveMigratable": true,
			"cdiManaged":     false,
			"storageBackend": "odf-ceph",
		})

		_, err := processor.ProcessAgentSessionResult(ctx, analysis, res)
		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.PostRCAContext).ToNot(BeNil())

		dl := analysis.Status.PostRCAContext.DetectedLabels
		Expect(dl.VirtualMachine).To(BeTrue(), "virtualMachine must be extracted as true")
		Expect(dl.LiveMigratable).To(BeTrue(), "liveMigratable must be extracted as true")
		Expect(dl.CDIManaged).To(BeFalse(), "cdiManaged must be extracted as false")
		Expect(dl.StorageBackend).To(Equal("odf-ceph"), "storageBackend must be extracted")
	})

	It("UT-AA-1400-002: non-CNV input produces struct with false CNV fields (backward compat)", func() {
		analysis = createAnalysisForPostRCA()

		res := buildResultWithDetectedLabels(map[string]interface{}{
			"gitOpsManaged": true,
			"stateful":      true,
		})

		_, err := processor.ProcessAgentSessionResult(ctx, analysis, res)
		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.PostRCAContext).ToNot(BeNil())

		dl := analysis.Status.PostRCAContext.DetectedLabels
		Expect(dl.VirtualMachine).To(BeFalse(), "virtualMachine defaults to false when absent")
		Expect(dl.LiveMigratable).To(BeFalse(), "liveMigratable defaults to false when absent")
		Expect(dl.CDIManaged).To(BeFalse(), "cdiManaged defaults to false when absent")
		Expect(dl.StorageBackend).To(BeEmpty(), "storageBackend defaults to empty when absent")
		Expect(dl.GitOpsManaged).To(BeTrue(), "non-CNV fields must still work")
	})

	It("UT-AA-1400-003: extractDetectedLabels maps storageBackend string field", func() {
		analysis = createAnalysisForPostRCA()

		res := buildResultWithDetectedLabels(map[string]interface{}{
			"virtualMachine": true,
			"storageBackend": "lvms",
		})

		_, err := processor.ProcessAgentSessionResult(ctx, analysis, res)
		Expect(err).ToNot(HaveOccurred())

		dl := analysis.Status.PostRCAContext.DetectedLabels
		Expect(dl.StorageBackend).To(Equal("lvms"), "storageBackend=lvms must round-trip")
	})

	It("IT-AA-1400-001: ProcessAgentSessionResult with CNV labels persists to PostRCAContext", func() {
		analysis = createAnalysisForPostRCA()

		res := buildResultWithDetectedLabels(map[string]interface{}{
			"gitOpsManaged":            true,
			"pdbProtected":             true,
			"hpaEnabled":               false,
			"stateful":                 true,
			"helmManaged":              false,
			"networkIsolated":          false,
			"serviceMesh":              "istio",
			"resourceQuotaConstrained": true,
			"virtualMachine":           true,
			"liveMigratable":           true,
			"cdiManaged":               true,
			"storageBackend":           "odf-ceph",
		})

		_, err := processor.ProcessAgentSessionResult(ctx, analysis, res)
		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.PostRCAContext).ToNot(BeNil())

		dl := analysis.Status.PostRCAContext.DetectedLabels
		Expect(dl.GitOpsManaged).To(BeTrue())
		Expect(dl.PDBProtected).To(BeTrue())
		Expect(dl.Stateful).To(BeTrue())
		Expect(dl.ServiceMesh).To(Equal("istio"))
		Expect(dl.ResourceQuotaConstrained).To(BeTrue())
		Expect(dl.VirtualMachine).To(BeTrue())
		Expect(dl.LiveMigratable).To(BeTrue())
		Expect(dl.CDIManaged).To(BeTrue())
		Expect(dl.StorageBackend).To(Equal("odf-ceph"))
	})

	It("IT-AA-1400-002: ProcessAgentSessionResult without CNV keys still populates PostRCAContext", func() {
		analysis = createAnalysisForPostRCA()

		res := buildResultWithDetectedLabels(map[string]interface{}{
			"gitOpsManaged": true,
			"stateful":      false,
		})

		_, err := processor.ProcessAgentSessionResult(ctx, analysis, res)
		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.PostRCAContext).ToNot(BeNil())

		dl := analysis.Status.PostRCAContext.DetectedLabels
		Expect(dl.GitOpsManaged).To(BeTrue())
		Expect(dl.VirtualMachine).To(BeFalse(), "absent CNV field must not cause error")
		Expect(dl.StorageBackend).To(BeEmpty())
	})
})
