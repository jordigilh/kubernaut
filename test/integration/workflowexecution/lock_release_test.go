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

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

// Issue #375: Lock Release After Terminal Phase
//
// BR-WE-010: Cooldown enforcement with lock release via ReconcileTerminal.
// Validates that after MarkCompleted schedules RequeueAfter, ReconcileTerminal
// runs after cooldown expires, deletes the execution resource, and unblocks
// subsequent WFEs for the same target.
//
// Suite CooldownPeriod: 10 seconds (see suite_test.go)

var _ = Describe("Lock Release After Terminal Phase (Issue #375, DD-WE-003)", func() {

	It("IT-WE-375-002: cooldown enforcement with eventual lock release via ReconcileTerminal", func() {
		targetResource := fmt.Sprintf("default/deployment/lock-release-%d", time.Now().UnixNano())

		By("Phase 1: Creating WFE1 and running it to completion")
		wfe1 := createUniqueJobWFE("lock-rel1", targetResource)
		defer cleanupJobWFE(wfe1)
		Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())

		By("Waiting for WFE1 to reach Running (Job created)")
		Eventually(func() string {
			updated, err := getWFE(wfe1.Name, wfe1.Namespace)
			if err != nil {
				return ""
			}
			return updated.Status.Phase
		}, 15*time.Second, 200*time.Millisecond).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

		By("Simulating Job completion for WFE1")
		job1, err := waitForJobCreation(wfe1.Name, 5*time.Second)
		Expect(err).ToNot(HaveOccurred())
		expectedJobName := executor.ExecutionResourceName(targetResource)
		Expect(job1.Name).To(Equal(expectedJobName))
		Expect(simulateJobCompletion(job1, true)).To(Succeed())

		By("Waiting for WFE1 to reach Completed")
		Eventually(func() string {
			updated, err := getWFE(wfe1.Name, wfe1.Namespace)
			if err != nil {
				return ""
			}
			return updated.Status.Phase
		}, 15*time.Second, 200*time.Millisecond).Should(Equal(workflowexecutionv1alpha1.PhaseCompleted))

		By("Phase 2: Creating WFE2 immediately (within cooldown window)")
		wfe2 := createUniqueJobWFE("lock-rel2", targetResource)
		defer cleanupJobWFE(wfe2)
		Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())

		By("Verifying WFE2 stays in Pending during cooldown (blocked)")
		Consistently(func() string {
			updated, err := getWFE(wfe2.Name, wfe2.Namespace)
			if err != nil {
				return ""
			}
			return updated.Status.Phase
		}, 5*time.Second, 500*time.Millisecond).Should(
			SatisfyAny(Equal(""), Equal(workflowexecutionv1alpha1.PhasePending)),
			"WFE2 should be blocked in Pending during cooldown (BR-WE-010)")

		By("Waiting for cooldown to expire + ReconcileTerminal to clean up WFE1's Job")
		// Suite configures 10s cooldown. ReconcileTerminal fires after cooldown via
		// the RequeueAfter scheduled by MarkCompleted (#375 fix). It then deletes the
		// stale Job, releasing the lock.
		// Total wait: 10s cooldown + buffer for reconcile cycles.

		By("Verifying WFE2 eventually transitions to Running after lock is released")
		Eventually(func() string {
			updated, err := getWFE(wfe2.Name, wfe2.Namespace)
			if err != nil {
				return ""
			}
			return updated.Status.Phase
		}, 30*time.Second, 500*time.Millisecond).Should(
			SatisfyAny(
				Equal(workflowexecutionv1alpha1.PhaseRunning),
				Equal(workflowexecutionv1alpha1.PhaseFailed),
			),
			"WFE2 should proceed past Pending after cooldown expires and lock is released (#375)")

		GinkgoWriter.Printf("IT-WE-375-002: Cooldown enforcement with lock release validated\n")
	})
})
