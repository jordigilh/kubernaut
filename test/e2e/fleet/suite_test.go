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

// Package fleet contains the Fleet E2E test suite (Issue #54).
//
// This suite deploys ALL Kubernaut services in a PRIMARY Kind cluster plus
// fleet infrastructure (Kuadrant MCP Gateway + FMC + Valkey + K8s MCP Server
// + Keycloak) and a genuinely separate REMOTE Kind cluster (DD-TEST-013),
// and validates the complete multi-cluster remediation lifecycle:
//
//	Alert → GW → SP(MCP enrich) → AA(MCP investigate) → WE(MCP dispatch) → EM → NT
//
// Every registered cluster identity (loopback-cluster, prod-east, prod-west)
// is backed by the SAME remote bridge to the remote cluster's kube-mcp-server
// (KubeMCPServerAuthConfig.AllRegistrationsRemote, test/infrastructure/fleet_e2e.go)
// -- unlike the "loopback" pattern this suite used before, there is no local
// K8s MCP Server and no cluster identity secretly resolves against the
// primary cluster. kube-mcp-server runs in passthrough mode with a real RFC
// 8693 Standard Token Exchange against Keycloak (mirrors the FMC E2E lane;
// Keycloak replaces DEX here because DEX has no Standard Token Exchange,
// Spike S17/S20). Tool names use the `loopback_cluster_` prefix from
// MCPServerRegistration.
//
// Because every fleet cluster identity is now remote, any K8s object a test
// wants Gateway/SP/RO/WE to discover, scope-check, or dispatch against via
// MCP (the memory-eater fixture, per-test target Deployments, CoreDNS pod
// discovery for enrichment) must be created against remoteK8sClient, NOT
// k8sClient. Kubernaut's own CRDs (RemediationRequest, SignalProcessing,
// AIAnalysis, WorkflowExecution, ...) are reconciled by controllers running
// in the PRIMARY cluster and stay on k8sClient.
//
// Authority: Issue #54, ADR-068, DD-TEST-013, Spike S17/S19/S20
//
// Test Execution:
//
//	FLEET_E2E=true ginkgo -v ./test/e2e/fleet/...
//
// IMPORTANT: This suite requires significant resources (primary cluster
// ~6.1GB RAM + remote cluster ~1.7-2.5GB RAM, Keycloak vs DEX's ~64MB
// accepted to validate the real production token-exchange wiring end-to-end).
package fleet

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
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

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

const (
	timeout  = 10 * time.Minute
	interval = 500 * time.Millisecond

	clusterName = "fleet-e2e"
	namespace   = "kubernaut-system"

	mcpGatewayURL = "http://localhost:31975/mcp"
)

var (
	ctx    context.Context
	cancel context.CancelFunc

	kubeconfigPath       string
	remoteKubeconfigPath string

	k8sClient client.Client
	apiReader client.Reader

	// remoteK8sClient targets the second Kind cluster (DD-TEST-013) that
	// backs loopback-cluster/prod-east/prod-west (AllRegistrationsRemote,
	// see test/infrastructure/fleet_e2e.go). Kubernaut's own CRDs
	// (RemediationRequest, SignalProcessing, etc.) are reconciled by
	// controllers running in the PRIMARY cluster and stay on k8sClient;
	// only the "target" resources a fleet alert claims to remediate (and
	// anything discovered for MCP enrichment, e.g. a CoreDNS Pod) must live
	// here, since that's the cluster kube-mcp-server actually reads from
	// once AllRegistrationsRemote is set.
	remoteK8sClient client.Client

	dataStorageClient *ogenclient.Client
	e2eAuthToken      string
	fpAuthToken       string

	afBaseURL    string
	afHTTPClient *http.Client

	workflowUUIDs map[string]string
	fpRemediateNS map[string]string

	anyTestFailed bool
)

// postWithFleetAuth sends an authenticated POST request to the Gateway.
// Uses fpAuthToken (BR-GATEWAY-036/037) provisioned in BeforeSuite.
func postWithFleetAuth(url, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	if fpAuthToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", fpAuthToken))
	}
	return http.DefaultClient.Do(req)
}

