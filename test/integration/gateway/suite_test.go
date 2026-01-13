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
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// ========================================
// Gateway Integration Test Suite
// ========================================
//
// PURPOSE:
// This suite runs Gateway business logic tests WITHOUT the HTTP layer.
// Tests call ProcessSignal() directly and share the same K8s client as Gateway.
//
// KEY DIFFERENCES FROM E2E:
// - ❌ NO HTTP server (no gatewayURL, no httpClient, no SendWebhook)
// - ❌ NO Kind cluster (uses existing test cluster or envtest)
// - ✅ Direct ProcessSignal() calls
// - ✅ Shared K8s client (Gateway and test)
// - ✅ Immediate CRD visibility (no cache mismatch)
// - ✅ 10-100x faster execution
//
// WHAT TO TEST HERE:
// - CRD creation logic (5 tests)
// - Deduplication logic (6 tests)
// - Audit event emission (4 tests)
// - Service resilience (3 tests)
// - Error handling (4 tests)
// - Observability (3 tests)
// Total: 28 tests (80% of current E2E suite)
//
// WHAT NOT TO TEST HERE:
// - HTTP adapter parsing (keep in E2E: Test 08, 31)
// - HTTP middleware (keep in E2E: Test 03, 18, 19, 20)
// - HTTP server lifecycle (keep in E2E: Test 28)
// ========================================

var (
	ctx       context.Context
	cancel    context.CancelFunc
	k8sClient client.Client
	logger    logr.Logger
	testEnv   *envtest.Environment
	k8sConfig *rest.Config
)

func TestGatewayIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Integration Test Suite")
}

var _ = BeforeSuite(func() {
	logger = zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))

	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Gateway Integration Test Suite - STARTING")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("")
	logger.Info("TEST ARCHITECTURE: Integration (business logic)")
	logger.Info("  • Direct ProcessSignal() calls (no HTTP)")
	logger.Info("  • Shared K8s client (Gateway + test)")
	logger.Info("  • Immediate CRD visibility")
	logger.Info("  • 10-100x faster than E2E")
	logger.Info("")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Create root context
	ctx, cancel = context.WithCancel(context.Background())

	logger.Info("Creating test infrastructure...")
	logger.Info("  • envtest (in-memory K8s API server)")
	logger.Info("  • RemediationRequest CRD auto-installation")
	logger.Info("  • Shared K8s client (Gateway + test)")

	// Set KUBEBUILDER_ASSETS if not already set
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		cmd := exec.Command("go", "run", "sigs.k8s.io/controller-runtime/tools/setup-envtest@latest", "use", "-p", "path")
		output, err := cmd.Output()
		if err != nil {
			logger.Error(err, "Failed to get KUBEBUILDER_ASSETS path")
			Expect(err).ToNot(HaveOccurred(), "Should get KUBEBUILDER_ASSETS path from setup-envtest")
		}
		assetsPath := strings.TrimSpace(string(output))
		_ = os.Setenv("KUBEBUILDER_ASSETS", assetsPath)
		logger.Info("   📍 Set KUBEBUILDER_ASSETS", "path", assetsPath)
	}

	// Create envtest with CRD auto-installation
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			"../../../config/crd/bases", // Relative path from test/integration/gateway/
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	k8sConfig, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred(), "envtest should start successfully")
	Expect(k8sConfig).ToNot(BeNil(), "K8s config should not be nil")

	// Disable rate limiting for in-memory K8s API
	k8sConfig.RateLimiter = nil
	k8sConfig.QPS = 1000
	k8sConfig.Burst = 2000

	logger.Info("   ✅ envtest started", "api", k8sConfig.Host)

	// Create scheme with RemediationRequest CRD
	scheme := k8sruntime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())

	// Create K8s client
	k8sClient, err = client.New(k8sConfig, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred(), "Failed to create K8s client")

	logger.Info("✅ K8s client created")
	logger.Info("✅ Suite setup complete")
})

var _ = AfterSuite(func() {
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Gateway Integration Test Suite - COMPLETE")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	if cancel != nil {
		cancel()
	}

	// Stop envtest
	if testEnv != nil {
		logger.Info("Stopping envtest...")
		err := testEnv.Stop()
		if err != nil {
			logger.Error(err, "Failed to stop envtest")
		}
	}
})

// getKubernetesClient returns the shared K8s client
// This is used by test helpers and Gateway initialization
func getKubernetesClient() client.Client {
	if k8sClient == nil {
		fmt.Fprintf(os.Stderr, "ERROR: K8s client not initialized\n")
		return nil
	}
	return k8sClient
}
