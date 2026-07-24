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

package custom_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/workflowcatalog"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/shared/integration"
)

// seedNamespace is the namespace RemediationWorkflow CRDs are seeded into
// directly (no AuthWebhook in this suite -- see SeedWorkflowsViaDirectCRDCreation),
// matching CreateIntegrationServiceAccountWithDataStorageAccess's "default" below.
const seedNamespace = "default"

func TestKubernautAgentCustomToolsIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kubernaut Agent Custom Tools Integration Suite — #433")
}

// Port allocation: DD-TEST-001 freed band 13322-13331 (former Immudb).
const (
	kaPostgresPort    = 13322
	kaRedisPort       = 13323
	kaDataStoragePort = 13324
	kaMetricsPort     = 13325
)

var (
	dsInfra       *infrastructure.DSBootstrapInfra
	ogenClient    *ogenclient.Client
	workflowUUIDs map[string]string

	// #1677 Phase 2e (DD-WORKFLOW-019): the 3 discovery tools are now
	// catalog-backed (KA's own informer cache), not DS-ogen-client-backed --
	// wfCatalog replaces ogenClient as custom.NewAllTools' first argument.
	// Built per-process (Phase 2 below) from the same envtest cluster
	// ogenClient's auth ServiceAccount was created against.
	wfCatalog       *workflowcatalog.Catalog
	wfCatalogCancel context.CancelFunc
)

var _ = SynchronizedBeforeSuite(
	// Phase 1: Start shared infrastructure (Process 1 only)
	func() []byte {
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("KA Custom Tools IT - PHASE 1: Infrastructure Setup")
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		By("Starting envtest for DataStorage authentication (DD-AUTH-014)")
		sharedTestEnv := &envtest.Environment{
			CRDDirectoryPaths:     []string{"../../../../../config/crd/bases"},
			ErrorIfCRDPathMissing: true,
		}
		sharedK8sConfig, err := sharedTestEnv.Start()
		Expect(err).ToNot(HaveOccurred(), "envtest should start")
		GinkgoWriter.Printf("✅ envtest started at %s\n", sharedK8sConfig.Host)

		kubeconfigPath, err := infrastructure.WriteEnvtestKubeconfigToFile(sharedK8sConfig, "ka-custom-tools")
		Expect(err).ToNot(HaveOccurred())

		By("Creating ServiceAccount for DataStorage authentication")
		authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(
			sharedK8sConfig, "ka-custom-tools-sa", seedNamespace, GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred())

		Expect(rwv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		seedK8sClient, err := client.New(sharedK8sConfig, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred(), "build seed k8s client")

		By("Starting DataStorage infrastructure (PostgreSQL, Redis, DataStorage)")
		cfg := infrastructure.NewDSBootstrapConfigWithAuth(
			"kubernautagent",
			kaPostgresPort, kaRedisPort, kaDataStoragePort, kaMetricsPort,
			"test/integration/kubernautagent/tools/custom/config",
			authConfig,
		)
		dsInfra, err = infrastructure.StartDSBootstrap(context.Background(), cfg, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "DS infrastructure must start")
		dsInfra.SharedTestEnv = sharedTestEnv

		By("Seeding ActionType CRDs (#1677 Phase 2e: required by KA's own catalog cache, not just DS)")
		Expect(infrastructure.SeedActionTypesViaCRD(context.Background(), kubeconfigPath, seedNamespace, GinkgoWriter)).To(Succeed())

		By("Seeding test workflows via direct CRD creation (#1661 Phase 55: no AuthWebhook in this suite)")
		testWorkflows := []infrastructure.WorkflowSeedSpec{
			{FixtureDir: "oom-recovery", Environment: "production"},
			{FixtureDir: "oomkill-increase-memory", Environment: "production"},
			{FixtureDir: "oom-recovery-aggressive", Environment: "production"},
		}
		wfUUIDs, err := infrastructure.SeedWorkflowsViaDirectCRDCreation(
			context.Background(), seedK8sClient, seedNamespace, testWorkflows, GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "workflow seeding must succeed")
		GinkgoWriter.Printf("✅ Seeded workflows: %v\n", wfUUIDs)

		GinkgoWriter.Println("✅ Phase 1 complete - DataStorage infrastructure ready")

		// Serialize kubeconfig path + token + workflow UUIDs for Phase 2.
		payload := kubeconfigPath + "\n" + authConfig.Token
		for k, v := range wfUUIDs {
			payload += "\n" + k + "=" + v
		}
		return []byte(payload)
	},

	// Phase 2: Per-process setup (all processes)
	func(data []byte) {
		lines := splitLines(string(data))
		kubeconfigPath := lines[0]
		token := lines[1]

		workflowUUIDs = make(map[string]string)
		for _, line := range lines[2:] {
			if parts := splitKV(line); len(parts) == 2 {
				workflowUUIDs[parts[0]] = parts[1]
			}
		}

		dsURL := fmt.Sprintf("http://127.0.0.1:%d", kaDataStoragePort)
		dsClients := integration.NewAuthenticatedDataStorageClients(dsURL, token, 5*time.Second)
		ogenClient = dsClients.OpenAPIClient

		// #1677 Phase 2e: build KA's own informer-backed catalog against the
		// same envtest cluster, one per Ginkgo process (mirrors production:
		// each KA replica owns its own cache, DD-WORKFLOW-019).
		restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "build rest.Config from kubeconfig")

		wfScheme, err := workflowcatalog.NewScheme()
		Expect(err).ToNot(HaveOccurred())

		wfCache, cancel, err := workflowcatalog.NewInformerCache(restConfig, wfScheme, logr.Discard())
		Expect(err).ToNot(HaveOccurred(), "workflow catalog cache must sync")
		wfCatalogCancel = cancel
		wfCatalog = workflowcatalog.NewCatalog(wfCache, logr.Discard())

		GinkgoWriter.Printf("✅ Phase 2 complete - ogen client + workflow catalog ready, %d workflow UUIDs\n", len(workflowUUIDs))
	},
)

var _ = SynchronizedAfterSuite(
	func() {
		if wfCatalogCancel != nil {
			wfCatalogCancel()
		}
	},
	func() {
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("KA Custom Tools IT - Infrastructure Cleanup")
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		if dsInfra != nil {
			infrastructure.MustGatherContainerLogs("kubernautagent", []string{
				dsInfra.DataStorageContainer,
				dsInfra.PostgresContainer,
				dsInfra.RedisContainer,
			}, GinkgoWriter)
			_ = infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
		}
		GinkgoWriter.Println("✅ Suite complete")
	},
)

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func splitKV(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return nil
}
