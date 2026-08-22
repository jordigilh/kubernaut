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
			analysis.Status.EnsureRCAResult().SelectedWorkflow = firstSW
			return k8sClient.Status().Update(ctx, analysis)
		}, timeout, interval).Should(Succeed(), "first write to a nil SelectedWorkflow must be accepted regardless of content")

		By("Re-reading the persisted SelectedWorkflow")
		var persisted aianalysisv1.AIAnalysis
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &persisted)).To(Succeed())
		Expect(persisted.Status.GetRCAResult().SelectedWorkflow).ToNot(BeNil())
		Expect(persisted.Status.GetRCAResult().SelectedWorkflow.SelectedAt).ToNot(BeNil(), "SelectedAt must persist as the immutability sentinel")
		Expect(persisted.Status.GetRCAResult().SelectedWorkflow.DeclaredParameterNames).To(Equal(map[string]bool{"TARGET_NAMESPACE": true, "REPLICAS": true}))

		// The real AA controller in this process keeps reconciling this same
		// object for the rest of the test (creating its AgentSession,
		// processing the investigation, transitioning phases) -- each of
		// those reconciles bumps ResourceVersion via unrelated status
		// fields. Reusing one early snapshot (e.g. `persisted`) across
		// multiple tampering attempts below would race that controller: a
		// stale ResourceVersion 409s as a plain Conflict, which would be
		// mistaken for the CEL rejection this test actually intends to
		// prove. Re-fetching immediately before each write (as introduced
		// in #2170) narrows that race window but does not close it -- the
		// controller can still land a reconcile between this helper's Get
		// and Update. Retry transparently on a plain Conflict (never on the
		// CEL Invalid rejection this test is asserting) so a losing race
		// against the controller can never be mistaken for the write-once
		// guard, or vice versa.
		attemptTamperedWrite := func(mutate func(sw *aianalysisv1.SelectedWorkflow)) error {
			deadline := time.Now().Add(timeout)
			for {
				var latest aianalysisv1.AIAnalysis
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &latest)).To(Succeed())
				mutate(latest.Status.RCAResult.SelectedWorkflow)
				err := k8sClient.Status().Update(ctx, &latest)
				if err == nil || !apierrors.IsConflict(err) || time.Now().After(deadline) {
					return err
				}
				time.Sleep(interval)
			}
		}

		By("Second status write: tampering with DeclaredParameterNames must be rejected by the API server")
		err := attemptTamperedWrite(func(sw *aianalysisv1.SelectedWorkflow) {
			sw.DeclaredParameterNames = map[string]bool{"TARGET_NAMESPACE": true, "INJECTED_PARAM": true}
		})
		Expect(err).To(HaveOccurred(), "CEL must reject any mutation once selectedAt is populated (DD-WORKFLOW-018)")
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "expected an Invalid admission error, got: %v", err)

		By("Second status write attempt: tampering with WorkflowID must also be rejected")
		err = attemptTamperedWrite(func(sw *aianalysisv1.SelectedWorkflow) {
			sw.WorkflowID = "a-different-workflow-v2"
		})
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "expected an Invalid admission error, got: %v", err)

		By("Verifying the original snapshot survived both rejected tampering attempts")
		var afterTamper aianalysisv1.AIAnalysis
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &afterTamper)).To(Succeed())
		Expect(afterTamper.Status.GetRCAResult().SelectedWorkflow.WorkflowID).To(Equal("increase-memory-v1"))
		Expect(afterTamper.Status.GetRCAResult().SelectedWorkflow.DeclaredParameterNames).To(Equal(map[string]bool{"TARGET_NAMESPACE": true, "REPLICAS": true}))

		By("Third status write: resubmitting an identical value must succeed (idempotent reconcile-retry safety)")
		// No field changes at all — simulates a reconciler retry re-applying the
		// same desired state after a transient conflict/requeue. Re-fetch
		// immediately before writing (rather than reusing afterTamper's
		// ResourceVersion) because the real AA controller in this process is
		// concurrently reconciling this same object (Investigating →
		// Completed) -- same class of fixture race documented in
		// decision_expired_status_test.go's Issue #2032 note. A stale
		// ResourceVersion here would 409 on this write-once CEL guard
		// exactly as it would on any other optimistic-concurrency write.
		Eventually(func() error {
			var latest aianalysisv1.AIAnalysis
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &latest); err != nil {
				return err
			}
			return k8sClient.Status().Update(ctx, &latest)
		}, timeout, interval).Should(Succeed(), "an update with an unchanged SelectedWorkflow value must not be rejected by the write-once CEL guard")
	})
})
