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

package authwebhook_test

import (
	"context"
	"time"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// waitForStatusField polls for a status field to be populated by webhook
// Per TESTING_GUIDELINES.md: Use Eventually(), NEVER time.Sleep()
func waitForStatusField(
	ctx context.Context,
	obj client.Object,
	fieldGetter func() string,
	timeout time.Duration,
) {
	Eventually(func() string {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		if err != nil {
			return ""
		}
		return fieldGetter()
	}, timeout, 500*time.Millisecond).ShouldNot(BeEmpty(),
		"Webhook should populate status field within %s", timeout)
}

// createAndWaitForCRD creates a CRD and waits for it to be ready
// Per TESTING_GUIDELINES.md: Use Eventually() for K8s eventual consistency
func createAndWaitForCRD(ctx context.Context, obj client.Object) {
	Expect(k8sClient.Create(ctx, obj)).To(Succeed(),
		"CRD creation should succeed")

	// Wait for CRD to be created (eventually consistent)
	Eventually(func() error {
		return k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
	}, 10*time.Second, 500*time.Millisecond).Should(Succeed(),
		"CRD should be retrievable after creation")
}

// updateStatusAndWaitForWebhook updates CRD status and waits for webhook mutation
// This is the core pattern for testing webhook side effects
//
// Per TESTING_GUIDELINES.md §1773-1862: Integration tests should:
// 1. Trigger business operation (CRD status update)
// 2. Wait for webhook to mutate object (side effect)
// 3. Verify webhook populated fields correctly
func updateStatusAndWaitForWebhook(
	ctx context.Context,
	obj client.Object,
	updateFunc func(),
	verifyFunc func() bool,
) {
	// Apply status update (business operation)
	updateFunc()
	Expect(k8sClient.Status().Update(ctx, obj)).To(Succeed(),
		"Status update should trigger webhook")

	// Wait for webhook to populate fields (side effect validation)
	Eventually(func() bool {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		if err != nil {
			return false
		}
		return verifyFunc()
	}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
		"Webhook should mutate CRD within 10 seconds")
}

// deleteAndWaitForAnnotations deletes a CRD and waits for webhook to add annotations
// Used for NotificationRequest DELETE attribution tests
func deleteAndWaitForAnnotations(
	ctx context.Context,
	obj client.Object,
	expectedAnnotationKey string,
) {
	Expect(k8sClient.Delete(ctx, obj)).To(Succeed(),
		"CRD deletion should succeed")

	// Wait for webhook to add annotations before finalizer cleanup
	Eventually(func() string {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		if err != nil {
			return ""
		}
		annotations := obj.GetAnnotations()
		if annotations == nil {
			return ""
		}
		return annotations[expectedAnnotationKey]
	}, 10*time.Second, 500*time.Millisecond).ShouldNot(BeEmpty(),
		"Webhook should add %s annotation on DELETE", expectedAnnotationKey)
}

