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

// ManagedLabelKey is the label key used by Kubernaut to opt resources and namespaces
// into managed scope. Resources/namespaces with this label set to "true" are managed
// by Kubernaut; those with "false" or without the label are unmanaged.
//
// Reference: BR-SCOPE-001, ADR-053
//
// NOTE: When pkg/shared/scope is implemented, this constant should be replaced by
// a reference to the canonical constant defined there.
const ManagedLabelKey = "kubernaut.ai/managed"

// NamespaceOption configures namespace creation behavior.
// Use With* functions to construct options.
type NamespaceOption func(*namespaceConfig)

// namespaceConfig holds configuration for namespace creation.
type namespaceConfig struct {
	// labels contains additional labels to apply to the namespace.
	// These are merged with the default kubernaut.ai/managed label.
	labels map[string]string

	// managed controls whether the kubernaut.ai/managed=true label is applied.
	// nil or true = apply the label (default). false = omit the label entirely.
	managed *bool
}

// WithoutManagedLabel creates a namespace without the kubernaut.ai/managed label.
// Use this for scope management rejection tests (BR-SCOPE-001, BR-SCOPE-002).
//
// Example:
//
//	ns := helpers.CreateTestNamespace(ctx, k8sClient, "unmanaged", helpers.WithoutManagedLabel())
func WithoutManagedLabel() NamespaceOption {
	return func(cfg *namespaceConfig) {
		f := false
		cfg.managed = &f
	}
}

// WithLabels adds extra labels to the namespace, merged with the default
// kubernaut.ai/managed label. If a label conflicts with the managed label,
// the explicitly provided label takes precedence.
//
// Example:
//
//	ns := helpers.CreateTestNamespace(ctx, k8sClient, "prod",
//	    helpers.WithLabels(map[string]string{"environment": "production"}))
func WithLabels(labels map[string]string) NamespaceOption {
	return func(cfg *namespaceConfig) {
		if cfg.labels == nil {
			cfg.labels = make(map[string]string, len(labels))
		}
		for k, v := range labels {
			cfg.labels[k] = v
		}
	}
}

// applyDefaults returns the final label map after applying managed default and options.
func (cfg *namespaceConfig) buildLabels() map[string]string {
	labels := make(map[string]string)

	// Apply managed label by default (nil or true = managed)
	if cfg.managed == nil || *cfg.managed {
		labels[ManagedLabelKey] = "true"
	}

	// Merge additional labels (may override managed if explicitly set)
	for k, v := range cfg.labels {
		labels[k] = v
	}

	return labels
}

// generateNamespaceName creates a unique namespace name safe for parallel execution.
// Format: {prefix}-{processID}-{uuid[:8]}
func generateNamespaceName(prefix string) string {
	return fmt.Sprintf("%s-%d-%s",
		prefix,
		GinkgoParallelProcess(),
		uuid.New().String()[:8])
}

// CreateTestNamespace creates a test namespace with the kubernaut.ai/managed=true label
// by default. It does NOT wait for the namespace to become Active, making it suitable
// for integration tests using envtest where namespace creation is synchronous.
//
// For E2E tests against a real cluster, use CreateTestNamespaceAndWait instead.
//
// The namespace name is auto-generated with process ID and UUID for parallel safety.
// Format: {prefix}-{processID}-{uuid[:8]}
//
// Default labels:
//
//	kubernaut.ai/managed: "true"
//
// Options:
//   - WithoutManagedLabel(): Omit the managed label (for scope rejection tests)
//   - WithLabels(map): Add additional labels (merged with defaults)
//
// Usage:
//
//	BeforeEach(func() {
//	    testNamespace = helpers.CreateTestNamespace(ctx, k8sClient, "mytest")
//	})
//
//	AfterEach(func() {
//	    helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
//	})
//
// Reference: BR-SCOPE-001 (managed label default), DD-E2E-PARALLEL (parallel safety)
func CreateTestNamespace(ctx context.Context, k8sClient client.Client, prefix string, opts ...NamespaceOption) string {
	cfg := &namespaceConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	name := generateNamespaceName(prefix)

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: cfg.buildLabels(),
		},
	}

	Expect(k8sClient.Create(ctx, ns)).To(Succeed(),
		fmt.Sprintf("Failed to create namespace %s", name))

	GinkgoWriter.Printf("Created test namespace: %s\n", name)
	return name
}

// CreateTestNamespaceAndWait creates a test namespace and waits for it to become Active.
// It applies the kubernaut.ai/managed=true label by default.
//
// CRITICAL: This function blocks until the namespace is ready, preventing "namespace not found"
// errors when services try to create resources immediately after namespace creation.
//
// Kubernetes namespace creation is asynchronous - API success doesn't mean the namespace is ready.
// This function ensures the namespace is fully Active before returning. Use this for E2E tests
// against real Kind clusters. For envtest-based integration tests, use CreateTestNamespace instead.
//
// The namespace name is auto-generated with process ID and UUID for parallel safety.
// Format: {prefix}-{processID}-{uuid[:8]}
//
// Default labels:
//
//	kubernaut.ai/managed: "true"
//
// Options:
//   - WithoutManagedLabel(): Omit the managed label (for scope rejection tests)
//   - WithLabels(map): Add additional labels (merged with defaults)
//
// Usage:
//
//	BeforeEach(func() {
//	    testNamespace = helpers.CreateTestNamespaceAndWait(k8sClient, "mytest")
//	})
//
//	// Unmanaged namespace for scope rejection tests:
//	unmanagedNs = helpers.CreateTestNamespaceAndWait(k8sClient, "unmanaged",
//	    helpers.WithoutManagedLabel())
//
// Reference: BR-SCOPE-001 (managed label default), DD-E2E-PARALLEL (parallel safety)
func CreateTestNamespaceAndWait(k8sClient client.Client, prefix string, opts ...NamespaceOption) string {
	cfg := &namespaceConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	name := generateNamespaceName(prefix)

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: cfg.buildLabels(),
		},
	}

	// Use background context for namespace operations (shouldn't be canceled by test timeouts)
	// Per DD-E2E-PARALLEL: Namespace operations should not be affected by test-level timeouts
	nsCtx := context.Background()

	// Create namespace with retry logic (handles K8s API rate limiting in parallel tests)
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
			GinkgoWriter.Printf("Namespace creation attempt %d/%d failed (will retry in %ds): %v\n",
				attempt+1, maxRetries, backoff, createErr)
			time.Sleep(time.Duration(backoff) * time.Second)
		}
	}

	Expect(createErr).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create namespace %s after %d attempts", name, maxRetries))

	// CRITICAL: Wait for namespace to be Active before returning
	// This prevents race conditions where services try to create resources in non-ready namespaces
	Eventually(func() bool {
		var createdNs corev1.Namespace
		if err := k8sClient.Get(nsCtx, client.ObjectKey{Name: name}, &createdNs); err != nil {
			GinkgoWriter.Printf("Namespace %s not ready yet (Get failed): %v\n", name, err)
			return false
		}
		if createdNs.Status.Phase != corev1.NamespaceActive {
			GinkgoWriter.Printf("Namespace %s phase: %v (waiting for Active)\n", name, createdNs.Status.Phase)
		}
		return createdNs.Status.Phase == corev1.NamespaceActive
	}, "60s", "500ms").Should(BeTrue(), fmt.Sprintf("Namespace %s should become Active", name))

	GinkgoWriter.Printf("Namespace ready: %s\n", name)
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
		GinkgoWriter.Printf("Failed to delete namespace %s: %v\n", name, err)
	}
}
