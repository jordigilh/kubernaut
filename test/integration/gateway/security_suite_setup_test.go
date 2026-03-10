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

package gateway

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/jordigilh/kubernaut/test/integration/gateway/helpers"
)

// SecurityTestTokens holds ServiceAccount tokens for security integration tests
// These are created ONCE in BeforeSuite and reused across all tests for performance
type SecurityTestTokens struct {
	AuthorizedToken   string
	UnauthorizedToken string
	AuthorizedSA      string
	UnauthorizedSA    string
	SAHelper          *helpers.ServiceAccountHelper
	Clientset         *kubernetes.Clientset
}

var securityTokens *SecurityTestTokens

// SetupSecurityTokens creates ServiceAccounts and extracts tokens ONCE for the entire test suite
// This dramatically improves test performance: ~30 seconds once vs ~30 seconds per test
//
// Creates ClusterRole 'gateway-test-remediation-creator' if it doesn't exist (matching testdata fixture),
// then creates ServiceAccounts + ClusterRoleBindings and extracts tokens.
// - Extracts tokens for authentication/authorization tests
func SetupSecurityTokens() *SecurityTestTokens {
	if securityTokens != nil {
		return securityTokens
	}

	// Add timeout to prevent hanging indefinitely
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	GinkgoWriter.Println("🔐 Setting up suite-level ServiceAccounts (one-time setup with 60s timeout)...")

	// Create K8s clientset from envtest REST config (suite-level k8sConfig)
	step1Start := time.Now()
	GinkgoWriter.Println("  📋 Step 1: Creating K8s clientset from envtest REST config...")

	Expect(k8sConfig).ToNot(BeNil(), "envtest REST config must be available (set in suite BeforeSuite)")

	config := rest.CopyConfig(k8sConfig)
	config.QPS = 50
	config.Burst = 100

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		GinkgoWriter.Printf("  ❌ Failed to create Kubernetes clientset: %v\n", err)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Kubernetes clientset")
	}
	GinkgoWriter.Printf("  ✓ K8s clientset created (took %v)\n", time.Since(step1Start))

	// Use shared K8s client from suite setup (package-level variable)
	step2Start := time.Now()
	GinkgoWriter.Println("  📋 Step 2: Verifying controller-runtime client from suite...")
	if k8sClient == nil {
		GinkgoWriter.Println("  ❌ K8s client is nil")
		Expect(k8sClient).ToNot(BeNil(), "K8s client should be available")
	}
	GinkgoWriter.Printf("  ✓ Controller-runtime client verified (took %v)\n", time.Since(step2Start))

	// Setup ServiceAccount helper
	step3Start := time.Now()
	GinkgoWriter.Println("  📋 Step 3: Setting up ServiceAccount helper...")
	saHelper := helpers.NewServiceAccountHelper(
		clientset,
		k8sClient, // Use package-level controller-runtime client from suite
		"kubernaut-system",
	)
	GinkgoWriter.Printf("  ✓ ServiceAccount helper ready (took %v)\n", time.Since(step3Start))

	// ServiceAccount names
	authorizedSA := "test-gateway-authorized-suite"
	unauthorizedSA := "test-gateway-unauthorized-suite"

	// Create ClusterRole if it doesn't exist (matches testdata/gateway-test-clusterrole.yaml).
	// In Kind clusters, setup-kind-cluster.sh applies this; in envtest, we create it programmatically.
	step4Start := time.Now()
	GinkgoWriter.Println("  📋 Step 4: Ensuring ClusterRole 'gateway-test-remediation-creator' exists...")
	testCR := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "gateway-test-remediation-creator"},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{"remediation.kubernaut.ai"},
			Resources: []string{"remediationrequests"},
			Verbs:     []string{"create", "get", "list", "watch", "update", "patch", "delete"},
		}},
	}
	err = k8sClient.Create(ctx, testCR)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		GinkgoWriter.Printf("  ❌ Failed to create ClusterRole 'gateway-test-remediation-creator': %v\n", err)
		Expect(err).ToNot(HaveOccurred(), "Failed to create gateway-test-remediation-creator ClusterRole")
	}
	GinkgoWriter.Printf("  ✓ ClusterRole 'gateway-test-remediation-creator' ready (took %v)\n", time.Since(step4Start))

	// Create authorized SA with RBAC
	step5Start := time.Now()
	GinkgoWriter.Printf("  📋 Step 5: Creating authorized ServiceAccount '%s'...\n", authorizedSA)
	err = saHelper.CreateServiceAccountWithRBAC(
		ctx,
		authorizedSA,
		"gateway-test-remediation-creator",
	)
	if err != nil {
		GinkgoWriter.Printf("  ❌ Failed to create authorized ServiceAccount: %v\n", err)
		Expect(err).ToNot(HaveOccurred(), "Should create authorized ServiceAccount")
	}
	GinkgoWriter.Printf("  ✓ Created authorized ServiceAccount: %s (took %v)\n", authorizedSA, time.Since(step5Start))

	// BR-GATEWAY-037: Also bind to gateway-signal-source so the SA passes SAR checks.
	// (gateway middleware verifies create verb on services/gateway-service)
	// Create binding directly since CreateServiceAccountWithRBAC uses a fixed binding
	// name that would collide with the gateway-test-remediation-creator binding above.
	sarBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-gateway-signal-source", authorizedSA),
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      authorizedSA,
			Namespace: "kubernaut-system",
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "gateway-signal-source",
		},
	}
	err = k8sClient.Create(ctx, sarBinding)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		GinkgoWriter.Printf("  ❌ Failed to bind SA to gateway-signal-source: %v\n", err)
		Expect(err).ToNot(HaveOccurred(), "Should bind authorized SA to gateway-signal-source")
	}
	GinkgoWriter.Printf("  ✓ Bound %s to gateway-signal-source ClusterRole\n", authorizedSA)

	// Extract token for authorized SA
	step6Start := time.Now()
	GinkgoWriter.Println("  📋 Step 6: Extracting authorized token...")
	authorizedToken, err := saHelper.GetServiceAccountToken(ctx, authorizedSA)
	if err != nil {
		GinkgoWriter.Printf("  ❌ Failed to extract authorized token: %v\n", err)
		Expect(err).ToNot(HaveOccurred(), "Should extract authorized token")
	}
	if authorizedToken == "" {
		GinkgoWriter.Println("  ❌ Authorized token is empty")
		Expect(authorizedToken).ToNot(BeEmpty(), "Authorized token should not be empty")
	}
	GinkgoWriter.Printf("  ✓ Extracted authorized token (%d chars, took %v)\n", len(authorizedToken), time.Since(step6Start))

	// Create unauthorized SA (no RBAC binding)
	step7Start := time.Now()
	GinkgoWriter.Printf("  📋 Step 7: Creating unauthorized ServiceAccount '%s'...\n", unauthorizedSA)
	err = saHelper.CreateServiceAccount(ctx, unauthorizedSA)
	if err != nil {
		GinkgoWriter.Printf("  ❌ Failed to create unauthorized ServiceAccount: %v\n", err)
		Expect(err).ToNot(HaveOccurred(), "Should create unauthorized ServiceAccount")
	}
	GinkgoWriter.Printf("  ✓ Created unauthorized ServiceAccount: %s (took %v)\n", unauthorizedSA, time.Since(step7Start))

	// Extract token for unauthorized SA
	step8Start := time.Now()
	GinkgoWriter.Println("  📋 Step 8: Extracting unauthorized token...")
	unauthorizedToken, err := saHelper.GetServiceAccountToken(ctx, unauthorizedSA)
	if err != nil {
		GinkgoWriter.Printf("  ❌ Failed to extract unauthorized token: %v\n", err)
		Expect(err).ToNot(HaveOccurred(), "Should extract unauthorized token")
	}
	if unauthorizedToken == "" {
		GinkgoWriter.Println("  ❌ Unauthorized token is empty")
		Expect(unauthorizedToken).ToNot(BeEmpty(), "Unauthorized token should not be empty")
	}
	GinkgoWriter.Printf("  ✓ Extracted unauthorized token (%d chars, took %v)\n", len(unauthorizedToken), time.Since(step8Start))

	totalTime := time.Since(step1Start)
	GinkgoWriter.Printf("✅ Suite-level ServiceAccounts ready! (total time: %v)\n", totalTime)

	securityTokens = &SecurityTestTokens{
		AuthorizedToken:   authorizedToken,
		UnauthorizedToken: unauthorizedToken,
		AuthorizedSA:      authorizedSA,
		UnauthorizedSA:    unauthorizedSA,
		SAHelper:          saHelper,
		Clientset:         clientset,
	}

	return securityTokens
}

