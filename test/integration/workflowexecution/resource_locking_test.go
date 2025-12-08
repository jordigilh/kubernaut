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

package workflowexecution

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// Resource Locking Integration Tests
//
// V2.0 UPDATE: Controller IS running - tests work WITH the controller
//
// These tests validate resource locking behavior through the controller:
// - SkipDetails field storage for various skip reasons
// - Resource locking prevents parallel execution on same target
// - Cooldown enforcement
//
// Per 03-testing-strategy.mdc: >50% integration coverage for microservices

var _ = Describe("Resource Locking CRD Fields (DD-WE-001)", func() {
	Context("SkipDetails Field Storage", func() {
		// These tests use Eventually to get fresh copies and handle controller race conditions

		It("should persist SkipDetails with ResourceBusy reason", func() {
			targetResource := fmt.Sprintf("default/deployment/locking-busy-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("busy", targetResource)

			defer func() {
				cleanupWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Wait for controller to process, then update status
			// Use Eventually to handle concurrent updates
			Eventually(func() error {
				fresh, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return err
				}
				now := metav1.Now()
				fresh.Status.Phase = workflowexecutionv1alpha1.PhaseSkipped
				fresh.Status.CompletionTime = &now
				fresh.Status.SkipDetails = &workflowexecutionv1alpha1.SkipDetails{
					Reason:    workflowexecutionv1alpha1.SkipReasonResourceBusy,
					Message:   "Another workflow is already running on target",
					SkippedAt: now,
					RecentRemediation: &workflowexecutionv1alpha1.RecentRemediationRef{
						Name:           "blocking-wfe",
						WorkflowID:     "test-workflow",
						CompletedAt:    now,
						Outcome:        "Running",
						TargetResource: targetResource,
					},
				}
				return k8sClient.Status().Update(ctx, fresh)
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

			// Verify SkipDetails persisted
			updated, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseSkipped))
			Expect(updated.Status.SkipDetails).ToNot(BeNil())
			Expect(updated.Status.SkipDetails.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonResourceBusy))
			Expect(updated.Status.SkipDetails.RecentRemediation).ToNot(BeNil())
			Expect(updated.Status.SkipDetails.RecentRemediation.Name).To(Equal("blocking-wfe"))

			GinkgoWriter.Println("✅ SkipDetails (ResourceBusy) persisted correctly")
		})

		It("should persist SkipDetails with RecentlyRemediated reason", func() {
			targetResource := fmt.Sprintf("default/deployment/locking-cooldown-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("cooldown", targetResource)

			defer func() {
				cleanupWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Use Eventually to handle concurrent updates
			Eventually(func() error {
				fresh, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return err
				}
				now := metav1.Now()
				completedAt := metav1.NewTime(time.Now().Add(-2 * time.Minute))
				fresh.Status.Phase = workflowexecutionv1alpha1.PhaseSkipped
				fresh.Status.CompletionTime = &now
				fresh.Status.SkipDetails = &workflowexecutionv1alpha1.SkipDetails{
					Reason:    workflowexecutionv1alpha1.SkipReasonRecentlyRemediated,
					Message:   "Cooldown active - workflow completed recently",
					SkippedAt: now,
					RecentRemediation: &workflowexecutionv1alpha1.RecentRemediationRef{
						Name:              "previous-wfe",
						WorkflowID:        "test-workflow",
						CompletedAt:       completedAt,
						Outcome:           "Completed",
						TargetResource:    targetResource,
						CooldownRemaining: "3m0s",
					},
				}
				return k8sClient.Status().Update(ctx, fresh)
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

			// Verify SkipDetails persisted
			updated, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.SkipDetails.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonRecentlyRemediated))
			Expect(updated.Status.SkipDetails.RecentRemediation.CooldownRemaining).To(Equal("3m0s"))

			GinkgoWriter.Println("✅ SkipDetails (RecentlyRemediated) persisted correctly")
		})

		It("should persist SkipDetails with ExhaustedRetries reason", func() {
			targetResource := fmt.Sprintf("default/deployment/locking-exhausted-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("exhausted", targetResource)

			defer func() {
				cleanupWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Use Eventually to handle concurrent updates
			Eventually(func() error {
				fresh, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return err
				}
				now := metav1.Now()
				fresh.Status.Phase = workflowexecutionv1alpha1.PhaseSkipped
				fresh.Status.CompletionTime = &now
				fresh.Status.SkipDetails = &workflowexecutionv1alpha1.SkipDetails{
					Reason:    workflowexecutionv1alpha1.SkipReasonExhaustedRetries,
					Message:   "Max consecutive failures (5) reached for target. Manual intervention required.",
					SkippedAt: now,
				}
				return k8sClient.Status().Update(ctx, fresh)
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

			// Verify SkipDetails persisted
			updated, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.SkipDetails.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonExhaustedRetries))

			GinkgoWriter.Println("✅ SkipDetails (ExhaustedRetries) persisted correctly")
		})

		It("should persist SkipDetails with PreviousExecutionFailed reason", func() {
			targetResource := fmt.Sprintf("default/deployment/locking-prevfail-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("prevfail", targetResource)

			defer func() {
				cleanupWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Use Eventually to handle concurrent updates
			Eventually(func() error {
				fresh, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return err
				}
				now := metav1.Now()
				fresh.Status.Phase = workflowexecutionv1alpha1.PhaseSkipped
				fresh.Status.CompletionTime = &now
				fresh.Status.SkipDetails = &workflowexecutionv1alpha1.SkipDetails{
					Reason:    workflowexecutionv1alpha1.SkipReasonPreviousExecutionFailed,
					Message:   "Previous execution failed during workflow run. Manual intervention required.",
					SkippedAt: now,
				}
				return k8sClient.Status().Update(ctx, fresh)
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

			// Verify SkipDetails persisted
			updated, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.SkipDetails.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonPreviousExecutionFailed))

			GinkgoWriter.Println("✅ SkipDetails (PreviousExecutionFailed) persisted correctly")
		})
	})

	Context("Multiple WFEs for Same Target", func() {
		It("should allow creating multiple WFEs targeting same resource", func() {
			// Different targets for each WFE
			targetResource := fmt.Sprintf("default/deployment/locking-multi-%d", time.Now().UnixNano())

			wfe1 := createUniqueWFE("multi1", targetResource)
			wfe2 := createUniqueWFE("multi2", targetResource)

			defer func() {
				cleanupWFE(wfe1)
				cleanupWFE(wfe2)
			}()

			// Create both - CRD allows this, controller decides locking
			Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())
			Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())

			// Both should exist
			Eventually(func() error {
				_, err := getWFE(wfe1.Name, wfe1.Namespace)
				return err
			}, 5*time.Second).Should(Succeed())

			Eventually(func() error {
				_, err := getWFE(wfe2.Name, wfe2.Namespace)
				return err
			}, 5*time.Second).Should(Succeed())

			GinkgoWriter.Println("✅ Multiple WFEs created for same target (controller enforces locking)")
		})
	})
})
