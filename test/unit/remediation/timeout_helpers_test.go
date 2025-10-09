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

package remediation_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationctrl "github.com/jordigilh/kubernaut/internal/controller/remediation"
)

// Business Requirement: BR-ORCHESTRATION-003 (Timeout Handling)
// Unit tests for timeout helper functions
var _ = Describe("RemediationRequest Controller - Timeout Helper Functions", func() {
	var (
		scheme     *runtime.Scheme
		reconciler *remediationctrl.RemediationRequestReconciler
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler = &remediationctrl.RemediationRequestReconciler{
			Client: k8sClient,
			Scheme: scheme,
		}
	})

	Describe("IsPhaseTimedOut", func() {
		It("should return false when StartTime is nil", func() {
			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "processing",
					StartTime:    nil,
				},
			}

			Expect(reconciler.IsPhaseTimedOut(remediation)).To(BeFalse())
		})

		// Table-driven tests for timeout detection
		DescribeTable("phase timeout detection",
			func(phase string, elapsed time.Duration, expectedTimeout bool) {
				now := metav1.Now()
				startTime := metav1.NewTime(now.Add(-elapsed))

				remediation := &remediationv1alpha1.RemediationRequest{
					Status: remediationv1alpha1.RemediationRequestStatus{
						OverallPhase: phase,
						StartTime:    &startTime,
					},
				}

				Expect(reconciler.IsPhaseTimedOut(remediation)).To(Equal(expectedTimeout))
			},
			// Pending phase (30 second threshold)
			Entry("pending phase: TIMEOUT after 1 minute", "pending", 1*time.Minute, true),
			Entry("pending phase: NO timeout at 10 seconds", "pending", 10*time.Second, false),

			// Processing phase (5 minute threshold)
			Entry("processing phase: TIMEOUT after 6 minutes", "processing", 6*time.Minute, true),
			Entry("processing phase: NO timeout at 2 minutes", "processing", 2*time.Minute, false),
			Entry("processing phase: TIMEOUT at exactly 5 minutes + 1ms", "processing", 5*time.Minute+time.Millisecond, true),

			// Analyzing phase (10 minute threshold)
			Entry("analyzing phase: TIMEOUT after 11 minutes", "analyzing", 11*time.Minute, true),
			Entry("analyzing phase: NO timeout at 5 minutes", "analyzing", 5*time.Minute, false),
			Entry("analyzing phase: TIMEOUT at exactly 10 minutes + 1ms", "analyzing", 10*time.Minute+time.Millisecond, true),

			// Executing phase (30 minute threshold)
			Entry("executing phase: TIMEOUT after 35 minutes", "executing", 35*time.Minute, true),
			Entry("executing phase: NO timeout at 20 minutes", "executing", 20*time.Minute, false),
			Entry("executing phase: TIMEOUT at exactly 30 minutes + 1ms", "executing", 30*time.Minute+time.Millisecond, true),

			// Edge cases
			Entry("unknown phase: NO timeout even after 1 hour", "unknown-phase", 1*time.Hour, false),
			Entry("completed phase: NO timeout (terminal state)", "completed", 1*time.Hour, false),
			Entry("failed phase: NO timeout (terminal state)", "failed", 1*time.Hour, false),
		)
	})
})
