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

// Business Requirement: BR-ORCHESTRATION-005 (24-Hour Retention)
// Unit tests for finalizer and retention logic
var _ = Describe("RemediationRequest Controller - Finalizer & Retention", func() {
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

	Describe("IsRetentionExpired", func() {
		DescribeTable("24-hour retention expiry detection",
			func(completedAt *metav1.Time, expectedExpired bool) {
				remediation := &remediationv1alpha1.RemediationRequest{
					Status: remediationv1alpha1.RemediationRequestStatus{
						OverallPhase: "completed",
						CompletedAt:  completedAt,
					},
				}

				Expect(reconciler.IsRetentionExpired(remediation)).To(Equal(expectedExpired))
			},
			// Retention expired cases
			Entry("completed 25 hours ago → expired", &metav1.Time{Time: time.Now().Add(-25 * time.Hour)}, true),
			Entry("completed 48 hours ago → expired", &metav1.Time{Time: time.Now().Add(-48 * time.Hour)}, true),
			Entry("completed 1 week ago → expired", &metav1.Time{Time: time.Now().Add(-7 * 24 * time.Hour)}, true),

			// Retention not expired cases
			Entry("completed 1 hour ago → not expired", &metav1.Time{Time: time.Now().Add(-1 * time.Hour)}, false),
			Entry("completed 23 hours ago → not expired", &metav1.Time{Time: time.Now().Add(-23 * time.Hour)}, false),
			Entry("completed 23 hours 59 minutes ago → not expired (just under boundary)", &metav1.Time{Time: time.Now().Add(-23*time.Hour - 59*time.Minute)}, false),

			// Edge cases
			Entry("completed just now → not expired", &metav1.Time{Time: time.Now()}, false),
			Entry("CompletedAt is nil → not expired", nil, false),
		)

		It("should return false for non-terminal phases", func() {
			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "processing",
					CompletedAt:  &metav1.Time{Time: time.Now().Add(-25 * time.Hour)},
				},
			}

			Expect(reconciler.IsRetentionExpired(remediation)).To(BeFalse())
		})

		It("should handle 'failed' terminal state", func() {
			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "failed",
					CompletedAt:  &metav1.Time{Time: time.Now().Add(-25 * time.Hour)},
				},
			}

			Expect(reconciler.IsRetentionExpired(remediation)).To(BeTrue())
		})
	})

	Describe("CalculateRequeueAfter", func() {
		It("should calculate time until retention expiry", func() {
			twentyThreeHoursAgo := metav1.NewTime(time.Now().Add(-23 * time.Hour))
			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "completed",
					CompletedAt:  &twentyThreeHoursAgo,
				},
			}

			requeueAfter := reconciler.CalculateRequeueAfter(remediation)

			// Should be approximately 1 hour (with some tolerance for test execution time)
			Expect(requeueAfter).To(BeNumerically("~", 1*time.Hour, 5*time.Second))
		})

		It("should return zero if retention already expired", func() {
			twentyFiveHoursAgo := metav1.NewTime(time.Now().Add(-25 * time.Hour))
			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "completed",
					CompletedAt:  &twentyFiveHoursAgo,
				},
			}

			requeueAfter := reconciler.CalculateRequeueAfter(remediation)
			Expect(requeueAfter).To(Equal(time.Duration(0)))
		})

		It("should return zero if CompletedAt is nil", func() {
			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "completed",
					CompletedAt:  nil,
				},
			}

			requeueAfter := reconciler.CalculateRequeueAfter(remediation)
			Expect(requeueAfter).To(Equal(time.Duration(0)))
		})
	})
})

