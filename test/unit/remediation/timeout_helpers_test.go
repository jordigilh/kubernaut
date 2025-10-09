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

	Describe("isPhaseTimedOut", func() {
		It("should return false when StartTime is nil", func() {
			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "processing",
					StartTime:    nil,
				},
			}

			Expect(reconciler.IsPhaseTimedOut(remediation)).To(BeFalse())
		})

		It("should detect timeout for pending phase after 30 seconds", func() {
			now := metav1.Now()
			oneMinuteAgo := metav1.NewTime(now.Add(-1 * time.Minute))

			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "pending",
					StartTime:    &oneMinuteAgo,
				},
			}

			Expect(reconciler.IsPhaseTimedOut(remediation)).To(BeTrue())
		})

		It("should NOT timeout for pending phase within 30 seconds", func() {
			now := metav1.Now()
			tenSecondsAgo := metav1.NewTime(now.Add(-10 * time.Second))

			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "pending",
					StartTime:    &tenSecondsAgo,
				},
			}

			Expect(reconciler.IsPhaseTimedOut(remediation)).To(BeFalse())
		})

		It("should detect timeout for processing phase after 5 minutes", func() {
			now := metav1.Now()
			sixMinutesAgo := metav1.NewTime(now.Add(-6 * time.Minute))

			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "processing",
					StartTime:    &sixMinutesAgo,
				},
			}

			Expect(reconciler.IsPhaseTimedOut(remediation)).To(BeTrue())
		})

		It("should NOT timeout for processing phase within 5 minutes", func() {
			now := metav1.Now()
			twoMinutesAgo := metav1.NewTime(now.Add(-2 * time.Minute))

			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "processing",
					StartTime:    &twoMinutesAgo,
				},
			}

			Expect(reconciler.IsPhaseTimedOut(remediation)).To(BeFalse())
		})

		It("should detect timeout for analyzing phase after 10 minutes", func() {
			now := metav1.Now()
			elevenMinutesAgo := metav1.NewTime(now.Add(-11 * time.Minute))

			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "analyzing",
					StartTime:    &elevenMinutesAgo,
				},
			}

			Expect(reconciler.IsPhaseTimedOut(remediation)).To(BeTrue())
		})

		It("should NOT timeout for analyzing phase within 10 minutes", func() {
			now := metav1.Now()
			fiveMinutesAgo := metav1.NewTime(now.Add(-5 * time.Minute))

			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "analyzing",
					StartTime:    &fiveMinutesAgo,
				},
			}

			Expect(reconciler.IsPhaseTimedOut(remediation)).To(BeFalse())
		})

		It("should detect timeout for executing phase after 30 minutes", func() {
			now := metav1.Now()
			thirtyFiveMinutesAgo := metav1.NewTime(now.Add(-35 * time.Minute))

			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "executing",
					StartTime:    &thirtyFiveMinutesAgo,
				},
			}

			Expect(reconciler.IsPhaseTimedOut(remediation)).To(BeTrue())
		})

		It("should NOT timeout for executing phase within 30 minutes", func() {
			now := metav1.Now()
			twentyMinutesAgo := metav1.NewTime(now.Add(-20 * time.Minute))

			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "executing",
					StartTime:    &twentyMinutesAgo,
				},
			}

			Expect(reconciler.IsPhaseTimedOut(remediation)).To(BeFalse())
		})

		It("should return false for unknown phases", func() {
			now := metav1.Now()
			oneHourAgo := metav1.NewTime(now.Add(-1 * time.Hour))

			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "unknown-phase",
					StartTime:    &oneHourAgo,
				},
			}

			Expect(reconciler.IsPhaseTimedOut(remediation)).To(BeFalse())
		})

		It("should return false for completed phase (terminal state)", func() {
			now := metav1.Now()
			oneHourAgo := metav1.NewTime(now.Add(-1 * time.Hour))

			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "completed",
					StartTime:    &oneHourAgo,
				},
			}

			Expect(reconciler.IsPhaseTimedOut(remediation)).To(BeFalse())
		})

		It("should return false for failed phase (terminal state)", func() {
			now := metav1.Now()
			oneHourAgo := metav1.NewTime(now.Add(-1 * time.Hour))

			remediation := &remediationv1alpha1.RemediationRequest{
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: "failed",
					StartTime:    &oneHourAgo,
				},
			}

			Expect(reconciler.IsPhaseTimedOut(remediation)).To(BeFalse())
		})
	})
})
