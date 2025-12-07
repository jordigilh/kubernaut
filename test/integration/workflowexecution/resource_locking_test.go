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
// NOTE: Controller is NOT running in EnvTest (Tekton CRDs not available)
// These tests validate CRD field storage for locking-related data.
// Actual locking behavior tests are in E2E suite (KIND + Tekton)
//
// Per 03-testing-strategy.mdc: >50% integration coverage for microservices

var _ = Describe("Resource Locking CRD Fields (DD-WE-001)", func() {
	Context("SkipDetails Field Storage", func() {
		It("should persist SkipDetails with ResourceBusy reason", func() {
			targetResource := fmt.Sprintf("default/deployment/locking-busy-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("busy", targetResource)

			defer func() {
				_ = deleteWFEAndWait(wfe, 10*time.Second)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Manually set Skipped status with ResourceBusy (simulating controller)
			wfe.Status.Phase = workflowexecutionv1alpha1.PhaseSkipped
			now := metav1.Now()
			wfe.Status.CompletionTime = &now
			wfe.Status.SkipDetails = &workflowexecutionv1alpha1.SkipDetails{
				Reason:    workflowexecutionv1alpha1.SkipReasonResourceBusy,
				Message:   "Another workflow is already running on target",
				SkippedAt: now,
				RecentRemediation: &workflowexecutionv1alpha1.RecentRemediationRef{
					Name:           "blocking-wfe",
					WorkflowID:     "test-workflow",
					CompletedAt:    now, // Required field
					Outcome:        "Running",
					TargetResource: targetResource,
				},
			}
			Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

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
				_ = deleteWFEAndWait(wfe, 10*time.Second)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Manually set Skipped status with RecentlyRemediated (simulating controller)
			now := metav1.Now()
			completedAt := metav1.NewTime(time.Now().Add(-2 * time.Minute))
			wfe.Status.Phase = workflowexecutionv1alpha1.PhaseSkipped
			wfe.Status.CompletionTime = &now
			wfe.Status.SkipDetails = &workflowexecutionv1alpha1.SkipDetails{
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
			Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

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
				_ = deleteWFEAndWait(wfe, 10*time.Second)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Manually set Skipped status with ExhaustedRetries (simulating controller)
			now := metav1.Now()
			wfe.Status.Phase = workflowexecutionv1alpha1.PhaseSkipped
			wfe.Status.CompletionTime = &now
			wfe.Status.SkipDetails = &workflowexecutionv1alpha1.SkipDetails{
				Reason:    workflowexecutionv1alpha1.SkipReasonExhaustedRetries,
				Message:   "Max consecutive failures (5) reached for target. Manual intervention required.",
				SkippedAt: now,
			}
			Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

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
				_ = deleteWFEAndWait(wfe, 10*time.Second)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Manually set Skipped status with PreviousExecutionFailed (simulating controller)
			now := metav1.Now()
			wfe.Status.Phase = workflowexecutionv1alpha1.PhaseSkipped
			wfe.Status.CompletionTime = &now
			wfe.Status.SkipDetails = &workflowexecutionv1alpha1.SkipDetails{
				Reason:    workflowexecutionv1alpha1.SkipReasonPreviousExecutionFailed,
				Message:   "Previous execution failed during workflow run. Manual intervention required.",
				SkippedAt: now,
			}
			Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

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
				_ = deleteWFEAndWait(wfe1, 10*time.Second)
				_ = deleteWFEAndWait(wfe2, 10*time.Second)
			}()

			// Create both - CRD allows this, controller decides locking
			Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())
			Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())

			// Both should exist
			_, err1 := getWFE(wfe1.Name, wfe1.Namespace)
			_, err2 := getWFE(wfe2.Name, wfe2.Namespace)
			Expect(err1).ToNot(HaveOccurred())
			Expect(err2).ToNot(HaveOccurred())

			GinkgoWriter.Println("✅ Multiple WFEs created for same target (controller enforces locking)")
		})
	})
})

