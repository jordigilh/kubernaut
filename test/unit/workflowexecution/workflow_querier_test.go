/*
Copyright 2025 Jordi Gil.

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
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	weclient "github.com/jordigilh/kubernaut/pkg/workflowexecution/client"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// mockWorkflowCatalogClient implements weclient.WorkflowCatalogClient for testing.
type mockWorkflowCatalogClient struct {
	response ogenclient.GetWorkflowByIDRes
	err      error
}

func (m *mockWorkflowCatalogClient) GetWorkflowByID(_ context.Context, _ ogenclient.GetWorkflowByIDParams) (ogenclient.GetWorkflowByIDRes, error) {
	return m.response, m.err
}

var _ = Describe("OgenWorkflowQuerier (DD-WE-006)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("GetWorkflowSchemaMetadata", func() {
		It("UT-WE-243-030: should return deps, param names, engine and workflowName from valid schema", func() {
			content := buildTestSchemaWithParams(
				&models.WorkflowDependencies{
					Secrets: []models.ResourceDependency{{Name: "gitea-repo-creds"}},
				},
				[]models.WorkflowParameter{
					{Name: "TARGET_NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
					{Name: "REPLICAS", Type: "integer", Required: false, Description: "Replica count"},
				},
			)
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.RemediationWorkflow{
					Content:         content,
					ExecutionEngine: "job",
					WorkflowName:    "crashloop-config-fix-v1",
				},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			meta, err := querier.GetWorkflowSchemaMetadata(ctx, uuid.New().String())
			Expect(err).ToNot(HaveOccurred())
			Expect(meta.Engine).To(Equal("job"))
			Expect(meta.WorkflowName).To(Equal("crashloop-config-fix-v1"))
			Expect(meta.Dependencies.Secrets).To(HaveLen(1))
			Expect(meta.Dependencies.Secrets[0].Name).To(Equal("gitea-repo-creds"))
			Expect(meta.DeclaredParameterNames).To(HaveLen(2))
			Expect(meta.DeclaredParameterNames).To(HaveKey("TARGET_NAMESPACE"))
			Expect(meta.DeclaredParameterNames).To(HaveKey("REPLICAS"))
		})

		It("UT-WE-243-031: should return nil when content is empty but still populate engine fields", func() {
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.RemediationWorkflow{
					Content:         "",
					ExecutionEngine: "tekton",
					WorkflowName:    "empty-schema-wf",
				},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			meta, err := querier.GetWorkflowSchemaMetadata(ctx, uuid.New().String())
			Expect(err).ToNot(HaveOccurred())
			Expect(meta.Engine).To(Equal("tekton"))
			Expect(meta.WorkflowName).To(Equal("empty-schema-wf"))
			Expect(meta).To(HaveField("Dependencies", BeNil()))
			Expect(meta).To(HaveField("DeclaredParameterNames", BeNil()))
			Expect(meta).To(HaveField("EngineConfig", BeNil()))
		})

		It("UT-WE-243-032: should return error with workflow ID context for malformed YAML", func() {
			workflowID := uuid.New().String()
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.RemediationWorkflow{Content: "{{invalid yaml: [broken"},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			_, err := querier.GetWorkflowSchemaMetadata(ctx, workflowID)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(workflowID),
				"Error should include workflow ID for operator debugging")
		})

		It("UT-WE-243-033: should return nil deps but non-nil param names when schema has no deps", func() {
			content := buildTestSchemaWithParams(
				nil,
				[]models.WorkflowParameter{
					{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target ns"},
				},
			)
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.RemediationWorkflow{Content: content},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			meta, err := querier.GetWorkflowSchemaMetadata(ctx, uuid.New().String())
			Expect(err).ToNot(HaveOccurred())
			Expect(meta).To(HaveField("Dependencies", BeNil()))
			Expect(meta.DeclaredParameterNames).To(HaveLen(1))
			Expect(meta.DeclaredParameterNames).To(HaveKey("NAMESPACE"))
		})

		It("UT-WE-243-034: should return error for invalid UUID", func() {
			querier := weclient.NewOgenWorkflowQuerier(&mockWorkflowCatalogClient{})

			_, err := querier.GetWorkflowSchemaMetadata(ctx, "not-a-uuid")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid workflow ID"))
		})

		It("UT-F6-001: should extract engineConfig as JSON from schema with ansible engine", func() {
			content := buildTestSchemaWithEngineConfig("ansible",
				map[string]interface{}{
					"playbookPath":    "playbooks/restart.yml",
					"jobTemplateName": "restart-pod",
				},
			)
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.RemediationWorkflow{
					Content:         content,
					ExecutionEngine: "ansible",
					WorkflowName:    "ansible-restart-wf",
				},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			meta, err := querier.GetWorkflowSchemaMetadata(ctx, uuid.New().String())
			Expect(err).ToNot(HaveOccurred())
			Expect(meta.Engine).To(Equal("ansible"))
			Expect(meta.WorkflowName).To(Equal("ansible-restart-wf"))

			var cfg map[string]interface{}
			Expect(json.Unmarshal(meta.EngineConfig, &cfg)).To(Succeed(),
				"EngineConfig should be valid JSON with ansible configuration")
			Expect(cfg).To(HaveKeyWithValue("playbookPath", "playbooks/restart.yml"))
			Expect(cfg).To(HaveKeyWithValue("jobTemplateName", "restart-pod"))
		})

		It("UT-F6-002: should return nil engineConfig when schema has no engineConfig section", func() {
			content := buildTestSchemaWithParams(nil,
				[]models.WorkflowParameter{
					{Name: "NAMESPACE", Type: "string", Required: true, Description: "ns"},
				},
			)
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.RemediationWorkflow{
					Content:         content,
					ExecutionEngine: "job",
					WorkflowName:    "no-ec-wf",
				},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			meta, err := querier.GetWorkflowSchemaMetadata(ctx, uuid.New().String())
			Expect(err).ToNot(HaveOccurred())
			Expect(meta).To(HaveField("EngineConfig", BeNil()))
		})

		It("UT-F6-003: should return error when workflow not found (404)", func() {
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.GetWorkflowByIDNotFound{},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			_, err := querier.GetWorkflowSchemaMetadata(ctx, uuid.New().String())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("UT-F6-004: should return error when DS query fails", func() {
			mock := &mockWorkflowCatalogClient{
				err: fmt.Errorf("connection refused"),
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			_, err := querier.GetWorkflowSchemaMetadata(ctx, uuid.New().String())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("DS query failed"))
		})
	})
})

// buildTestSchemaWithParams builds a minimal valid workflow schema YAML with
// configurable dependencies and parameters.
func buildTestSchemaWithParams(deps *models.WorkflowDependencies, params []models.WorkflowParameter) string {
	crd := testutil.NewTestWorkflowCRD("test-workflow", "CertificateRenewal", "job")
	crd.Spec.Labels.Component = "deployment"
	crd.Spec.Execution.Bundle = "ghcr.io/test/bundle:latest@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	crd.Spec.Parameters = params
	crd.Spec.Dependencies = deps
	return testutil.MarshalWorkflowCRD(crd)
}

// buildTestSchemaWithEngineConfig builds a workflow schema YAML with an
// engineConfig section for the specified engine.
func buildTestSchemaWithEngineConfig(engine string, engineConfig map[string]interface{}) string {
	crd := testutil.NewTestWorkflowCRD("test-workflow", "CertificateRenewal", engine)
	crd.Spec.Labels.Component = "deployment"
	crd.Spec.Execution.Bundle = "ghcr.io/test/bundle:latest@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	crd.Spec.Execution.EngineConfig = engineConfig
	crd.Spec.Parameters = []models.WorkflowParameter{
		{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target ns"},
	}
	return testutil.MarshalWorkflowCRD(crd)
}
