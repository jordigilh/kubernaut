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
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	weclient "github.com/jordigilh/kubernaut/pkg/workflowexecution/client"
)

// mockWorkflowCatalogClient implements weclient.WorkflowCatalogClient for testing.
type mockWorkflowCatalogClient struct {
	response ogenclient.GetWorkflowByIDRes
	err      error
}

func (m *mockWorkflowCatalogClient) GetWorkflowByID(_ context.Context, _ ogenclient.GetWorkflowByIDParams) (ogenclient.GetWorkflowByIDRes, error) {
	return m.response, m.err
}

// buildTestSchema builds a minimal valid workflow schema YAML with an optional
// dependencies section appended. Eliminates YAML boilerplate across tests.
func buildTestSchema(dependenciesYAML string) string {
	base := `apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowSchema
metadata:
  workflowId: test-workflow
  version: "1.0.0"
  description:
    what: "Test workflow"
    when_to_use: "Testing"
    when_not_to_use: ""
    preconditions: ""
  labels:
    severity: critical
    component: deployment
    environment: production
actionType: certificate_renewal
execution:
  engine: job
  bundle: "ghcr.io/test/bundle:latest"
parameters:
  - name: TARGET_NAMESPACE
    type: string
    required: true`
	if dependenciesYAML != "" {
		return base + "\n" + dependenciesYAML
	}
	return base
}

var _ = Describe("OgenWorkflowQuerier (DD-WE-006)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("GetWorkflowDependencies", func() {
		It("UT-WE-006-001: should extract secret dependencies from workflow content", func() {
			content := buildTestSchema(`dependencies:
  secrets:
    - name: gitea-repo-creds`)
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.RemediationWorkflow{Content: content},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			deps, err := querier.GetWorkflowDependencies(ctx, uuid.New().String())
			Expect(err).ToNot(HaveOccurred())
			Expect(deps.Secrets).To(HaveLen(1))
			Expect(deps.Secrets[0].Name).To(Equal("gitea-repo-creds"))
			Expect(deps.ConfigMaps).To(BeEmpty())
		})

		It("UT-WE-006-002: should extract both secrets and configMaps", func() {
			content := buildTestSchema(`dependencies:
  secrets:
    - name: gitea-repo-creds
  configMaps:
    - name: remediation-config`)
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.RemediationWorkflow{Content: content},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			deps, err := querier.GetWorkflowDependencies(ctx, uuid.New().String())
			Expect(err).ToNot(HaveOccurred())
			Expect(deps.Secrets).To(HaveLen(1))
			Expect(deps.Secrets[0].Name).To(Equal("gitea-repo-creds"))
			Expect(deps.ConfigMaps).To(HaveLen(1))
			Expect(deps.ConfigMaps[0].Name).To(Equal("remediation-config"))
		})

		It("UT-WE-006-003: should return nil when workflow has no dependencies", func() {
			content := buildTestSchema("")
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.RemediationWorkflow{Content: content},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			deps, err := querier.GetWorkflowDependencies(ctx, uuid.New().String())
			Expect(err).ToNot(HaveOccurred())
			Expect(deps).To(BeNil())
		})

		It("UT-WE-006-004: should return error for invalid UUID", func() {
			querier := weclient.NewOgenWorkflowQuerier(&mockWorkflowCatalogClient{})

			_, err := querier.GetWorkflowDependencies(ctx, "not-a-uuid")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid workflow ID"))
		})

		It("UT-WE-006-005: should return error when workflow not found (404)", func() {
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.GetWorkflowByIDNotFound{},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			_, err := querier.GetWorkflowDependencies(ctx, uuid.New().String())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("UT-WE-006-006: should return error when DS query fails", func() {
			mock := &mockWorkflowCatalogClient{
				err: fmt.Errorf("connection refused"),
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			_, err := querier.GetWorkflowDependencies(ctx, uuid.New().String())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("DS query failed"))
		})

		It("UT-WE-006-007: should return nil when content is empty", func() {
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.RemediationWorkflow{Content: ""},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			deps, err := querier.GetWorkflowDependencies(ctx, uuid.New().String())
			Expect(err).ToNot(HaveOccurred())
			Expect(deps).To(BeNil())
		})

		It("UT-WE-006-008: should return parse error for malformed YAML content", func() {
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.RemediationWorkflow{Content: "{{invalid yaml: [broken"},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			_, err := querier.GetWorkflowDependencies(ctx, uuid.New().String())
			Expect(err).To(HaveOccurred(),
				"malformed YAML from DS should produce an error, not a panic")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("parse"),
				ContainSubstring("yaml"),
				ContainSubstring("unmarshal"),
			))
		})
	})
})
