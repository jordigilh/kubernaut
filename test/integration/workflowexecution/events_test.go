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

// DD-EVENT-001: WorkflowExecution K8s Event Observability Integration Tests
//
// BR-WE-095: K8s Event Observability business requirements
//
// These tests validate event emission in the context of the envtest framework
// with real EventRecorder (k8sManager.GetEventRecorderFor). They use the
// pattern: create CR → wait for target phase → list corev1.Events filtered
// by involvedObject.name → assert expected event reasons.
//
// IMPORTANT: These tests require the full integration environment (CRDs,
// DataStorage, etc.) to run. Structure compiles; execution depends on
// `make test-integration-workflowexecution`.
package workflowexecution

import (
	"context"
	"fmt"
	"sort"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// listEventsForObject returns corev1.Events for the given object name in the namespace,
// sorted by FirstTimestamp for deterministic ordering.
func listEventsForObject(ctx context.Context, c client.Client, objectName, namespace string) []corev1.Event {
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

func eventReasons(evts []corev1.Event) []string {
	reasons := make([]string, len(evts))
	for i, e := range evts {
		reasons[i] = e.Reason
	}
	return reasons
}

func containsReason(reasons []string, reason string) bool {
	for _, r := range reasons {
		if r == reason {
			return true
		}
	}
	return false
}

var _ = Describe("WorkflowExecution K8s Event Observability (DD-EVENT-001, BR-WE-095)", Label("integration", "events"), func() {

	Context("IT-WE-095-01: Job happy path", func() {
		It("should emit WorkflowValidated, ExecutionCreated, PhaseTransition (Pending→Running), WorkflowCompleted", func() {
			targetResource := fmt.Sprintf("default/deployment/job-events-happy-%d", time.Now().UnixNano())
			wfe := createUniqueJobWFE("events-happy", targetResource)

			defer func() {
				cleanupJobWFE(wfe)
			}()

			By("Creating a WFE with executionEngine=job")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Running phase")
			Eventually(func() string {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseRunning)))

			By("Getting the created Job and simulating completion")
			job, err := waitForJobCreation(wfe.Name, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(simulateJobCompletion(job, true)).To(Succeed())

			By("Waiting for Completed phase")
			Eventually(func() string {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseCompleted)))

			By("Listing events and asserting expected reasons")
			var evts []corev1.Event
			Eventually(func() bool {
				evts = listEventsForObject(ctx, k8sClient, wfe.Name, wfe.Namespace)
				reasons := eventReasons(evts)
				return containsReason(reasons, events.EventReasonWorkflowValidated) &&
					containsReason(reasons, events.EventReasonExecutionCreated) &&
					containsReason(reasons, events.EventReasonPhaseTransition) &&
					containsReason(reasons, events.EventReasonWorkflowCompleted)
			}, 5*time.Second, 250*time.Millisecond).Should(BeTrue())

			reasons := eventReasons(evts)
			Expect(containsReason(reasons, events.EventReasonWorkflowValidated)).To(BeTrue())
			Expect(containsReason(reasons, events.EventReasonExecutionCreated)).To(BeTrue())
			Expect(containsReason(reasons, events.EventReasonPhaseTransition)).To(BeTrue())
			Expect(containsReason(reasons, events.EventReasonWorkflowCompleted)).To(BeTrue())
		})
	})

	Context("IT-WE-095-02: Validation failure", func() {
		It("should emit WorkflowValidationFailed when spec is invalid", func() {
			// Create WFE with empty ContainerImage (ValidateSpec rejects this)
			wfe := createUniqueWFE("events-invalid", "default/deployment/test-pod")
			wfe.Spec.WorkflowRef.ContainerImage = ""

			defer func() {
				_ = k8sClient.Delete(ctx, wfe)
			}()

			By("Creating a WFE with invalid spec (empty ContainerImage)")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Failed phase")
			Eventually(func() string {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 10*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseFailed)))

			By("Listing events and asserting WorkflowValidationFailed")
			var evts []corev1.Event
			Eventually(func() bool {
				evts = listEventsForObject(ctx, k8sClient, wfe.Name, wfe.Namespace)
				reasons := eventReasons(evts)
				return containsReason(reasons, events.EventReasonWorkflowValidationFailed)
			}, 5*time.Second, 250*time.Millisecond).Should(BeTrue())

			reasons := eventReasons(evts)
			Expect(containsReason(reasons, events.EventReasonWorkflowValidationFailed)).To(BeTrue())
		})
	})

	Context("IT-WE-095-03: Cooldown blocking", func() {
		It("should emit CooldownActive when cooldown is active", func() {
			targetResource := fmt.Sprintf("default/deployment/job-events-cooldown-%d", time.Now().UnixNano())

			By("Creating first WFE and completing it to activate cooldown")
			wfe1 := createUniqueJobWFE("events-cooldown-1", targetResource)
			Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())

			Eventually(func() string {
				updated, err := getWFE(wfe1.Name, wfe1.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseRunning)))

			job, err := waitForJobCreation(wfe1.Name, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(simulateJobCompletion(job, true)).To(Succeed())

			Eventually(func() string {
				updated, err := getWFE(wfe1.Name, wfe1.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseCompleted)))

			By("Creating second WFE for SAME target (within cooldown)")
			wfe2 := createUniqueJobWFE("events-cooldown-2", targetResource)
			Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())

			defer func() {
				cleanupJobWFE(wfe1)
				cleanupJobWFE(wfe2)
			}()

			By("Waiting for reconciliation (second WFE stays Pending due to cooldown)")
			Eventually(func() bool {
				evts := listEventsForObject(ctx, k8sClient, wfe2.Name, wfe2.Namespace)
				reasons := eventReasons(evts)
				return containsReason(reasons, events.EventReasonCooldownActive)
			}, 15*time.Second, 250*time.Millisecond).Should(BeTrue())

			evts := listEventsForObject(ctx, k8sClient, wfe2.Name, wfe2.Namespace)
			reasons := eventReasons(evts)
			Expect(containsReason(reasons, events.EventReasonCooldownActive)).To(BeTrue())
		})
	})

	Context("IT-WE-095-04: Job failure", func() {
		It("should emit WorkflowValidated, ExecutionCreated, WorkflowFailed", func() {
			targetResource := fmt.Sprintf("default/deployment/job-events-fail-%d", time.Now().UnixNano())
			wfe := createUniqueJobWFE("events-fail", targetResource)

			defer func() {
				cleanupJobWFE(wfe)
			}()

			By("Creating a WFE with executionEngine=job")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Running phase")
			Eventually(func() string {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseRunning)))

			By("Simulating Job failure")
			job, err := waitForJobCreation(wfe.Name, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(simulateJobCompletion(job, false)).To(Succeed())

			By("Waiting for Failed phase")
			Eventually(func() string {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseFailed)))

			By("Listing events and asserting expected reasons")
			var evts []corev1.Event
			Eventually(func() bool {
				evts = listEventsForObject(ctx, k8sClient, wfe.Name, wfe.Namespace)
				reasons := eventReasons(evts)
				return containsReason(reasons, events.EventReasonWorkflowValidated) &&
					containsReason(reasons, events.EventReasonExecutionCreated) &&
					containsReason(reasons, events.EventReasonWorkflowFailed)
			}, 5*time.Second, 250*time.Millisecond).Should(BeTrue())

			reasons := eventReasons(evts)
			Expect(containsReason(reasons, events.EventReasonWorkflowValidated)).To(BeTrue())
			Expect(containsReason(reasons, events.EventReasonExecutionCreated)).To(BeTrue())
			Expect(containsReason(reasons, events.EventReasonWorkflowFailed)).To(BeTrue())
		})
	})
})
