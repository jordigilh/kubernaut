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
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notificationcontroller "github.com/jordigilh/kubernaut/internal/controller/notification"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/notification/sanitization"
)

// These tests are E2E tests that validate the complete notification lifecycle with audit integration
// using envtest for Kubernetes infrastructure.

var (
	cfg              *rest.Config
	k8sClient        client.Client
	testEnv          *envtest.Environment
	ctx              context.Context
	cancel           context.CancelFunc
	logger           *zap.Logger
	e2eFileOutputDir string // DD-NOT-002: E2E file delivery output directory
)

func TestNotificationE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Notification E2E Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(crzap.New(crzap.WriteTo(GinkgoWriter), crzap.UseDevMode(true)))
	logger, _ = zap.NewDevelopment()

	ctx, cancel = context.WithCancel(context.TODO())

	By("Bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = notificationv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// ========================================
	// DD-NOT-002 V3.0: E2E Controller Setup with FileService
	// ========================================
	// Set up E2E_FILE_OUTPUT directory for file-based message validation
	e2eFileOutputDir = filepath.Join(os.TempDir(), "kubernaut-e2e-notifications")
	err = os.MkdirAll(e2eFileOutputDir, 0755)
	Expect(err).ToNot(HaveOccurred())

	// Initialize delivery services
	consoleService := delivery.NewConsoleDeliveryService()
	fileService := delivery.NewFileDeliveryService(e2eFileOutputDir)
	sanitizer := sanitization.NewSanitizer()

	// Start controller manager for E2E tests
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	// Set up NotificationRequest controller with FileService
	// Note: AuditStore is nil in E2E tests (audit not required for file delivery validation)
	err = (&notificationcontroller.NotificationRequestReconciler{
		Client:         k8sManager.GetClient(),
		Scheme:         k8sManager.GetScheme(),
		ConsoleService: consoleService,
		FileService:    fileService, // DD-NOT-002: E2E file delivery
		Sanitizer:      sanitizer,
		// SlackService: nil (not needed for file delivery E2E tests)
		// AuditStore: nil (audit not required for file delivery validation)
		// AuditHelpers: nil (audit not required for file delivery validation)
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	// Start the manager in a goroutine
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())

	// Clean up E2E file output directory
	if e2eFileOutputDir != "" {
		os.RemoveAll(e2eFileOutputDir)
	}
})

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
