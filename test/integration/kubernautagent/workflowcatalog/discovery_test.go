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

// Issue #1677 Phase 2b (DD-WORKFLOW-019): proves Catalog -- the
// discovery/scoring logic ported from
// pkg/datastorage/repository/workflow -- correctly wraps the Phase 2a
// informer-backed Cache to serve the three-step discovery protocol plus the
// generic List, against a real envtest API server. The pure filter/scoring
// logic itself already has dedicated UT coverage
// (discovery_cache_test.go/cache_filter_test.go/list_cache_test.go in the
// production package); these specs exist to prove the wiring between
// Catalog and Cache (Pyramid Invariant: "IT proves wiring").
//
// Business Requirements: BR-WORKFLOW-006, BR-WORKFLOW-007, BR-HAPI-017-001,
// BR-STORAGE-012.
var _ = Describe("IT-KA-1677-DISC Workflow Catalog discovery (Catalog wrapping Cache)", Label("integration", "kubernautagent", "workflow-catalog"), func() {

	var (
		wfCache     *workflowcatalog.Cache
		cacheCancel func()
		catalog     *workflowcatalog.Catalog
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
					What:      "IT-KA-1677-DISC test fixture",
					WhenToUse: "For workflow catalog discovery integration testing",
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
					What:      "IT-KA-1677-DISC action type test fixture",
					WhenToUse: "For workflow catalog discovery integration testing",
				},
			},
		}
	}

	// markActive patches an ActionType's status.catalogStatus to Active via
	// the status subresource -- ListActions only counts Active action types.
	markActive := func(at *atv1alpha1.ActionType) {
		at.Status.CatalogStatus = sharedtypes.CatalogStatusActive
		Expect(k8sClient.Status().Update(ctx, at)).To(Succeed())
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

	It("IT-KA-1677-DISC-001: ListWorkflowsByActionType returns cache-sourced workflows matching context filters", func() {
		actionType := uniquePascalName("CatalogWiredAction")
		matchName := uniqueName("it-1677-disc-match")
		otherSeverityName := uniqueName("it-1677-disc-othersev")
		matchRW := validRW(matchName, actionType, []string{"critical"})
		otherSeverityRW := validRW(otherSeverityName, actionType, []string{"low"})
		Expect(k8sClient.Create(ctx, matchRW)).To(Succeed())
		Expect(k8sClient.Create(ctx, otherSeverityRW)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, matchRW) })
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, otherSeverityRW) })

		filters := &models.WorkflowDiscoveryFilters{Severity: "critical"}

		Eventually(func() ([]models.RemediationWorkflow, error) {
			results, _, err := catalog.ListWorkflowsByActionType(ctx, actionType, filters, 0, 10)
			return results, err
		}, 5*time.Second, 100*time.Millisecond).Should(ContainElement(WithTransform(
			func(wf models.RemediationWorkflow) string { return wf.WorkflowName }, Equal(matchName),
		)), "the Catalog must return the matching workflow once the informer observes it")

		results, totalCount, err := catalog.ListWorkflowsByActionType(ctx, actionType, filters, 0, 10)
		Expect(err).ToNot(HaveOccurred())
		Expect(totalCount).To(Equal(1), "only the critical-severity workflow matches; the low-severity one must be excluded")
		Expect(results).To(HaveLen(1))
		Expect(results[0].WorkflowName).To(Equal(matchName))
		Expect(results[0].ActionType).To(Equal(actionType))
	})

	It("IT-KA-1677-DISC-002: ListActions counts only Active action types with at least one matching workflow", func() {
		activeActionType := uniquePascalName("ActiveCatalogAction")
		disabledActionType := uniquePascalName("DisabledCatalogAction")

		activeAT := validAT(uniqueName("it-1677-disc-at-active"), activeActionType)
		disabledAT := validAT(uniqueName("it-1677-disc-at-disabled"), disabledActionType)
		Expect(k8sClient.Create(ctx, activeAT)).To(Succeed())
		Expect(k8sClient.Create(ctx, disabledAT)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, activeAT) })
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, disabledAT) })
		markActive(activeAT)
		// disabledAT is left at its zero-value CatalogStatus (never marked Active).

		activeRW := validRW(uniqueName("it-1677-disc-wf-active"), activeActionType, []string{"critical"})
		disabledRW := validRW(uniqueName("it-1677-disc-wf-disabled"), disabledActionType, []string{"critical"})
		Expect(k8sClient.Create(ctx, activeRW)).To(Succeed())
		Expect(k8sClient.Create(ctx, disabledRW)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, activeRW) })
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, disabledRW) })

		filters := &models.WorkflowDiscoveryFilters{Severity: "critical"}

		Eventually(func() ([]models.ActionTypeEntry, error) {
			entries, _, err := catalog.ListActions(ctx, filters, 0, 100)
			return entries, err
		}, 5*time.Second, 100*time.Millisecond).Should(ContainElement(WithTransform(
			func(e models.ActionTypeEntry) string { return e.ActionType }, Equal(activeActionType),
		)), "the Active action type with a matching workflow must appear")

		entries, _, err := catalog.ListActions(ctx, filters, 0, 100)
		Expect(err).ToNot(HaveOccurred())
		for _, e := range entries {
			Expect(e.ActionType).ToNot(Equal(disabledActionType), "a non-Active action type must never appear, even with a matching workflow")
		}
	})

	It("IT-KA-1677-DISC-003: GetByID returns the cache-sourced workflow by its content-hash workflow_id, with spec.parameters populated", func() {
		actionType := uniquePascalName("CatalogGetByIDAction")
		name := uniqueName("it-1677-disc-getbyid")
		rw := validRW(name, actionType, []string{"critical"})
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rw) })

		rw.Status.WorkflowID = uniqueName("wfid")
		Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())

		Eventually(func() (*models.RemediationWorkflow, error) {
			return catalog.GetByID(ctx, rw.Status.WorkflowID)
		}, 5*time.Second, 100*time.Millisecond).ShouldNot(BeNil(), "the Catalog must find the workflow once the informer observes it")

		got, err := catalog.GetByID(ctx, rw.Status.WorkflowID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got).ToNot(BeNil())
		Expect(got.WorkflowName).To(Equal(name))
		Expect(got.ActionType).To(Equal(actionType))
		Expect(got.Parameters).ToNot(BeNil(), "spec.parameters[] must be populated -- get_workflow's documented contract for LLM parameter validation")
	})

	It("IT-KA-1677-DISC-004: GetWorkflowWithContextFilters applies the security gate -- matching context returns the workflow, non-matching returns ErrNotFound without distinguishing not-found from filtered-out", func() {
		actionType := uniquePascalName("CatalogContextGateAction")
		name := uniqueName("it-1677-disc-gate")
		rw := validRW(name, actionType, []string{"critical"})
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rw) })

		rw.Status.WorkflowID = uniqueName("wfid-gate")
		Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())

		matchingFilters := &models.WorkflowDiscoveryFilters{Severity: "critical"}
		Eventually(func() (*models.RemediationWorkflow, error) {
			return catalog.GetWorkflowWithContextFilters(ctx, rw.Status.WorkflowID, matchingFilters)
		}, 5*time.Second, 100*time.Millisecond).ShouldNot(BeNil(), "matching context filters must return the workflow once the informer observes it")

		nonMatchingFilters := &models.WorkflowDiscoveryFilters{Severity: "low"}
		got, err := catalog.GetWorkflowWithContextFilters(ctx, rw.Status.WorkflowID, nonMatchingFilters)
		Expect(errors.Is(err, workflowcatalog.ErrNotFound)).To(BeTrue(), "a workflow that exists but fails the context-filter security gate must return workflowcatalog.ErrNotFound -- same as not-found (DD-WORKFLOW-016: prevent info leakage)")
		Expect(got).To(BeNil())

		got, err = catalog.GetWorkflowWithContextFilters(ctx, "nonexistent-"+uniqueName("wfid"), nonMatchingFilters)
		Expect(errors.Is(err, workflowcatalog.ErrNotFound)).To(BeTrue(), "a genuinely nonexistent workflow_id must also return workflowcatalog.ErrNotFound -- indistinguishable from the filtered-out case above")
		Expect(got).To(BeNil())
	})

	It("IT-KA-1677-DISC-005: List returns cache-sourced workflows with no filters, honors workflow_name/status filters", func() {
		actionType := uniquePascalName("CatalogListAction")
		nameActive := uniqueName("it-1677-disc-list-active")
		nameDisabled := uniqueName("it-1677-disc-list-disabled")

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

		Eventually(func() (int, error) {
			_, total, err := catalog.List(ctx, &models.WorkflowSearchFilters{}, 50, 0)
			return total, err
		}, 5*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 2), "List with no filters must see workflows of every status once the informer observes them")

		results, _, err := catalog.List(ctx, &models.WorkflowSearchFilters{WorkflowName: nameActive}, 50, 0)
		Expect(err).ToNot(HaveOccurred())
		Expect(results).To(HaveLen(1))
		Expect(results[0].WorkflowName).To(Equal(nameActive))
		Expect(results[0].ActionType).To(Equal(actionType))

		activeOnly, _, err := catalog.List(ctx, &models.WorkflowSearchFilters{Status: []string{"Active"}, WorkflowName: nameActive}, 50, 0)
		Expect(err).ToNot(HaveOccurred())
		Expect(activeOnly).To(HaveLen(1))

		noneMatch, _, err := catalog.List(ctx, &models.WorkflowSearchFilters{Status: []string{"Disabled"}, WorkflowName: nameActive}, 50, 0)
		Expect(err).ToNot(HaveOccurred())
		Expect(noneMatch).To(BeEmpty(), "an Active workflow must not match a Disabled-only status filter")
	})
})
