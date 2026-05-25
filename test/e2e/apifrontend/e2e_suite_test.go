package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Suite — AF + KA + DS Integration")
}

const (
	e2eClusterName = "apifrontend-e2e"
	e2eNamespace   = "kubernaut-system"
)

var (
	setupSucceeded bool
	anyTestFailed  bool
	kubeconfigPath string
	k8sClient      client.Client
	clientset      *kubernetes.Clientset
)

var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
		kinfra.MarkTestFailure(e2eClusterName)
	}
})

var _ = SynchronizedBeforeSuite(
	func() []byte {
		homeDir, err := os.UserHomeDir()
		Expect(err).NotTo(HaveOccurred())
		kubeconfigPath = fmt.Sprintf("%s/.kube/apifrontend-e2e-config", homeDir)

		if os.Getenv("AF_E2E_SKIP_INFRA") == "true" {
			_, _ = fmt.Fprintln(GinkgoWriter, "Skipping infra deployment (AF_E2E_SKIP_INFRA=true)")
			setupSucceeded = true
			return []byte(kubeconfigPath)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()

		err = kinfra.SetupAPIFrontendE2EInfrastructure(ctx, e2eClusterName, kubeconfigPath, e2eNamespace, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred(), "E2E infrastructure setup failed")

		if os.Getenv("AF_E2E_SKIP_PROMETHEUS") != "true" {
			_, _ = fmt.Fprintln(GinkgoWriter, "\nDeploying Prometheus for severity triage testing...")
			err = kinfra.DeployPrometheusForSeverityTriage(ctx, e2eNamespace, kubeconfigPath, GinkgoWriter)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Prometheus deployment failed (non-fatal for non-triage tests): %v\n", err)
			} else {
				promURL := "http://localhost:9190"
				_, _ = fmt.Fprintln(GinkgoWriter, "  Injecting OTLP metrics for severity triage alerts...")
				if ierr := kinfra.AFInjectOTLPMetrics(ctx, promURL, "e2e_cpu_usage_percent", 95, map[string]string{
					"namespace": "default", "kind": "Deployment", "name": "test-firing-target",
				}); ierr != nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "  WARNING: CPU metric injection failed: %v\n", ierr)
				}
				if ierr := kinfra.AFInjectOTLPMetrics(ctx, promURL, "e2e_memory_usage_percent", 90, map[string]string{
					"namespace": "default", "kind": "Deployment", "name": "test-pending-target",
				}); ierr != nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "  WARNING: Memory metric injection failed: %v\n", ierr)
				}
				// NOTE: e2e_disk_usage_percent is NOT injected here — injected at test
				// time in TC-E2E-SEV-03 to exploit the rule evaluation timing window.
				_, _ = fmt.Fprintln(GinkgoWriter, "  Waiting for HighCPU alert to fire...")
				if werr := kinfra.WaitForPrometheusRuleState(ctx, promURL, "HighCPU", kinfra.RuleStateFiring, 60*time.Second); werr != nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "  WARNING: HighCPU did not reach firing state: %v\n", werr)
				}
			}
		}

		_, _ = fmt.Fprintln(GinkgoWriter, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		_, _ = fmt.Fprintln(GinkgoWriter, "E2E Infrastructure Ready")
		_, _ = fmt.Fprintln(GinkgoWriter, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		setupSucceeded = true
		return []byte(kubeconfigPath)
	},
	func(data []byte) {
		kubeconfigPath = string(data)
		baseURL = "https://localhost:18443"
		caCertPath = filepath.Join(os.TempDir(), "apifrontend-e2e-certs", "ca.crt")
		dexURL = "http://localhost:5556/dex"
		clientID = "kubernaut-apifrontend"
		clientSecret = "e2e-client-secret"
		username = "e2e-user@kubernaut.ai"
		password = "password"
		httpClient = newTLSClient(caCertPath)

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

		healthURL := "http://localhost:18081"
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
		}, 30*time.Second, 2*time.Second).Should(Succeed(), "AF should be reachable over TLS (https://localhost:18443)")
	},
)

var _ = SynchronizedAfterSuite(
	func() {},
	func() {
		_, _ = fmt.Fprintln(GinkgoWriter, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		_, _ = fmt.Fprintln(GinkgoWriter, "AF E2E Test Suite - Teardown")
		_, _ = fmt.Fprintln(GinkgoWriter, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		setupFailed := !setupSucceeded
		anyFailure := setupFailed || anyTestFailed || kinfra.CheckTestFailure(e2eClusterName)
		defer kinfra.CleanupFailureMarker(e2eClusterName)

		if anyFailure {
			_, _ = fmt.Fprintln(GinkgoWriter, "⚠️  Failure detected — collecting must-gather logs BEFORE teardown")
			kinfra.MustGatherPodLogs(e2eClusterName, kubeconfigPath,
				e2eNamespace, "apifrontend", GinkgoWriter)
		}

		_, _ = fmt.Fprintln(GinkgoWriter, "\nCollecting E2E binary coverage data (DD-TEST-007)...")
		if err := kinfra.CollectAFE2EBinaryCoverage(e2eClusterName, GinkgoWriter); err != nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Coverage collection failed (non-fatal): %v\n", err)
		}

		if os.Getenv("AF_E2E_SKIP_TEARDOWN") == "true" {
			_, _ = fmt.Fprintln(GinkgoWriter, "Skipping teardown (AF_E2E_SKIP_TEARDOWN=true)")
			return
		}
		if os.Getenv("AF_E2E_SKIP_INFRA") == "true" {
			return
		}

		if err := kinfra.DeleteCluster(e2eClusterName, "apifrontend", anyFailure, GinkgoWriter); err != nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Cluster deletion failed: %v\n", err)
		}
	},
)
