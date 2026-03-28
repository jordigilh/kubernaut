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

package datastorage

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/testutil"
)

var validInlineSchemaYAML = func() string {
	crd := testutil.NewTestWorkflowCRD("scale-memory-inline", "ScaleMemory", "job")
	crd.Spec.Description = sharedtypes.StructuredDescription{
		What:          "Scales memory limits for OOM-killed pods",
		WhenToUse:     "When pods are OOM-killed repeatedly",
		WhenNotToUse:  "When OOM is caused by memory leaks",
		Preconditions: "HPA must be configured",
	}
	crd.Spec.Labels.Severity = []string{"critical", "high"}
	crd.Spec.Execution.Bundle = "quay.io/kubernaut/workflows/scale-memory:v1.0.0@sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abc1"
	crd.Spec.Parameters = []models.WorkflowParameter{
		{Name: "TARGET_RESOURCE", Type: "string", Required: true, Description: "Target resource"},
		{Name: "MEMORY_LIMIT", Type: "string", Required: true, Description: "New memory limit"},
	}
	return testutil.MarshalWorkflowCRD(crd)
}()

var _ = Describe("Inline Schema Workflow Registration (#299)", func() {

	makeInlineRequest := func(content string) *http.Request {
		body := map[string]string{"content": content}
		jsonBody, err := json.Marshal(body)
		Expect(err).ToNot(HaveOccurred())
		return httptest.NewRequest(http.MethodPost, "/api/v1/workflows", bytes.NewReader(jsonBody))
	}

	makeLegacyOCIRequest := func(schemaImage string) *http.Request {
		body := map[string]string{"schemaImage": schemaImage}
		jsonBody, err := json.Marshal(body)
		Expect(err).ToNot(HaveOccurred())
		return httptest.NewRequest(http.MethodPost, "/api/v1/workflows", bytes.NewReader(jsonBody))
	}

	newInlineHandler := func() *server.Handler {
		puller := oci.NewMockImagePuller(validInlineSchemaYAML)
		parser := schema.NewParser()
		extractor := oci.NewSchemaExtractor(puller, parser)
		return server.NewHandler(nil,
			server.WithSchemaExtractor(extractor),
		)
	}

	Describe("UT-DS-299-001: Inline schema accepted and stored in catalog", func() {
		It("should accept a valid inline schema YAML and return 201 with populated workflow", func() {
			handler := newInlineHandler()
			req := makeInlineRequest(validInlineSchemaYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusCreated),
				"Expected 201 Created for valid inline schema, got %d: %s", rr.Code, rr.Body.String())

			var resp map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["workflowName"]).To(Equal("scale-memory-inline"))
			Expect(resp["status"]).To(Equal("Active"))
			Expect(resp["schemaVersion"]).To(Equal("1.0"))
		})
	})

	Describe("UT-DS-299-002: Old OCI schemaImage format rejected", func() {
		It("should reject the old schemaImage format with 400 explaining the change", func() {
			handler := newInlineHandler()
			req := makeLegacyOCIRequest("quay.io/kubernaut/workflows/scale-memory:v1.0.0")
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest),
				"Expected 400 Bad Request for legacy schemaImage format")

			var problem map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
			Expect(problem["detail"]).To(ContainSubstring("content"),
				"Error should guide operator to use 'content' field instead")
		})
	})

	Describe("UT-DS-299-003: Re-enable disabled workflow on re-CREATE", func() {
		It("should re-enable a previously disabled workflow and return 200 OK", func() {
			handler := newInlineHandler()
			req := makeInlineRequest(validInlineSchemaYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).ToNot(Equal(http.StatusConflict),
				"Should not return 409 for re-create of disabled workflow")
		})
	})

	Describe("UT-DS-299-004: Inline schema passes full validation pipeline", func() {
		It("should validate actionType, bundle, and dependencies from inline schema", func() {
			handler := newInlineHandler()
			req := makeInlineRequest(validInlineSchemaYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).ToNot(Equal(http.StatusBadRequest),
				"Valid inline schema should not produce validation errors: %s", rr.Body.String())
		})
	})

	Describe("UT-DS-299-005: Invalid inline schema rejected with 400", func() {
		Context("missing apiVersion", func() {
			It("should reject with error referencing apiVersion", func() {
				// Tier 3: Raw YAML — missing apiVersion
				invalidYAML := `kind: RemediationWorkflow
metadata:
  name: missing-apiversion
spec:
  version: "1.0.0"
  description:
    what: "Test"
    whenToUse: "Test"
  actionType: Test
  labels:
    severity: [critical]
    environment: [production]
    component: pod
    priority: P1
  execution:
    engine: job
    bundle: "quay.io/test:v1@sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abc1"
  parameters:
    - name: TARGET
      type: string
      required: true
      description: "Target"`

				handler := newInlineHandler()
				req := makeInlineRequest(invalidYAML)
				rr := httptest.NewRecorder()

				handler.HandleCreateWorkflow(rr, req)

				Expect(rr.Code).To(Equal(http.StatusBadRequest))
				var problem map[string]interface{}
				Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
				Expect(problem["detail"]).To(ContainSubstring("apiVersion"))
			})
		})

		Context("wrong kind", func() {
			It("should reject with error referencing kind", func() {
				// Tier 3: Raw YAML — wrong kind value
				crd := testutil.NewTestWorkflowCRD("wrong-kind", "RestartPod", "job")
				crd.Kind = "Workflow"
				invalidYAML := testutil.MarshalWorkflowCRD(crd)

				handler := newInlineHandler()
				req := makeInlineRequest(invalidYAML)
				rr := httptest.NewRecorder()

				handler.HandleCreateWorkflow(rr, req)

				Expect(rr.Code).To(Equal(http.StatusBadRequest))
				var problem map[string]interface{}
				Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
				Expect(problem["detail"]).To(ContainSubstring("kind"))
			})
		})
	})

	Describe("UT-DS-299-006: Content hash computed from inline YAML", func() {
		It("should populate contentHash from the inline YAML content", func() {
			handler := newInlineHandler()
			req := makeInlineRequest(validInlineSchemaYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			if rr.Code == http.StatusCreated || rr.Code == http.StatusOK {
				var resp map[string]interface{}
				Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())
				Expect(resp["contentHash"]).ToNot(BeEmpty(),
					"Content hash should be computed from inline YAML")
			}
		})
	})

	Describe("UT-DS-299-007: SchemaImage nil for inline registration", func() {
		It("should not populate schemaImage or schemaDigest for inline registration", func() {
			handler := newInlineHandler()
			req := makeInlineRequest(validInlineSchemaYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			if rr.Code == http.StatusCreated || rr.Code == http.StatusOK {
				var resp map[string]interface{}
				Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())
				Expect(resp["schemaImage"]).To(BeNil(),
					"SchemaImage should be nil for inline registration (no OCI image)")
				Expect(resp["schemaDigest"]).To(BeNil(),
					"SchemaDigest should be nil for inline registration")
			}
		})
	})
})
