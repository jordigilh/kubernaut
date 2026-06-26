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
// This suite deploys ALL Kubernaut services plus fleet infrastructure (Kuadrant
// MCP Gateway + FMC + Valkey + K8s MCP Server + DEX) in a single Kind cluster
// and validates the complete multi-cluster remediation lifecycle using the
// loopback pattern:
//
//	Alert → Gateway → SP(MCP enrich) → RO → WE(MCP dispatch) → EM
//
// The loopback pattern treats the same cluster as both local (hub) and remote
// (loopback-cluster) by routing MCP calls through Kuadrant → K8s MCP Server.
// Tool names use the `loopback_cluster_` prefix from MCPServerRegistration.
//
// Authority: Issue #54, ADR-068
//
// Test Execution:
//
//	FLEET_E2E=true ginkgo -v ./test/e2e/fleet/...
//
// IMPORTANT: This suite requires significant resources (~6.1GB RAM).
package fleet

import (
	"context"
	"crypto/tls"
	"encoding/json"
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

	kubeconfigPath string

	k8sClient client.Client
	apiReader client.Reader

	dataStorageClient *ogenclient.Client
	e2eAuthToken      string
	fpAuthToken       string

	afBaseURL    string
	afHTTPClient *http.Client
	afAuthToken  string

	workflowUUIDs map[string]string
	fpRemediateNS map[string]string

	anyTestFailed bool
)

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

		By("Setting up Fleet E2E infrastructure (Issue #54)")
		ctx := context.Background()
		images, seededUUIDs, remediateNS, err := infrastructure.SetupFleetE2EInfrastructure(
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

		By("Labeling kubernaut-system namespace as managed (for fleet E2E tests)")
		labelCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", tempKubeconfigPath,
			"label", "namespace", namespace, "kubernaut.ai/managed=true", "--overwrite")
		labelOut, labelErr := labelCmd.CombinedOutput()
		Expect(labelErr).ToNot(HaveOccurred(), "Failed to label namespace: %s", string(labelOut))

		By("Creating per-test remediate namespaces (dynamic, UUID-based)")
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

		By("Deploying memory-eater in kubernaut-system for fleet E2E tests")
		err = infrastructure.DeployMemoryEater(ctx, namespace, tempKubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Failed to deploy memory-eater")

		By("Setting KUBECONFIG for all processes")
		err = os.Setenv("KUBECONFIG", tempKubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		GinkgoWriter.Println("Fleet E2E infrastructure ready (Process 1)")

		uuidsJSON, jsonErr := json.Marshal(seededUUIDs)
		Expect(jsonErr).ToNot(HaveOccurred())
		remediateNSJSON, nsErr := json.Marshal(remediateNS)
		Expect(nsErr).ToNot(HaveOccurred())
		return []byte(fmt.Sprintf("%s|%s|%s|%s|%s", tempKubeconfigPath, token, string(uuidsJSON), gatewayToken, string(remediateNSJSON)))
	},
	func(data []byte) {
		parts := strings.SplitN(string(data), "|", 5)
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
			homeDir, _ := os.UserHomeDir()
			kp := fmt.Sprintf("%s/.kube/%s-config", homeDir, clusterName)
			infrastructure.MustGatherPodLogs(clusterName, kp, "kubernaut-system", "fleet", GinkgoWriter)
			infrastructure.MustGatherPodLogs(clusterName, kp, "kubernaut-workflows", "fleet", GinkgoWriter)
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
