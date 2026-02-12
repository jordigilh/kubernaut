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

// DD-EVENT-001: SignalProcessing K8s Event Observability Integration Tests
//
// BR-SP-095: K8s Event Observability business requirements
//
// These tests validate event emission in the context of the envtest framework
// with real EventRecorder (k8sManager.GetEventRecorderFor). They use the
// pattern: create CR → wait for target phase → list corev1.Events filtered
// by involvedObject.name → assert expected event reasons.
//
// IMPORTANT: These tests require the full integration environment (CRDs,
// DataStorage, etc.) to run. Structure compiles; execution depends on
// `make test-integration-signalprocessing`.
package signalprocessing

import (
	"context"
	"sort"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// listEventsForObject returns corev1.Events for the given object name in the namespace,
// sorted by FirstTimestamp for deterministic ordering.
func listEventsForObjectSP(ctx context.Context, c client.Client, objectName, namespace string) []corev1.Event {
	eventList := &corev1.EventList{}
	_ = c.List(ctx, eventList, client.InNamespace(namespace))
	var filtered []corev1.Event
	for _, evt := range eventList.Items {
		if evt.InvolvedObject.Name == objectName {
			filtered = append(filtered, evt)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].FirstTimestamp.Before(&filtered[j].FirstTimestamp)
	})
	return filtered
}

func eventReasonsSP(evts []corev1.Event) []string {
	reasons := make([]string, len(evts))
	for i, e := range evts {
		reasons[i] = e.Reason
	}
	return reasons
}

func containsReasonSP(reasons []string, reason string) bool {
	for _, r := range reasons {
		if r == reason {
			return true
		}
	}
	return false
}

var _ = Describe("SignalProcessing K8s Event Observability (DD-EVENT-001, BR-SP-095)", Label("integration", "events"), func() {

	Context("IT-SP-095-01: Happy path", func() {
		It("should emit PhaseTransition (Pending→Enriching), SignalEnriched, PhaseTransition (Enriching→Classifying), SignalProcessed", func() {
			By("Creating production namespace with environment label")
			ns := createTestNamespaceWithLabels("events-happy", map[string]string{
				"kubernaut.ai/environment": "production",
			})
			defer deleteTestNamespace(ns)

			By("Creating test pod")
			podLabels := map[string]string{"app": "events-happy-pod"}
			_ = createTestPod(ns, "events-happy-pod", podLabels, nil)

			By("Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "events-happy-pod",
				Namespace: ns,
			}
			rrName := "events-happy-rr"
			rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["reconciler-01"], "critical", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR")
			sp := CreateTestSignalProcessingWithParent("events-happy-sp", ns, rr, ValidTestFingerprints["reconciler-01"], targetResource)
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred(), "SignalProcessing should complete")

			By("Listing events and asserting expected reasons")
			var evts []corev1.Event
			Eventually(func() bool {
				evts = listEventsForObjectSP(ctx, k8sClient, sp.Name, ns)
				reasons := eventReasonsSP(evts)
				return containsReasonSP(reasons, events.EventReasonPhaseTransition) &&
					containsReasonSP(reasons, events.EventReasonSignalEnriched) &&
					containsReasonSP(reasons, events.EventReasonSignalProcessed)
			}, 5*time.Second, interval).Should(BeTrue())

			reasons := eventReasonsSP(evts)
			Expect(containsReasonSP(reasons, events.EventReasonPhaseTransition)).To(BeTrue())
			Expect(containsReasonSP(reasons, events.EventReasonSignalEnriched)).To(BeTrue())
			Expect(containsReasonSP(reasons, events.EventReasonSignalProcessed)).To(BeTrue())
		})
	})

	Context("IT-SP-095-02: Rego failure", func() {
		It("should emit PhaseTransition, PolicyEvaluationFailed when severity triggers policy error", func() {
			By("Creating namespace")
			ns := createTestNamespace("events-rego")
			defer deleteTestNamespace(ns)

			By("Creating parent RemediationRequest with severity that triggers policy error")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "test-pod",
				Namespace: ns,
			}
			// Severity "trigger-error" causes severity policy to return "invalid-severity-enum" → PolicyEvaluationFailed
			rrName := "events-rego-rr"
			rr := CreateTestRemediationRequest(rrName, ns, GenerateTestFingerprint("events-rego"), "trigger-error", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR")
			sp := CreateTestSignalProcessingWithParent("events-rego-sp", ns, rr, GenerateTestFingerprint("events-rego"), targetResource)
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for Failed phase")
			err := waitForPhase(sp.Name, sp.Namespace, signalprocessingv1alpha1.PhaseFailed, timeout)
			Expect(err).ToNot(HaveOccurred(), "SignalProcessing should transition to Failed on policy error")

			By("Listing events and asserting PhaseTransition, PolicyEvaluationFailed")
			var evts []corev1.Event
			Eventually(func() bool {
				evts = listEventsForObjectSP(ctx, k8sClient, sp.Name, ns)
				reasons := eventReasonsSP(evts)
				return containsReasonSP(reasons, events.EventReasonPhaseTransition) &&
					containsReasonSP(reasons, events.EventReasonPolicyEvaluationFailed)
			}, 5*time.Second, interval).Should(BeTrue())

			reasons := eventReasonsSP(evts)
			Expect(containsReasonSP(reasons, events.EventReasonPhaseTransition)).To(BeTrue())
			Expect(containsReasonSP(reasons, events.EventReasonPolicyEvaluationFailed)).To(BeTrue())
		})
	})

	Context("IT-SP-095-03: Degraded enrichment", func() {
		It("should emit PhaseTransition, EnrichmentDegraded, SignalProcessed when target pod not found", func() {
			By("Creating namespace")
			ns := createTestNamespace("events-degraded")
			defer deleteTestNamespace(ns)

			By("Creating parent RemediationRequest for non-existent pod")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "non-existent-pod",
				Namespace: ns,
			}
			rrName := "events-degraded-rr"
			rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["edge-case-02"], "high", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR for non-existent pod (triggers degraded enrichment)")
			sp := CreateTestSignalProcessingWithParent("events-degraded-sp", ns, rr, ValidTestFingerprints["edge-case-02"], targetResource)
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion (processing continues with partial/degraded data)")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred(), "SignalProcessing should complete despite degraded enrichment")

			By("Listing events and asserting PhaseTransition, EnrichmentDegraded, SignalProcessed")
			var evts []corev1.Event
			Eventually(func() bool {
				evts = listEventsForObjectSP(ctx, k8sClient, sp.Name, ns)
				reasons := eventReasonsSP(evts)
				return containsReasonSP(reasons, events.EventReasonPhaseTransition) &&
					containsReasonSP(reasons, events.EventReasonEnrichmentDegraded) &&
					containsReasonSP(reasons, events.EventReasonSignalProcessed)
			}, 5*time.Second, interval).Should(BeTrue())

			reasons := eventReasonsSP(evts)
			Expect(containsReasonSP(reasons, events.EventReasonPhaseTransition)).To(BeTrue())
			Expect(containsReasonSP(reasons, events.EventReasonEnrichmentDegraded)).To(BeTrue())
			Expect(containsReasonSP(reasons, events.EventReasonSignalProcessed)).To(BeTrue())
		})
	})
})
