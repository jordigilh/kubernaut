/*
Copyright 2026 Jordi Gil.

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

// Issue #1661 Change 11b (DD-WORKFLOW-018): proves the API server enforces
// write-once semantics on AIAnalysis.Status.SelectedWorkflow once SelectedAt
// is populated, mirroring PostRCAContext's existing ADR-056 CEL guard. This
// closes the tampering gap the user flagged: once KA's selection is recorded,
// nothing (a buggy reconciler retry, an operator kubectl edit) may silently
// mutate the workflow snapshot RemediationOrchestrator/WorkflowExecution will
// later trust and execute against.
package aianalysis

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("SelectedWorkflow write-once immutability (Issue #1661 Change 11b)", Label("integration", "aianalysis"), func() {
	const (
		timeout  = 30 * time.Second
		interval = 500 * time.Millisecond
	)

	var analysis *aianalysisv1.AIAnalysis

	newSelectedWorkflow := func(selectedAt *metav1.Time, declared map[string]bool) *aianalysisv1.SelectedWorkflow {
		return &aianalysisv1.SelectedWorkflow{
			WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
				WorkflowID:      "increase-memory-v1",
				WorkflowName:    "increase-memory-v1",
				ActionType:      "RestartPod",
				Version:         "v1.0.0",
				ExecutionBundle: "ghcr.io/kubernaut/increase-memory:v1.0",
				Dependencies: &sharedtypes.WorkflowDependencies{
					Secrets: []sharedtypes.WorkflowResourceDependency{{Name: "db-creds"}},
				},
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
				},
				DeclaredParameterNames: declared,
			},
			Confidence: 0.92,
			Rationale:  "memory pressure detected",
			SelectedAt: selectedAt,
		}
	}

	BeforeEach(func() {
		rrName := helpers.UniqueTestName("test-selectedworkflow-rr")
		analysis = &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      helpers.UniqueTestName("selectedworkflow-immutability-test"),
				Namespace: testNamespace,
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name:      rrName,
					Namespace: testNamespace,
				},
				RemediationID: rrName,
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint: "test-fingerprint-selectedworkflow",
						Severity:    "critical",
						// MOCK_NO_WORKFLOW_FOUND (test/services/mock-llm/scenarios/scenario_mock_keywords.go)
						// keeps the live envtest AIAnalysis controller's own real reconcile from ever
						// populating Status.SelectedWorkflow (handleNoMatchingWorkflowsCompleted never
						// touches it). Without this, the controller races ahead of this test's manual
						// "first write" below with its own real KA-driven selection, and the CEL
						// write-once guard correctly rejects the test's differing content as a second
						// writer -- not a CEL bug, a fixture race.
						SignalName:       "MOCK_NO_WORKFLOW_FOUND",
						Environment:      "production",
						BusinessPriority: "P1",
						TargetResource: aianalysisv1.TargetResource{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: testNamespace,
						},
						EnrichmentResults: sharedtypes.EnrichmentResults{},
					},
					AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
				},
			},
		}
	})

	It("IT-AA-344-001: locks SelectedWorkflow once SelectedAt is populated, while tolerating idempotent no-op retries", func() {
		defer func() {
			_ = k8sClient.Delete(ctx, analysis)
		}()

		By("Creating the AIAnalysis CRD")
		Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

		By("First status write: setting SelectedWorkflow for the first time must succeed")
		now := metav1.Now()
		firstSW := newSelectedWorkflow(&now, map[string]bool{"TARGET_NAMESPACE": true, "REPLICAS": true})
		Eventually(func() error {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
				return err
			}
			analysis.Status.SelectedWorkflow = firstSW
			return k8sClient.Status().Update(ctx, analysis)
		}, timeout, interval).Should(Succeed(), "first write to a nil SelectedWorkflow must be accepted regardless of content")

		By("Re-reading the persisted SelectedWorkflow")
		var persisted aianalysisv1.AIAnalysis
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &persisted)).To(Succeed())
		Expect(persisted.Status.SelectedWorkflow).ToNot(BeNil())
		Expect(persisted.Status.SelectedWorkflow.SelectedAt).ToNot(BeNil(), "SelectedAt must persist as the immutability sentinel")
		Expect(persisted.Status.SelectedWorkflow.DeclaredParameterNames).To(Equal(map[string]bool{"TARGET_NAMESPACE": true, "REPLICAS": true}))

		By("Second status write: tampering with DeclaredParameterNames must be rejected by the API server")
		tampered := persisted.DeepCopy()
		tampered.Status.SelectedWorkflow.DeclaredParameterNames = map[string]bool{"TARGET_NAMESPACE": true, "INJECTED_PARAM": true}
		err := k8sClient.Status().Update(ctx, tampered)
		Expect(err).To(HaveOccurred(), "CEL must reject any mutation once selectedAt is populated (DD-WORKFLOW-018)")
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "expected an Invalid admission error, got: %v", err)

		By("Second status write attempt: tampering with WorkflowID must also be rejected")
		tamperedID := persisted.DeepCopy()
		tamperedID.Status.SelectedWorkflow.WorkflowID = "a-different-workflow-v2"
		err = k8sClient.Status().Update(ctx, tamperedID)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "expected an Invalid admission error, got: %v", err)

		By("Verifying the original snapshot survived both rejected tampering attempts")
		var afterTamper aianalysisv1.AIAnalysis
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &afterTamper)).To(Succeed())
		Expect(afterTamper.Status.SelectedWorkflow.WorkflowID).To(Equal("increase-memory-v1"))
		Expect(afterTamper.Status.SelectedWorkflow.DeclaredParameterNames).To(Equal(map[string]bool{"TARGET_NAMESPACE": true, "REPLICAS": true}))

		By("Third status write: resubmitting an identical value must succeed (idempotent reconcile-retry safety)")
		idempotentRetry := afterTamper.DeepCopy()
		// No field changes at all — simulates a reconciler retry re-applying the
		// same desired state after a transient conflict/requeue.
		err = k8sClient.Status().Update(ctx, idempotentRetry)
		Expect(err).ToNot(HaveOccurred(), "an update with an unchanged SelectedWorkflow value must not be rejected by the write-once CEL guard")
	})
})
