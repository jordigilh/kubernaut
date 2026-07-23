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

package workflowcatalog_test

import (
	"errors"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/workflowcatalog"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// Issue #1677 Phase 2g (DD-WORKFLOW-019): relocates the DataStorage-side
// discovery edge-case IT coverage this replaces:
//   - pkg/datastorage/repository/workflow's DD-HAPI-017 three-step protocol +
//     wildcard-label (#464) + issue #522 reproduction
//     (test/integration/datastorage/workflow_discovery_repository_test.go)
//   - cluster classification filter (#1511, BR-FLEET-003)
//     (test/integration/datastorage/workflow_discovery_cluster_test.go)
//   - case-insensitive label matching (#595, DD-WORKFLOW-001 v2.9)
//     (test/integration/datastorage/workflow_discovery_case_insensitive_test.go)
//
// Unlike the DS originals (Serial, shared-cache, testID-prefixed to avoid
// cross-spec count pollution), every spec here builds its own isolated
// cache+catalog (BeforeEach, matching discovery_test.go) and uses a
// process-unique action type per spec, so no Serial decorator or global-count
// scoping is needed -- each spec's asserted counts are scoped to its own
// unique action type by construction.
//
// Business Requirements: BR-HAPI-017-001, BR-WORKFLOW-001, BR-WORKFLOW-004,
// BR-WORKFLOW-016, BR-FLEET-003.
var _ = Describe("IT-KA-1677 Workflow Catalog discovery edge cases (wildcards, cluster, case-insensitivity)", Label("integration", "kubernautagent", "workflow-catalog"), func() {

	var (
		wfCache     *workflowcatalog.Cache
		cacheCancel func()
		catalog     *workflowcatalog.Catalog
	)

	uniqueName := func(prefix string) string {
		return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), GinkgoParallelProcess())
	}
	uniquePascalName := func(prefix string) string {
		return fmt.Sprintf("%s%d%d", prefix, time.Now().UnixNano(), GinkgoParallelProcess())
	}

	BeforeEach(func() {
		scheme, schemeErr := workflowcatalog.NewScheme()
		Expect(schemeErr).ToNot(HaveOccurred())

		var err error
		wfCache, cacheCancel, err = workflowcatalog.NewInformerCache(k8sConfig, scheme, logger)
		Expect(err).ToNot(HaveOccurred(), "workflow catalog cache should build and sync against the shared envtest")
		catalog = workflowcatalog.NewCatalog(wfCache, logger)
	})

	AfterEach(func() {
		if cacheCancel != nil {
			cacheCancel()
		}
	})

	// seedActiveActionType creates an ActionType CRD with a unique spec.Name,
	// marks it Active (ListActions only ever surfaces Active action types --
	// discovery_cache.go's listActionsFromCache), and defers its cleanup.
	// Returns the spec.Name workflows should reference as their actionType.
	seedActiveActionType := func(prefix string) string {
		specName := uniquePascalName(prefix)
		// k8s object names must be lowercase RFC 1123 -- prefix is PascalCase
		// (e.g. "DHAPI017a"), so the metadata.name (unlike specName, the
		// ActionType's free-form spec.Name) must be lowercased.
		at := &atv1alpha1.ActionType{
			ObjectMeta: metav1.ObjectMeta{Name: uniqueName("at-" + strings.ToLower(prefix)), Namespace: "default"},
			Spec: atv1alpha1.ActionTypeSpec{
				Name: specName,
				Description: atv1alpha1.ActionTypeDescription{
					What:      "IT-KA-1677 discovery edge-case action type fixture",
					WhenToUse: "For workflow catalog discovery edge-case integration testing",
				},
			},
		}
		Expect(k8sClient.Create(ctx, at)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, at) })
		at.Status.CatalogStatus = sharedtypes.CatalogStatusActive
		Expect(k8sClient.Status().Update(ctx, at)).To(Succeed())

		Eventually(func() (sharedtypes.CatalogStatus, error) {
			got, err := wfCache.GetActionType(ctx, specName)
			if err != nil || got == nil {
				return "", err
			}
			return got.Status.CatalogStatus, nil
		}, 5*time.Second, 100*time.Millisecond).Should(Equal(sharedtypes.CatalogStatusActive),
			"cache must observe %s as Active before the spec relies on ListActions seeing it", specName)
		return specName
	}

	// rwFixture is the field set the ported scenarios below vary; zero-value
	// fields keep sensible single-value defaults (see newRW).
	type rwFixture struct {
		Severity    []string
		Component   []string
		Environment []string
		Priority    string
		Cluster     []string
	}

	newRW := func(name, actionType string, f rwFixture) *rwv1alpha1.RemediationWorkflow {
		severity := f.Severity
		if len(severity) == 0 {
			severity = []string{"critical"}
		}
		component := f.Component
		if len(component) == 0 {
			component = []string{"v1/Pod"}
		}
		environment := f.Environment
		if len(environment) == 0 {
			environment = []string{"production"}
		}
		priority := f.Priority
		if priority == "" {
			priority = "P1"
		}

		rw := &rwv1alpha1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: rwv1alpha1.RemediationWorkflowSpec{
				Version:    "1.0.0",
				ActionType: actionType,
				Description: rwv1alpha1.RemediationWorkflowDescription{
					What:      "IT-KA-1677 discovery edge-case test fixture",
					WhenToUse: "For workflow catalog discovery edge-case integration testing",
				},
				Labels: rwv1alpha1.RemediationWorkflowLabels{
					Severity:    severity,
					Environment: environment,
					Component:   component,
					Priority:    priority,
					Cluster:     f.Cluster,
				},
				Execution: rwv1alpha1.RemediationWorkflowExecution{
					Engine: "job",
					Bundle: testutil.ValidBundleRef,
				},
				Parameters: []rwv1alpha1.RemediationWorkflowParameter{
					{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
				},
			},
		}
		return rw
	}

	// createRW creates rw, defers its cleanup, optionally status-patches
	// WorkflowID, and blocks until this spec's own cache has observed it --
	// closes the same informer-lag race seedWorkflowCRD's Eventually guarded
	// against in the DS original.
	createRW := func(rw *rwv1alpha1.RemediationWorkflow) {
		// Callers set rw.Status.WorkflowID before calling createRW, but the
		// CRD's status subresource means Create's server response (which
		// controller-runtime unmarshals back into rw) always has an empty
		// status -- so the pre-set value must be captured here and
		// re-applied via a real Status().Update() after Create, or it's
		// silently dropped and GetWorkflowByID-backed lookups never find it.
		wantWorkflowID := rw.Status.WorkflowID
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rw) })
		if wantWorkflowID != "" {
			rw.Status.WorkflowID = wantWorkflowID
			Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())
			// The informer cache indexes status.workflowId via a separate
			// field indexer from spec.actionType/metadata.name, populated by
			// a second (Update) watch event. Waiting only on the
			// ListWorkflowsByActionType/name-index below can race ahead of
			// that second event, so GetWorkflowByID-backed callers
			// (GetWorkflowWithContextFilters, GetByID) intermittently see
			// "not found" right after createRW returns. Wait on the
			// workflow_id index directly when one was set.
			Eventually(func() (*rwv1alpha1.RemediationWorkflow, error) {
				return wfCache.GetWorkflowByID(ctx, rw.Status.WorkflowID)
			}, 5*time.Second, 100*time.Millisecond).ShouldNot(BeNil(), "cache must index workflow_id=%s before the spec asserts on it", rw.Status.WorkflowID)
		}
		Eventually(func() ([]models.RemediationWorkflow, error) {
			results, _, err := catalog.ListWorkflowsByActionType(ctx, rw.Spec.ActionType, &models.WorkflowDiscoveryFilters{}, 0, 1000)
			return results, err
		}, 5*time.Second, 100*time.Millisecond).Should(ContainElement(WithTransform(
			func(wf models.RemediationWorkflow) string { return wf.WorkflowName }, Equal(rw.Name),
		)), "cache must observe %s before the spec asserts on it", rw.Name)
	}

	// ========================================
	// DD-HAPI-017 three-step discovery protocol (was IT-DS-017-001-*)
	// ========================================
	Describe("DD-HAPI-017 three-step discovery protocol", func() {
		It("IT-KA-1677-DHAPI017-001: ListActions counts all workflows for the matching action type", func() {
			actionType := seedActiveActionType("DHAPI017a")
			createRW(newRW(uniqueName("wf-scale-1"), actionType, rwFixture{Component: []string{"v1/Pod"}, Environment: []string{"production"}, Priority: "P0"}))
			createRW(newRW(uniqueName("wf-scale-2"), actionType, rwFixture{Severity: []string{"high"}, Component: []string{"apps/v1/Deployment"}, Environment: []string{"staging"}, Priority: "P1"}))

			result, totalCount, err := catalog.ListActions(ctx, &models.WorkflowDiscoveryFilters{}, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1))
			Expect(result).To(HaveLen(1))
			Expect(result[0].ActionType).To(Equal(actionType))
			Expect(result[0].WorkflowCount).To(Equal(2))
		})

		It("IT-KA-1677-DHAPI017-002: ListActions paginates action types correctly", func() {
			actionTypes := make([]string, 5)
			for i := range actionTypes {
				actionTypes[i] = seedActiveActionType(fmt.Sprintf("DHAPI017pg%d", i))
				createRW(newRW(uniqueName(fmt.Sprintf("wf-pg-%d", i)), actionTypes[i], rwFixture{}))
			}

			filters := &models.WorkflowDiscoveryFilters{}
			var result1 []models.ActionTypeEntry
			Eventually(func() (int, error) {
				var totalCount int
				var err error
				result1, totalCount, err = catalog.ListActions(ctx, filters, 0, 3)
				return totalCount, err
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(5), "cache must observe all 5 seeded action types")
			Expect(result1).To(HaveLen(3))

			result2, totalCount2, err := catalog.ListActions(ctx, filters, 3, 3)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount2).To(Equal(5))
			Expect(result2).To(HaveLen(2))

			page1Types := make(map[string]bool, len(result1))
			for _, entry := range result1 {
				page1Types[entry.ActionType] = true
			}
			for _, entry := range result2 {
				Expect(page1Types).ToNot(HaveKey(entry.ActionType), "pages should not overlap")
			}
		})

		It("IT-KA-1677-DHAPI017-003: ListWorkflowsByActionType filters by action_type AND signal context", func() {
			actionType := seedActiveActionType("DHAPI017b")
			matchName := uniqueName("wf-scale-conservative")
			createRW(newRW(matchName, actionType, rwFixture{Component: []string{"v1/Pod"}, Environment: []string{"production"}, Priority: "P0"}))
			createRW(newRW(uniqueName("wf-scale-aggressive"), actionType, rwFixture{Severity: []string{"high"}, Component: []string{"apps/v1/Deployment"}, Environment: []string{"staging"}, Priority: "P1"}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "critical", Component: "v1/Pod", Environment: "production", Priority: "P0"}
			results, totalCount, err := catalog.ListWorkflowsByActionType(ctx, actionType, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1))
			Expect(results).To(HaveLen(1))
			Expect(results[0].WorkflowName).To(Equal(matchName))
		})

		It("IT-KA-1677-DHAPI017-005: GetWorkflowWithContextFilters returns the workflow when context matches", func() {
			actionType := seedActiveActionType("DHAPI017c")
			workflowID := uniqueName("wfid-match")
			rw := newRW(uniqueName("wf-scale-match"), actionType, rwFixture{Component: []string{"v1/Pod"}, Environment: []string{"production"}, Priority: "P0"})
			rw.Status.WorkflowID = workflowID
			createRW(rw)

			filters := &models.WorkflowDiscoveryFilters{Severity: "critical", Component: "v1/Pod", Environment: "production", Priority: "P0"}
			result, err := catalog.GetWorkflowWithContextFilters(ctx, workflowID, filters)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.WorkflowID).To(Equal(workflowID))
			Expect(result.ActionType).To(Equal(actionType))
		})

		It("IT-KA-1677-DHAPI017-006: GetWorkflowWithContextFilters returns ErrNotFound on context mismatch (security gate)", func() {
			actionType := seedActiveActionType("DHAPI017d")
			workflowID := uniqueName("wfid-mismatch")
			rw := newRW(uniqueName("wf-scale-mismatch"), actionType, rwFixture{Component: []string{"v1/Pod"}, Environment: []string{"production"}, Priority: "P0"})
			rw.Status.WorkflowID = workflowID
			createRW(rw)

			filters := &models.WorkflowDiscoveryFilters{Severity: "high", Component: "apps/v1/Deployment", Environment: "staging", Priority: "P1"}
			result, err := catalog.GetWorkflowWithContextFilters(ctx, workflowID, filters)
			Expect(errors.Is(err, workflowcatalog.ErrNotFound)).To(BeTrue(), "security gate should surface ErrNotFound for mismatched workflow")
			Expect(result).To(BeNil())
		})
	})

	// ========================================
	// Issue #464: wildcard mandatory label matching
	// ========================================
	Describe("Wildcard mandatory label matching (#464)", func() {
		It("IT-KA-1677-464-001: ListActions matches wildcard component + priority", func() {
			actionType := seedActiveActionType("Wc464a")
			createRW(newRW(uniqueName("wf-wc-comp-pri"), actionType, rwFixture{Component: []string{"*"}, Priority: "*"}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "critical", Component: "v1/Pod", Environment: "production", Priority: "P1"}
			result, totalCount, err := catalog.ListActions(ctx, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1), "wildcard component/priority workflow must be discovered")
			Expect(result).To(HaveLen(1))
			Expect(result[0].ActionType).To(Equal(actionType))
		})

		It("IT-KA-1677-464-002: ListActions matches an all-wildcard workflow for any filter values", func() {
			actionType := seedActiveActionType("Wc464b")
			createRW(newRW(uniqueName("wf-wc-all"), actionType, rwFixture{Severity: []string{"*"}, Component: []string{"*"}, Environment: []string{"*"}, Priority: "*"}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "high", Component: "apps/v1/Deployment", Environment: "staging", Priority: "P3"}
			result, totalCount, err := catalog.ListActions(ctx, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1), "all-wildcard workflow must be discoverable with any filter values")
			Expect(result).To(HaveLen(1))
		})

		It("IT-KA-1677-464-003: ListWorkflowsByActionType returns a wildcard-labeled workflow in Step 2", func() {
			actionType := seedActiveActionType("Wc464c")
			createRW(newRW(uniqueName("wf-wc-step2"), actionType, rwFixture{Component: []string{"*"}, Priority: "*"}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "critical", Component: "v1/Pod", Environment: "production", Priority: "P1"}
			results, totalCount, err := catalog.ListWorkflowsByActionType(ctx, actionType, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1), "wildcard workflow must appear in Step 2 results")
			Expect(results).To(HaveLen(1))
		})

		It("IT-KA-1677-464-004: GetWorkflowWithContextFilters security gate passes for a wildcard-labeled workflow", func() {
			actionType := seedActiveActionType("Wc464d")
			workflowID := uniqueName("wfid-wc-gate")
			rw := newRW(uniqueName("wf-wc-gate"), actionType, rwFixture{Component: []string{"*"}, Priority: "*"})
			rw.Status.WorkflowID = workflowID
			createRW(rw)

			filters := &models.WorkflowDiscoveryFilters{Severity: "critical", Component: "v1/Pod", Environment: "production", Priority: "P1"}
			result, err := catalog.GetWorkflowWithContextFilters(ctx, workflowID, filters)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil(), "security gate must not reject a wildcard-labeled workflow")
			Expect(result.WorkflowID).To(Equal(workflowID))
		})

		It("IT-KA-1677-464-005: demo scenario -- mixed wildcard and exact labels", func() {
			actionType := seedActiveActionType("Wc464e")
			createRW(newRW(uniqueName("wf-wc-demo"), actionType, rwFixture{
				Severity: []string{"critical", "high"}, Component: []string{"*"}, Environment: []string{"production", "staging", "*"}, Priority: "*",
			}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "critical", Component: "v1/Pod", Environment: "staging", Priority: "P1"}
			result, totalCount, err := catalog.ListActions(ctx, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1), "the demo scenario workflow must be discovered")
			Expect(result).To(HaveLen(1))
		})

		It("IT-KA-1677-464-006: severity=['*'] matches a concrete severity filter", func() {
			actionType := seedActiveActionType("Wc464f")
			createRW(newRW(uniqueName("wf-wc-sev"), actionType, rwFixture{Severity: []string{"*"}}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "critical", Component: "v1/Pod", Environment: "production", Priority: "P1"}
			result, totalCount, err := catalog.ListActions(ctx, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1), "severity ['*'] must match a concrete severity filter")
			Expect(result).To(HaveLen(1))
		})
	})

	// ========================================
	// Issue #522: wildcard labels returning 0 results (regression)
	// ========================================
	Describe("Issue #522 reproduction: wildcard component/environment/priority", func() {
		It("IT-KA-1677-522-001: matches severity=[critical,high], component/environment/priority wildcarded", func() {
			actionType := seedActiveActionType("Wc522a")
			createRW(newRW(uniqueName("wf-522-emptydir"), actionType, rwFixture{
				Severity: []string{"critical", "high"}, Component: []string{"*"}, Environment: []string{"*"}, Priority: "*",
			}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "high", Component: "v1/Node", Environment: "unknown", Priority: "P3"}
			result, totalCount, err := catalog.ListActions(ctx, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1))
			Expect(result).To(HaveLen(1))
		})

		It("IT-KA-1677-522-002: matches a fully-wildcarded workflow when environment=unknown", func() {
			actionType := seedActiveActionType("Wc522b")
			createRW(newRW(uniqueName("wf-522-allwild"), actionType, rwFixture{
				Severity: []string{"*"}, Component: []string{"*"}, Environment: []string{"*"}, Priority: "*",
			}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "high", Component: "v1/Node", Environment: "unknown", Priority: "P3"}
			result, totalCount, err := catalog.ListActions(ctx, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1))
			Expect(result).To(HaveLen(1))
		})

		It("IT-KA-1677-522-003: ListWorkflowsByActionType returns the wildcard workflow", func() {
			actionType := seedActiveActionType("Wc522c")
			createRW(newRW(uniqueName("wf-522-step2"), actionType, rwFixture{
				Severity: []string{"critical", "high"}, Component: []string{"*"}, Environment: []string{"*"}, Priority: "*",
			}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "high", Component: "v1/Node", Environment: "unknown", Priority: "P3"}
			results, totalCount, err := catalog.ListWorkflowsByActionType(ctx, actionType, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1))
			Expect(results).To(HaveLen(1))
		})
	})

	// ========================================
	// Cluster classification filter (BR-FLEET-003, #1511)
	// ========================================
	Describe("Cluster classification filter (BR-FLEET-003, #1511)", func() {
		It("IT-KA-1677-1511-001: cluster filter, exact match", func() {
			actionType := seedActiveActionType("Fleet1511a")
			prodName := uniqueName("wf-prod-only")
			createRW(newRW(prodName, actionType, rwFixture{Cluster: []string{"production"}}))
			createRW(newRW(uniqueName("wf-staging-only"), actionType, rwFixture{Cluster: []string{"staging"}}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "critical", Component: "v1/Pod", Environment: "production", Priority: "P1", Cluster: "production"}
			result, totalCount, err := catalog.ListActions(ctx, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1), "only the 'production'-labeled workflow must match cluster=production")
			Expect(result).To(HaveLen(1))

			workflows, wfCount, err := catalog.ListWorkflowsByActionType(ctx, actionType, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(wfCount).To(Equal(1))
			Expect(workflows).To(HaveLen(1))
			Expect(workflows[0].WorkflowName).To(Equal(prodName))
		})

		It("IT-KA-1677-1511-002: cluster filter excludes unlabeled workflows once active", func() {
			actionType := seedActiveActionType("Fleet1511b")
			createRW(newRW(uniqueName("wf-no-cluster-label"), actionType, rwFixture{}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "critical", Component: "v1/Pod", Environment: "production", Priority: "P1", Cluster: "production"}
			result, totalCount, err := catalog.ListActions(ctx, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(0), "workflow with no cluster label must be excluded once cluster filter is active")
			Expect(result).To(BeEmpty())
		})

		It("IT-KA-1677-1511-002b: cluster:['*'] matches any concrete cluster filter value", func() {
			actionType := seedActiveActionType("Fleet1511c")
			createRW(newRW(uniqueName("wf-wildcard-cluster"), actionType, rwFixture{Cluster: []string{"*"}}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "critical", Component: "v1/Pod", Environment: "production", Priority: "P1", Cluster: "staging-eu"}
			result, totalCount, err := catalog.ListActions(ctx, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1), "cluster:['*'] must match any concrete cluster filter value")
			Expect(result).To(HaveLen(1))
		})

		It("IT-KA-1677-1511-003: no cluster param is a zero behavioral change (regression)", func() {
			actionType := seedActiveActionType("Fleet1511d")
			createRW(newRW(uniqueName("wf-labeled"), actionType, rwFixture{Cluster: []string{"production"}}))
			createRW(newRW(uniqueName("wf-unlabeled"), actionType, rwFixture{}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "critical", Component: "v1/Pod", Environment: "production", Priority: "P1"}
			result, totalCount, err := catalog.ListActions(ctx, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1))
			Expect(result).To(HaveLen(1))
			Expect(result[0].WorkflowCount).To(Equal(2), "with no cluster filter, both the labeled and unlabeled workflow must be counted")
		})
	})

	// ========================================
	// Case-insensitive label matching (#595, DD-WORKFLOW-001 v2.9)
	// ========================================
	Describe("Case-insensitive label matching (#595)", func() {
		It("IT-KA-1677-595-001: PascalCase environment query matches lowercase label", func() {
			actionType := seedActiveActionType("Ci595a")
			createRW(newRW(uniqueName("wf-env-case"), actionType, rwFixture{Environment: []string{"production"}, Priority: "P0"}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "critical", Component: "v1/Pod", Environment: "Production", Priority: "P0"}
			result, totalCount, err := catalog.ListActions(ctx, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1), "PascalCase 'Production' must match lowercase label ['production']")
			Expect(result).To(HaveLen(1))
		})

		It("IT-KA-1677-595-002: PascalCase severity query matches lowercase label", func() {
			actionType := seedActiveActionType("Ci595b")
			createRW(newRW(uniqueName("wf-sev-case"), actionType, rwFixture{Priority: "P0"}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "Critical", Component: "v1/Pod", Environment: "production", Priority: "P0"}
			result, totalCount, err := catalog.ListActions(ctx, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1), "PascalCase 'Critical' must match lowercase label ['critical']")
			Expect(result).To(HaveLen(1))
		})

		It("IT-KA-1677-595-004: full reproduction -- all 4 mandatory labels case-mismatched", func() {
			actionType := seedActiveActionType("Ci595c")
			createRW(newRW(uniqueName("wf-full-repro"), actionType, rwFixture{Component: []string{"apps/v1/Deployment"}, Priority: "P0"}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "Critical", Component: "Apps/V1/Deployment", Environment: "Production", Priority: "P0"}
			result, totalCount, err := catalog.ListActions(ctx, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1), "all PascalCase filters must match lowercase labels")
			Expect(result).To(HaveLen(1))

			results, totalCount2, err := catalog.ListWorkflowsByActionType(ctx, actionType, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount2).To(Equal(1), "ListWorkflowsByActionType must also match case-insensitively")
			Expect(results).To(HaveLen(1))
		})

		It("IT-KA-1677-595-005: wildcard labels still match PascalCase queries", func() {
			actionType := seedActiveActionType("Ci595d")
			createRW(newRW(uniqueName("wf-wc-case"), actionType, rwFixture{Severity: []string{"*"}, Environment: []string{"*"}, Priority: "P0"}))

			filters := &models.WorkflowDiscoveryFilters{Severity: "Critical", Component: "v1/Pod", Environment: "Production", Priority: "P0"}
			result, totalCount, err := catalog.ListActions(ctx, filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1), "wildcard labels must still match PascalCase queries")
			Expect(result).To(HaveLen(1))
		})

		It("IT-KA-1677-595-006: GetWorkflowWithContextFilters security gate passes with case-mismatched environment", func() {
			actionType := seedActiveActionType("Ci595e")
			workflowID := uniqueName("wfid-gate-case")
			rw := newRW(uniqueName("wf-gate-case"), actionType, rwFixture{Priority: "P0"})
			rw.Status.WorkflowID = workflowID
			createRW(rw)

			filters := &models.WorkflowDiscoveryFilters{Severity: "critical", Component: "v1/Pod", Environment: "Production", Priority: "P0"}
			result, err := catalog.GetWorkflowWithContextFilters(ctx, workflowID, filters)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil(), "security gate must pass with case-insensitive environment matching")
			Expect(result.WorkflowID).To(Equal(workflowID))
		})
	})
})