// CleanupSecurityTokens removes suite-level ServiceAccounts
// Note: ClusterRole is NOT deleted (managed by setup-kind-cluster.sh)
func CleanupSecurityTokens() {
	if securityTokens == nil {
		return
	}

	ctx := context.Background()
	GinkgoWriter.Println("🧹 Cleaning up suite-level ServiceAccounts...")

	saNames := []string{securityTokens.AuthorizedSA, securityTokens.UnauthorizedSA}
	err := securityTokens.SAHelper.Cleanup(ctx, saNames)
	if err != nil {
		GinkgoWriter.Printf("  ⚠️  Warning: Failed to cleanup ServiceAccounts: %v\n", err)
	} else {
		GinkgoWriter.Println("  ✓ ServiceAccounts cleaned up")
	}

	// Note: ClusterRole is NOT deleted - it's managed by setup-kind-cluster.sh
	// This allows multiple test runs without re-running the setup script

	securityTokens = nil
	GinkgoWriter.Println("✅ Cleanup complete")
}

// GetSecurityTokens returns the suite-level security tokens
// Panics if tokens haven't been set up yet
func GetSecurityTokens() *SecurityTestTokens {
	if securityTokens == nil {
		panic("Security tokens not initialized. Call SetupSecurityTokens() in BeforeSuite first.")
	}
	return securityTokens
}

// Helper function to get K8s clientset (used by tests)
