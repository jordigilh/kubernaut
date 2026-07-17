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
// Keycloak (no DataStorage: FMC is audit-exempt per DD-AUDIT-003 and never
// calls DataStorage's API), and directly validates FMC's own HTTP API
// contract:
//
//   - Real Keycloak OAuth2 client_credentials token acquisition
//   - Real MCPServerRegistration discovery via the Kuadrant MCP Gateway
//   - Real resource sync (kubernaut.ai/managed=true label) via kube-mcp-server,
//     which runs in passthrough mode and performs a real RFC 8693 Standard
//     Token Exchange against Keycloak to reach the Kubernetes API server
//     (Spike S17/S18) -- not the "fleet" suite's kubeconfig/fixed-SA mode
//   - FMC's /api/v1/clusters and /api/v1/scope/check REST endpoints
//   - AC-6 least privilege: FMC's ServiceAccount RBAC surface, and that the
//     token-exchange step is a real security boundary (an un-exchanged
//     token is rejected by the API server, not silently accepted)
//   - SC-7 boundary re-closure: a de-labeled resource stops being reported
//     managed once its cache entry lapses (real resync, not a seeded key)
//   - SI-4/CP-10 resilience: /readyz genuinely degrades and auto-recovers
//     across a real Valkey pod restart
//   - SI-4/CM-6 dynamic reconfiguration: a real MCPServerRegistration
//     create/delete is reflected live in FMC's cluster registry
//
// Before this suite existed, FMC's own journeys were only exercised
// indirectly through Gateway/RO fleet tests gated behind FLEET_E2E=true,
// which was never set in CI -- a pyramid invariant violation (E2E claims
// coverage that unit/integration tests cannot prove: real OAuth2 + real
// Kuadrant discovery + real kube-mcp-server calls).
//
// Keycloak replaces DEX in this lane only: DEX does not implement RFC 8693
// Standard Token Exchange, which kube-mcp-server's production passthrough
// auth mode relies on. The "fleet" full-pipeline suite still uses DEX.
//
// Test scenarios: E2E-FMC-054-010 (sync_journey_test.go), E2E-FMC-054-011
// (least_privilege_test.go), E2E-FMC-054-012 (resilience_test.go),
// E2E-FMC-054-013 (dynamic_registration_test.go), E2E-FMC-054-014
// (token_exchange_test.go). See docs/testing/BR-INTEGRATION-054/TEST_PLAN.md
// for the full scenario inventory and FedRAMP control coverage matrix.
//
// Authority: Issue #54, ADR-068, Spike S17/S18, BR-INTEGRATION-065.
//
// Test Execution:
//
//	ginkgo -v ./test/e2e/fleetmetadatacache/...
//
// Memory footprint: ~1.2-1.7GB (Keycloak + fleet-core in the primary cluster,
// plus a second, minimal Kind cluster running only kube-mcp-server for the
// cross-cluster isolation bridge, DD-TEST-013 -- Keycloak costs substantially
// more than DEX's ~64MB, accepted to validate the real production
// token-exchange wiring end-to-end), still lighter than the full "fleet"
// suite (~6.1GB).
package fleetmetadatacache

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/test/e2e/fleetmetadatacache/shared"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	trueFixture = "true"
)

const (
	clusterName = "fmc-e2e"
	namespace   = "kubernaut-system"

	// fmcAPIBaseURL is FMC's own REST API (ScopeCheckPath, ClustersPath),
	// exposed via NodePort per DD-TEST-001 (no kubectl port-forward).
	// See test/infrastructure/kind-fleetmetadatacache-config.yaml and
	// SetupFMCE2EInfrastructure's Phase 9.
	fmcAPIBaseURL = "http://localhost:8150"
)

// harness carries the state every shared FMC scenario needs (see
// shared.Harness); its fields are populated below in SynchronizedBeforeSuite.
// Declared here (not in variant.go) so the *_test.go wiring files that read
// it (sync_journey_test.go etc.) have a single, obvious source.
var harness = &shared.Harness{
	Namespace:     namespace,
	FMCAPIBaseURL: fmcAPIBaseURL,
}

