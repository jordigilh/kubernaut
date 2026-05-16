package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
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
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()

		// Full infrastructure setup: kubernaut stack (KA+DS+PostgreSQL+Redis+
		// mock-LLM+DEX+CRDs) + AF image + kustomize deploy.
		err = infrastructure.SetupE2EInfrastructure(ctx, e2eClusterName, kubeconfigPath, e2eNamespace, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred(), "E2E infrastructure setup failed")

		// Deploy Prometheus for severity triage pipeline testing (G12).
		if os.Getenv("AF_E2E_SKIP_PROMETHEUS") != "true" {
			_, _ = fmt.Fprintln(GinkgoWriter, "\nDeploying Prometheus for severity triage testing...")
			err = infrastructure.DeployPrometheusForSeverityTriage(ctx, e2eNamespace, kubeconfigPath, GinkgoWriter)
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Prometheus deployment failed (non-fatal for non-triage tests): %v\n", err)
			}
		}

		_, _ = fmt.Fprintln(GinkgoWriter, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		_, _ = fmt.Fprintln(GinkgoWriter, "E2E Infrastructure Ready")
		_, _ = fmt.Fprintln(GinkgoWriter, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		return nil
	},
	func(_ []byte) {
		baseURL = getEnvOrDefault("AF_E2E_BASE_URL", "https://localhost:18443")
		caCertPath = getEnvOrDefault("AF_E2E_CA_CERT", "")
		dexURL = getEnvOrDefault("AF_E2E_DEX_URL", "http://localhost:15556/dex")
		clientID = getEnvOrDefault("AF_E2E_CLIENT_ID", "kubernaut-apifrontend")
		clientSecret = getEnvOrDefault("AF_E2E_CLIENT_SECRET", "e2e-client-secret")
		username = getEnvOrDefault("AF_E2E_USERNAME", "e2e-user@kubernaut.ai")
		password = getEnvOrDefault("AF_E2E_PASSWORD", "password")
		httpClient = newTLSClient(caCertPath)

		Eventually(func() error {
			resp, err := httpClient.Get(baseURL + "/healthz")
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
		// DD-TEST-007: Collect binary coverage data when E2E_COVERAGE=true.
		// Ref: https://go.dev/doc/build-cover
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
