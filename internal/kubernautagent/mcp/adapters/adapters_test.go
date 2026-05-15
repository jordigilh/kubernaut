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

package adapters_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/adapters"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	wfclient "github.com/jordigilh/kubernaut/pkg/workflowexecution/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockWorkflowQuerier is a test double for wfclient.WorkflowQuerier.
type mockWorkflowQuerier struct {
	meta *wfclient.WorkflowCatalogMetadata
	err  error
}

func (m *mockWorkflowQuerier) ResolveWorkflowCatalogMetadata(_ context.Context, _ string) (*wfclient.WorkflowCatalogMetadata, error) {
	return m.meta, m.err
}

func (m *mockWorkflowQuerier) GetWorkflowDependencies(_ context.Context, _ string) (*models.WorkflowDependencies, error) {
	return nil, nil
}

func (m *mockWorkflowQuerier) GetWorkflowEngineConfig(_ context.Context, _ string) (json.RawMessage, error) {
	return nil, nil
}

func (m *mockWorkflowQuerier) GetWorkflowExecutionEngine(_ context.Context, _ string) (string, string, error) {
	return "", "", nil
}

func (m *mockWorkflowQuerier) GetWorkflowExecutionBundle(_ context.Context, _ string) (string, string, error) {
	return "", "", nil
}

func (m *mockWorkflowQuerier) GetWorkflowSchemaMetadata(_ context.Context, _ string) (*wfclient.SchemaMetadata, error) {
	return nil, nil
}

var _ = Describe("WorkflowCatalogAdapter — PR6a", func() {

	Describe("UT-KA-PR6A-ADAPT-001: maps DS metadata to CatalogWorkflow", func() {
		It("should map all fields correctly", func() {
			querier := &mockWorkflowQuerier{
				meta: &wfclient.WorkflowCatalogMetadata{
					WorkflowName:       "restart-pod",
					ExecutionEngine:    "tekton",
					ExecutionBundle:    "ghcr.io/kubernaut/restart-pod:v1",
					ServiceAccountName: "remediation-sa",
				},
			}

			adapter := adapters.NewWorkflowCatalogAdapter(querier)
			result, err := adapter.GetWorkflowByID(context.Background(), "wf-123")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.WorkflowID).To(Equal("wf-123"))
			Expect(result.WorkflowName).To(Equal("restart-pod"))
			Expect(result.ExecutionEngine).To(Equal("tekton"))
			Expect(result.ExecutionBundle).To(Equal("ghcr.io/kubernaut/restart-pod:v1"))
			Expect(result.ServiceAccountName).To(Equal("remediation-sa"))
		})
	})

	Describe("UT-KA-PR6A-ADAPT-002: propagates DS errors", func() {
		It("should wrap and return the error", func() {
			querier := &mockWorkflowQuerier{err: fmt.Errorf("DS unavailable")}

			adapter := adapters.NewWorkflowCatalogAdapter(querier)
			_, err := adapter.GetWorkflowByID(context.Background(), "wf-bad")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("DS unavailable"))
		})
	})

	Describe("UT-KA-PR6A-ADAPT-003: compile-time interface check", func() {
		It("should satisfy tools.WorkflowCatalog", func() {
			var _ tools.WorkflowCatalog = &adapters.WorkflowCatalogAdapter{}
		})
	})
})

var _ = Describe("ExtractContent — QE-01", func() {

	DescribeTable("maps LoopResult variants to string",
		func(result investigator.LoopResult, expectedContent string, expectErr bool) {
			content, err := adapters.ExtractContent(result)
			if expectErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(content).To(Equal(expectedContent))
			}
		},
		Entry("UT-KA-QE01-001: TextResult",
			&investigator.TextResult{Content: "analysis in progress"},
			"analysis in progress", false),
		Entry("UT-KA-QE01-002: SubmitResult",
			&investigator.SubmitResult{Content: "root cause: OOM"},
			"root cause: OOM", false),
		Entry("UT-KA-QE01-003: SubmitWithWorkflowResult",
			&investigator.SubmitWithWorkflowResult{Content: "execute restart-pod"},
			"execute restart-pod", false),
		Entry("UT-KA-QE01-004: SubmitNoWorkflowResult",
			&investigator.SubmitNoWorkflowResult{Content: "no remediation needed"},
			"no remediation needed", false),
		Entry("UT-KA-QE01-005: ExhaustedResult returns error",
			&investigator.ExhaustedResult{Reason: "max turns reached"},
			"", true),
		Entry("UT-KA-QE01-006: CancelledResult returns error",
			&investigator.CancelledResult{Turn: 3},
			"", true),
		Entry("UT-KA-QE01-007: nil result returns error",
			nil,
			"", true),
	)
})
