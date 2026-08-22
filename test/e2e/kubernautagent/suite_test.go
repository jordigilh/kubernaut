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

package kubernautagent

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// Kubernaut Agent E2E Test Suite (#433)
//
// #2190: KA's HTTP session API (pkg/agentclient, /api/v1/incident/*) has
// been retired in favor of the AgentSession CRD (DD-AA-KA-001). Tests drive
// investigations by creating AgentSession objects directly via k8sClient
// (playing AA's role) and polling Status, instead of the retired
// ogen-generated session client.
//
// Infrastructure: Kind cluster + DataStorage + Mock LLM + Kubernaut Agent (Go)
// Replaces: test/e2e/kubernaut-agent/ (Python KA E2E tests)

func TestKubernautAgentE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(15 * time.Minute)
	RunSpecs(t, "Kubernaut Agent E2E Suite — #433")
}

// goconst dedup: test-fixture literal shared across many files in this
// package (previously defined in the now-deleted session_endpoints_test.go,
// #2190).
const errorFixture = "error"

var (
	ctx    context.Context
	cancel context.CancelFunc
	logger logr.Logger

	clusterName    string
	kubeconfigPath string

	// Same port mapping as KA (DD-TEST-001 v2.9)
	kaURL          string // http://localhost:8088  (API port)
	kaHealthURL    string // http://localhost:28088 (Issue #753: dedicated health port)
	kaMetricsURL   string // http://localhost:9088  (Issue #753: dedicated metrics port)
	dataStorageURL string // http://localhost:8089

	sharedNamespace string = "kubernaut-agent-e2e"

	// authHTTPClient carries the ServiceAccount Bearer token for raw HTTP
	// tests against services other than KA's retired session API (e.g.
	// DataStorage audit queries) and for KA's own metrics/health endpoints.
	authHTTPClient *http.Client

	// authHTTPClientB carries a DIFFERENT ServiceAccount token (kubernaut-agent-e2e-sa-2)
	// for cross-user authorization tests (E2E-KA-AUTHZ-001).
	authHTTPClientB *http.Client

	// k8sClient talks directly to the Kind cluster's AgentSession CRD
	// (issue #2190): tests that Create an AgentSession and poll its
	// Status play AA's role directly, since no AA controller runs in this
	// KA-focused suite. Replaces sessionClient.Investigate() for tests
	// migrated off the retired pkg/agentclient HTTP channel.
	k8sClient client.Client

	anyTestFailed  bool
	setupSucceeded bool
	projectRoot    string
)

var _ = SynchronizedBeforeSuite(
	func() []byte {
		ctx, cancel = context.WithCancel(context.Background())
		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Kubernaut Agent E2E Test Suite — Cluster Setup (#433)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		clusterName = "kubernaut-agent-e2e"
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		kubeconfigPath = fmt.Sprintf("%s/.kube/kubernaut-agent-e2e-config", homeDir)

		cwd, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred())
		projectRoot = filepath.Join(cwd, "../../..")

		logger.Info("🚀 Setting up Kubernaut Agent E2E infrastructure...")
		err = infrastructure.SetupKubernautAgentInfrastructure(ctx, clusterName, kubeconfigPath, sharedNamespace, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		kaURL = "https://localhost:8088"
		kaHealthURL = "http://localhost:28088"
		kaMetricsURL = "http://localhost:9088"
		dataStorageURL = "https://localhost:8089"

		// Issue #785: Configure http.DefaultTransport to trust the inter-service CA.
		tlsTransport, tlsErr := infrastructure.NewTLSAwareTransport(kubeconfigPath)
		Expect(tlsErr).ToNot(HaveOccurred(), "Failed to create TLS-aware transport (Issue #785)")
		http.DefaultTransport = tlsTransport

		logger.Info("⏳ Waiting for Kind NodePort mapping to stabilize...")
		time.Sleep(5 * time.Second)

		// Issue #753: Health probes moved to dedicated port 8081 (NodePort 30281 → host 28089)
		dataStorageHealthURL := "http://localhost:28089"
		logger.Info("⏳ Waiting for Data Storage service to be ready...")
		Eventually(func() error {
			resp, err := http.Get(dataStorageHealthURL + "/readyz")
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health check returned %d", resp.StatusCode)
			}
			return nil
		}, 90*time.Second, 2*time.Second).Should(Succeed(), "Data Storage health check should succeed")

		// Issue #753: KA health probes on dedicated port 8081 (NodePort 30188 → host 28088)
		logger.Info("⏳ Waiting for Kubernaut Agent service to be ready...")
		Eventually(func() error {
			resp, err := http.Get(kaHealthURL + "/readyz")
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health check returned %d", resp.StatusCode)
			}
			return nil
		}, 90*time.Second, 2*time.Second).Should(Succeed(), "Kubernaut Agent health check should succeed")

		logger.Info("✅ Kubernaut Agent E2E infrastructure ready")

		// DD-AUTH-014: Authenticate with ServiceAccount
		saToken, err := infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "kubernaut-agent-e2e-sa", kubeconfigPath)
		if err != nil {
			Fail(fmt.Sprintf("Failed to get ServiceAccount token: %v", err))
		}

		saTransport := testauth.NewRetryOn429Transport(testauth.NewServiceAccountTransport(saToken))

		authHTTPClient = &http.Client{
			Transport: saTransport,
			Timeout:   30 * time.Second,
		}

		// E2E-KA-AUTHZ-001: Create a second ServiceAccount with KA API access
		// for cross-user authorization testing. Uses the same Role (can call KA)
		// but a different identity so object-level authz denies access to other
		// users' sessions.
		saNameB := "kubernaut-agent-e2e-sa-2"
		err = infrastructure.CreateServiceAccount(ctx, sharedNamespace, kubeconfigPath, saNameB, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Failed to create second E2E ServiceAccount")

		err = infrastructure.CreateKAE2EClientRBACForSA(ctx, sharedNamespace, kubeconfigPath, saNameB, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Failed to create RBAC for second E2E ServiceAccount")

		saTokenB, err := infrastructure.GetServiceAccountToken(ctx, sharedNamespace, saNameB, kubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to get second ServiceAccount token")

		saTransportB := testauth.NewRetryOn429Transport(testauth.NewServiceAccountTransport(saTokenB))
		authHTTPClientB = &http.Client{
			Transport: saTransportB,
			Timeout:   30 * time.Second,
		}

		k8sClient, err = infrastructure.NewKubeconfigAgentSessionClient(kubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to create AgentSession-scoped k8s client (#2190)")

		setupSucceeded = true
		return []byte(kubeconfigPath)
	},
	func(kubeconfigBytes []byte) {
		kubeconfigPath = string(kubeconfigBytes)
		ctx, cancel = context.WithCancel(context.Background())
		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())

		kaURL = "https://localhost:8088"
		kaHealthURL = "http://localhost:28088"
		kaMetricsURL = "http://localhost:9088"
		dataStorageURL = "https://localhost:8089"

		// Issue #785: Configure http.DefaultTransport to trust the inter-service CA.
		tlsTransport, tlsErr := infrastructure.NewTLSAwareTransport(kubeconfigPath)
		Expect(tlsErr).ToNot(HaveOccurred(), "Failed to create TLS-aware transport (Issue #785)")
		http.DefaultTransport = tlsTransport

		cwd, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred())
		projectRoot = filepath.Join(cwd, "../../..")

		saToken, err := infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "kubernaut-agent-e2e-sa", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to get ServiceAccount token")

		saTransport := testauth.NewRetryOn429Transport(testauth.NewServiceAccountTransport(saToken))

		authHTTPClient = &http.Client{
			Transport: saTransport,
			Timeout:   30 * time.Second,
		}

		saTokenB, err := infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "kubernaut-agent-e2e-sa-2", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to get second ServiceAccount token")
		saTransportB := testauth.NewRetryOn429Transport(testauth.NewServiceAccountTransport(saTokenB))
		authHTTPClientB = &http.Client{
			Transport: saTransportB,
			Timeout:   30 * time.Second,
		}

		k8sClient, err = infrastructure.NewKubeconfigAgentSessionClient(kubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to create AgentSession-scoped k8s client (#2190)")
	},
)

var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
		infrastructure.MarkTestFailure(clusterName)
	}
})