// fmcSyncTimeout bounds the retry window for postFleetAlertUntilAccepted.
//
// Root cause (Issue #54 SOC2 gap RCA, CI run 28495045499): every fleet E2E
// alert carries a non-empty cluster label, so Gateway's scope check
// (pkg/gateway/server.go validateScope) routes through
// pkg/fleet.FederatedScopeChecker.IsManagedResource, which for non-empty
// ClusterID ALWAYS delegates to the remote FleetMetadataCache (FMC) HTTP
// backend (pkg/fleet/fmc/http_client.go) -- never the local K8s informer
// cache (pkg/shared/scope/manager.go), which only applies to empty
// ClusterID (hub-local) signals.
//
// FMC does not watch resources live: pkg/fleet/fmc/syncer.go polls every
// registered cluster (via MCP tools, one List call per resource kind) on a
// fixed ticker and writes results to Valkey; the HTTP scope-check endpoint
// only ever answers from that cache. The e2e fleetmetadatacache-config
// ConfigMap (test/infrastructure/fleet_e2e.go) sets sync.interval=10s, so a
// resource created/labeled by a test immediately before posting an alert is
// only guaranteed to be visible to FMC's scope-check endpoint after the next
// full sync tick. Because syncAll() iterates every registered cluster
// (loopback-cluster, prod-east, prod-west) x 6 resource kinds sequentially,
// a single cycle can itself take non-trivial wall time, so worst-case
// staleness exceeds the nominal 10s interval. 45s gives ~2 sync cycles of
// margin; a 15s window (the previous value) was measured insufficient and
// caused persistent "resource not managed by Kubernaut" rejections for the
// full retry window. Rejected responses have no side effects (no RR is
// created), so retrying the POST is safe.
const fmcSyncTimeout = 45 * time.Second

// postFleetAlertUntilAccepted posts a Prometheus alert payload to the Gateway and
// retries while the response status is not one of acceptableStatus (defaults to
// 201 Created). See fmcSyncTimeout for why the retry window must exceed FMC's
// sync interval.

func postFleetAlertUntilAccepted(gatewayURL string, payload []byte, acceptableStatus ...int) (int, []byte) {
	if len(acceptableStatus) == 0 {
		acceptableStatus = []int{http.StatusCreated}
	}
	var (
		statusCode int
		respBody   []byte
	)
	Eventually(func(g Gomega) {
		resp, err := postWithFleetAuth(gatewayURL+"/api/v1/signals/prometheus",
			"application/json", strings.NewReader(string(payload)))
		g.Expect(err).ToNot(HaveOccurred())
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		g.Expect(readErr).ToNot(HaveOccurred())

		statusCode, respBody = resp.StatusCode, body
		g.Expect(acceptableStatus).To(ContainElement(resp.StatusCode),
			"Gateway should accept the alert (body: %s)", string(body))
	}, fmcSyncTimeout, 1*time.Second).Should(Succeed())
	return statusCode, respBody
}

// newFleetMCPClient creates an MCP client with auto-discovered tool prefix.
// Kuadrant uses "loopback_cluster_" (from MCPServerRegistration spec.prefix),
// not the EAIGW "{clusterID}__" convention. DiscoverToolPrefix queries
// tools/list and extracts the correct prefix for the given cluster.
//
// Retries up to 90s to handle the broker sync delay where the MCP gateway
// hasn't finished syncing tools from kube-mcp-server yet (~60s observed in
// spike S15).
func newFleetMCPClient(ctx context.Context, clusterID string) (*mcpclient.Client, error) {
	const (
		maxRetries    = 18
		retryInterval = 5 * time.Second
	)

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		c, err := mcpclient.New(ctx, mcpGatewayURL)
		if err != nil {
			lastErr = fmt.Errorf("connect to MCP gateway (attempt %d/%d): %w", attempt+1, maxRetries+1, err)
			time.Sleep(retryInterval)
			continue
		}

		prefix, err := mcpclient.DiscoverToolPrefix(ctx, c.Session(), clusterID)
		if err != nil {
			c.Close()
			lastErr = fmt.Errorf("discover tool prefix for %q (attempt %d/%d): %w", clusterID, attempt+1, maxRetries+1, err)
			if attempt < maxRetries {
				GinkgoWriter.Printf("  Waiting for broker sync (attempt %d/%d): %v\n", attempt+1, maxRetries+1, err)
				time.Sleep(retryInterval)
				continue
			}
			return nil, lastErr
		}
		c.Close()

		return mcpclient.New(ctx, mcpGatewayURL, mcpclient.WithClusterID(clusterID), mcpclient.WithToolPrefix(prefix))
	}
	return nil, fmt.Errorf("broker sync timeout after %d attempts: %w", maxRetries+1, lastErr)
}

