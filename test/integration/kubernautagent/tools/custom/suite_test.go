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
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/shared/integration"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

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
	dsInfra      *infrastructure.DSBootstrapInfra
	ogenClient   *ogenclient.Client
	workflowUUIDs map[string]string
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
			sharedK8sConfig, "ka-custom-tools-sa", "default", GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred())
		_ = kubeconfigPath

		By("Starting DataStorage infrastructure (PostgreSQL, Redis, DataStorage)")
		cfg := infrastructure.NewDSBootstrapConfigWithAuth(
			"kubernautagent",
			kaPostgresPort, kaRedisPort, kaDataStoragePort, kaMetricsPort,
			"test/integration/kubernautagent/tools/custom/config",
			authConfig,
		)
		dsInfra, err = infrastructure.StartDSBootstrap(cfg, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "DS infrastructure must start")
		dsInfra.SharedTestEnv = sharedTestEnv

		By("Seeding test workflows via DataStorage API")
		dsURL := fmt.Sprintf("http://127.0.0.1:%d", kaDataStoragePort)
		dsClients := integration.NewAuthenticatedDataStorageClients(dsURL, authConfig.Token, 5*time.Second)

		testWorkflows := []infrastructure.TestWorkflow{
			{
				WorkflowID:  "oom-recovery-v1",
				Name:        "oom-recovery",
				Description: "OOM recovery workflow",
				ActionType:  "IncreaseMemoryLimits",
				Environment: "production",
			},
			{
				WorkflowID:  "oomkill-increase-memory-v1",
				Name:        "oomkill-increase-memory",
				Description: "OOMKill memory increase workflow",
				ActionType:  "IncreaseMemoryLimits",
				Environment: "production",
			},
			{
				WorkflowID:  "oom-recovery-aggressive-v1",
				Name:        "oom-recovery-aggressive",
				Description: "Aggressive OOM recovery with wildcard labels",
				ActionType:  "IncreaseMemoryLimits",
				Environment: "production",
			},
		}
		wfUUIDs, err := infrastructure.SeedWorkflowsInDataStorage(
			dsClients.OpenAPIClient, testWorkflows, "ka-custom-tools", GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "workflow seeding must succeed")
		GinkgoWriter.Printf("✅ Seeded workflows: %v\n", wfUUIDs)

		GinkgoWriter.Println("✅ Phase 1 complete - DataStorage infrastructure ready")

		// Serialize token + workflow UUIDs for Phase 2
		payload := authConfig.Token
		for k, v := range wfUUIDs {
			payload += "\n" + k + "=" + v
		}
		return []byte(payload)
	},

	// Phase 2: Per-process setup (all processes)
	func(data []byte) {
		lines := splitLines(string(data))
		token := lines[0]

		workflowUUIDs = make(map[string]string)
		for _, line := range lines[1:] {
			if parts := splitKV(line); len(parts) == 2 {
				workflowUUIDs[parts[0]] = parts[1]
			}
		}

		dsURL := fmt.Sprintf("http://127.0.0.1:%d", kaDataStoragePort)
		dsClients := integration.NewAuthenticatedDataStorageClients(dsURL, token, 5*time.Second)
		ogenClient = dsClients.OpenAPIClient

		GinkgoWriter.Printf("✅ Phase 2 complete - ogen client ready, %d workflow UUIDs\n", len(workflowUUIDs))
	},
)

var _ = SynchronizedAfterSuite(
	func() { /* per-process cleanup */ },
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
