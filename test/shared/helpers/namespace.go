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

package helpers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateTestNamespaceAndWait creates a test namespace and waits for it to become Active.
//
// CRITICAL: This function blocks until the namespace is ready, preventing "namespace not found"
// errors when services try to create resources immediately after namespace creation.
//
// Kubernetes namespace creation is asynchronous - API success doesn't mean the namespace is ready.
// This function ensures the namespace is fully Active before returning.
//
// Usage:
//
//	BeforeEach(func() {
//	    testNamespace = helpers.CreateTestNamespaceAndWait(k8sClient, "mytest")
//	})
//
//	AfterEach(func() {
//	    helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
//	})
//
// Pattern: Gateway E2E (test/e2e/gateway/gateway_e2e_suite_test.go)
// Reference: DD-E2E-PARALLEL (parallel test execution safety)
func CreateTestNamespaceAndWait(k8sClient client.Client, prefix string) string {
	// Generate unique name with UUID (prevents collisions in parallel runs)
	// Pattern: {prefix}-{processID}-{uuid}
	name := fmt.Sprintf("%s-%d-%s",
		prefix,
		GinkgoParallelProcess(),
		uuid.New().String()[:8])

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"kubernaut.io/test": "e2e",
			},
		},
	}

	// Use background context for namespace operations (shouldn't be canceled by test timeouts)
	// Per DD-E2E-PARALLEL: Namespace operations should not be affected by test-level timeouts
	nsCtx := context.Background()

	// Create namespace with retry logic (handles K8s API rate limiting in parallel tests)
	// Pattern: Gateway E2E (test/e2e/gateway/deduplication_helpers.go:303-355)
	maxRetries := 5
	var createErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		createErr = k8sClient.Create(nsCtx, ns)

		// Success - continue to waiting phase
		if createErr == nil {
			break
		}

		// Check if namespace already exists (race condition in parallel tests)
		var existingNs corev1.Namespace
		if getErr := k8sClient.Get(nsCtx, client.ObjectKey{Name: name}, &existingNs); getErr == nil {
			// Namespace exists, treat as success
			createErr = nil
			break
		}

		// Retry with exponential backoff (1s, 2s, 4s, 8s, 16s)
		if attempt < maxRetries-1 {
			backoff := 1 << uint(attempt) // 1, 2, 4, 8, 16 seconds
			GinkgoWriter.Printf("⚠️  Namespace creation attempt %d/%d failed (will retry in %ds): %v\n",
				attempt+1, maxRetries, backoff, createErr)
			time.Sleep(time.Duration(backoff) * time.Second)
		}
	}

	Expect(createErr).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create namespace %s after %d attempts", name, maxRetries))

	// CRITICAL: Wait for namespace to be Active before returning
	// This prevents race conditions where services try to create resources in non-ready namespaces
	Eventually(func() bool {
		var createdNs corev1.Namespace
		if err := k8sClient.Get(nsCtx, client.ObjectKey{Name: name}, &createdNs); err != nil {
			GinkgoWriter.Printf("⚠️  Namespace %s not ready yet (Get failed): %v\n", name, err)
			return false
		}
		if createdNs.Status.Phase != corev1.NamespaceActive {
			GinkgoWriter.Printf("⚠️  Namespace %s phase: %v (waiting for Active)\n", name, createdNs.Status.Phase)
		}
		return createdNs.Status.Phase == corev1.NamespaceActive
	}, "60s", "500ms").Should(BeTrue(), fmt.Sprintf("Namespace %s should become Active", name))

	return name
}

// DeleteTestNamespace cleans up a test namespace after test completion.
//
// Safe to call with empty namespace names (handles BeforeEach failures gracefully).
//
// Usage:
//
//	AfterEach(func() {
//	    helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
//	})
func DeleteTestNamespace(ctx context.Context, k8sClient client.Client, name string) {
	// Guard against empty namespace names (can happen if BeforeEach fails/cancels)
	if name == "" {
		return
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	err := k8sClient.Delete(ctx, ns)
	if err != nil && !apierrors.IsNotFound(err) {
		GinkgoWriter.Printf("⚠️  Failed to delete namespace %s: %v\n", name, err)
	}
}