func TestFleetE2E(t *testing.T) {
	if os.Getenv("FLEET_E2E") != "true" {
		t.Skip("FLEET_E2E=true required for fleet E2E tests")
	}
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fleet E2E Suite (Issue #54)")
}

var _ = SynchronizedBeforeSuite(
	func() []byte {
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		By("Setting up isolated kubeconfig path (per TESTING_GUIDELINES.md)")
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		tempKubeconfigPath := fmt.Sprintf("%s/.kube/%s-config", homeDir, clusterName)
		GinkgoWriter.Printf("Using isolated kubeconfig: %s\n", tempKubeconfigPath)

		if os.Getenv("FLEET_E2E_REUSE_CLUSTER") == "true" {
			GinkgoWriter.Println("⚡ FLEET_E2E_REUSE_CLUSTER=true — skipping infrastructure setup, reusing existing cluster")

			By("Retrieving existing ServiceAccount tokens")
			ctx := context.Background()
			token, tokenErr := infrastructure.GetServiceAccountToken(ctx, namespace, "fleet-e2e-sa", tempKubeconfigPath)
			Expect(tokenErr).ToNot(HaveOccurred(), "fleet-e2e-sa must exist in reused cluster")
			gatewayToken, gwErr := infrastructure.GetServiceAccountToken(ctx, namespace, "fleet-gateway-sa", tempKubeconfigPath)
			Expect(gwErr).ToNot(HaveOccurred(), "fleet-gateway-sa must exist in reused cluster")

			Expect(os.Setenv("KUBECONFIG", tempKubeconfigPath)).To(Succeed())
			GinkgoWriter.Println("Fleet E2E reuse-cluster ready (Process 1)")

			// Deterministic path SetupFleetE2EInfrastructure/SetupRemoteClusterForFMC
			// always uses -- the remote cluster from the prior run is still up.
			reuseRemoteKcPath := fmt.Sprintf("%s/.kube/%s-remote-config", homeDir, clusterName)
			return []byte(fmt.Sprintf("%s|%s|%s|%s|%s|%s", tempKubeconfigPath, token, "{}", gatewayToken, "{}", reuseRemoteKcPath))
		}

		By("Setting up Fleet E2E infrastructure (Issue #54)")
		ctx := context.Background()
		images, seededUUIDs, remediateNS, remoteKcPath, err := infrastructure.SetupFleetE2EInfrastructure(
			ctx, clusterName, tempKubeconfigPath, GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Fleet E2E infrastructure setup failed")
		_ = images

		Expect(seededUUIDs).To(HaveKey("crashloop-config-fix-v1:production"))
		Expect(seededUUIDs).To(HaveKey("oomkill-increase-memory-v1:production"))

		By("Creating E2E ServiceAccount for DataStorage authentication (DD-AUTH-014)")
		e2eSAName := "fleet-e2e-sa"
		err = infrastructure.CreateE2EServiceAccountWithDataStorageAccess(
			ctx, namespace, tempKubeconfigPath, e2eSAName, GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create E2E ServiceAccount")

		token, err := infrastructure.GetServiceAccountToken(ctx, namespace, e2eSAName, tempKubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to get E2E ServiceAccount token")

		By("Creating E2E ServiceAccount for Gateway authentication (BR-GATEWAY-036/037)")
		gatewaySAName := "fleet-gateway-sa"
		err = infrastructure.CreateE2EServiceAccountWithGatewayAccess(
			ctx, namespace, tempKubeconfigPath, gatewaySAName, GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Gateway SA")
		gatewayToken, err := infrastructure.GetServiceAccountToken(ctx, namespace, gatewaySAName, tempKubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to get Gateway SA token")

		By("Labeling kubernaut-system namespace as managed (primary + remote, for fleet E2E tests)")
		for _, kc := range []string{tempKubeconfigPath, remoteKcPath} {
			labelCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kc,
				"label", "namespace", namespace, "kubernaut.ai/managed=true", "--overwrite")
			labelOut, labelErr := labelCmd.CombinedOutput()
			Expect(labelErr).ToNot(HaveOccurred(), "Failed to label namespace on %s: %s", kc, string(labelOut))
		}

		By("Creating per-test remediate namespaces (dynamic, UUID-based; primary cluster -- Kubernaut CRDs)")
		for key, ns := range remediateNS {
			GinkgoWriter.Printf("  Creating namespace %s (scenario: %s)\n", ns, key)
			createNSCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", tempKubeconfigPath,
				"create", "namespace", ns)
			createNSOut, createNSErr := createNSCmd.CombinedOutput()
			if createNSErr != nil && !strings.Contains(string(createNSOut), "already exists") {
				Expect(createNSErr).ToNot(HaveOccurred(), "Failed to create namespace %s: %s", ns, string(createNSOut))
			}
			labelNSCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", tempKubeconfigPath,
				"label", "namespace", ns,
				"kubernaut.ai/managed=true", "kubernaut.ai/environment=staging", "--overwrite")
			labelNSOut, labelNSErr := labelNSCmd.CombinedOutput()
			Expect(labelNSErr).ToNot(HaveOccurred(), "Failed to label namespace %s: %s", ns, string(labelNSOut))
		}

		// The shared "memory-eater" fixture (referenced by clusterID=loopback-cluster/
		// prod-east/prod-west across 01_signal_ingestion_test.go and
		// 03_ro_clusterid_routing_test.go) must live on the REMOTE cluster now that
		// AllRegistrationsRemote backs every registered cluster identity with the
		// same remote bridge -- kube-mcp-server reads it from there, not the
		// primary cluster.
		By("Deploying memory-eater in remote cluster's kubernaut-system for fleet E2E tests")
		err = infrastructure.DeployMemoryEater(ctx, namespace, remoteKcPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Failed to deploy memory-eater")

		By("Setting KUBECONFIG for all processes")
		err = os.Setenv("KUBECONFIG", tempKubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		GinkgoWriter.Println("Fleet E2E infrastructure ready (Process 1)")

		uuidsJSON, jsonErr := json.Marshal(seededUUIDs)
		Expect(jsonErr).ToNot(HaveOccurred())
		remediateNSJSON, nsErr := json.Marshal(remediateNS)
		Expect(nsErr).ToNot(HaveOccurred())
		return []byte(fmt.Sprintf("%s|%s|%s|%s|%s|%s", tempKubeconfigPath, token, string(uuidsJSON), gatewayToken, string(remediateNSJSON), remoteKcPath))
	},
	func(data []byte) {
		parts := strings.SplitN(string(data), "|", 6)
		kubeconfigPath = parts[0]
		if len(parts) > 1 {
			e2eAuthToken = parts[1]
		}
		if len(parts) > 2 {
			Expect(json.Unmarshal([]byte(parts[2]), &workflowUUIDs)).To(Succeed(),
				"Failed to decode workflow UUIDs from Process 1")
		}
		if len(parts) > 3 {
			fpAuthToken = parts[3]
		}
		if len(parts) > 4 {
			Expect(json.Unmarshal([]byte(parts[4]), &fpRemediateNS)).To(Succeed(),
				"Failed to decode remediate namespaces from Process 1")
		}
		if len(parts) > 5 {
			remoteKubeconfigPath = parts[5]
		}

		ctx, cancel = context.WithCancel(context.TODO())

		GinkgoWriter.Printf("Fleet E2E - Setup (Process %d)\n", GinkgoParallelProcess())

		By("Setting KUBECONFIG")
		err := os.Setenv("KUBECONFIG", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		tlsTransport, tlsTErr := infrastructure.NewTLSAwareTransport(kubeconfigPath)
		Expect(tlsTErr).ToNot(HaveOccurred(), "Failed to create TLS-aware transport (Issue #785)")
		http.DefaultTransport = tlsTransport

		By("Registering ALL CRD schemes")
		Expect(remediationv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(eav1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(isv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

		By("Creating Kubernetes client")
		cfg, err := config.GetConfig()
		Expect(err).ToNot(HaveOccurred())

		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())

		apiReader, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())

		By("Creating remote cluster's Kubernetes client (DD-TEST-013: backs loopback-cluster/prod-east/prod-west)")
		remoteCfg, remoteCfgErr := clientcmd.BuildConfigFromFlags("", remoteKubeconfigPath)
		Expect(remoteCfgErr).ToNot(HaveOccurred(), "failed to build remote cluster kubeconfig")
		remoteK8sClient, err = client.New(remoteCfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())

		By("Setting up authenticated DataStorage client (DD-TEST-001: port 30081, Issue #785: HTTPS)")
		dataStorageURL := "https://localhost:30081"
		tlsBase, tlsCErr := infrastructure.NewTLSAwareTransport(kubeconfigPath)
		Expect(tlsCErr).ToNot(HaveOccurred(), "TLS transport for DataStorage client")
		saTransport := testauth.NewServiceAccountTransportWithBase(e2eAuthToken, tlsBase)
		httpClient := &http.Client{
			Timeout:   20 * time.Second,
			Transport: saTransport,
		}
		dataStorageClient, err = ogenclient.NewClient(dataStorageURL, ogenclient.WithClient(httpClient))
		Expect(err).ToNot(HaveOccurred())

		By("Setting up API Frontend HTTP client (NodePort 30443, self-signed TLS)")
		afBaseURL = "https://localhost:30443"
		afHTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // E2E self-signed cert
			},
			Timeout: 30 * time.Second,
		}

		GinkgoWriter.Printf("Fleet E2E Setup Complete - Process %d ready\n", GinkgoParallelProcess())
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
		By("Cleaning up Fleet E2E environment")

		setupFailed := k8sClient == nil
		anyFailure := infrastructure.ResolveAnyFailure(clusterName, setupFailed, anyTestFailed, GinkgoWriter)
		defer infrastructure.CleanupFailureMarker(clusterName)
		preserveCluster := os.Getenv("PRESERVE_E2E_CLUSTER") == "true" || os.Getenv("KEEP_CLUSTER") == "true" || os.Getenv("FLEET_E2E_REUSE_CLUSTER") == "true"

		remoteClusterName := clusterName + "-remote"

		if preserveCluster {
			GinkgoWriter.Println("CLUSTER PRESERVED FOR DEBUGGING")
			GinkgoWriter.Printf("   To access: export KUBECONFIG=%s\n", kubeconfigPath)
			GinkgoWriter.Printf("   To delete: kind delete cluster --name %s\n", clusterName)
			GinkgoWriter.Printf("   Remote cluster (DD-TEST-013, backs loopback-cluster/prod-east/prod-west): export KUBECONFIG=%s\n", remoteKubeconfigPath)
			GinkgoWriter.Printf("   To delete: kind delete cluster --name %s\n", remoteClusterName)
			return
		}

		if anyFailure && !setupFailed {
			homeDir, _ := os.UserHomeDir()
			kp := fmt.Sprintf("%s/.kube/%s-config", homeDir, clusterName)
			infrastructure.MustGatherPodLogs(clusterName, kp, "kubernaut-system", "fleet", GinkgoWriter)
			infrastructure.MustGatherPodLogs(clusterName, kp, "kubernaut-workflows", "fleet", GinkgoWriter)

			for _, ns := range []string{"mcp-system", "gateway-system", "istio-system"} {
				infrastructure.MustGatherPodLogs(clusterName, kp, ns, "fleet", GinkgoWriter)
			}
			if remoteKubeconfigPath != "" {
				infrastructure.MustGatherPodLogs(remoteClusterName, remoteKubeconfigPath, "kubernaut-system", "fleet", GinkgoWriter)
			}
		}

		if !setupFailed {
			infrastructure.CleanupFullPipelineTestResources(kubeconfigPath, GinkgoWriter)
		}

		if os.Getenv("E2E_COVERAGE") == "true" && !setupFailed {
			for _, svc := range []struct{ service, deployment string }{
				{"signalprocessing", "signalprocessing-controller"},
				{"remediationorchestrator", "remediationorchestrator-controller"},
				{"aianalysis", "aianalysis-controller"},
				{"workflowexecution", "workflowexecution-controller"},
				{"effectivenessmonitor", "effectivenessmonitor-controller"},
			} {
				if err := infrastructure.CollectE2EBinaryCoverage(infrastructure.E2ECoverageOptions{
					ServiceName:    svc.service,
					ClusterName:    clusterName,
					DeploymentName: svc.deployment,
					Namespace:      namespace,
					KubeconfigPath: kubeconfigPath,
				}, GinkgoWriter); err != nil {
					GinkgoWriter.Printf("Coverage collection failed for %s (non-fatal): %v\n", svc.service, err)
				}
			}
		}

		By("Deleting Kind cluster")
		if err := infrastructure.DeleteCluster(clusterName, "fleet", anyFailure, GinkgoWriter); err != nil {
			GinkgoWriter.Printf("Warning: Failed to delete cluster: %v\n", err)
		}

		By("Deleting remote Kind cluster (DD-TEST-013)")
		if err := infrastructure.TeardownRemoteClusterForFMC(remoteClusterName, remoteKubeconfigPath, "fleet", anyFailure, GinkgoWriter); err != nil {
			GinkgoWriter.Printf("Warning: Failed to delete remote cluster: %v\n", err)
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

		GinkgoWriter.Println("Fleet E2E cleanup complete")
	},
)
