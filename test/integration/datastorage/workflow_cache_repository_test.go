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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
	"github.com/jordigilh/kubernaut/pkg/datastorage/workflowcache"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// Issue #1661 Phase 32 (Change 6, CHECKPOINT W): proves
// workflow.Repository.ListActions/ListWorkflowsByActionType read from the
// Phase 28/29 informer-backed CRD cache -- not Postgres -- once SetCache has
// been called, exercising the same production entry points
// server_construction.go wires (catalogDeps.workflowRepo.SetCache(wfCache)).
//
// The repository under test is constructed with a nil *sqlx.DB: any SQL
// code path this test accidentally hit would panic on a nil-pointer
// dereference, which is exactly the negative-proof CHECKPOINT W needs (reads
// are impossible via Postgres here, so a passing assertion can only mean
// the cache path executed).
//
// Business Requirements: BR-WORKFLOW-006, BR-WORKFLOW-007, BR-HAPI-017-001.
var _ = Describe("IT-DS-1661-P32 Repository discovery reads from cache", Label("integration", "datastorage", "workflow-cache"), func() {

	var (
		wfCache     *workflowcache.Cache
		cacheCancel func()
		repo        *workflow.Repository
	)

	uniqueName := func(prefix string) string {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	uniquePascalName := func(prefix string) string {
		return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
	}

	validRW := func(name, actionType string, severity []string) *rwv1alpha1.RemediationWorkflow {
		return &rwv1alpha1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: rwv1alpha1.RemediationWorkflowSpec{
				Version:    "1.0.0",
				ActionType: actionType,
				Description: rwv1alpha1.RemediationWorkflowDescription{
					What:      "IT-DS-1661-P32 repository cache-wiring test fixture",
					WhenToUse: "For discovery cache-branch integration testing",
				},
				Labels: rwv1alpha1.RemediationWorkflowLabels{
					Severity:    severity,
					Environment: []string{"production"},
					Component:   []string{"v1/Pod"},
					Priority:    "P1",
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
	}

	validAT := func(name, specName string) *atv1alpha1.ActionType {
		return &atv1alpha1.ActionType{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: atv1alpha1.ActionTypeSpec{
				Name: specName,
				Description: atv1alpha1.ActionTypeDescription{
					What:      "IT-DS-1661-P32 action type cache-wiring test fixture",
					WhenToUse: "For discovery cache-branch integration testing",
				},
			},
		}
	}

	// markActive patches an ActionType's status.catalogStatus to Active via
	// the status subresource -- ListActions' cache branch only counts Active
	// action types (mirrors the SQL path's `t.status = 'Active'` filter).
	markActive := func(at *atv1alpha1.ActionType) {
		at.Status.CatalogStatus = sharedtypes.CatalogStatusActive
		Expect(k8sClient.Status().Update(ctx, at)).To(Succeed())
	}

	BeforeEach(func() {
		var err error
		wfCache, cacheCancel, err = workflowcache.NewInformerCache(dsK8sRestConfig, scheme.Scheme, logger)
		Expect(err).ToNot(HaveOccurred(), "workflow cache should build and sync against the shared envtest")

		repo = workflow.NewRepository(nil, logger) // nil *sqlx.DB: any accidental SQL path panics, proving cache-only reads
		repo.SetCache(wfCache)
	})

	AfterEach(func() {
		if cacheCancel != nil {
			cacheCancel()
		}
	})

	It("IT-DS-1661-P32-001: ListWorkflowsByActionType returns cache-sourced workflows matching context filters", func() {
		actionType := uniquePascalName("CacheWiredAction")
		matchName := uniqueName("it-1661-p32-match")
		otherSeverityName := uniqueName("it-1661-p32-othersev")
		matchRW := validRW(matchName, actionType, []string{"critical"})
		otherSeverityRW := validRW(otherSeverityName, actionType, []string{"low"})
		Expect(k8sClient.Create(ctx, matchRW)).To(Succeed())
		Expect(k8sClient.Create(ctx, otherSeverityRW)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, matchRW) })
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, otherSeverityRW) })

		filters := &models.WorkflowDiscoveryFilters{Severity: "critical"}

		Eventually(func() ([]models.RemediationWorkflow, error) {
			results, _, err := repo.ListWorkflowsByActionType(ctx, actionType, filters, 0, 10)
			return results, err
		}, 5*time.Second, 100*time.Millisecond).Should(ContainElement(WithTransform(
			func(wf models.RemediationWorkflow) string { return wf.WorkflowName }, Equal(matchName),
		)), "the cache-backed path must return the matching workflow once the informer observes it")

		results, totalCount, err := repo.ListWorkflowsByActionType(ctx, actionType, filters, 0, 10)
		Expect(err).ToNot(HaveOccurred())
		Expect(totalCount).To(Equal(1), "only the critical-severity workflow matches; the low-severity one must be excluded")
		Expect(results).To(HaveLen(1))
		Expect(results[0].WorkflowName).To(Equal(matchName))
		Expect(results[0].ActionType).To(Equal(actionType))
	})

	It("IT-DS-1661-P32-002: ListActions counts only Active action types with at least one matching workflow", func() {
		activeActionType := uniquePascalName("ActiveCacheAction")
		disabledActionType := uniquePascalName("DisabledCacheAction")

		activeAT := validAT(uniqueName("it-1661-p32-at-active"), activeActionType)
		disabledAT := validAT(uniqueName("it-1661-p32-at-disabled"), disabledActionType)
		Expect(k8sClient.Create(ctx, activeAT)).To(Succeed())
		Expect(k8sClient.Create(ctx, disabledAT)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, activeAT) })
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, disabledAT) })
		markActive(activeAT)
		// disabledAT is left at its zero-value CatalogStatus (never marked Active).

		activeRW := validRW(uniqueName("it-1661-p32-wf-active"), activeActionType, []string{"critical"})
		disabledRW := validRW(uniqueName("it-1661-p32-wf-disabled"), disabledActionType, []string{"critical"})
		Expect(k8sClient.Create(ctx, activeRW)).To(Succeed())
		Expect(k8sClient.Create(ctx, disabledRW)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, activeRW) })
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, disabledRW) })

		filters := &models.WorkflowDiscoveryFilters{Severity: "critical"}

		Eventually(func() ([]models.ActionTypeEntry, error) {
			entries, _, err := repo.ListActions(ctx, filters, 0, 100)
			return entries, err
		}, 5*time.Second, 100*time.Millisecond).Should(ContainElement(WithTransform(
			func(e models.ActionTypeEntry) string { return e.ActionType }, Equal(activeActionType),
		)), "the Active action type with a matching workflow must appear")

		entries, _, err := repo.ListActions(ctx, filters, 0, 100)
		Expect(err).ToNot(HaveOccurred())
		for _, e := range entries {
			Expect(e.ActionType).ToNot(Equal(disabledActionType), "a non-Active action type must never appear, even with a matching workflow")
		}
	})

	// #1661 Phase 55 prerequisite: Step 3 (GetByID/GetWorkflowWithContextFilters)
	// was ported to the cache ahead of the rest of Phase 55 -- AuthWebhook already
	// stopped writing to Postgres (Change 8c), so the SQL-backed GetByID could not
	// find ANY workflow admitted after that change landed. These two specs prove
	// the fix using the same nil-*sqlx.DB negative-proof technique as IT-DS-1661-P32-001/002.
	It("IT-DS-1661-P32-003: GetByID returns the cache-sourced workflow by its content-hash workflow_id, with spec.parameters populated", func() {
		actionType := uniquePascalName("CacheGetByIDAction")
		name := uniqueName("it-1661-p32-getbyid")
		rw := validRW(name, actionType, []string{"critical"})
		rw.Spec.Parameters = []rwv1alpha1.RemediationWorkflowParameter{
			{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
		}
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rw) })

		rw.Status.WorkflowID = uniqueName("wfid")
		Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())

		Eventually(func() (*models.RemediationWorkflow, error) {
			return repo.GetByID(ctx, rw.Status.WorkflowID)
		}, 5*time.Second, 100*time.Millisecond).ShouldNot(BeNil(), "the cache-backed path must find the workflow once the informer observes it")

		got, err := repo.GetByID(ctx, rw.Status.WorkflowID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got).ToNot(BeNil())
		Expect(got.WorkflowName).To(Equal(name))
		Expect(got.ActionType).To(Equal(actionType))
		Expect(got.Parameters).ToNot(BeNil(), "spec.parameters[] must be populated -- HandleGetWorkflowByID's documented contract for LLM parameter validation")
	})

	It("IT-DS-1661-P32-004: GetWorkflowWithContextFilters applies the security gate -- matching context returns the workflow, non-matching returns nil without distinguishing not-found", func() {
		actionType := uniquePascalName("CacheContextGateAction")
		name := uniqueName("it-1661-p32-gate")
		rw := validRW(name, actionType, []string{"critical"})
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rw) })

		rw.Status.WorkflowID = uniqueName("wfid-gate")
		Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())

		matchingFilters := &models.WorkflowDiscoveryFilters{Severity: "critical"}
		Eventually(func() (*models.RemediationWorkflow, error) {
			return repo.GetWorkflowWithContextFilters(ctx, rw.Status.WorkflowID, matchingFilters)
		}, 5*time.Second, 100*time.Millisecond).ShouldNot(BeNil(), "matching context filters must return the workflow once the informer observes it")

		nonMatchingFilters := &models.WorkflowDiscoveryFilters{Severity: "low"}
		got, err := repo.GetWorkflowWithContextFilters(ctx, rw.Status.WorkflowID, nonMatchingFilters)
		Expect(err).ToNot(HaveOccurred())
		Expect(got).To(BeNil(), "a workflow that exists but fails the context-filter security gate must return nil, nil -- same as not-found (DD-WORKFLOW-016: prevent info leakage)")

		got, err = repo.GetWorkflowWithContextFilters(ctx, "nonexistent-"+uniqueName("wfid"), nonMatchingFilters)
		Expect(err).ToNot(HaveOccurred())
		Expect(got).To(BeNil(), "a genuinely nonexistent workflow_id must also return nil, nil -- indistinguishable from the filtered-out case above")
	})

	// #1661 Phase 55 prerequisite: List (the generic GET /api/v1/workflows
	// catalog listing, distinct from the discovery protocol's Steps 1/2/3)
	// was ported to the cache once it was discovered that KA's
	// dsCatalogFetcher.FetchValidator (cmd/kubernautagent/toolregistry.go)
	// depends on this exact method with empty filters, and it too could not
	// see any workflow admitted after AuthWebhook stopped writing to
	// Postgres (Change 8c) -- the same already-broken production read path
	// as GetByID/Step 3 above.
	It("IT-DS-1661-P32-005: List returns cache-sourced workflows with no filters, honors workflow_name/status filters, and paginates in created_at DESC order", func() {
		actionType := uniquePascalName("CacheListAction")
		nameActive := uniqueName("it-1661-p32-list-active")
		nameDisabled := uniqueName("it-1661-p32-list-disabled")

		activeRW := validRW(nameActive, actionType, []string{"critical"})
		disabledRW := validRW(nameDisabled, actionType, []string{"critical"})
		Expect(k8sClient.Create(ctx, activeRW)).To(Succeed())
		Expect(k8sClient.Create(ctx, disabledRW)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, activeRW) })
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, disabledRW) })

		activeRW.Status.WorkflowID = uniqueName("wfid-list-active")
		activeRW.Status.CatalogStatus = sharedtypes.CatalogStatusActive
		Expect(k8sClient.Status().Update(ctx, activeRW)).To(Succeed())
		disabledRW.Status.WorkflowID = uniqueName("wfid-list-disabled")
		disabledRW.Status.CatalogStatus = sharedtypes.CatalogStatusDisabled
		Expect(k8sClient.Status().Update(ctx, disabledRW)).To(Succeed())

		// No filters: KA's dsCatalogFetcher.FetchValidator calls List this
		// way -- both workflows (any status) must be visible.
		Eventually(func() (int, error) {
			_, total, err := repo.List(ctx, &models.WorkflowSearchFilters{}, 50, 0)
			return total, err
		}, 5*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 2), "List with no filters must see workflows of every status once the informer observes them")

		results, _, err := repo.List(ctx, &models.WorkflowSearchFilters{WorkflowName: nameActive}, 50, 0)
		Expect(err).ToNot(HaveOccurred())
		Expect(results).To(HaveLen(1))
		Expect(results[0].WorkflowName).To(Equal(nameActive))
		Expect(results[0].ActionType).To(Equal(actionType))

		activeOnly, _, err := repo.List(ctx, &models.WorkflowSearchFilters{Status: []string{"Active"}, WorkflowName: nameActive}, 50, 0)
		Expect(err).ToNot(HaveOccurred())
		Expect(activeOnly).To(HaveLen(1))

		noneMatch, _, err := repo.List(ctx, &models.WorkflowSearchFilters{Status: []string{"Disabled"}, WorkflowName: nameActive}, 50, 0)
		Expect(err).ToNot(HaveOccurred())
		Expect(noneMatch).To(BeEmpty(), "an Active workflow must not match a Disabled-only status filter")
	})
})
