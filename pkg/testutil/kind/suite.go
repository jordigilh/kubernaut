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

// Package kind provides reusable integration test setup for Kind clusters.
// This package assumes a Kind cluster already exists (created via `make bootstrap-dev`).
// It provides standardized patterns for:
// - Connecting to existing Kind cluster
// - Creating test namespaces
// - Deploying test resources
// - Cleanup management
//
// Usage:
//
//	var suite *kind.IntegrationSuite
//
//	var _ = BeforeSuite(func() {
//	    suite = kind.Setup("my-service-test")
//	})
//
//	var _ = AfterSuite(func() {
//	    suite.Cleanup()
//	})
package kind

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// IntegrationSuite provides standard Kind cluster integration test setup.
// It assumes a Kind cluster already exists (created via `make bootstrap-dev`).
type IntegrationSuite struct {
	// Client is the Kubernetes client connected to the Kind cluster
	Client kubernetes.Interface

	// Context is the context for Kubernetes operations
	Context context.Context

	// Namespaces are the test namespaces created by this suite
	Namespaces []string

	// cleanupFuncs are cleanup functions to execute in AfterSuite
	cleanupFuncs []func()
}

// Setup initializes an integration test suite with Kind cluster.
// It connects to an existing Kind cluster (via kubeconfig) and creates the specified test namespaces.
//
// Prerequisites:
//   - Kind cluster must be running: `make bootstrap-dev`
//   - Kubeconfig must be configured: `~/.kube/config` or `KUBECONFIG` env var
//
// Parameters:
//   - namespaces: One or more namespace names to create for test isolation
//
// Returns:
//   - *IntegrationSuite: Configured suite ready for testing
//
// Example:
//
//	var _ = BeforeSuite(func() {
//	    suite = kind.Setup("my-service-test", "kubernaut-system")
//	})
func Setup(namespaces ...string) *IntegrationSuite {
	suite := &IntegrationSuite{
		Context:      context.Background(),
		Namespaces:   namespaces,
		cleanupFuncs: []func(){},
	}

	// Get Kind cluster client (cluster already running via make bootstrap-dev)
	cfg, err := config.GetConfig()
	if err != nil {
		Fail(fmt.Sprintf(`Kind cluster not running or kubeconfig not found.

Error: %v

To fix:
  1. Create Kind cluster: make bootstrap-dev
  2. Verify cluster: kubectl get nodes
  3. Check kubeconfig: echo $KUBECONFIG

See: docs/testing/KIND_CLUSTER_TEST_TEMPLATE.md
`, err))
	}

	suite.Client, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		Fail(fmt.Sprintf(`Cannot connect to Kind cluster.

Error: %v

To fix:
  1. Verify cluster is healthy: kubectl get nodes
  2. Check kubeconfig is valid: kubectl cluster-info
  3. Ensure Kind cluster is running: kind get clusters

See: docs/testing/KIND_CLUSTER_TEST_TEMPLATE.md
`, err))
	}

	// Verify cluster connectivity
	_, err = suite.Client.CoreV1().Nodes().List(suite.Context, metav1.ListOptions{Limit: 1})
	if err != nil {
		Fail(fmt.Sprintf(`Kind cluster is not responding.

Error: %v

To fix:
  1. Restart Kind cluster: make cleanup-dev && make bootstrap-dev
  2. Verify cluster health: kubectl get pods -A

See: docs/testing/KIND_CLUSTER_TEST_TEMPLATE.md
`, err))
	}

	// Create test namespaces
	for _, ns := range namespaces {
		suite.createNamespace(ns)
	}

	GinkgoWriter.Printf("✅ Integration suite connected to Kind cluster\n")
	GinkgoWriter.Printf("   Namespaces created: %v\n", namespaces)

	return suite
}

// Cleanup executes all registered cleanup functions and deletes test namespaces.
// Call this in AfterSuite to ensure proper test cleanup.
//
// Example:
//
//	var _ = AfterSuite(func() {
//	    suite.Cleanup()
//	})
func (s *IntegrationSuite) Cleanup() {
	// Execute custom cleanup functions first (in reverse order)
	for i := len(s.cleanupFuncs) - 1; i >= 0; i-- {
		s.cleanupFuncs[i]()
	}

	// Delete test namespaces
	for _, ns := range s.Namespaces {
		err := s.Client.CoreV1().Namespaces().Delete(
			s.Context, ns, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			GinkgoWriter.Printf("⚠️  Warning: Failed to delete namespace %s: %v\n", ns, err)
		}
	}

	GinkgoWriter.Printf("✅ Integration suite cleanup complete\n")
}

// RegisterCleanup adds a cleanup function to be executed in Cleanup().
// Cleanup functions are executed in reverse order (LIFO).
//
// Example:
//
//	svc, _ := suite.DeployService(...)
//	suite.RegisterCleanup(func() {
//	    suite.Client.CoreV1().Services(ns).Delete(ctx, svc.Name, metav1.DeleteOptions{})
//	})
func (s *IntegrationSuite) RegisterCleanup(fn func()) {
	s.cleanupFuncs = append(s.cleanupFuncs, fn)
}

// createNamespace creates a namespace in the Kind cluster.
// If the namespace already exists, it's treated as success (idempotent).
func (s *IntegrationSuite) createNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"test-suite": "kubernaut-integration",
			},
		},
	}

	_, err := s.Client.CoreV1().Namespaces().Create(
		s.Context, ns, metav1.CreateOptions{})

	if err != nil {
		if errors.IsAlreadyExists(err) {
			GinkgoWriter.Printf("   Namespace %s already exists (reusing)\n", name)
			return
		}
		Fail(fmt.Sprintf("Failed to create test namespace %s: %v", name, err))
	}

	GinkgoWriter.Printf("   Created namespace: %s\n", name)
}
