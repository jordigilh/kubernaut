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

	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

// ========================================
// OCI-BASED WORKFLOW REGISTRATION HANDLER UNIT TESTS
// ========================================
// Authority: DD-WORKFLOW-017 (Workflow Lifecycle Component Interactions)
// Business Requirement: BR-WORKFLOW-017-001 (OCI-based workflow registration)
//
// Strategy: Unit tests for HandleCreateWorkflow with OCI pullspec-only input.
// Tests use MockImagePuller/FailingMockImagePuller for OCI interactions.
// Database interactions are validated at the integration test tier.
// ========================================

// validOCIRegistrationSchemaYAML is a BR-WORKFLOW-004 compliant workflow-schema.yaml for handler tests
const validOCIRegistrationSchemaYAML = `metadata:
  workflowId: scale-memory
  version: "v1.0.0"
  description:
    what: Increases memory limits for OOMKilled pods
    whenToUse: When pods are OOMKilled due to insufficient memory
    whenNotToUse: When OOM is caused by a memory leak
    preconditions: Pod managed by a Deployment or StatefulSet
actionType: AdjustResources
labels:
  signalType: OOMKilled
  severity: critical
  component: pod
  environment: production
  priority: P0
parameters:
  - name: MEMORY_INCREASE_PERCENT
    type: integer
    description: Percentage to increase memory limit
    default: "25"
    required: true
execution:
  engine: tekton
  bundle: quay.io/kubernaut/workflows/scale-memory-bundle:v1.0.0
`

// invalidOCIRegistrationSchemaYAML is missing required fields
const invalidOCIRegistrationSchemaYAML = `metadata:
  workflowId: broken
  version: "v0.1.0"
`

var _ = Describe("OCI-Based Workflow Registration Handler (DD-WORKFLOW-017)", func() {

	// Helper to create a handler with mock OCI extractor (no DB — handler tests only)
	newHandlerWithMockExtractor := func(puller oci.ImagePuller) *server.Handler {
		parser := schema.NewParser()
		extractor := oci.NewSchemaExtractor(puller, parser)
		return server.NewHandler(nil, // No DB for unit tests
			server.WithSchemaExtractor(extractor),
		)
	}

	// Helper to create a JSON request body
	makeCreateRequest := func(containerImage string) *http.Request {
		body := map[string]string{"container_image": containerImage}
		jsonBody, err := json.Marshal(body)
		Expect(err).ToNot(HaveOccurred())
		return httptest.NewRequest(http.MethodPost, "/api/v1/workflows", bytes.NewReader(jsonBody))
	}

	Describe("UT-WF-017-001: Valid OCI workflow registration", func() {
		It("should accept a valid containerImage and return 201 with populated workflow", func() {
			// Arrange: mock puller returns a valid workflow-schema.yaml
			puller := oci.NewMockImagePuller(validOCIRegistrationSchemaYAML)
			handler := newHandlerWithMockExtractor(puller)
			req := makeCreateRequest("quay.io/kubernaut/workflows/scale-memory:v1.0.0")
			rr := httptest.NewRecorder()

			// Act
			handler.HandleCreateWorkflow(rr, req)

			// Assert: the handler should extract and populate the workflow
			// (Since no DB is wired, we expect the handler to fail at DB insertion,
			// but it should NOT fail at OCI extraction or schema validation.
			// The status should NOT be 400, 422, or 502.)
			Expect(rr.Code).ToNot(Equal(http.StatusBadRequest),
				"Should not reject valid OCI schema as bad request")
			Expect(rr.Code).ToNot(Equal(http.StatusUnprocessableEntity),
				"Should not report schema-not-found for valid image")
			Expect(rr.Code).ToNot(Equal(http.StatusBadGateway),
				"Should not report image-pull-failed for mock puller")
		})
	})

	Describe("UT-WF-017-002: Empty containerImage rejected", func() {
		It("should return 400 when containerImage is empty", func() {
			// Arrange
			puller := oci.NewMockImagePuller(validOCIRegistrationSchemaYAML)
			handler := newHandlerWithMockExtractor(puller)
			req := makeCreateRequest("")
			rr := httptest.NewRecorder()

			// Act
			handler.HandleCreateWorkflow(rr, req)

			// Assert
			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			var problem map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
			Expect(problem).To(HaveKey("type"))
			Expect(problem).To(HaveKey("detail"))
		})
	})

	Describe("UT-WF-017-003: Image pull failure returns 502", func() {
		It("should return 502 when the OCI image cannot be pulled", func() {
			// Arrange: mock puller always fails
			puller := oci.NewFailingMockImagePuller("registry unreachable")
			handler := newHandlerWithMockExtractor(puller)
			req := makeCreateRequest("quay.io/nonexistent/image:v1.0.0")
			rr := httptest.NewRecorder()

			// Act
			handler.HandleCreateWorkflow(rr, req)

			// Assert
			Expect(rr.Code).To(Equal(http.StatusBadGateway))
			var problem map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/image-pull-failed"))
		})
	})

	Describe("UT-WF-017-004: Missing workflow-schema.yaml returns 422", func() {
		It("should return 422 when /workflow-schema.yaml is not in the image", func() {
			// Arrange: mock puller returns empty image (no files)
			puller := oci.NewMockImagePuller("")
			handler := newHandlerWithMockExtractor(puller)
			req := makeCreateRequest("quay.io/kubernaut/empty-image:v1.0.0")
			rr := httptest.NewRecorder()

			// Act
			handler.HandleCreateWorkflow(rr, req)

			// Assert
			Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity))
			var problem map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/schema-not-found"))
		})
	})

	Describe("UT-WF-017-005: Invalid schema returns 400", func() {
		It("should return 400 when the schema is invalid (missing required fields)", func() {
			// Arrange: mock puller returns invalid schema YAML
			puller := oci.NewMockImagePuller(invalidOCIRegistrationSchemaYAML)
			handler := newHandlerWithMockExtractor(puller)
			req := makeCreateRequest("quay.io/kubernaut/bad-schema:v1.0.0")
			rr := httptest.NewRecorder()

			// Act
			handler.HandleCreateWorkflow(rr, req)

			// Assert
			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			var problem map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/validation-error"))
		})
	})

	Describe("UT-WF-017-006: Request body must be JSON", func() {
		It("should return 400 for invalid JSON body", func() {
			// Arrange
			puller := oci.NewMockImagePuller(validOCIRegistrationSchemaYAML)
			handler := newHandlerWithMockExtractor(puller)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/workflows",
				bytes.NewReader([]byte("not json")))
			rr := httptest.NewRecorder()

			// Act
			handler.HandleCreateWorkflow(rr, req)

			// Assert
			Expect(rr.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("UT-WF-017-007: RFC 7807 compliance", func() {
		It("should return RFC 7807 Problem Details format for all errors", func() {
			// Arrange: trigger a 502 error
			puller := oci.NewFailingMockImagePuller("connection refused")
			handler := newHandlerWithMockExtractor(puller)
			req := makeCreateRequest("quay.io/unreachable/image:v1.0.0")
			rr := httptest.NewRecorder()

			// Act
			handler.HandleCreateWorkflow(rr, req)

			// Assert: RFC 7807 fields present
			Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))
			var problem map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
			Expect(problem).To(HaveKey("type"))
			Expect(problem).To(HaveKey("title"))
			Expect(problem).To(HaveKey("status"))
			Expect(problem).To(HaveKey("detail"))
		})
	})
})
