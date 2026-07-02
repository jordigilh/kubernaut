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

// Package eaigw is the Envoy AI Gateway (EAIGW) sibling of
// test/e2e/fleetmetadatacache (Issue #54, Spike S18 Phase B). It is nested
// under test/e2e/fleetmetadatacache/ (rather than a hyphenated sibling
// directory) to make the sibling relationship -- same FMC journeys, gateway
// swapped out -- explicit in the package layout, not just in prose.
//
// It mirrors that suite's full sync-journey/least-privilege/resilience/
// dynamic-registration/token-exchange coverage, but fronts kube-mcp-server
// with Envoy AI Gateway (Envoy Gateway + AI Gateway layer, Backend + MCPRoute
// CRDs) instead of Kuadrant (Istio + controller + broker + MCPServerRegistration).
//
// The RFC 8693 Standard Token Exchange itself lives entirely inside
// kube-mcp-server (pkg/kubernetes/sts.go) and is gateway-agnostic -- only the
// edge routing/OAuth validation layer differs between the two lanes (see the
// "Key design decision" section of the EAIGW plan doc, and ADR-068 Decision
// #9). This suite therefore reuses the exact same Keycloak realm and
// passthrough+STS kube-mcp-server config as the Kuadrant lane.
//
// Test scenarios: E2E-FMC-EAIGW-054-010 (sync_journey_test.go),
// E2E-FMC-EAIGW-054-011 (least_privilege_test.go), E2E-FMC-EAIGW-054-012
// (resilience_test.go), E2E-FMC-EAIGW-054-013 (dynamic_registration_test.go),
// E2E-FMC-EAIGW-054-014 (token_exchange_test.go). See
// docs/testing/BR-INTEGRATION-054/TEST_PLAN.md for the full scenario
// inventory and FedRAMP control coverage matrix.
//
// Authority: Issue #54, ADR-068, Spike S18, BR-INTEGRATION-065.
//
// Test Execution:
//
//	ginkgo -v ./test/e2e/fleetmetadatacache/eaigw/...
//
// Memory footprint: ~1.5-2.0GB (Keycloak + fleet-core w/ EAIGW in the primary
// cluster, plus a second, minimal Kind cluster running only kube-mcp-server
// for the cross-cluster isolation bridge, DD-TEST-013; no DataStorage -- FMC
// is audit-exempt per DD-AUDIT-003. EAIGW's own footprint (~316MB, Spike
// S18) is comparable to or lighter than Kuadrant's Istio+controller+broker
// stack).
package eaigw

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

const (
	clusterName = "fmc-eaigw-e2e"
	namespace   = "kubernaut-system"

	// fmcAPIBaseURL is FMC's own REST API (ScopeCheckPath, ClustersPath),
	// exposed via NodePort per DD-TEST-001 (no kubectl port-forward). Same
	// host port as the Kuadrant lane -- safe because this runs in its own
	// isolated Kind cluster. See
	// test/infrastructure/kind-fleetmetadatacache-eaigw-config.yaml and
	// SetupFMCE2EInfrastructureEAIGW's Phase 9.
	fmcAPIBaseURL = "http://localhost:8150"
)

// harness carries the state every shared FMC scenario needs (see
// shared.Harness); its fields are populated below in SynchronizedBeforeSuite.
var harness = &shared.Harness{
	Namespace:     namespace,
	FMCAPIBaseURL: fmcAPIBaseURL,
}

var (
	cancel context.CancelFunc

	anyTestFailed bool
)

func TestFleetMetadataCacheEAIGWE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fleet Metadata Cache EAIGW E2E Suite (Issue #54, Spike S18)")
}

var _ = SynchronizedBeforeSuite(
	func() []byte {
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		By("Setting up isolated kubeconfig path (per TESTING_GUIDELINES.md)")
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		tempKubeconfigPath := fmt.Sprintf("%s/.kube/%s-config", homeDir, clusterName)
		GinkgoWriter.Printf("Using isolated kubeconfig: %s\n", tempKubeconfigPath)

		By("Setting up FMC EAIGW E2E infrastructure (Issue #54, Spike S18)")
		bgCtx := context.Background()
		fmcImage, remoteKubeconfigPath, setupErr := infrastructure.SetupFMCE2EInfrastructureEAIGW(bgCtx, clusterName, tempKubeconfigPath, GinkgoWriter)
		Expect(setupErr).ToNot(HaveOccurred(), "FMC EAIGW E2E infrastructure setup failed")
		_ = fmcImage

		By("Setting KUBECONFIG for all processes")
		Expect(os.Setenv("KUBECONFIG", tempKubeconfigPath)).To(Succeed())

		GinkgoWriter.Println("FMC EAIGW E2E infrastructure ready (Process 1)")
		return []byte(tempKubeconfigPath + "\n" + remoteKubeconfigPath)
	},
	func(data []byte) {
		parts := strings.SplitN(string(data), "\n", 2)
		harness.KubeconfigPath = parts[0]
		harness.RemoteKubeconfigPath = parts[1]
		harness.Ctx, cancel = context.WithCancel(context.TODO())

		GinkgoWriter.Printf("FMC EAIGW E2E - Setup (Process %d)\n", GinkgoParallelProcess())

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

		GinkgoWriter.Printf("FMC EAIGW E2E Setup Complete - Process %d ready\n", GinkgoParallelProcess())
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
		By("Cleaning up FMC EAIGW E2E environment")

		setupFailed := harness.KubeconfigPath == ""
		anyFailure := infrastructure.ResolveAnyFailure(clusterName, setupFailed, anyTestFailed, GinkgoWriter)
		defer infrastructure.CleanupFailureMarker(clusterName)
		preserveCluster := os.Getenv("PRESERVE_E2E_CLUSTER") == "true" || os.Getenv("KEEP_CLUSTER") == "true"

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

			// Envoy Gateway (envoy-gateway-system) and the AI Gateway
			// controller (envoy-ai-gateway-system) are deployed by
			// DeployFleetCoreInfra's deployEnvoyAIGatewayInfra but live
			// outside kubernaut-system, so the call above never captures
			// them -- mirrors the Kuadrant lane's mcp-system/gateway-system/
			// istio-system must-gather (see suite_test.go there).
			for _, ns := range []string{"envoy-gateway-system", "envoy-ai-gateway-system"} {
				infrastructure.MustGatherPodLogs(clusterName, harness.KubeconfigPath, ns, "fleetmetadatacache", GinkgoWriter)
			}

			// Remote cluster (DD-TEST-013, Spike S19): only kube-mcp-server
			// runs there, but it's the component the "prod-east" cross-cluster
			// isolation scenario depends on most heavily.
			if harness.RemoteKubeconfigPath != "" {
				infrastructure.MustGatherPodLogs(remoteClusterName, harness.RemoteKubeconfigPath, namespace, "fleetmetadatacache", GinkgoWriter)
			}
		}

		if os.Getenv("E2E_COVERAGE") == "true" && !setupFailed {
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
		if delErr := infrastructure.DeleteCluster(clusterName, "fleetmetadatacache-eaigw", anyFailure, GinkgoWriter); delErr != nil {
			GinkgoWriter.Printf("Warning: Failed to delete cluster: %v\n", delErr)
		}

		By("Deleting remote Kind cluster (DD-TEST-013, Spike S19)")
		if delErr := infrastructure.TeardownRemoteClusterForFMC(remoteClusterName, harness.RemoteKubeconfigPath, "fleetmetadatacache-eaigw", anyFailure, GinkgoWriter); delErr != nil {
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

		GinkgoWriter.Println("FMC EAIGW E2E cleanup complete")
	},
)
