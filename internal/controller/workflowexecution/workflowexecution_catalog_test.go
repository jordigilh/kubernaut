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

package workflowexecution

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	weclient "github.com/jordigilh/kubernaut/pkg/workflowexecution/client"
)

func TestWorkflowExecutionCatalogUnit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "WorkflowExecution Catalog Unit Suite")
}

// mockCatalogWorkflowQuerier implements weclient.WorkflowQuerier for testing
// resolveWorkflowCatalog. Only ResolveWorkflowCatalogMetadata is exercised by
// this function; the remaining methods are unused stubs required to satisfy
// the interface.
type mockCatalogWorkflowQuerier struct {
	meta *weclient.WorkflowCatalogMetadata
	err  error
}

func (m *mockCatalogWorkflowQuerier) GetWorkflowDependencies(context.Context, string) (*models.WorkflowDependencies, error) {
	panic("not used by resolveWorkflowCatalog")
}

func (m *mockCatalogWorkflowQuerier) GetWorkflowEngineConfig(context.Context, string) (json.RawMessage, error) {
	panic("not used by resolveWorkflowCatalog")
}

func (m *mockCatalogWorkflowQuerier) GetWorkflowExecutionEngine(context.Context, string) (string, string, error) {
	panic("not used by resolveWorkflowCatalog")
}

func (m *mockCatalogWorkflowQuerier) GetWorkflowExecutionBundle(context.Context, string) (string, string, error) {
	panic("not used by resolveWorkflowCatalog")
}

func (m *mockCatalogWorkflowQuerier) ResolveWorkflowCatalogMetadata(_ context.Context, _ string) (*weclient.WorkflowCatalogMetadata, error) {
	return m.meta, m.err
}

func (m *mockCatalogWorkflowQuerier) GetWorkflowSchemaMetadata(context.Context, string) (*weclient.SchemaMetadata, error) {
	panic("not used by resolveWorkflowCatalog")
}

// ========================================
// Issue #1674 (nilnil sentinel-error refactor), Batch 2: resolveWorkflowCatalog
// previously returned (nil, nil) for the idempotent "already resolved" case,
// ambiguous with a caller forgetting to check the error. This package had
// zero unit-test coverage before this change.
// BR-WE-003 (WorkflowExecution catalog resolution), Issue #650, Issue #518.
// ========================================
var _ = Describe("resolveWorkflowCatalog (Issue #1674 Batch 2)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-WE-1674-001: returns ErrAlreadyResolved when the execution engine is already set", func() {
		wfe := &workflowexecutionv1alpha1.WorkflowExecution{}
		wfe.Status.ExecutionEngine = "tekton"
		r := &WorkflowExecutionReconciler{
			WorkflowQuerier: &mockCatalogWorkflowQuerier{},
		}

		meta, err := r.resolveWorkflowCatalog(ctx, wfe)

		Expect(errors.Is(err, ErrAlreadyResolved)).To(BeTrue())
		Expect(meta).To(BeNil())
	})

	It("UT-WE-1674-002: resolves engine, service account, and resources from the DS catalog", func() {
		wfe := &workflowexecutionv1alpha1.WorkflowExecution{}
		wfe.Spec.WorkflowRef.WorkflowID = "wf-1"
		r := &WorkflowExecutionReconciler{
			WorkflowQuerier: &mockCatalogWorkflowQuerier{
				meta: &weclient.WorkflowCatalogMetadata{
					ExecutionEngine:    "tekton",
					WorkflowName:       "restart-pod",
					ServiceAccountName: "wf-sa",
				},
			},
		}

		meta, err := r.resolveWorkflowCatalog(ctx, wfe)

		Expect(err).ToNot(HaveOccurred())
		Expect(meta).ToNot(BeNil())
		Expect(wfe.Status.ExecutionEngine).To(Equal("tekton"))
		Expect(wfe.Status.ServiceAccountName).To(Equal("wf-sa"))
	})
})
