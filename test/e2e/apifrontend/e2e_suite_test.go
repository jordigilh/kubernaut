package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	investigationsessionv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	kinfra "github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Suite — AF + KA + DS Integration")
}

const (
	e2eNamespace                 = "kubernaut-system"
	trueFixture                  = "true"
	fleetAFEnvoyGatewayNamespace = "envoy-gateway-system"
	fleetAFAIGatewayNamespace    = "envoy-ai-gateway-system"
)

var (
	e2eClusterName        = getEnvOrDefault("AF_E2E_CLUSTER_NAME", kinfra.AFDefaultClusterName)
	setupSucceeded        bool
	anyTestFailed         bool
	fleetAFSetupAttempted bool
	kubeconfigPath        string
	k8sClient             client.Client
	clientset             *kubernetes.Clientset
)

// apifrontendE2EClusterNames returns every cluster that may contain partial
// lane state and therefore needs failure diagnostics or teardown.
func apifrontendE2EClusterNames(fleetSetupAttempted bool) []string {
	clusters := []string{e2eClusterName}
	if fleetSetupAttempted {
		clusters = append(clusters, fleetAFClusterName)
	}
	return clusters
}

// apifrontendMustGatherExtraNamespaces adds Fleet's EAIGW controller namespaces
// to its bundle; those controller logs are outside the Helm release namespace.
func apifrontendMustGatherExtraNamespaces(clusterName string, fleetSetupAttempted bool) []string {
	if !fleetSetupAttempted || clusterName != fleetAFClusterName {
		return nil
	}
	return []string{fleetAFEnvoyGatewayNamespace, fleetAFAIGatewayNamespace}
}

var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
		kinfra.MarkTestFailure(e2eClusterName)
		if fleetAFEnabled {
			kinfra.MarkTestFailure(fleetAFClusterName)
		}
	}
})

