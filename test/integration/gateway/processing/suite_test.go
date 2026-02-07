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

package processing

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

// Suite-level resources
var (
	suiteCtx    context.Context
	suiteCancel context.CancelFunc
	suiteLogger logr.Logger
	testEnv     *envtest.Environment
	k8sConfig   *rest.Config
	k8sClient   client.Client
	k8sManager  ctrl.Manager // Manager with field indexers
)

var _ = BeforeSuite(func() {
	suiteCtx, suiteCancel = context.WithCancel(context.Background())

	// DD-005: Use shared logging library
	suiteLogger = kubelog.NewLogger(kubelog.Options{
		Development: true,
		Level:       0, // INFO
		ServiceName: "processing-integration-test",
	})

	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Processing Integration Test Suite - envtest Setup")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Creating test infrastructure...")
	suiteLogger.Info("  • envtest (in-memory K8s API server)")
	suiteLogger.Info("  • RemediationRequest CRD with field indexers")
	suiteLogger.Info("  • Field selector support (spec.signalFingerprint)")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// KUBEBUILDER_ASSETS is set by Makefile via setup-envtest dependency
	Expect(os.Getenv("KUBEBUILDER_ASSETS")).ToNot(BeEmpty(), "KUBEBUILDER_ASSETS must be set by Makefile (test-integration-% → setup-envtest)")
	suiteLogger.Info("   📍 KUBEBUILDER_ASSETS set by Makefile", "path", os.Getenv("KUBEBUILDER_ASSETS"))

	// Start envtest
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			"../../../../config/crd/bases", // Relative path from test/integration/gateway/processing/
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

	suiteLogger.Info("   ✅ envtest started", "api", k8sConfig.Host)

	// Create scheme with RemediationRequest CRD
	scheme := k8sruntime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())

	// Create controller-runtime manager with field indexers
	// This is necessary for field selector support in List() operations
	k8sManager, err = ctrl.NewManager(k8sConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: ":0", // Dynamic port allocation (prevents conflicts with E2E tests)
		},
	})
	Expect(err).ToNot(HaveOccurred(), "Manager should be created")

	// Register field indexer for spec.signalFingerprint
	// DD-GATEWAY-011: This enables efficient deduplication queries via field selectors
	err = k8sManager.GetFieldIndexer().IndexField(
		suiteCtx,
		&remediationv1alpha1.RemediationRequest{},
		"spec.signalFingerprint",
		func(obj client.Object) []string {
			rr := obj.(*remediationv1alpha1.RemediationRequest)
			return []string{rr.Spec.SignalFingerprint}
		},
	)
	Expect(err).ToNot(HaveOccurred(), "Field indexer should be registered")

	// Start manager in background
	go func() {
		defer GinkgoRecover()
		err := k8sManager.Start(suiteCtx)
		if err != nil {
			suiteLogger.Error(err, "Manager failed to start")
		}
	}()

	// Wait for cache to sync
	suiteLogger.Info("   ⏳ Waiting for manager cache to sync...")
	Expect(k8sManager.GetCache().WaitForCacheSync(suiteCtx)).To(BeTrue(),
		"Manager cache should sync")
	suiteLogger.Info("   ✅ Manager cache synced")

	// Note: Metrics server uses dynamic port allocation (":0") to prevent conflicts
	// Port discovery is not exposed by controller-runtime Manager interface

	// Get client from manager (this client supports field selectors)
	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).ToNot(BeNil(), "Manager client should not be nil")

	// Create test namespaces
	testNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "processing-test",
			Labels: map[string]string{"kubernaut.ai/managed": "true"},
		},
	}
	Expect(k8sClient.Create(suiteCtx, testNS)).To(Succeed())

	fallbackNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "kubernaut-system",
			Labels: map[string]string{"kubernaut.ai/managed": "true"},
		},
	}
	Expect(k8sClient.Create(suiteCtx, fallbackNS)).To(Succeed())

	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Processing Integration Test Infrastructure - Ready")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("  K8s API:              ", "host", k8sConfig.Host)
	suiteLogger.Info("  Field Indexers:       spec.signalFingerprint")
	suiteLogger.Info("  Test Namespace:       processing-test")
	suiteLogger.Info("  Fallback Namespace:   kubernaut-system")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})

var _ = AfterSuite(func() {
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Processing Integration Test Suite - Teardown")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Cancel context to stop manager
	if suiteCancel != nil {
		suiteCancel()
	}

	// Wait for manager to stop
	time.Sleep(1 * time.Second)

	// Stop envtest
	if testEnv != nil {
		suiteLogger.Info("Stopping envtest...")
		err := testEnv.Stop()
		if err != nil {
			suiteLogger.Info("Failed to stop envtest", "error", err)
		}
	}

	kubelog.Sync(suiteLogger)

	suiteLogger.Info("   ✅ All services stopped")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})

func TestProcessingIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Processing Integration Suite (envtest)")
}
