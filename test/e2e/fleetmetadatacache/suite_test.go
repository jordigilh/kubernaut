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

// Package fleetmetadatacache contains the Fleet Metadata Cache (FMC) E2E test
// suite (Issue #54 SOC2 gap remediation, pyramid invariant).
//
// Unlike the "fleet" E2E suite (test/e2e/fleet/), which deploys ALL Kubernaut
// services to validate the end-to-end remediation journey through Gateway and
// RemediationOrchestrator, this suite deploys ONLY the fleet-core stack
// (Istio + Kuadrant MCP Gateway + kube-mcp-server + Valkey + FMC) plus
// DataStorage + DEX, and directly validates FMC's own HTTP API contract:
//
//   - Real DEX OAuth2 client_credentials token acquisition
//   - Real MCPServerRegistration discovery via the Kuadrant MCP Gateway
//   - Real resource sync (kubernaut.ai/managed=true label) via kube-mcp-server
//   - FMC's /api/v1/clusters and /api/v1/scope/check REST endpoints
//   - AC-6 least privilege: FMC's ServiceAccount RBAC surface
//
// Before this suite existed, FMC's own journeys were only exercised
// indirectly through Gateway/RO fleet tests gated behind FLEET_E2E=true,
// which was never set in CI -- a pyramid invariant violation (E2E claims
// coverage that unit/integration tests cannot prove: real OAuth2 + real
// Kuadrant discovery + real kube-mcp-server calls).
//
// Authority: Issue #54, ADR-068, BR-INTEGRATION-065.
//
// Test Execution:
//
//	ginkgo -v ./test/e2e/fleetmetadatacache/...
//
// Memory footprint: ~450MB (DataStorage + DEX + fleet-core), substantially
// lighter than the full "fleet" suite (~6.1GB).
package fleetmetadatacache

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

const (
	timeout  = 60 * time.Second
	interval = 2 * time.Second

	clusterName = "fmc-e2e"
	namespace   = "kubernaut-system"

	// fmcAPIBaseURL is FMC's own REST API (ScopeCheckPath, ClustersPath),
	// exposed via NodePort per DD-TEST-001 (no kubectl port-forward).
	// See test/infrastructure/kind-fleetmetadatacache-config.yaml and
	// SetupFMCE2EInfrastructure's Phase 9.
	fmcAPIBaseURL = "http://localhost:8150"
)

var (
	ctx    context.Context
	cancel context.CancelFunc

	kubeconfigPath string

	k8sClient client.Client

	fmcHTTPClient *http.Client

	anyTestFailed bool
)

func TestFleetMetadataCacheE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fleet Metadata Cache E2E Suite (Issue #54)")
}

var _ = SynchronizedBeforeSuite(
	func() []byte {
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		By("Setting up isolated kubeconfig path (per TESTING_GUIDELINES.md)")
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		tempKubeconfigPath := fmt.Sprintf("%s/.kube/%s-config", homeDir, clusterName)
		GinkgoWriter.Printf("Using isolated kubeconfig: %s\n", tempKubeconfigPath)

		By("Setting up FMC E2E infrastructure (Issue #54)")
		bgCtx := context.Background()
		fmcImage, setupErr := infrastructure.SetupFMCE2EInfrastructure(bgCtx, clusterName, tempKubeconfigPath, GinkgoWriter)
		Expect(setupErr).ToNot(HaveOccurred(), "FMC E2E infrastructure setup failed")
		_ = fmcImage

		By("Setting KUBECONFIG for all processes")
		Expect(os.Setenv("KUBECONFIG", tempKubeconfigPath)).To(Succeed())

		GinkgoWriter.Println("FMC E2E infrastructure ready (Process 1)")
		return []byte(tempKubeconfigPath)
	},
	func(data []byte) {
		kubeconfigPath = string(data)
		ctx, cancel = context.WithCancel(context.TODO())

		GinkgoWriter.Printf("FMC E2E - Setup (Process %d)\n", GinkgoParallelProcess())

		By("Setting KUBECONFIG")
		Expect(os.Setenv("KUBECONFIG", kubeconfigPath)).To(Succeed())

		tlsTransport, tlsErr := infrastructure.NewTLSAwareTransport(kubeconfigPath)
		Expect(tlsErr).ToNot(HaveOccurred(), "Failed to create TLS-aware transport (Issue #785)")
		http.DefaultTransport = tlsTransport

		By("Creating Kubernetes client")
		cfg, cfgErr := config.GetConfig()
		Expect(cfgErr).ToNot(HaveOccurred())

		var clientErr error
		k8sClient, clientErr = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(clientErr).ToNot(HaveOccurred())

		By("Setting up FMC HTTP client (NodePort 30150, host 8150 per DD-TEST-001)")
		fmcHTTPClient = &http.Client{Timeout: 20 * time.Second}

		GinkgoWriter.Printf("FMC E2E Setup Complete - Process %d ready\n", GinkgoParallelProcess())
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
		if cancel != nil {
			cancel()
		}
	},
	func() {
		By("Cleaning up FMC E2E environment")

		setupFailed := kubeconfigPath == ""
		anyFailure := setupFailed || anyTestFailed || infrastructure.CheckTestFailure(clusterName)
		defer infrastructure.CleanupFailureMarker(clusterName)
		preserveCluster := os.Getenv("PRESERVE_E2E_CLUSTER") == "true" || os.Getenv("KEEP_CLUSTER") == "true"

		if preserveCluster {
			GinkgoWriter.Println("CLUSTER PRESERVED FOR DEBUGGING")
			GinkgoWriter.Printf("   To access: export KUBECONFIG=%s\n", kubeconfigPath)
			GinkgoWriter.Printf("   To delete: kind delete cluster --name %s\n", clusterName)
			return
		}

		if anyFailure && !setupFailed {
			infrastructure.MustGatherPodLogs(clusterName, kubeconfigPath, namespace, "fleetmetadatacache", GinkgoWriter)
		}

		if os.Getenv("E2E_COVERAGE") == "true" && !setupFailed {
			if covErr := infrastructure.CollectE2EBinaryCoverage(infrastructure.E2ECoverageOptions{
				ServiceName:    "fleetmetadatacache",
				ClusterName:    clusterName,
				DeploymentName: "fleetmetadatacache",
				Namespace:      namespace,
				KubeconfigPath: kubeconfigPath,
			}, GinkgoWriter); covErr != nil {
				GinkgoWriter.Printf("Coverage collection failed (non-fatal): %v\n", covErr)
			}
		}

		By("Deleting Kind cluster")
		if delErr := infrastructure.DeleteCluster(clusterName, "fleetmetadatacache", anyFailure, GinkgoWriter); delErr != nil {
			GinkgoWriter.Printf("Warning: Failed to delete cluster: %v\n", delErr)
		}

		By("Removing isolated kubeconfig file")
		if kubeconfigPath != "" {
			defaultConfig := os.ExpandEnv("$HOME/.kube/config")
			if kubeconfigPath != defaultConfig {
				_ = os.Remove(kubeconfigPath)
				GinkgoWriter.Printf("Removed kubeconfig: %s\n", kubeconfigPath)
			}
		}

		By("Cleaning up built images")
		if !infrastructure.IsRunningInCICD() {
			pruneCmd := exec.Command("podman", "image", "prune", "-f")
			_, _ = pruneCmd.CombinedOutput()
		}

		GinkgoWriter.Println("FMC E2E cleanup complete")
	},
)