var _ = SynchronizedBeforeSuite(
	func() []byte {
		homeDir, err := os.UserHomeDir()
		Expect(err).NotTo(HaveOccurred())
		kubeconfigPath = getEnvOrDefault("AF_E2E_KUBECONFIG", filepath.Join(homeDir, ".kube", e2eClusterName+"-config"))

		// Helper-only specs use httptest and do not need a Kind cluster.
		helperTestsOnly := os.Getenv("AF_E2E_HELPER_TESTS_ONLY") == trueFixture
		if os.Getenv("AF_E2E_SKIP_INFRA") == trueFixture || helperTestsOnly {
			if helperTestsOnly {
				_, _ = fmt.Fprintln(GinkgoWriter, "Skipping cluster setup (AF_E2E_HELPER_TESTS_ONLY=true)")
			} else {
				_, _ = fmt.Fprintln(GinkgoWriter, "Skipping infra deployment (AF_E2E_SKIP_INFRA=true)")
			}
			setupSucceeded = true
			setup, marshalErr := json.Marshal(afE2ESuiteSetup{
				LocalKubeconfigPath: kubeconfigPath,
				LocalCertDir:        getEnvOrDefault("AF_E2E_CERT_DIR", filepath.Join(os.TempDir(), "apifrontend-e2e-certs", e2eClusterName)),
				FleetClusterName:    fleetAFClusterName,
				LocalHostPortOffset: e2eHostPort(0),
			})
			Expect(marshalErr).NotTo(HaveOccurred())
			return setup
		}

		localHostPortOffset, err := fleetAFIsolatedLocalPortOffset()
		Expect(err).NotTo(HaveOccurred(), "local AF host ports must be offset from the Fleet cluster")
		Expect(os.Setenv("AF_E2E_HOST_PORT_OFFSET", strconv.Itoa(localHostPortOffset))).To(Succeed())

		fleetClusterName := fleetAFClusterName
		Expect(fleetClusterName).NotTo(Equal(e2eClusterName), "local and Fleet AF suites require distinct Kind clusters")
		fleetKubeconfigPath := getEnvOrDefault("AF_E2E_FLEET_KUBECONFIG", filepath.Join(homeDir, ".kube", fleetClusterName+"-config"))
		Expect(fleetKubeconfigPath).NotTo(Equal(kubeconfigPath), "local and Fleet AF clusters require distinct kubeconfigs")
		localCertDir := getEnvOrDefault("AF_E2E_CERT_DIR", filepath.Join(os.TempDir(), "apifrontend-e2e-certs", e2eClusterName))
		fleetCertDir := filepath.Join(os.TempDir(), "apifrontend-e2e-certs", fleetClusterName)
		Expect(os.Setenv("AF_E2E_CERT_DIR", localCertDir)).To(Succeed())

		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
		defer cancel()

		apiFrontendImages, err := kinfra.SetupAPIFrontendE2EInfrastructure(ctx, e2eClusterName, kubeconfigPath, e2eNamespace, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred(), "E2E infrastructure setup failed")

		if os.Getenv("AF_E2E_SKIP_PROMETHEUS") != trueFixture {
			_, _ = fmt.Fprintln(GinkgoWriter, "\nDeploying Prometheus for severity triage testing...")
			// The standalone AF lane is single-cluster and has no Gateway
			// registration identity; fleet attribution is covered separately by
			// test/e2e/fleet.
			err = kinfra.DeployPrometheusForSeverityTriage(ctx, e2eNamespace, "", kubeconfigPath, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred(), "Prometheus deployment must succeed for severity triage tests")

			promURL := e2eHostURL("http", 9190)

			_, _ = fmt.Fprintln(GinkgoWriter, "  Waiting for Prometheus readiness...")
			Expect(kinfra.WaitForPrometheusReady(ctx, promURL, 90*time.Second, GinkgoWriter)).
				To(Succeed(), "Prometheus must become ready within 90s")

			_, _ = fmt.Fprintln(GinkgoWriter, "  Injecting OTLP metrics for severity triage alerts...")
			// #1839: dedicated namespaces (not "default") so these fixtures
			// cannot accidentally backstop an unrelated test that shares the
			// "default" namespace but forgot to configure its own grounding
			// (see apifrontend_prometheus_e2e.go's SeverityTriageAlertRulesYAML).
			Expect(kinfra.AFInjectOTLPMetrics(ctx, promURL, "e2e_cpu_usage_percent", 95, map[string]string{
				"namespace": "sev-tier1-ns", "kind": "Deployment", "name": "test-firing-target",
			})).To(Succeed(), "CPU metric injection must succeed")
			Expect(kinfra.AFInjectOTLPMetrics(ctx, promURL, "e2e_memory_usage_percent", 90, map[string]string{
				"namespace": "sev-tier15-ns", "kind": "Deployment", "name": "test-pending-target",
			})).To(Succeed(), "Memory metric injection must succeed")

			// NOTE: e2e_disk_usage_percent is NOT injected here — injected at test
			// time in TC-E2E-SEV-03 to exploit the rule evaluation timing window.
			_, _ = fmt.Fprintln(GinkgoWriter, "  Waiting for HighCPU alert to fire...")
			Expect(kinfra.WaitForPrometheusRuleState(ctx, promURL, "HighCPU", kinfra.RuleStateFiring, 120*time.Second)).
				To(Succeed(), "HighCPU alert must reach firing state within 120s")
		}

		_, _ = fmt.Fprintln(GinkgoWriter, "\nSetting up the isolated Fleet-mode AF E2E cluster...")
		fleetAFSetupAttempted = true
		fmcImage, fmcImageErr := kinfra.BuildImageForKind(ctx, kinfra.E2EImageConfig{
			ServiceName:    "fleetmetadatacache",
			ImageName:      "fleetmetadatacache",
			DockerfilePath: "docker/fleetmetadatacache.Dockerfile",
			EnableCoverage: os.Getenv("E2E_COVERAGE") == trueFixture,
		}, GinkgoWriter)
		Expect(fmcImageErr).NotTo(HaveOccurred(), "Fleet Metadata Cache image must be available")
		fleetSetupErr := kinfra.SetupAPIFrontendFleetE2EInfrastructure(
			ctx, fleetClusterName, fleetKubeconfigPath, e2eNamespace, apiFrontendImages, fmcImage, GinkgoWriter)
		Expect(fleetSetupErr).NotTo(HaveOccurred(), "Fleet AF infrastructure setup failed")
		Expect(os.Setenv("KUBECONFIG", kubeconfigPath)).To(Succeed(), "restore the local AF cluster as the default kubeconfig")

		Expect(prepareFleetAFFixtures(ctx, fleetKubeconfigPath)).To(Succeed(), "Fleet AF resource fixtures must be ready")
		By("IT-INFRA-AF-FLEET-2462-008: confirming the lean AF Fleet setup created no remote Kind cluster")
		Expect(verifyFleetAFHubOnlyTopology(ctx, e2eClusterName, fleetClusterName)).To(Succeed())

		Expect(configureFleetAFMockLLM(ctx, fleetKubeconfigPath, GinkgoWriter)).To(Succeed(),
			"Fleet AF mock-LLM must include cluster-attributed test scenarios")
		Expect(kinfra.DeployPrometheusForSeverityTriage(ctx, e2eNamespace, "hub", fleetKubeconfigPath, GinkgoWriter)).To(Succeed(),
			"Fleet AF severity fixtures must carry the registered hub identity")
		Expect(injectMetricForTier2(ctx, fleetAFPrometheusURL, "e2e_cpu_usage_percent", 95, map[string]string{
			"namespace": "sev-tier1-ns", "kind": "Deployment", "name": "test-firing-target",
		})).To(Succeed(), "Fleet AF firing-alert metric must be injected")
		Expect(injectMetricForTier2(ctx, fleetAFPrometheusURL, "e2e_memory_usage_percent", 90, map[string]string{
			"namespace": "sev-tier15-ns", "kind": "Deployment", "name": "test-pending-target",
		})).To(Succeed(), "Fleet AF pending-alert metric must be injected")
		Expect(injectMetricForTier2(ctx, fleetAFPrometheusURL, "e2e_disk_usage_percent", 80, map[string]string{
			"namespace": "sev-tier2-ns", "kind": "Deployment", "name": "test-inactive-target",
		})).To(Succeed(), "Fleet AF inactive-rule metric must be injected")
		for _, rule := range []struct {
			name  string
			state kinfra.PrometheusRuleState
		}{
			{"HighCPU", kinfra.RuleStateFiring},
			{"HighMemory", kinfra.RuleStatePending},
			{"AFInvestigateGrounding", kinfra.RuleStateFiring},
			{"FleetSessionActiveGrounding", kinfra.RuleStateFiring},
			{"FleetUnregisteredClusterGrounding", kinfra.RuleStateFiring},
			{"StructuredDecisionGrounding", kinfra.RuleStateFiring},
		} {
			Expect(waitForFleetAFPrometheusRule(ctx, rule.name, rule.state)).To(Succeed(),
				"Fleet AF grounding rule %s must reach %s", rule.name, rule.state)
		}

		_, _ = fmt.Fprintln(GinkgoWriter, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		_, _ = fmt.Fprintln(GinkgoWriter, "E2E Infrastructure Ready")
		_, _ = fmt.Fprintln(GinkgoWriter, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		setupSucceeded = true
		Expect(os.Setenv("AF_E2E_CERT_DIR", localCertDir)).To(Succeed())
		Expect(os.Setenv("CERT_DIR", localCertDir)).To(Succeed())
		Expect(os.Setenv("AF_E2E_CA_CERT", filepath.Join(localCertDir, "ca.crt"))).To(Succeed())
		setup, marshalErr := json.Marshal(afE2ESuiteSetup{
			LocalKubeconfigPath: kubeconfigPath,
			LocalCertDir:        localCertDir,
			FleetKubeconfigPath: fleetKubeconfigPath,
			FleetCertDir:        fleetCertDir,
			FleetClusterName:    fleetClusterName,
			LocalHostPortOffset: localHostPortOffset,
		})
		Expect(marshalErr).NotTo(HaveOccurred())
		return setup
	},
	func(data []byte) {
		var setup afE2ESuiteSetup
		Expect(json.Unmarshal(data, &setup)).To(Succeed(), "suite setup context must decode")
		kubeconfigPath = setup.LocalKubeconfigPath
		Expect(os.Setenv("KUBECONFIG", kubeconfigPath)).To(Succeed())
		Expect(os.Setenv("AF_E2E_HOST_PORT_OFFSET", strconv.Itoa(setup.LocalHostPortOffset))).To(Succeed())
		baseURL = e2eHostURL("https", 18443)
		caCertPath = filepath.Join(setup.LocalCertDir, "ca.crt")
		dexURL = e2eHostURL("https", 5556) + "/dex"
		clientID = "kubernaut-apifrontend"
		clientSecret = "e2e-client-secret"
		username = "e2e-user@kubernaut.ai"
		password = "password"
		if os.Getenv("AF_E2E_HELPER_TESTS_ONLY") != trueFixture {
			httpClient = newTLSClient(caCertPath)
		} else {
			return
		}

		By("Building Kubernetes clients from kubeconfig")
		restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		Expect(err).NotTo(HaveOccurred(), "failed to build REST config from kubeconfig")

		crScheme := k8sscheme.Scheme
		Expect(remediationv1alpha1.AddToScheme(crScheme)).To(Succeed())
		Expect(investigationsessionv1alpha1.AddToScheme(crScheme)).To(Succeed())

		k8sClient, err = client.New(restCfg, client.Options{Scheme: crScheme})
		Expect(err).NotTo(HaveOccurred(), "failed to create controller-runtime client")
		clientset, err = kubernetes.NewForConfig(restCfg)
		Expect(err).NotTo(HaveOccurred(), "failed to create kubernetes clientset")

		fleetAFClusterName = setup.FleetClusterName
		fleetAFKubeconfigPath = setup.FleetKubeconfigPath
		fleetAFEnabled = fleetAFKubeconfigPath != ""
		if fleetAFEnabled {
			fleetAFBaseURL = "https://localhost:18443"
			fleetAFHTTPClient = newTLSClientForKubeconfig(filepath.Join(setup.FleetCertDir, "ca.crt"), fleetAFKubeconfigPath)

			fleetRESTConfig, fleetConfigErr := clientcmd.BuildConfigFromFlags("", fleetAFKubeconfigPath)
			Expect(fleetConfigErr).NotTo(HaveOccurred(), "failed to build Fleet REST config")
			fleetScheme := k8sscheme.Scheme
			Expect(remediationv1alpha1.AddToScheme(fleetScheme)).To(Succeed())
			Expect(investigationsessionv1alpha1.AddToScheme(fleetScheme)).To(Succeed())
			fleetAFK8sClient, err = client.New(fleetRESTConfig, client.Options{Scheme: fleetScheme})
			Expect(err).NotTo(HaveOccurred(), "failed to create Fleet Kubernetes client")

			Eventually(func() error {
				healthCtx, healthCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer healthCancel()
				req, requestErr := http.NewRequestWithContext(healthCtx, http.MethodGet, fleetAFBaseURL+"/healthz", nil)
				if requestErr != nil {
					return requestErr
				}
				resp, requestErr := fleetAFHTTPClient.Do(req)
				if requestErr != nil {
					return requestErr
				}
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					return fmt.Errorf("fleet AF healthz returned %d", resp.StatusCode)
				}
				return nil
			}, 60*time.Second, 2*time.Second).Should(Succeed(), "Fleet-mode AF must be healthy over TLS")
		}

		// #2022/#2025/ADR-053: the mock-LLM's dedicated investigate fixture
		// (af_investigate/af_progressive_investigate scenarios, see
		// deploy/apifrontend/overlays/e2e/mock-llm.yaml) targets this
		// namespace, which — unlike sev-tier1-ns et al. — was never actually
		// created as a real Namespace object (only referenced as a string in
		// RR specs and Prometheus alert labels). AF's scope check now
		// fail-closes to unmanaged when neither the target resource nor its
		// namespace exists/is labeled, so this namespace must exist and
		// carry the managed label for kubernaut_investigate to proceed past
		// scope validation, matching a real Kubernaut deployment's setup.
		Expect(kinfra.EnsureManagedNamespace(context.Background(), k8sClient, "af-investigate-e2e")).
			To(Succeed(), "af-investigate-e2e namespace must exist and be labeled managed")

		// structured_decision_e2e_test.go's groundSession helper uses its own
		// dedicated namespace/target (StructuredDecisionGrounding alert,
		// apifrontend_prometheus_e2e.go) rather than reusing af-investigate-e2e
		// above, to avoid fixture contention with concurrent specs under
		// Ginkgo --procs>1 (CI run 31320575553, E2E-AF-1395-001).
		Expect(kinfra.EnsureManagedNamespace(context.Background(), k8sClient, "af-structured-decision-e2e")).
			To(Succeed(), "af-structured-decision-e2e namespace must exist and be labeled managed")
		helpers.EnsureTestPods(context.Background(), k8sClient, "af-structured-decision-e2e",
			"structured-decision-target", "structured-decision-target-2",
			"structured-decision-target-3", "structured-decision-target-4")

		healthURL := e2eHostURL("http", 18081)
		Eventually(func() error {
			resp, err := http.Get(healthURL + "/healthz") //nolint:gosec,noctx // E2E health probe
			if err != nil {
				return err
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("healthz returned %d", resp.StatusCode)
			}
			return nil
		}, 60*time.Second, 2*time.Second).Should(Succeed(), "AF should become healthy on HTTP")

		Eventually(func() error {
			resp, err := httpClient.Get(baseURL + "/healthz")
			if err != nil {
				return fmt.Errorf("TLS healthz failed: %w", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("TLS healthz returned %d", resp.StatusCode)
			}
			return nil
		}, 30*time.Second, 2*time.Second).Should(Succeed(), "AF should be reachable over TLS ("+baseURL+")")
	},
)

var _ = SynchronizedAfterSuite(
	func() {},
	func() {
		_, _ = fmt.Fprintln(GinkgoWriter, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		_, _ = fmt.Fprintln(GinkgoWriter, "AF E2E Test Suite - Teardown")
		_, _ = fmt.Fprintln(GinkgoWriter, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		setupFailed := !setupSucceeded
		anyFailure := kinfra.ResolveAnyFailure(e2eClusterName, setupFailed, anyTestFailed, GinkgoWriter)
		defer kinfra.CleanupFailureMarker(e2eClusterName)
		if fleetAFSetupAttempted {
			fleetFailure := kinfra.ResolveAnyFailure(fleetAFClusterName, setupFailed, anyTestFailed, GinkgoWriter)
			defer kinfra.CleanupFailureMarker(fleetAFClusterName)
			anyFailure = anyFailure || fleetFailure
		}

		if anyFailure {
			_, _ = fmt.Fprintln(GinkgoWriter, "⚠️  Failure detected — collecting must-gather diagnostics BEFORE teardown")
			var existingClusters []string
			for _, cluster := range apifrontendE2EClusterNames(fleetAFSetupAttempted) {
				if kindClusterExists(context.Background(), cluster) {
					existingClusters = append(existingClusters, cluster)
				}
			}
			if len(existingClusters) == 0 {
				_, _ = fmt.Fprintln(GinkgoWriter, "  No Kind clusters were created; skipping must-gather build")
			} else {
				// DD-TESTING-003: production must-gather image on the "kind" network.
				mustGatherImage, buildErr := kinfra.BuildMustGatherImageForE2E(context.Background(), GinkgoWriter)
				if buildErr != nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Failed to build must-gather image (non-fatal): %v\n", buildErr)
				} else {
					for _, cluster := range existingClusters {
						mustGatherOutputDir := filepath.Join("/tmp", "kubernaut-must-gather", "apifrontend", cluster)
						if err := kinfra.RunMustGatherImage(context.Background(), kinfra.RunMustGatherImageOptions{
							ClusterName:     cluster,
							Image:           mustGatherImage,
							OutputDir:       mustGatherOutputDir,
							Namespace:       e2eNamespace,
							UsePodman:       true,
							ExtraNamespaces: apifrontendMustGatherExtraNamespaces(cluster, fleetAFSetupAttempted),
						}, GinkgoWriter); err != nil {
							_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Failed to collect must-gather from %s (non-fatal): %v\n", cluster, err)
						}
					}
				}
			}
		}

		if kindClusterExists(context.Background(), e2eClusterName) {
			_, _ = fmt.Fprintln(GinkgoWriter, "\nCollecting E2E binary coverage data (DD-TEST-007)...")
			if err := kinfra.CollectAFE2EBinaryCoverage(e2eClusterName, GinkgoWriter); err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Coverage collection failed (non-fatal): %v\n", err)
			}
		}

		if os.Getenv("AF_E2E_SKIP_TEARDOWN") == trueFixture {
			_, _ = fmt.Fprintln(GinkgoWriter, "Skipping teardown (AF_E2E_SKIP_TEARDOWN=true)")
			return
		}
		if os.Getenv("AF_E2E_SKIP_INFRA") == trueFixture {
			return
		}

		for _, cluster := range apifrontendE2EClusterNames(fleetAFSetupAttempted) {
			if !kindClusterExists(context.Background(), cluster) {
				continue
			}
			if err := kinfra.DeleteCluster(cluster, "apifrontend", anyFailure, GinkgoWriter); err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Cluster %s deletion failed: %v\n", cluster, err)
			}
		}
	},
)