var (
	cancel context.CancelFunc

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
		fmcImage, remoteKubeconfigPath, setupErr := infrastructure.SetupFMCE2EInfrastructure(bgCtx, clusterName, tempKubeconfigPath, GinkgoWriter)
		Expect(setupErr).ToNot(HaveOccurred(), "FMC E2E infrastructure setup failed")
		_ = fmcImage

		By("Setting KUBECONFIG for all processes")
		Expect(os.Setenv("KUBECONFIG", tempKubeconfigPath)).To(Succeed())

		GinkgoWriter.Println("FMC E2E infrastructure ready (Process 1)")
		return []byte(tempKubeconfigPath + "\n" + remoteKubeconfigPath)
	},
	func(data []byte) {
		parts := strings.SplitN(string(data), "\n", 2)
		harness.KubeconfigPath = parts[0]
		harness.RemoteKubeconfigPath = parts[1]
		harness.Ctx, cancel = context.WithCancel(context.TODO())

		GinkgoWriter.Printf("FMC E2E - Setup (Process %d)\n", GinkgoParallelProcess())

		By("Setting KUBECONFIG")
		Expect(os.Setenv("KUBECONFIG", harness.KubeconfigPath)).To(Succeed())

		tlsTransport, tlsErr := infrastructure.NewTLSAwareTransport(harness.KubeconfigPath)
		Expect(tlsErr).ToNot(HaveOccurred(), "Failed to create TLS-aware transport (Issue #785)")
		http.DefaultTransport = tlsTransport

		By("Creating Kubernetes client")
		cfg, cfgErr := config.GetConfig()
		Expect(cfgErr).ToNot(HaveOccurred())

		var clientErr error
		harness.K8sClient, clientErr = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(clientErr).ToNot(HaveOccurred())

		By("Creating remote cluster's Kubernetes client (DD-TEST-013, Spike S19)")
		remoteCfg, remoteCfgErr := clientcmd.BuildConfigFromFlags("", harness.RemoteKubeconfigPath)
		Expect(remoteCfgErr).ToNot(HaveOccurred(), "failed to build remote cluster kubeconfig")
		harness.RemoteK8sClient, clientErr = client.New(remoteCfg, client.Options{Scheme: scheme.Scheme})
		Expect(clientErr).ToNot(HaveOccurred())

		By("Setting up FMC HTTP client (NodePort 30150, host 8150 per DD-TEST-001)")
		harness.FMCHTTPClient = &http.Client{Timeout: 20 * time.Second}

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

		setupFailed := harness.KubeconfigPath == ""
		anyFailure := infrastructure.ResolveAnyFailure(clusterName, setupFailed, anyTestFailed, GinkgoWriter)
		defer infrastructure.CleanupFailureMarker(clusterName)
		preserveCluster := os.Getenv("PRESERVE_E2E_CLUSTER") == trueFixture || os.Getenv("KEEP_CLUSTER") == trueFixture

		remoteClusterName := clusterName + "-remote"

		if preserveCluster {
			GinkgoWriter.Println("CLUSTER PRESERVED FOR DEBUGGING")
			GinkgoWriter.Printf("   To access: export KUBECONFIG=%s\n", harness.KubeconfigPath)
			GinkgoWriter.Printf("   To delete: kind delete cluster --name %s\n", clusterName)
			GinkgoWriter.Printf("   Remote cluster (prod-east, DD-TEST-013): export KUBECONFIG=%s\n", harness.RemoteKubeconfigPath)
			GinkgoWriter.Printf("   To delete: kind delete cluster --name %s\n", remoteClusterName)
			return
		}

		if anyFailure && !setupFailed {
			infrastructure.MustGatherPodLogs(clusterName, harness.KubeconfigPath, namespace, "fleetmetadatacache", GinkgoWriter)

			// Kuadrant's controller+broker (mcp-system) and the Istio Gateway/Envoy
			// proxy (gateway-system, istio-system) are deployed by DeployFleetCoreInfra
			// but live outside kubernaut-system, so the call above never captures them.
			// A prior FMC E2E failure (Issue #54 RCA: syncKind calls rejected upstream
			// with an empty-Content-Type Envoy local reply) went undiagnosed at the
			// Envoy/AuthPolicy layer because these namespaces were never gathered.
			for _, ns := range []string{"mcp-system", "gateway-system", "istio-system"} {
				infrastructure.MustGatherPodLogs(clusterName, harness.KubeconfigPath, ns, "fleetmetadatacache", GinkgoWriter)
			}

			// Remote cluster (DD-TEST-013, Spike S19): only kube-mcp-server
			// runs there, but it's the component the "prod-east" cross-cluster
			// isolation scenario depends on most heavily.
			if harness.RemoteKubeconfigPath != "" {
				infrastructure.MustGatherPodLogs(remoteClusterName, harness.RemoteKubeconfigPath, namespace, "fleetmetadatacache", GinkgoWriter)
			}
		}

		if os.Getenv("E2E_COVERAGE") == trueFixture && !setupFailed {
			if covErr := infrastructure.CollectE2EBinaryCoverage(infrastructure.E2ECoverageOptions{
				ServiceName:    "fleetmetadatacache",
				ClusterName:    clusterName,
				DeploymentName: "fleetmetadatacache",
				Namespace:      namespace,
				KubeconfigPath: harness.KubeconfigPath,
			}, GinkgoWriter); covErr != nil {
				GinkgoWriter.Printf("Coverage collection failed (non-fatal): %v\n", covErr)
			}
		}

		By("Deleting Kind cluster")
		if delErr := infrastructure.DeleteCluster(clusterName, "fleetmetadatacache", anyFailure, GinkgoWriter); delErr != nil {
			GinkgoWriter.Printf("Warning: Failed to delete cluster: %v\n", delErr)
		}

		By("Deleting remote Kind cluster (DD-TEST-013, Spike S19)")
		if delErr := infrastructure.TeardownRemoteClusterForFMC(remoteClusterName, harness.RemoteKubeconfigPath, "fleetmetadatacache", anyFailure, GinkgoWriter); delErr != nil {
			GinkgoWriter.Printf("Warning: Failed to delete remote cluster: %v\n", delErr)
		}

		By("Removing isolated kubeconfig file")
		if harness.KubeconfigPath != "" {
			defaultConfig := os.ExpandEnv("$HOME/.kube/config")
			if harness.KubeconfigPath != defaultConfig {
				_ = os.Remove(harness.KubeconfigPath)
				GinkgoWriter.Printf("Removed kubeconfig: %s\n", harness.KubeconfigPath)
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
