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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

// Test Plan: docs/testing/DD-WORKFLOW-017/execution_bundle_test_plan_v1.0.md
// Authority: BR-WORKFLOW-004 (Workflow Schema Format Specification)
// Authority: DD-WORKFLOW-017 (Workflow Lifecycle Component Interactions)
// TDD Phase: RED

// baseSchemaPrefix provides a valid schema where only the execution section varies.
// All fields satisfy BR-WORKFLOW-004 non-execution requirements.
const baseSchemaPrefix = `metadata:
  workflowId: exec-bundle-test
  version: "v1.0.0"
  description:
    what: Tests execution.bundle validation
    whenToUse: When validating digest enforcement
    whenNotToUse: N/A
    preconditions: None
actionType: RestartPod
labels:
  signalType: OOMKilled
  severity: [critical]
  component: pod
  environment: [production]
  priority: P0
parameters:
  - name: NAMESPACE
    type: string
    description: Target namespace
    required: true
`

// UT-DS-017-011: digest-only execution.bundle (positive)
const validDigestOnlyBundleSchemaYAML = baseSchemaPrefix + `execution:
  engine: tekton
  bundle: quay.io/kubernaut/workflows/scale-memory-bundle@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
`

// UT-DS-017-012: tag+digest execution.bundle (positive)
const validTagDigestBundleSchemaYAML = baseSchemaPrefix + `execution:
  engine: tekton
  bundle: quay.io/kubernaut/workflows/scale-memory-bundle:v1.0.0@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
`

// UT-DS-017-013: tag-only execution.bundle (negative — must be rejected)
const tagOnlyBundleSchemaYAML = baseSchemaPrefix + `execution:
  engine: tekton
  bundle: quay.io/kubernaut/workflows/scale-memory-bundle:v1.0.0
`

// UT-DS-017-014: no execution section (negative — must be rejected)
const noExecutionSectionSchemaYAML = baseSchemaPrefix

// UT-DS-017-015: empty bundle string (negative — must be rejected)
const emptyBundleSchemaYAML = baseSchemaPrefix + `execution:
  engine: tekton
  bundle: ""
`

// UT-DS-017-016: execution section without bundle field (negative — must be rejected)
const executionNoBundleSchemaYAML = baseSchemaPrefix + `execution:
  engine: tekton
`

// UT-DS-017-017: non-sha256 digest algorithm (negative — must be rejected)
const wrongAlgorithmBundleSchemaYAML = baseSchemaPrefix + `execution:
  engine: tekton
  bundle: quay.io/kubernaut/test@md5:abc123def456
`

// UT-DS-017-018: short sha256 digest (negative — must be rejected)
const shortDigestBundleSchemaYAML = baseSchemaPrefix + `execution:
  engine: tekton
  bundle: quay.io/kubernaut/test@sha256:abc123
`

