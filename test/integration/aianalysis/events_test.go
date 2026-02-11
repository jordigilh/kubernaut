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

// DD-EVENT-001: AIAnalysis K8s Event Observability Integration Tests
//
// BR-AA-095 / BR-ORCH-095: K8s Event Observability business requirements
//
// These tests validate event emission in the context of the envtest framework
// with real EventRecorder (k8sManager.GetEventRecorderFor). They use the
// pattern: create CR → wait for target phase → list corev1.Events filtered
// by involvedObject.name → assert expected event reasons.
//
// IMPORTANT: These tests require the full integration environment (CRDs,
// Mock HAPI, DataStorage, etc.) to run. Structure compiles; execution
// depends on `make test-integration-aianalysis`.
package aianalysis

import (
	"context"
	"sort"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// listEventsForObject returns corev1.Events for the given object name in the namespace,
// sorted by FirstTimestamp for deterministic ordering.
func listEventsForObject(ctx context.Context, c client.Client, objectName, namespace string) ([]corev1.Event, error) {
	list := &corev1.EventList{}
	if err := c.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	var result []corev1.Event
	for _, e := range list.Items {
		if e.InvolvedObject.Name == objectName {
			result = append(result, e)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].FirstTimestamp.Before(&result[j].FirstTimestamp)
	})
	return result, nil
}

// eventReasons returns the ordered list of event reasons from the given events.
func eventReasons(evts []corev1.Event) []string {
	reasons := make([]string, len(evts))
	for i, e := range evts {
		reasons[i] = e.Reason
	}
	return reasons
}

