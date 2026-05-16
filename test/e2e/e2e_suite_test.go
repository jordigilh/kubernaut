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

	"github.com/jordigilh/kubernaut-apifrontend/test/infrastructure"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Suite — AF + KA + DS Integration")
}

const (
	e2eClusterName = "apifrontend-e2e"
	e2eNamespace   = "kubernaut-system"
)

var _ = SynchronizedBeforeSuite(
	func() []byte {
		homeDir, err := os.UserHomeDir()
		Expect(err).NotTo(HaveOccurred())
		kubeconfigPath := fmt.Sprintf("%s/.kube/apifrontend-e2e-config", homeDir)

		if os.Getenv("AF_E2E_SKIP_INFRA") == "true" {
			_, _ = fmt.Fprintln(GinkgoWriter, "Skipping infra deployment (AF_E2E_SKIP_INFRA=true)")
			return []byte(kubeconfigPath)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()

		err = infrastructure.SetupE2EInfrastructure(ctx, e2eClusterName, kubeconfigPath, e2eNamespace, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred(), "E2E infrastructure setup failed")

		if os.Getenv("AF_E2E_SKIP_PROMETHEUS") != "true" {
			_, _ = fmt.Fprintln(GinkgoWriter, "\nDeploying Prometheus for severity triage testing...")
			err = infrastructure.DeployPrometheusForSeverityTriage(ctx, e2eNamespace, kubeconfigPath, GinkgoWriter)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Prometheus deployment failed (non-fatal for non-triage tests): %v\n", err)
			} else {
				promURL := "http://localhost:9190"
				_, _ = fmt.Fprintln(GinkgoWriter, "  Injecting OTLP metrics for severity triage alerts...")
				if ierr := infrastructure.InjectOTLPMetrics(ctx, promURL, "e2e_cpu_usage_percent", 95, map[string]string{
					"namespace": "default", "kind": "Deployment", "name": "test-firing-target",
				}); ierr != nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "  WARNING: CPU metric injection failed: %v\n", ierr)
				}
				if ierr := infrastructure.InjectOTLPMetrics(ctx, promURL, "e2e_memory_usage_percent", 90, map[string]string{
					"namespace": "default", "kind": "Deployment", "name": "test-pending-target",
				}); ierr != nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "  WARNING: Memory metric injection failed: %v\n", ierr)
				}
				// NOTE: e2e_disk_usage_percent is NOT injected here — injected at test
				// time in TC-E2E-SEV-03 to exploit the rule evaluation timing window.
				_, _ = fmt.Fprintln(GinkgoWriter, "  Waiting for HighCPU alert to fire...")
				if werr := infrastructure.WaitForPrometheusRuleState(ctx, promURL, "HighCPU", infrastructure.RuleStateFiring, 60*time.Second); werr != nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "  WARNING: HighCPU did not reach firing state: %v\n", werr)
				}
			}
		}

		_, _ = fmt.Fprintln(GinkgoWriter, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		_, _ = fmt.Fprintln(GinkgoWriter, "E2E Infrastructure Ready")
		_, _ = fmt.Fprintln(GinkgoWriter, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		return []byte(kubeconfigPath)
	},
	func(_ []byte) {
		baseURL = "https://localhost:18443"
		caCertPath = filepath.Join(os.TempDir(), "apifrontend-e2e-certs", "ca.crt")
		dexURL = "http://localhost:15556/dex"
		clientID = "kubernaut-apifrontend"
		clientSecret = "e2e-client-secret"
		username = "e2e-user@kubernaut.ai"
		password = "password"
		httpClient = newTLSClient(caCertPath)

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
		}, 60*time.Second, 2*time.Second).Should(Succeed(), "AF should become healthy")
	},
)

var _ = SynchronizedAfterSuite(
	func() {},
	func() {
		if os.Getenv("E2E_COVERAGE") == "true" {
			_, _ = fmt.Fprintln(GinkgoWriter, "\nCollecting E2E binary coverage data (DD-TEST-007)...")
			profilePath, err := infrastructure.CollectE2EBinaryCoverage(e2eClusterName, GinkgoWriter)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Coverage collection failed: %v\n", err)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Coverage profile: %s\n", profilePath)
			}
		}

		if os.Getenv("AF_E2E_SKIP_TEARDOWN") == "true" {
			_, _ = fmt.Fprintln(GinkgoWriter, "Skipping teardown (AF_E2E_SKIP_TEARDOWN=true)")
			return
		}
		if os.Getenv("AF_E2E_SKIP_INFRA") == "true" {
			return
		}
		_ = infrastructure.TeardownE2EInfrastructure(GinkgoWriter)
	},
)
