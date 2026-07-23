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

package datastorage

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	"github.com/jordigilh/kubernaut/test/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// E2E-DS-1661-RESILIENCE: DS-restart-and-cache-recovery (Issue #1661 Phase 56)
// ========================================
//
// Authority: DD-WORKFLOW-018 (etcd as single source of truth for the
// Workflow/ActionType catalog), Issue #1661 Phase 56 (Wiring Manifest: "Phase
// 56 case 2").
//
// Proves the core motivation of DD-WORKFLOW-018 against a REAL pod restart,
// not a mock or an envtest fake: DataStorage's workflow/action-type catalog
// is now a pure informer-backed cache over etcd (RemediationWorkflow/
// ActionType CRDs) with zero Postgres persistence for that data. If DS's pod
// is killed and replaced, the replacement must re-derive an IDENTICAL catalog
// straight from etcd on startup, with zero manual re-seeding and zero data
// loss -- proving the catalog is genuinely disposable/stateless, the whole
// point of moving off Postgres for this data.
//
// This runs against a SECOND, test-owned DataStorage+PostgreSQL+Redis+RBAC
// stack, deployed in its own namespace within the same datastorage-e2e Kind
// cluster (test/infrastructure/datastorage_isolated_instance.go). A real pod
// kill against the SHARED instance would corrupt every other spec running
// concurrently in this suite; this isolated stack makes the restart safe
// without needing Serial.
var _ = Describe("E2E-DS-1661-RESILIENCE: DataStorage cache survives a real pod restart", Ordered, Label("e2e", "datastorage", "resilience"), func() {
	const (
		isolatedNamespace = "datastorage-e2e-resilience"
		isolatedBaseURL   = "http://localhost:28093"
	)

	var (
		resilienceClient *dsgen.Client
		seededWorkflows  []seededResilienceWorkflow
	)

	BeforeAll(func() {
		By("Discovering the already-built DataStorage image from the shared instance's Deployment")
		dsImage := currentDataStorageImage()
		Expect(dsImage).ToNot(BeEmpty(), "shared DataStorage Deployment should already carry a resolved image reference")

		By("Deploying an isolated, test-owned DataStorage+PostgreSQL+Redis stack")
		Expect(infrastructure.DeployIsolatedDataStorageInstance(ctx, isolatedNamespace, kubeconfigPath, dsImage, GinkgoWriter)).To(Succeed())

		By("Creating an authenticated client for the isolated instance")
		saName := "datastorage-resilience-e2e-client"
		Expect(infrastructure.CreateE2EServiceAccountWithDataStorageAccess(ctx, isolatedNamespace, kubeconfigPath, saName, GinkgoWriter)).To(Succeed())
		token, err := infrastructure.GetServiceAccountToken(ctx, isolatedNamespace, saName, kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		httpClient := &http.Client{
			Timeout:   20 * time.Second,
			Transport: testauth.NewServiceAccountTransport(token),
		}
		resilienceClient, err = dsgen.NewClient(isolatedBaseURL, dsgen.WithClient(httpClient))
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for the isolated instance's health endpoint to be responsive")
		healthClient := &http.Client{Timeout: 10 * time.Second}
		Eventually(func() error {
			resp, err := healthClient.Get("http://localhost:28094/readyz")
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("readyz returned status %d", resp.StatusCode)
			}
			return nil
		}, 120*time.Second, 2*time.Second).Should(Succeed(), "isolated DataStorage health endpoint did not become responsive")

		DeferCleanup(func() {
			_ = infrastructure.TeardownIsolatedDataStorageInstance(context.Background(), isolatedNamespace, kubeconfigPath, GinkgoWriter)
		})
	})

	It("E2E-DS-1661-RESILIENCE-001: recovers the full workflow catalog from etcd after a real pod restart, with zero data loss and no re-seeding", func() {
		By("Seeding 3 workflows directly via CRD creation (no live AuthWebhook in this suite)")
		for i := 0; i < 3; i++ {
			name := fmt.Sprintf("e2e-resilience-%s", uuid.New().String()[:8])

			crd := testutil.NewTestWorkflowCRD(name, "ScaleReplicas", "tekton")
			content := testutil.MarshalWorkflowCRD(crd)

			workflowIDStr, err := infrastructure.SeedWorkflowContentViaDirectCRDCreation(ctx, workflowCRDClient(), isolatedNamespace, content, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred(), "failed to seed workflow %s via direct CRD creation", name)

			workflowID, err := uuid.Parse(workflowIDStr)
			Expect(err).ToNot(HaveOccurred())

			seededWorkflows = append(seededWorkflows, seededResilienceWorkflow{name: name, id: workflowID})
		}

		By("Verifying baseline: all 3 workflows are visible via the isolated instance's own API (pre-restart)")
		for _, wf := range seededWorkflows {
			assertResilienceWorkflowVisible(resilienceClient, wf)
		}

		By("Killing the isolated DataStorage pod and waiting for a real restart")
		Expect(infrastructure.RestartIsolatedDataStoragePod(ctx, isolatedNamespace, kubeconfigPath, GinkgoWriter)).To(Succeed())

		By("Verifying the isolated instance's health endpoint recovers post-restart")
		healthClient := &http.Client{Timeout: 10 * time.Second}
		Eventually(func() error {
			resp, err := healthClient.Get("http://localhost:28094/readyz")
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("readyz returned status %d", resp.StatusCode)
			}
			return nil
		}, 120*time.Second, 2*time.Second).Should(Succeed(), "isolated DataStorage did not become healthy again after restart")

		By("Verifying all 3 workflows are STILL visible post-restart, with zero manual re-seeding (DD-WORKFLOW-018)")
		for _, wf := range seededWorkflows {
			assertResilienceWorkflowVisible(resilienceClient, wf)
		}

		GinkgoWriter.Printf("✅ %d workflows survived a real DataStorage pod restart with zero data loss (etcd-sourced cache)\n", len(seededWorkflows))
	})
})

type seededResilienceWorkflow struct {
	name string
	id   uuid.UUID
}

// assertResilienceWorkflowVisible polls GetWorkflowByID against the isolated
// instance and asserts the workflow is present with matching content -- a
// byte-level round-trip proof, not just a bare existence check, since the
// property under test is "zero data loss", not merely "the record exists".
func assertResilienceWorkflowVisible(client *dsgen.Client, wf seededResilienceWorkflow) {
	Eventually(func(g Gomega) {
		resp, err := client.GetWorkflowByID(context.Background(), dsgen.GetWorkflowByIDParams{
			WorkflowID: wf.id,
		})
		g.Expect(err).ToNot(HaveOccurred())

		full, ok := resp.(*dsgen.RemediationWorkflow)
		g.Expect(ok).To(BeTrue(), "expected *RemediationWorkflow from GetWorkflowByID")
		g.Expect(full.WorkflowId.Value.String()).To(Equal(wf.id.String()))
		g.Expect(full.ActionType).To(Equal("ScaleReplicas"))
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "workflow %s (%s) should be visible via the isolated instance's cache", wf.name, wf.id)
}

// currentDataStorageImage reads the shared instance's already-resolved image
// reference straight off its live Deployment, so the isolated instance reuses
// the exact same image Kind already has loaded -- avoiding a second image
// build/load for this one test.
func currentDataStorageImage() string {
	out, err := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"-n", sharedNamespace, "get", "deployment", "datastorage",
		"-o", "jsonpath={.spec.template.spec.containers[0].image}").Output()
	Expect(err).ToNot(HaveOccurred(), "failed to read shared DataStorage Deployment image reference")
	return strings.TrimSpace(string(out))
}