var _ = Describe("AIAnalysis K8s Event Observability (DD-EVENT-001, BR-AA-095)", Label("integration", "events"), func() {
	const (
		eventTimeout  = 2 * time.Minute
		eventInterval = time.Second
	)

	Context("IT-AA-095-01: Happy path event trail", func() {
		It("should emit AIAnalysisCreated, InvestigationComplete, AnalysisCompleted in order when lifecycle completes", func() {
			rrName := helpers.UniqueTestName("test-remediation")
			analysisName := helpers.UniqueTestName("integration-events-happy")
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      analysisName,
					Namespace: testNamespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      rrName,
						Namespace: testNamespace,
					},
					RemediationID: rrName,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "test-fingerprint-events-001",
							Severity:         "medium",
							SignalType:       "CrashLoopBackOff",
							Environment:      "staging",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: testNamespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
					},
				},
			}

			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			By("Creating AIAnalysis CRD")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for Completed phase")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, eventTimeout, eventInterval).Should(Equal("Completed"))

			By("Listing events for object and asserting expected reasons")
			var evts []corev1.Event
			Eventually(func() bool {
				var err error
				evts, err = listEventsForObject(ctx, k8sClient, analysisName, testNamespace)
				if err != nil {
					return false
				}
				reasons := eventReasons(evts)
				// Must contain at least AIAnalysisCreated, InvestigationComplete, AnalysisCompleted
				hasCreated := containsReason(reasons, events.EventReasonAIAnalysisCreated)
				hasInvestigation := containsReason(reasons, events.EventReasonInvestigationComplete)
				hasCompleted := containsReason(reasons, events.EventReasonAnalysisCompleted)
				return hasCreated && hasInvestigation && hasCompleted
			}, eventTimeout, eventInterval).Should(BeTrue(), "Expected events to appear within timeout")

			reasons := eventReasons(evts)
			Expect(containsReason(reasons, events.EventReasonAIAnalysisCreated)).To(BeTrue())
			Expect(containsReason(reasons, events.EventReasonInvestigationComplete)).To(BeTrue())
			Expect(containsReason(reasons, events.EventReasonAnalysisCompleted)).To(BeTrue())
			// Verify order: AIAnalysisCreated must precede InvestigationComplete which must precede AnalysisCompleted
			Expect(assertEventOrder(reasons, events.EventReasonAIAnalysisCreated, events.EventReasonInvestigationComplete, events.EventReasonAnalysisCompleted)).To(BeTrue())
		})
	})

	Context("IT-AA-095-02: Investigation failure event trail", func() {
		It("should emit AIAnalysisCreated and AnalysisFailed when HAPI returns permanent error", func() {
			// NOTE: This test requires Mock HAPI configured with a scenario that returns
			// permanent error (e.g., mock_rca_permanent_error). Without that config,
			// the test may reach Completed instead of Failed.
			rrName := helpers.UniqueTestName("test-remediation-fail")
			analysisName := helpers.UniqueTestName("integration-events-fail")
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      analysisName,
					Namespace: testNamespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      rrName,
						Namespace: testNamespace,
					},
					RemediationID: rrName,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "test-fingerprint-events-fail",
							Severity:         "medium",
							SignalType:       "CrashLoopBackOff",
							Environment:      "staging",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: testNamespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}

			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			By("Creating AIAnalysis CRD")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for Failed phase (requires Mock HAPI permanent error scenario)")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, eventTimeout, eventInterval).Should(Equal("Failed"))

			By("Listing events and asserting AIAnalysisCreated, AnalysisFailed")
			var evts []corev1.Event
			Eventually(func() bool {
				var err error
				evts, err = listEventsForObject(ctx, k8sClient, analysisName, testNamespace)
				if err != nil {
					return false
				}
				reasons := eventReasons(evts)
				return containsReason(reasons, events.EventReasonAIAnalysisCreated) &&
					containsReason(reasons, events.EventReasonAnalysisFailed)
			}, eventTimeout, eventInterval).Should(BeTrue())

			reasons := eventReasons(evts)
			Expect(containsReason(reasons, events.EventReasonAIAnalysisCreated)).To(BeTrue())
			Expect(containsReason(reasons, events.EventReasonAnalysisFailed)).To(BeTrue())
		})
	})

	Context("IT-AA-095-03: Human review event trail", func() {
		It("should emit AIAnalysisCreated and HumanReviewRequired when HAPI flags needs_human_review=true", func() {
			// NOTE: This test requires Mock HAPI configured with a scenario that returns
			// needs_human_review=true. SignalType "MOCK_NO_WORKFLOW_FOUND" in production
			// environment triggers this in the recovery human review flow.
			rrName := helpers.UniqueTestName("test-remediation-hr")
			analysisName := helpers.UniqueTestName("integration-events-hr")
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      analysisName,
					Namespace: testNamespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      rrName,
						Namespace: testNamespace,
					},
					RemediationID: rrName,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "test-fingerprint-events-hr",
							Severity:         "critical",
							SignalType:       "MOCK_NO_WORKFLOW_FOUND",
							Environment:      "production",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "failing-pod",
								Namespace: testNamespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"recovery-analysis", "workflow-selection"},
					},
				},
			}

			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			By("Creating AIAnalysis CRD")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for reconciliation (Completed or Failed depending on HAPI response)")
			Eventually(func() bool {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				phase := string(analysis.Status.Phase)
				return phase == "Completed" || phase == "Failed"
			}, eventTimeout, eventInterval).Should(BeTrue())

			By("Listing events and asserting AIAnalysisCreated, HumanReviewRequired")
			var evts []corev1.Event
			Eventually(func() bool {
				var err error
				evts, err = listEventsForObject(ctx, k8sClient, analysisName, testNamespace)
				if err != nil {
					return false
				}
				reasons := eventReasons(evts)
				return containsReason(reasons, events.EventReasonAIAnalysisCreated) &&
					containsReason(reasons, events.EventReasonHumanReviewRequired)
			}, eventTimeout, eventInterval).Should(BeTrue())

			reasons := eventReasons(evts)
			Expect(containsReason(reasons, events.EventReasonAIAnalysisCreated)).To(BeTrue())
			Expect(containsReason(reasons, events.EventReasonHumanReviewRequired)).To(BeTrue())
		})
	})

	// ========================================
	// BR-AA-HAPI-064: Session Lifecycle Event Tests
	// CRD Events team handoff (issues #71-#73 completed)
	// ========================================

	Context("IT-AA-064-01a: SessionCreated on happy path", Label("session"), func() {
		It("should emit SessionCreated event when HAPI investigation session is submitted", func() {
			rrName := helpers.UniqueTestName("test-remediation-session")
			analysisName := helpers.UniqueTestName("integration-events-session-created")
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      analysisName,
					Namespace: testNamespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      rrName,
						Namespace: testNamespace,
					},
					RemediationID: rrName,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "test-fingerprint-session-created",
							Severity:         "medium",
							SignalType:       "CrashLoopBackOff",
							Environment:      "staging",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "test-pod-session",
								Namespace: testNamespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
					},
				},
			}

			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			By("Creating AIAnalysis CRD")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for Completed phase (session flow: submit -> poll -> result -> analysis)")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, eventTimeout, eventInterval).Should(Equal("Completed"))

			By("Asserting SessionCreated event was emitted")
			var evts []corev1.Event
			Eventually(func() bool {
				var err error
				evts, err = listEventsForObject(ctx, k8sClient, analysisName, testNamespace)
				if err != nil {
					return false
				}
				return containsReason(eventReasons(evts), events.EventReasonSessionCreated)
			}, eventTimeout, eventInterval).Should(BeTrue(), "Expected SessionCreated event")

			// Verify it's a Normal event (not Warning)
			for _, e := range evts {
				if e.Reason == events.EventReasonSessionCreated {
					Expect(e.Type).To(Equal(corev1.EventTypeNormal))
					Expect(e.Message).To(ContainSubstring("session created"))
					break
				}
			}
		})
	})

	Context("IT-AA-064-01b: SessionLost on stale session (404)", Label("session"), func() {
		It("should emit SessionLost warning and regenerate when session ID is unknown to HAPI", func() {
			rrName := helpers.UniqueTestName("test-remediation-session-lost")
			analysisName := helpers.UniqueTestName("integration-events-session-lost")
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      analysisName,
					Namespace: testNamespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      rrName,
						Namespace: testNamespace,
					},
					RemediationID: rrName,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "test-fingerprint-session-lost",
							Severity:         "medium",
							SignalType:       "CrashLoopBackOff",
							Environment:      "staging",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "test-pod-session-lost",
								Namespace: testNamespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
					},
				},
			}

			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			By("Creating AIAnalysis CRD and waiting for initial session")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			// Wait for the controller to get a real session ID from HAPI
			Eventually(func() bool {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return analysis.Status.InvestigationSession != nil &&
					analysis.Status.InvestigationSession.ID != ""
			}, eventTimeout, eventInterval).Should(BeTrue(), "Expected session to be created")

			By("Injecting stale session ID to trigger 404 on next poll")
			// Set a fabricated UUID that HAPI has never seen
			analysis.Status.InvestigationSession.ID = "00000000-dead-beef-0000-000000000000"
			analysis.Status.InvestigationSession.PollCount = 0 // Reset to ensure next reconcile polls
			Expect(k8sClient.Status().Update(ctx, analysis)).To(Succeed())

			By("Waiting for reconciliation to complete (session lost -> regenerate -> complete)")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, eventTimeout, eventInterval).Should(Equal("Completed"))

			By("Asserting SessionLost warning event was emitted")
			var evts []corev1.Event
			Eventually(func() bool {
				var err error
				evts, err = listEventsForObject(ctx, k8sClient, analysisName, testNamespace)
				if err != nil {
					return false
				}
				reasons := eventReasons(evts)
				return containsReason(reasons, events.EventReasonSessionLost) &&
					containsReason(reasons, events.EventReasonSessionCreated)
			}, eventTimeout, eventInterval).Should(BeTrue(), "Expected SessionLost + SessionCreated events")

			// Verify SessionLost is a Warning event
			for _, e := range evts {
				if e.Reason == events.EventReasonSessionLost {
					Expect(e.Type).To(Equal(corev1.EventTypeWarning))
					Expect(e.Message).To(ContainSubstring("session lost"))
					break
				}
			}

			// Verify at least 2 SessionCreated events (initial + after regeneration)
			sessionCreatedCount := 0
			for _, e := range evts {
				if e.Reason == events.EventReasonSessionCreated {
					sessionCreatedCount++
				}
			}
			Expect(sessionCreatedCount).To(BeNumerically(">=", 2),
				"Expected at least 2 SessionCreated events (initial + regenerated)")
		})
	})

	Context("IT-AA-064-01c: SessionRegenerationExceeded", Label("session"), func() {
		It("should emit SessionRegenerationExceeded warning and fail when cap is reached", func() {
			rrName := helpers.UniqueTestName("test-remediation-regen-exceeded")
			analysisName := helpers.UniqueTestName("integration-events-regen-exceeded")
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      analysisName,
					Namespace: testNamespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      rrName,
						Namespace: testNamespace,
					},
					RemediationID: rrName,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "test-fingerprint-regen-exceeded",
							Severity:         "medium",
							SignalType:       "CrashLoopBackOff",
							Environment:      "staging",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "test-pod-regen",
								Namespace: testNamespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
					},
				},
			}

			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			By("Creating AIAnalysis CRD and waiting for initial session")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			// Wait for the controller to get a real session ID
			Eventually(func() bool {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return analysis.Status.InvestigationSession != nil &&
					analysis.Status.InvestigationSession.ID != ""
			}, eventTimeout, eventInterval).Should(BeTrue(), "Expected session to be created")

			By("Setting generation to MaxSessionRegenerations-1 and injecting stale session ID")
			// This ensures the next 404 triggers the regeneration cap
			analysis.Status.InvestigationSession.ID = "00000000-dead-beef-0000-000000000001"
			analysis.Status.InvestigationSession.Generation = 4 // MaxSessionRegenerations(5) - 1
			analysis.Status.InvestigationSession.PollCount = 0
			Expect(k8sClient.Status().Update(ctx, analysis)).To(Succeed())

			By("Waiting for Failed phase (regeneration cap exceeded)")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, eventTimeout, eventInterval).Should(Equal("Failed"))

			By("Asserting SessionLost + SessionRegenerationExceeded events")
			var evts []corev1.Event
			Eventually(func() bool {
				var err error
				evts, err = listEventsForObject(ctx, k8sClient, analysisName, testNamespace)
				if err != nil {
					return false
				}
				reasons := eventReasons(evts)
				return containsReason(reasons, events.EventReasonSessionLost) &&
					containsReason(reasons, events.EventReasonSessionRegenerationExceeded)
			}, eventTimeout, eventInterval).Should(BeTrue(),
				"Expected SessionLost + SessionRegenerationExceeded events")

			// Verify SessionRegenerationExceeded is a Warning event
			for _, e := range evts {
				if e.Reason == events.EventReasonSessionRegenerationExceeded {
					Expect(e.Type).To(Equal(corev1.EventTypeWarning))
					Expect(e.Message).To(ContainSubstring("exceeded"))
					break
				}
			}

			// Verify the AA status reflects the failure reason
			Expect(analysis.Status.SubReason).To(Equal("SessionRegenerationExceeded"))
		})
	})
})

func containsReason(reasons []string, reason string) bool {
	for _, r := range reasons {
		if r == reason {
			return true
		}
	}
	return false
}

func assertEventOrder(reasons []string, orderedReasons ...string) bool {
	indices := make(map[string]int)
	for i, r := range reasons {
		indices[r] = i
	}
	prevIdx := -1
	for _, target := range orderedReasons {
		idx, ok := indices[target]
		if !ok {
			return false
		}
		if idx <= prevIdx {
			return false
		}
		prevIdx = idx
	}
	return true
}