var _ = Describe("UT-DS/WF-017: Execution Bundle Validation", func() {

	// =========================================================================
	// 1. Schema Validation (UT-DS-017-011 through UT-DS-017-018)
	// =========================================================================
	// SUT: schema.Parser.ParseAndValidate()
	// Preconditions: parser instantiated in BeforeEach; base schema satisfies
	// all BR-WORKFLOW-004 non-execution requirements.

	Context("Schema validation: execution.bundle digest enforcement", func() {
		var parser *schema.Parser

		BeforeEach(func() {
			parser = schema.NewParser()
		})

		// --- 1.1 Positive Scenarios ---

		It("UT-DS-017-011: should accept valid digest-only execution.bundle", func() {
			parsedSchema, err := parser.ParseAndValidate(validDigestOnlyBundleSchemaYAML)
			Expect(err).ToNot(HaveOccurred(),
				"parser must not reject digest-only bundle")
			Expect(parsedSchema.Execution).ToNot(BeNil(),
				"execution section must be parsed")
			Expect(parsedSchema.Execution.Bundle).To(ContainSubstring("@sha256:"),
				"digest reference must be preserved in parsed struct")
		})

		It("UT-DS-017-012: should accept valid tag+digest execution.bundle", func() {
			parsedSchema, err := parser.ParseAndValidate(validTagDigestBundleSchemaYAML)
			Expect(err).ToNot(HaveOccurred(),
				"parser must not reject tag+digest bundle")
			Expect(parsedSchema.Execution).ToNot(BeNil(),
				"execution section must be parsed")
			Expect(parsedSchema.Execution.Bundle).To(ContainSubstring(":v1.0.0@sha256:"),
				"both tag and digest must be preserved in parsed struct")
		})

		// --- 1.2 Negative Scenarios ---

		It("UT-DS-017-013: should reject tag-only execution.bundle", func() {
			_, err := parser.ParseAndValidate(tagOnlyBundleSchemaYAML)
			Expect(err).To(HaveOccurred(),
				"tag-only bundle must be rejected")

			var schemaErr *models.SchemaValidationError
			Expect(errors.As(err, &schemaErr)).To(BeTrue(),
				"error must be *models.SchemaValidationError")
			Expect(schemaErr.Field).To(Equal("execution.bundle"),
				"Field must identify the offending field")
			Expect(schemaErr.Message).To(ContainSubstring("sha256"),
				"Message must tell the operator which digest algorithm is required")
		})

		It("UT-DS-017-014: should reject schema with missing execution section", func() {
			_, err := parser.ParseAndValidate(noExecutionSectionSchemaYAML)
			Expect(err).To(HaveOccurred(),
				"missing execution section must be rejected")

			var schemaErr *models.SchemaValidationError
			Expect(errors.As(err, &schemaErr)).To(BeTrue(),
				"error must be *models.SchemaValidationError")
			Expect(schemaErr.Field).To(Equal("execution"),
				"Field must reference the missing section, not a field within it")
		})

		It("UT-DS-017-015: should reject empty bundle string", func() {
			_, err := parser.ParseAndValidate(emptyBundleSchemaYAML)
			Expect(err).To(HaveOccurred(),
				"empty bundle string must be rejected")

			var schemaErr *models.SchemaValidationError
			Expect(errors.As(err, &schemaErr)).To(BeTrue(),
				"error must be *models.SchemaValidationError")
			Expect(schemaErr.Field).To(Equal("execution.bundle"),
				"Field must identify the offending field")
		})

		It("UT-DS-017-016: should reject execution section without bundle field", func() {
			_, err := parser.ParseAndValidate(executionNoBundleSchemaYAML)
			Expect(err).To(HaveOccurred(),
				"execution without bundle must be rejected")

			var schemaErr *models.SchemaValidationError
			Expect(errors.As(err, &schemaErr)).To(BeTrue(),
				"error must be *models.SchemaValidationError")
			Expect(schemaErr.Field).To(Equal("execution.bundle"),
				"Field must identify the missing field specifically")
		})

		It("UT-DS-017-017: should reject non-sha256 digest algorithm", func() {
			_, err := parser.ParseAndValidate(wrongAlgorithmBundleSchemaYAML)
			Expect(err).To(HaveOccurred(),
				"non-sha256 digest must be rejected")

			var schemaErr *models.SchemaValidationError
			Expect(errors.As(err, &schemaErr)).To(BeTrue(),
				"error must be *models.SchemaValidationError")
			Expect(schemaErr.Field).To(Equal("execution.bundle"),
				"Field must identify the offending field")
			Expect(schemaErr.Message).To(ContainSubstring("sha256"),
				"Message must guide operator toward the correct algorithm")
		})

		It("UT-DS-017-018: should reject short sha256 digest (not 64 hex chars)", func() {
			_, err := parser.ParseAndValidate(shortDigestBundleSchemaYAML)
			Expect(err).To(HaveOccurred(),
				"short digest must be rejected")

			var schemaErr *models.SchemaValidationError
			Expect(errors.As(err, &schemaErr)).To(BeTrue(),
				"error must be *models.SchemaValidationError")
			Expect(schemaErr.Field).To(Equal("execution.bundle"),
				"Field must identify the offending field")
			Expect(schemaErr.Message).To(ContainSubstring("64"),
				"Message must indicate the expected hex character count")
		})
	})

	// =========================================================================
	// 2. Handler Validation (UT-WF-017-010 through UT-WF-017-012)
	// =========================================================================
	// SUT: server.Handler.HandleCreateWorkflow()
	// Preconditions: Handler wired with MockImagePuller + SchemaExtractor,
	// no DB (nil repository). RFC 7807 Problem Details on rejection.

	Context("Handler validation: execution.bundle in OCI registration", func() {

		newHandlerWithMockExtractor := func(puller oci.ImagePuller) *server.Handler {
			p := schema.NewParser()
			extractor := oci.NewSchemaExtractor(puller, p)
			return server.NewHandler(nil, server.WithSchemaExtractor(extractor))
		}

		makeCreateRequest := func(schemaImage string) *http.Request {
			body := map[string]string{"schemaImage": schemaImage}
			jsonBody, err := json.Marshal(body)
			Expect(err).ToNot(HaveOccurred())
			return httptest.NewRequest(http.MethodPost, "/api/v1/workflows", bytes.NewReader(jsonBody))
		}

		It("UT-WF-017-010: should accept OCI registration with valid digest-pinned bundle", func() {
			puller := oci.NewMockImagePuller(validDigestOnlyBundleSchemaYAML)
			handler := newHandlerWithMockExtractor(puller)
			req := makeCreateRequest("quay.io/kubernaut/schemas/exec-bundle-test:v1.0.0")
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).ToNot(Equal(http.StatusBadRequest),
				"valid bundle must not be rejected as bad request")
			Expect(rr.Code).ToNot(Equal(http.StatusUnprocessableEntity),
				"valid schema must not be reported as missing")
			Expect(rr.Code).ToNot(Equal(http.StatusBadGateway),
				"mock puller must not cause image pull failure")
		})

		It("UT-WF-017-011: should reject OCI registration when bundle has tag-only reference", func() {
			puller := oci.NewMockImagePuller(tagOnlyBundleSchemaYAML)
			handler := newHandlerWithMockExtractor(puller)
			req := makeCreateRequest("quay.io/kubernaut/schemas/exec-bundle-test:v1.0.0")
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest),
				"tag-only execution.bundle must be rejected with 400")

			var problem map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed(),
				"response body must be valid RFC 7807 JSON")
			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/validation-error"),
				"RFC 7807 type must be validation-error")
			Expect(problem["detail"]).To(ContainSubstring("execution.bundle"),
				"RFC 7807 detail must identify the offending field")
		})

		It("UT-WF-017-012: should reject OCI registration when execution section is missing", func() {
			puller := oci.NewMockImagePuller(noExecutionSectionSchemaYAML)
			handler := newHandlerWithMockExtractor(puller)
			req := makeCreateRequest("quay.io/kubernaut/schemas/exec-bundle-test:v1.0.0")
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest),
				"missing execution section must be rejected with 400")

			var problem map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed(),
				"response body must be valid RFC 7807 JSON")
			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/validation-error"),
				"RFC 7807 type must be validation-error")
			Expect(problem["detail"]).To(ContainSubstring("execution"),
				"RFC 7807 detail must reference the missing section")
		})

		It("UT-WF-017-013: should reject OCI registration when execution.bundle image does not exist in registry", func() {
			puller := oci.NewMockImagePullerWithFailingExists(
				validDigestOnlyBundleSchemaYAML,
				fmt.Errorf("MANIFEST_UNKNOWN: manifest unknown"),
			)
			handler := newHandlerWithMockExtractor(puller)
			req := makeCreateRequest("quay.io/kubernaut/schemas/exec-bundle-test:v1.0.0")
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest),
				"non-existent execution.bundle must be rejected with 400")
			Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"),
				"error response must use RFC 7807 content type")

			var problem map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed(),
				"response body must be valid RFC 7807 JSON")
			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/bundle-not-found"),
				"RFC 7807 type must be bundle-not-found (distinct from validation-error)")
			Expect(problem["detail"]).To(ContainSubstring("execution.bundle"),
				"RFC 7807 detail must reference the bundle field")
		})
	})
})
