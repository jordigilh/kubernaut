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

	"github.com/jordigilh/kubernaut/pkg/agentclient"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// Kubernaut Agent E2E Test Suite (#433)
//
// Validates API contract parity with the retired Python KA service.
// Uses the same ogen-generated client (pkg/agentclient) since KA
// implements the same OpenAPI Handler interface.
//
// Infrastructure: Kind cluster + DataStorage + Mock LLM + Kubernaut Agent (Go)
// Replaces: test/e2e/holmesgpt-api/ (Python KA E2E tests)

func TestKubernautAgentE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(15 * time.Minute)
	RunSpecs(t, "Kubernaut Agent E2E Suite — #433")
}

var (
	ctx    context.Context
	cancel context.CancelFunc
	logger logr.Logger

	clusterName    string
	kubeconfigPath string

	// Same port mapping as KA (DD-TEST-001 v2.9)
	kaURL          string // http://localhost:8088
	dataStorageURL string // http://localhost:8089

	sharedNamespace string = "kubernaut-agent-e2e"

	// kaClient is the ogen-generated client (error-path tests)
	kaClient *agentclient.Client

	// sessionClient is the session-aware wrapper (submit/poll/result)
	sessionClient *agentclient.KubernautAgentClient

	// authHTTPClient carries the ServiceAccount Bearer token for raw HTTP tests
	// (e.g., RFC 7807 validation) that bypass the ogen client.
	authHTTPClient *http.Client

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

		kaURL = "http://localhost:8088"
		dataStorageURL = "http://localhost:8089"

		logger.Info("⏳ Waiting for Kind NodePort mapping to stabilize...")
		time.Sleep(5 * time.Second)

		logger.Info("⏳ Waiting for Data Storage service to be ready...")
		Eventually(func() error {
			resp, err := http.Get(dataStorageURL + "/health/ready")
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health check returned %d", resp.StatusCode)
			}
			return nil
		}, 90*time.Second, 2*time.Second).Should(Succeed(), "Data Storage health check should succeed")

		logger.Info("⏳ Waiting for Kubernaut Agent service to be ready...")
		Eventually(func() error {
			resp, err := http.Get(kaURL + "/health")
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

		kaClient, err = agentclient.NewClient(
			kaURL,
			agentclient.WithClient(&http.Client{
				Transport: testauth.NewServiceAccountTransport(saToken),
				Timeout:   60 * time.Second,
			}),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create authenticated client")

		sessionClient, err = agentclient.NewKubernautAgentClientWithTransport(
			agentclient.Config{BaseURL: kaURL},
			testauth.NewServiceAccountTransport(saToken),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create session client")

		authHTTPClient = &http.Client{
			Transport: testauth.NewServiceAccountTransport(saToken),
			Timeout:   30 * time.Second,
		}

		setupSucceeded = true
		return []byte(kubeconfigPath)
	},
	func(kubeconfigBytes []byte) {
		kubeconfigPath = string(kubeconfigBytes)
		ctx, cancel = context.WithCancel(context.Background())
		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())

		kaURL = "http://localhost:8088"
		dataStorageURL = "http://localhost:8089"

		cwd, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred())
		projectRoot = filepath.Join(cwd, "../../..")

		saToken, err := infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "kubernaut-agent-e2e-sa", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to get ServiceAccount token")

		kaClient, err = agentclient.NewClient(
			kaURL,
			agentclient.WithClient(&http.Client{
				Transport: testauth.NewServiceAccountTransport(saToken),
				Timeout:   60 * time.Second,
			}),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create authenticated client")

		sessionClient, err = agentclient.NewKubernautAgentClientWithTransport(
			agentclient.Config{BaseURL: kaURL},
			testauth.NewServiceAccountTransport(saToken),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create session client")

		authHTTPClient = &http.Client{
			Transport: testauth.NewServiceAccountTransport(saToken),
			Timeout:   30 * time.Second,
		}
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
		anyFailure := setupFailed || anyTestFailed || infrastructure.CheckTestFailure(clusterName)
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

		logger.Info("🧹 Deleting Kind cluster...")
		err := infrastructure.DeleteCluster(clusterName, "kubernaut-agent", anyFailure, GinkgoWriter, sharedNamespace)
		if err != nil {
			logger.Info("⚠️  Warning: Failed to delete cluster", "error", err)
		} else {
			logger.Info("✅ Cluster deleted successfully")
		}
	},
)
