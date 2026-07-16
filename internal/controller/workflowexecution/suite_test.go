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
	"sync/atomic"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	weclient "github.com/jordigilh/kubernaut/pkg/workflowexecution/client"
)

// Issue #1661 Change 11e (DD-WORKFLOW-018): this is the first pure-unit
// (non-envtest) Ginkgo suite for internal/controller/workflowexecution.
// Prior to Phase 49, resolveSchemaMetadata/resolveWorkflowCatalog were only
// exercised indirectly through the full envtest reconciler in
// test/integration/workflowexecution. These two DS call sites are being
// removed entirely (Phase 50), so the RED tests here assert the new
// contract directly against the private methods -- Pyramid Invariant: UT
// proves logic, IT (existing suite) proves wiring.
func TestWorkflowExecutionControllerUnit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "WorkflowExecution Controller Unit Suite")
}

// forbiddenWorkflowQuerier is a weclient.WorkflowQuerier test double that
// counts every call it receives. Phase 49 RED: once WorkflowExecution stops
// consulting DataStorage for catalog/schema metadata (Phase 50), every
// method on this double must remain uncalled. Deliberately returns values
// that DIFFER from the WFE's own wfe.Spec.WorkflowRef snapshot -- if the
// reconciler were to (incorrectly) apply any of these returned values, the
// test's assertions on the *source* fields would fail, catching a
// regression even if the call-count assertion were ever weakened.
type forbiddenWorkflowQuerier struct {
	getDependenciesCalls    atomic.Int64
	getEngineConfigCalls    atomic.Int64
	getExecutionEngineCalls atomic.Int64
	getExecutionBundleCalls atomic.Int64
	resolveCatalogCalls     atomic.Int64
	getSchemaMetadataCalls  atomic.Int64
}

func (f *forbiddenWorkflowQuerier) totalCalls() int64 {
	return f.getDependenciesCalls.Load() +
		f.getEngineConfigCalls.Load() +
		f.getExecutionEngineCalls.Load() +
		f.getExecutionBundleCalls.Load() +
		f.resolveCatalogCalls.Load() +
		f.getSchemaMetadataCalls.Load()
}

func (f *forbiddenWorkflowQuerier) GetWorkflowDependencies(_ context.Context, _ string) (*models.WorkflowDependencies, error) {
	f.getDependenciesCalls.Add(1)
	return &models.WorkflowDependencies{Secrets: []models.ResourceDependency{{Name: "forbidden-canary-secret"}}}, nil
}

func (f *forbiddenWorkflowQuerier) GetWorkflowEngineConfig(_ context.Context, _ string) (json.RawMessage, error) {
	f.getEngineConfigCalls.Add(1)
	return json.RawMessage(`{"canary":"forbidden"}`), nil
}

func (f *forbiddenWorkflowQuerier) GetWorkflowExecutionEngine(_ context.Context, _ string) (string, string, error) {
	f.getExecutionEngineCalls.Add(1)
	return "forbidden-canary-engine", "forbidden-canary-name", nil
}

func (f *forbiddenWorkflowQuerier) GetWorkflowExecutionBundle(_ context.Context, _ string) (string, string, error) {
	f.getExecutionBundleCalls.Add(1)
	return "forbidden-canary-bundle", "forbidden-canary-digest", nil
}

func (f *forbiddenWorkflowQuerier) ResolveWorkflowCatalogMetadata(_ context.Context, _ string) (*weclient.WorkflowCatalogMetadata, error) {
	f.resolveCatalogCalls.Add(1)
	return &weclient.WorkflowCatalogMetadata{
		ExecutionEngine:       "forbidden-canary-engine",
		WorkflowName:          "forbidden-canary-name",
		ActionType:            "ForbiddenCanaryAction",
		ExecutionBundle:       "forbidden-canary-bundle",
		ExecutionBundleDigest: "forbidden-canary-digest",
		ServiceAccountName:    "forbidden-canary-sa",
		Dependencies:          &models.WorkflowDependencies{Secrets: []models.ResourceDependency{{Name: "forbidden-canary-secret"}}},
	}, nil
}

func (f *forbiddenWorkflowQuerier) GetWorkflowSchemaMetadata(_ context.Context, _ string) (*weclient.SchemaMetadata, error) {
	f.getSchemaMetadataCalls.Add(1)
	return &weclient.SchemaMetadata{
		Engine:       "forbidden-canary-engine",
		WorkflowName: "forbidden-canary-name",
		EngineConfig: json.RawMessage(`{"canary":"forbidden"}`),
	}, nil
}

var _ weclient.WorkflowQuerier = (*forbiddenWorkflowQuerier)(nil)

// expectNoQuerierCalls is a shared assertion used by both RED spec files.
func expectNoQuerierCalls(q *forbiddenWorkflowQuerier) {
	ExpectWithOffset(1, q.totalCalls()).To(BeZero(),
		"Issue #1661 Change 11e: WorkflowExecution must resolve catalog/schema "+
			"metadata from wfe.Spec.WorkflowRef alone -- zero DataStorage/"+
			"WorkflowQuerier calls expected")
}