var _ = SynchronizedAfterSuite(
	func() {
		logger.Info("Process cleanup...")
	},
	func() {
		defer cancel()

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Kubernaut Agent E2E — Teardown")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		setupFailed := !setupSucceeded
		anyFailure := infrastructure.ResolveAnyFailure(clusterName, setupFailed, anyTestFailed, GinkgoWriter)
		defer infrastructure.CleanupFailureMarker(clusterName)
		preserveCluster := os.Getenv("KEEP_CLUSTER") == "true"

		if preserveCluster {
			logger.Info("⚠️  CLUSTER PRESERVED FOR DEBUGGING (KEEP_CLUSTER=true)")
			logger.Info("  export KUBECONFIG=" + kubeconfigPath)
			logger.Info("  kubectl get pods -n " + sharedNamespace)
			return
		}

		// DD-TEST-007: Collect E2E binary coverage BEFORE cluster deletion
		if os.Getenv("E2E_COVERAGE") == "true" && !setupFailed {
			if err := infrastructure.CollectE2EBinaryCoverage(infrastructure.E2ECoverageOptions{
				ServiceName:    "kubernautagent",
				ClusterName:    clusterName,
				DeploymentName: "kubernaut-agent",
				Namespace:      sharedNamespace,
				KubeconfigPath: kubeconfigPath,
			}, GinkgoWriter); err != nil {
				GinkgoWriter.Printf("⚠️  Failed to collect E2E binary coverage (non-fatal): %v\n", err)
			}
		}

		// DD-TESTING-003 / Issue #2036: production must-gather image as a local
		// podman container on the cluster's "kind" network, replacing the old
		// in-process kubectl-log-scraping (MustGatherPodLogs, previously invoked
		// internally by DeleteCluster below). KA deploys to sharedNamespace
		// ("kubernaut-agent-e2e"), not the default "kubernaut-system".
		if anyFailure {
			mustGatherImage, buildErr := infrastructure.BuildMustGatherImageForE2E(ctx, GinkgoWriter)
			if buildErr != nil {
				logger.Error(buildErr, "Failed to build must-gather image (non-fatal, no diagnostics collected)")
			} else {
				mustGatherOutputDir := filepath.Join("/tmp", "kubernaut-must-gather", "kubernautagent", clusterName)
				if err := infrastructure.RunMustGatherImage(ctx, infrastructure.RunMustGatherImageOptions{
					ClusterName: clusterName,
					Image:       mustGatherImage,
					OutputDir:   mustGatherOutputDir,
					Namespace:   sharedNamespace,
					UsePodman:   true,
				}, GinkgoWriter); err != nil {
					logger.Error(err, "Failed to run must-gather image (non-fatal, no diagnostics collected)")
				}
			}
		}

		logger.Info("🧹 Deleting Kind cluster...")
		err := infrastructure.DeleteCluster(clusterName, "kubernaut-agent", anyFailure, GinkgoWriter)
		if err != nil {
			logger.Info("⚠️  Warning: Failed to delete cluster", errorFixture, err)
		} else {
			logger.Info("✅ Cluster deleted successfully")
		}
	},
)
