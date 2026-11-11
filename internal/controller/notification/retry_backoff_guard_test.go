/*
Copyright 2026 Jordi Gil.

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
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

var _ = Describe("Notification retry backoff guard (BR-NOT-054)", func() {
	It("requeues when the fresh API state is Retrying even if the cache says Sending", func() {
		const (
			name      = "retry-backoff-guard"
			namespace = "default"
		)

		scheme := runtime.NewScheme()
		Expect(notificationv1alpha1.AddToScheme(scheme)).To(Succeed())

		staleCachedObject := &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  namespace,
				Generation: 1,
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				RetryPolicy: &notificationv1alpha1.RetryPolicy{
					MaxAttempts:           5,
					InitialBackoffSeconds: 60,
					BackoffMultiplier:     2,
					MaxBackoffSeconds:     60,
				},
			},
			Status: notificationv1alpha1.NotificationRequestStatus{
				Phase: notificationv1alpha1.NotificationPhaseSending,
			},
		}

		freshAPIObject := staleCachedObject.DeepCopy()
		freshAPIObject.Status = notificationv1alpha1.NotificationRequestStatus{
			Phase: notificationv1alpha1.NotificationPhaseRetrying,
			DeliveryAttempts: []notificationv1alpha1.DeliveryAttempt{
				{
					Channel:   notificationv1alpha1.DeliveryChannelName(notificationv1alpha1.ChannelFile),
					Attempt:   1,
					Timestamp: metav1.Now(),
					Status:    notificationv1alpha1.DeliveryAttemptStatusFailed,
					Error:     "temporary failure",
				},
			},
		}

		cachedClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(staleCachedObject).
			Build()
		apiReader := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(freshAPIObject).
			Build()

		reconciler := &NotificationRequestReconciler{
			Client:    cachedClient,
			APIReader: apiReader,
		}

		result, skip, err := reconciler.refreshAndGuardBeforeDelivery(
			context.Background(),
			ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: namespace}},
			&notificationv1alpha1.NotificationRequest{},
			ctrl.Log.WithName("retry-backoff-guard-test"),
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(skip).To(BeTrue(), "a stale cached Sending phase must not bypass fresh retry backoff")
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(result.RequeueAfter).To(BeNumerically("<=", 60*time.Second))
	})
})
