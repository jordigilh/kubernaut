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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/workflowcatalog"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// Issue #1677 Phase 2a (DD-WORKFLOW-019): KubernautAgent's informer-backed
// workflow/action-type cache, ported from DataStorage (DD-WORKFLOW-018)
// verbatim -- KA now owns discovery directly instead of proxying through DS.
//
// Business Requirements: BR-WORKFLOW-006, BR-WORKFLOW-007.
var _ = Describe("IT-KA-1677-CACHE Workflow Catalog Cache (informer-backed)", Label("integration", "kubernautagent", "workflow-catalog"), func() {

	var wfCache *workflowcatalog.Cache
	var cacheCancel func()

	uniqueName := func(prefix string) string {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}

	// uniquePascalName generates a unique spec.name for ActionType fixtures.
	// spec.name is constrained by kubebuilder Pattern `^[A-Z][A-Za-z0-9]*$`
	// (PascalCase, no hyphens) -- unlike metadata.name, which allows the
	// standard DNS-label uniqueName() above.
	uniquePascalName := func(prefix string) string {
		return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
	}

	validRW := func(name, actionType string) *rwv1alpha1.RemediationWorkflow {
		return &rwv1alpha1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: rwv1alpha1.RemediationWorkflowSpec{
				Version:    "1.0.0",
				ActionType: actionType,
				Description: rwv1alpha1.RemediationWorkflowDescription{
					What:      "IT-KA-1677-CACHE test fixture",
					WhenToUse: "For workflow catalog cache integration testing",
				},
				Labels: rwv1alpha1.RemediationWorkflowLabels{
					Severity:    []string{"critical"},
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
					What:      "IT-KA-1677-CACHE action type test fixture",
					WhenToUse: "For workflow catalog cache integration testing",
				},
			},
		}
	}

	BeforeEach(func() {
		scheme, schemeErr := workflowcatalog.NewScheme()
		Expect(schemeErr).ToNot(HaveOccurred())

		var err error
		wfCache, cacheCancel, err = workflowcatalog.NewInformerCache(k8sConfig, scheme, logger)
		Expect(err).ToNot(HaveOccurred(), "workflow catalog cache should build and sync against the shared envtest")
	})

	AfterEach(func() {
		if cacheCancel != nil {
			cacheCancel()
		}
	})

	It("IT-KA-1677-CACHE-001: GetWorkflow returns a RemediationWorkflow CRD that exists in etcd", func() {
		name := uniqueName("it-1677-get")
		rw := validRW(name, "ScaleReplicas")
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rw) })

		var got *rwv1alpha1.RemediationWorkflow
		Eventually(func() bool {
			var err error
			got, err = wfCache.GetWorkflow(ctx, name)
			Expect(err).ToNot(HaveOccurred())
			return got != nil
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(), "cache should observe the workflow via its informer")

		Expect(got.Spec.ActionType).To(Equal("ScaleReplicas"))
	})

	It("IT-KA-1677-CACHE-002: GetWorkflow returns (nil, nil) for a name that does not exist", func() {
		got, err := wfCache.GetWorkflow(ctx, uniqueName("it-1677-missing"))
		Expect(err).ToNot(HaveOccurred())
		Expect(got).To(BeNil(), "not-found must be (nil, nil), matching the DS cache's GetWorkflow convention")
	})

	It("IT-KA-1677-CACHE-003: ListWorkflowsByActionType returns only workflows matching the given action type", func() {
		matchName := uniqueName("it-1677-match")
		otherName := uniqueName("it-1677-other")
		matchRW := validRW(matchName, "RestartPod")
		otherRW := validRW(otherName, "ScaleReplicas")
		Expect(k8sClient.Create(ctx, matchRW)).To(Succeed())
		Expect(k8sClient.Create(ctx, otherRW)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, matchRW) })
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, otherRW) })

		var results []rwv1alpha1.RemediationWorkflow
		Eventually(func() []string {
			var err error
			results, err = wfCache.ListWorkflowsByActionType(ctx, "RestartPod")
			Expect(err).ToNot(HaveOccurred())
			names := make([]string, 0, len(results))
			for _, r := range results {
				names = append(names, r.Name)
			}
			return names
		}, 5*time.Second, 100*time.Millisecond).Should(ContainElement(matchName))

		for _, r := range results {
			Expect(r.Name).ToNot(Equal(otherName), "ListWorkflowsByActionType must not return workflows of other action types")
		}
	})

	It("IT-KA-1677-CACHE-004: GetActionType returns an ActionType CRD by its spec.name (not metadata.name)", func() {
		crdName := uniqueName("it-1677-at-crd")
		specName := uniquePascalName("TestActionTypeGet")
		at := validAT(crdName, specName)
		Expect(k8sClient.Create(ctx, at)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, at) })

		var got *atv1alpha1.ActionType
		Eventually(func() bool {
			var err error
			got, err = wfCache.GetActionType(ctx, specName)
			Expect(err).ToNot(HaveOccurred())
			return got != nil && got.Name == crdName
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(), "cache should observe the ActionType via its informer")

		Expect(got.Name).To(Equal(crdName), "lookup is keyed by spec.name, resolving to the CRD whose metadata.name may differ")
	})

	It("IT-KA-1677-CACHE-005: cache observes a workflow created after the initial sync (Watch, not just List)", func() {
		name := uniqueName("it-1677-watch")

		// Confirm the cache is already running (initial sync complete) before creating.
		_, err := wfCache.GetWorkflow(ctx, uniqueName("it-1677-presync-noop"))
		Expect(err).ToNot(HaveOccurred())

		rw := validRW(name, "ScaleReplicas")
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rw) })

		Eventually(func() (*rwv1alpha1.RemediationWorkflow, error) {
			return wfCache.GetWorkflow(ctx, name)
		}, 5*time.Second, 100*time.Millisecond).ShouldNot(BeNil(), "cache's Watch must observe post-sync creates")
	})

	// IT-KA-1677-CACHE-006 (#1677 follow-up, DD-WORKFLOW-018/019): proves the
	// cache's read-your-writes consistency on DELETE, not just CREATE.
	// Previously this property was only ever proven E2E against DS's now-
	// retired REST catalog (test/e2e/authwebhook/02_workflow_content_
	// integrity_test.go's E2E-INTEGRITY-002); since that catalog moved to
	// KA, the same guarantee needs a home here against KA's own informer
	// cache -- the property was at risk of being silently dropped entirely
	// during the OpenAPI/E2E cleanup, not just relocated.
	It("IT-KA-1677-CACHE-006: cache observes a workflow deletion (Watch, not just List)", func() {
		name := uniqueName("it-1677-delete")
		rw := validRW(name, "ScaleReplicas")
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())

		Eventually(func() (*rwv1alpha1.RemediationWorkflow, error) {
			return wfCache.GetWorkflow(ctx, name)
		}, 5*time.Second, 100*time.Millisecond).ShouldNot(BeNil(), "cache should observe the workflow before deletion")

		Expect(k8sClient.Delete(ctx, rw)).To(Succeed())

		Eventually(func() (*rwv1alpha1.RemediationWorkflow, error) {
			return wfCache.GetWorkflow(ctx, name)
		}, 5*time.Second, 100*time.Millisecond).Should(BeNil(), "cache's Watch must observe deletes -- no stale entry survives a real CRD delete")
	})
})
