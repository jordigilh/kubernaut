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

// buildTestSchema builds a minimal valid workflow schema YAML with optional
// dependencies. Uses the builder pattern to avoid brittle string concatenation.
func buildTestSchema(deps *models.WorkflowDependencies) string {
	crd := testutil.NewTestWorkflowCRD("test-workflow", "CertificateRenewal", "job")
	crd.Spec.Labels.Component = "deployment"
	crd.Spec.Execution.Bundle = "ghcr.io/test/bundle:latest@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	crd.Spec.Parameters = []models.WorkflowParameter{
		{Name: "TARGET_NAMESPACE", Type: "string", Required: true, Description: "Target namespace for certificate renewal"},
	}
	crd.Spec.Dependencies = deps
	return testutil.MarshalWorkflowCRD(crd)
}

var _ = Describe("OgenWorkflowQuerier (DD-WE-006)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("GetWorkflowDependencies", func() {
		It("UT-WE-006-001: should extract secret dependencies from workflow content", func() {
			content := buildTestSchema(&models.WorkflowDependencies{
				Secrets: []models.ResourceDependency{{Name: "gitea-repo-creds"}},
			})
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
			content := buildTestSchema(&models.WorkflowDependencies{
				Secrets:    []models.ResourceDependency{{Name: "gitea-repo-creds"}},
				ConfigMaps: []models.ResourceDependency{{Name: "remediation-config"}},
			})
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
			content := buildTestSchema(nil)
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

	Context("GetWorkflowExecutionBundle", func() {
		It("UT-WE-006-009: should return bundle and digest from catalog", func() {
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.RemediationWorkflow{
					ExecutionBundle:       ogenclient.NewOptString("ghcr.io/test/exec:v1.0.0"),
					ExecutionBundleDigest: ogenclient.NewOptString("sha256:abc123"),
				},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			bundle, digest, err := querier.GetWorkflowExecutionBundle(ctx, uuid.New().String())
			Expect(err).ToNot(HaveOccurred())
			Expect(bundle).To(Equal("ghcr.io/test/exec:v1.0.0"))
			Expect(digest).To(Equal("sha256:abc123"))
		})

		It("UT-WE-006-010: should return empty strings when bundle is not set", func() {
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.RemediationWorkflow{},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			bundle, digest, err := querier.GetWorkflowExecutionBundle(ctx, uuid.New().String())
			Expect(err).ToNot(HaveOccurred())
			Expect(bundle).To(BeEmpty())
			Expect(digest).To(BeEmpty())
		})

		It("UT-WE-006-011: should return error for invalid UUID", func() {
			querier := weclient.NewOgenWorkflowQuerier(&mockWorkflowCatalogClient{})

			_, _, err := querier.GetWorkflowExecutionBundle(ctx, "not-a-uuid")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid workflow ID"))
		})

		It("UT-WE-006-012: should return error when workflow not found", func() {
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.GetWorkflowByIDNotFound{},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			_, _, err := querier.GetWorkflowExecutionBundle(ctx, uuid.New().String())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("UT-WE-006-013: should return error when DS query fails", func() {
			mock := &mockWorkflowCatalogClient{
				err: fmt.Errorf("connection refused"),
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			_, _, err := querier.GetWorkflowExecutionBundle(ctx, uuid.New().String())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("DS query failed"))
		})
	})

	// ========================================
	// Issue #658: Error classification — distinguish 404 vs 500 vs unexpected
	// ========================================
	Context("Error classification (Issue #658)", func() {
		It("UT-WE-658-001: ResolveWorkflowCatalogMetadata should classify DS 500 as server error, not 'not found'", func() {
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.GetWorkflowByIDInternalServerError{},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			_, err := querier.ResolveWorkflowCatalogMetadata(ctx, uuid.New().String())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).ToNot(ContainSubstring("not found"),
				"DS 500 must not be misclassified as 'not found'")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("server error"),
				ContainSubstring("internal server error"),
				ContainSubstring("500"),
			), "error should indicate a Data Storage server failure")
		})

		It("UT-WE-658-002: ResolveWorkflowCatalogMetadata should classify DS 404 as 'not found'", func() {
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.GetWorkflowByIDNotFound{},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			_, err := querier.ResolveWorkflowCatalogMetadata(ctx, uuid.New().String())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("UT-WE-658-003: GetWorkflowExecutionEngine should classify DS 500 as server error", func() {
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.GetWorkflowByIDInternalServerError{},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			_, _, err := querier.GetWorkflowExecutionEngine(ctx, uuid.New().String())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).ToNot(ContainSubstring("not found"))
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("server error"),
				ContainSubstring("internal server error"),
				ContainSubstring("500"),
			))
		})

		It("UT-WE-658-004: GetWorkflowExecutionBundle should classify DS 500 as server error", func() {
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.GetWorkflowByIDInternalServerError{},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			_, _, err := querier.GetWorkflowExecutionBundle(ctx, uuid.New().String())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).ToNot(ContainSubstring("not found"))
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("server error"),
				ContainSubstring("internal server error"),
				ContainSubstring("500"),
			))
		})

		It("UT-WE-658-005: GetWorkflowDependencies should classify DS 500 as server error", func() {
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.GetWorkflowByIDInternalServerError{},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			_, err := querier.GetWorkflowDependencies(ctx, uuid.New().String())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).ToNot(ContainSubstring("not found"))
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("server error"),
				ContainSubstring("internal server error"),
				ContainSubstring("500"),
			))
		})

		It("UT-WE-658-006: GetWorkflowEngineConfig should classify DS 500 as server error", func() {
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.GetWorkflowByIDInternalServerError{},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			_, err := querier.GetWorkflowEngineConfig(ctx, uuid.New().String())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).ToNot(ContainSubstring("not found"))
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("server error"),
				ContainSubstring("internal server error"),
				ContainSubstring("500"),
			))
		})

		It("UT-WE-658-007: 404 regression guard — type switch still classifies correctly", func() {
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.GetWorkflowByIDNotFound{},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			_, err := querier.GetWorkflowDependencies(ctx, uuid.New().String())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	// ========================================
	// Issue #650: Consolidated querier — single DS call for all metadata
	// ========================================
	Context("ResolveWorkflowCatalogMetadata (Issue #650)", func() {
		It("UT-WE-650-003: should return all metadata from single DS call", func() {
			content := buildTestSchema(&models.WorkflowDependencies{
				Secrets: []models.ResourceDependency{{Name: "my-secret"}},
			})
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.RemediationWorkflow{
					ExecutionEngine:       "tekton",
					WorkflowName:          "cert-renewal",
					ExecutionBundle:       ogenclient.NewOptString("ghcr.io/test/exec:v1"),
					ExecutionBundleDigest: ogenclient.NewOptString("sha256:abc123"),
					ServiceAccountName:    ogenclient.NewOptString("workflow-sa"),
					Content:               content,
				},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			meta, err := querier.ResolveWorkflowCatalogMetadata(ctx, uuid.New().String())
			Expect(err).ToNot(HaveOccurred())
			Expect(meta.ExecutionEngine).To(Equal("tekton"))
			Expect(meta.WorkflowName).To(Equal("cert-renewal"))
			Expect(meta.ExecutionBundle).To(Equal("ghcr.io/test/exec:v1"))
			Expect(meta.ExecutionBundleDigest).To(Equal("sha256:abc123"))
			Expect(meta.ServiceAccountName).To(Equal("workflow-sa"))
			Expect(meta.Dependencies.Secrets).To(HaveLen(1))
			Expect(meta.Dependencies.Secrets[0].Name).To(Equal("my-secret"))
		})

		It("UT-WE-650-002: should return empty SA when catalog entry has no SA", func() {
			content := buildTestSchema(nil)
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.RemediationWorkflow{
					ExecutionEngine: "job",
					WorkflowName:    "restart-pod",
					Content:         content,
				},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			meta, err := querier.ResolveWorkflowCatalogMetadata(ctx, uuid.New().String())
			Expect(err).ToNot(HaveOccurred())
			Expect(meta.ServiceAccountName).To(BeEmpty())
			Expect(meta.ExecutionEngine).To(Equal("job"))
		})

		It("UT-WE-650-004: should return error when DS is unreachable", func() {
			mock := &mockWorkflowCatalogClient{
				err: fmt.Errorf("connection refused"),
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			_, err := querier.ResolveWorkflowCatalogMetadata(ctx, uuid.New().String())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("DS query failed"))
		})

		It("UT-WE-650-010: should extract dependencies and engineConfig from Content", func() {
			content := buildTestSchema(&models.WorkflowDependencies{
				Secrets:    []models.ResourceDependency{{Name: "secret-a"}},
				ConfigMaps: []models.ResourceDependency{{Name: "config-b"}},
			})
			mock := &mockWorkflowCatalogClient{
				response: &ogenclient.RemediationWorkflow{
					ExecutionEngine: "tekton",
					Content:         content,
				},
			}
			querier := weclient.NewOgenWorkflowQuerier(mock)

			meta, err := querier.ResolveWorkflowCatalogMetadata(ctx, uuid.New().String())
			Expect(err).ToNot(HaveOccurred())
			Expect(meta.Dependencies.Secrets).To(HaveLen(1))
			Expect(meta.Dependencies.ConfigMaps).To(HaveLen(1))
		})
	})
})
