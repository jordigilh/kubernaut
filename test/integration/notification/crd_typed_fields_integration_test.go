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

package notification

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
)

// Issue #453 Phase A: typed enum fields on NotificationRequest (spec + status) round-trip via the Kubernetes API.
//
// IT-NOT-453A-001 — BR-ORCH-036: manual-review + ReviewSource persistence and routing attributes.
// IT-NOT-453A-002 — BR-NOT-058: typed enum fields preserved through Create/Get and status update/Get.

var _ = Describe("Issue #453 Phase A: Typed Enum Fields Integration", Label("integration", "crd-typed-fields"), func() {
	Context("IT-NOT-453A-001: Manual review notification with typed ReviewSource via K8s API", func() {
		It("should persist typed ReviewSource and produce correct routing attributes", func() {
			notifName := fmt.Sprintf("it-453a-001-%d", GinkgoRandomSeed())
			nr := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      notifName,
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:         notificationv1alpha1.NotificationTypeManualReview,
					Priority:     notificationv1alpha1.NotificationPriorityHigh,
					Severity:     "critical",
					ReviewSource: notificationv1alpha1.ReviewSourceAIAnalysis,
					Subject:      "IT-453A-001: Manual Review",
					Body:         "Integration test for typed ReviewSource",
				},
			}
			Expect(k8sClient.Create(ctx, nr)).To(Succeed())

			fetched := &notificationv1alpha1.NotificationRequest{}
			Expect(k8sAPIReader.Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, fetched)).To(Succeed())
			Expect(fetched.Spec.ReviewSource).To(Equal(notificationv1alpha1.ReviewSourceAIAnalysis))

			attrs := routing.RoutingAttributesFromSpec(fetched)
			Expect(attrs["review-source"]).To(Equal("AIAnalysis"))
			Expect(attrs["type"]).To(Equal("ManualReview"))
			Expect(attrs["severity"]).To(Equal("critical"))
			Expect(attrs["priority"]).To(Equal("High"))
		})
	})

	Context("IT-NOT-453A-002: Typed enum fields round-trip through K8s API", func() {
		It("should preserve typed enum field values through Create and status update", func() {
			notifName := fmt.Sprintf("it-453a-002-%d", GinkgoRandomSeed())
			nr := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      notifName,
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:         notificationv1alpha1.NotificationTypeManualReview,
					Priority:     notificationv1alpha1.NotificationPriorityCritical,
					Severity:     "warning",
					ReviewSource: notificationv1alpha1.ReviewSourceWorkflowExecution,
					Subject:      "IT-453A-002: Round-trip",
					Body:         "Integration test for typed field round-trip",
				},
			}
			Expect(k8sClient.Create(ctx, nr)).To(Succeed())

			fetched := &notificationv1alpha1.NotificationRequest{}
			Expect(k8sAPIReader.Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, fetched)).To(Succeed())
			Expect(fetched.Spec.ReviewSource).To(Equal(notificationv1alpha1.ReviewSourceWorkflowExecution))
			Expect(fetched.Spec.Type).To(Equal(notificationv1alpha1.NotificationTypeManualReview))
			Expect(fetched.Spec.Priority).To(Equal(notificationv1alpha1.NotificationPriorityCritical))

			key := client.ObjectKeyFromObject(fetched)
			Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
				if err := k8sAPIReader.Get(ctx, key, fetched); err != nil {
					return err
				}
				fetched.Status.Phase = notificationv1alpha1.NotificationPhaseSending
				fetched.Status.Reason = notificationv1alpha1.StatusReasonPartialFailureRetrying
				fetched.Status.DeliveryAttempts = []notificationv1alpha1.DeliveryAttempt{
					{
						Channel:   "console",
						Attempt:   1,
						Timestamp: metav1.Now(),
						Status:    notificationv1alpha1.DeliveryAttemptStatusSuccess,
					},
					{
						Channel:   "slack",
						Attempt:   1,
						Timestamp: metav1.Now(),
						Status:    notificationv1alpha1.DeliveryAttemptStatusFailed,
						Error:     "webhook timeout",
					},
				}
				return k8sClient.Status().Update(ctx, fetched)
			})).To(Succeed())

			updated := &notificationv1alpha1.NotificationRequest{}
			Expect(k8sAPIReader.Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, updated)).To(Succeed())
			Expect(updated.Status.Reason).To(Equal(notificationv1alpha1.StatusReasonPartialFailureRetrying))
			Expect(updated.Status.DeliveryAttempts).To(HaveLen(2))
			Expect(updated.Status.DeliveryAttempts[0].Status).To(Equal(notificationv1alpha1.DeliveryAttemptStatusSuccess))
			Expect(updated.Status.DeliveryAttempts[0].Channel).To(Equal(notificationv1alpha1.DeliveryChannelName("console")))
			Expect(updated.Status.DeliveryAttempts[1].Status).To(Equal(notificationv1alpha1.DeliveryAttemptStatusFailed))
		})
	})
})
