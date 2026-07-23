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
	"github.com/jordigilh/kubernaut/pkg/datastorage/workflowcache"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// Issue #1661 Phase 28 (Change 5): DS informer-backed cache.
//
// DD-WORKFLOW-018: etcd is the sole source of truth for RemediationWorkflow/
// ActionType definitions. DataStorage no longer maintains a Postgres catalog
// for these -- it maintains an in-memory, informer-backed cache instead. This
// suite proves the cache primitive itself (Change 6, Phase 31-33, will
// rewire discovery/scoring to consume it instead of SQL).
//
// Business Requirements: BR-WORKFLOW-006, BR-WORKFLOW-007.
var _ = Describe("IT-DS-1661 Workflow Cache (informer-backed)", Label("integration", "datastorage", "workflow-cache"), func() {

	var wfCache *workflowcache.Cache
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
					What:      "IT-DS-1661 workflow cache test fixture",
					WhenToUse: "For workflow cache integration testing",
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
					What:      "IT-DS-1661 action type cache test fixture",
					WhenToUse: "For workflow cache integration testing",
				},
			},
		}
	}

	BeforeEach(func() {
		var err error
		wfCache, cacheCancel, err = workflowcache.NewInformerCache(dsK8sRestConfig, scheme.Scheme, logger)
		Expect(err).ToNot(HaveOccurred(), "workflow cache should build and sync against the shared envtest")
	})

	AfterEach(func() {
		if cacheCancel != nil {
			cacheCancel()
		}
	})

	It("IT-DS-1661-001: GetWorkflow returns a RemediationWorkflow CRD that exists in etcd", func() {
		name := uniqueName("it-1661-get")
		rw := validRW(name, "ScaleReplicas")
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())
		DeferCleanup(func() { deleteWorkflowAndWaitForSharedCache(rw) })

		var got *rwv1alpha1.RemediationWorkflow
		Eventually(func() bool {
			var err error
			got, err = wfCache.GetWorkflow(ctx, name)
			Expect(err).ToNot(HaveOccurred())
			return got != nil
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(), "cache should observe the workflow via its informer")

		Expect(got.Spec.ActionType).To(Equal("ScaleReplicas"))
	})

	It("IT-DS-1661-002: GetWorkflow returns (nil, nil) for a name that does not exist", func() {
		got, err := wfCache.GetWorkflow(ctx, uniqueName("it-1661-missing"))
		Expect(err).ToNot(HaveOccurred())
		Expect(got).To(BeNil(), "not-found must be (nil, nil), matching the Postgres repository's GetByID convention")
	})

	It("IT-DS-1661-003: ListWorkflowsByActionType returns only workflows matching the given action type", func() {
		matchName := uniqueName("it-1661-match")
		otherName := uniqueName("it-1661-other")
		matchRW := validRW(matchName, "RestartPod")
		otherRW := validRW(otherName, "ScaleReplicas")
		Expect(k8sClient.Create(ctx, matchRW)).To(Succeed())
		Expect(k8sClient.Create(ctx, otherRW)).To(Succeed())
		DeferCleanup(func() { deleteWorkflowAndWaitForSharedCache(matchRW) })
		DeferCleanup(func() { deleteWorkflowAndWaitForSharedCache(otherRW) })

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

	It("IT-DS-1661-004: GetActionType returns an ActionType CRD by its spec.name (not metadata.name)", func() {
		crdName := uniqueName("it-1661-at-crd")
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

	It("IT-DS-1661-005: ListActionTypes returns every ActionType CRD", func() {
		crdName := uniqueName("it-1661-at-list")
		at := validAT(crdName, uniquePascalName("TestActionTypeList"))
		Expect(k8sClient.Create(ctx, at)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, at) })

		Eventually(func() []string {
			results, err := wfCache.ListActionTypes(ctx)
			Expect(err).ToNot(HaveOccurred())
			names := make([]string, 0, len(results))
			for _, r := range results {
				names = append(names, r.Name)
			}
			return names
		}, 5*time.Second, 100*time.Millisecond).Should(ContainElement(crdName))
	})

	It("IT-DS-1661-006: cache observes a workflow created after the initial sync (Watch, not just List)", func() {
		name := uniqueName("it-1661-watch")

		// Confirm the cache is already running (initial sync complete) before creating.
		_, err := wfCache.GetWorkflow(ctx, uniqueName("it-1661-presync-noop"))
		Expect(err).ToNot(HaveOccurred())

		rw := validRW(name, "ScaleReplicas")
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())
		DeferCleanup(func() { deleteWorkflowAndWaitForSharedCache(rw) })

		Eventually(func() (*rwv1alpha1.RemediationWorkflow, error) {
			return wfCache.GetWorkflow(ctx, name)
		}, 5*time.Second, 100*time.Millisecond).ShouldNot(BeNil(), "cache's Watch must observe post-sync creates")
	})
})
