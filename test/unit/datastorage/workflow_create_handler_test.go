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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

// mockActionTypeValidator implements server.ActionTypeValidator for testing.
type mockActionTypeValidator struct {
	existsFn func(ctx context.Context, actionType string) (bool, error)
}

func (m *mockActionTypeValidator) ActionTypeExists(ctx context.Context, actionType string) (bool, error) {
	if m.existsFn != nil {
		return m.existsFn(ctx, actionType)
	}
	return true, nil // default: all types valid
}

// ========================================
// INLINE WORKFLOW REGISTRATION HANDLER UNIT TESTS
// ========================================
// Authority: DD-WORKFLOW-017 (Workflow Lifecycle Component Interactions)
// Business Requirement: BR-WORKFLOW-006 (Inline CRD-based workflow registration)
// ADR-058: Webhook-driven workflow registration
//
// Strategy: Unit tests for HandleCreateWorkflow with inline schema content.
// Tests send content (raw YAML) directly to the handler.
// Database interactions are validated at the integration test tier.
// ========================================

const validInlineRegistrationSchemaYAML = `apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: scale-memory
spec:
  metadata:
    workflowName: scale-memory
    version: "v1.0.0"
    description:
      what: Increases memory limits for OOMKilled pods
      whenToUse: When pods are OOMKilled due to insufficient memory
      whenNotToUse: When OOM is caused by a memory leak
      preconditions: Pod managed by a Deployment or StatefulSet
  actionType: IncreaseMemoryLimits
  labels:
    signalType: OOMKilled
    severity: [critical]
    component: pod
    environment: [production]
    priority: P0
  parameters:
    - name: MEMORY_INCREASE_PERCENT
      type: integer
      description: Percentage to increase memory limit
      default: "25"
      required: true
  execution:
    engine: tekton
    bundle: quay.io/kubernaut/workflows/scale-memory-bundle:v1.0.0@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
`

const invalidInlineRegistrationSchemaYAML = `apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: broken
spec:
  metadata:
    workflowName: broken
    version: "v0.1.0"
`

var _ = Describe("Inline Workflow Registration Handler (DD-WORKFLOW-017)", func() {

	makeInlineRequest := func(content string) *http.Request {
		body := map[string]string{"content": content}
		jsonBody, err := json.Marshal(body)
		Expect(err).ToNot(HaveOccurred())
		return httptest.NewRequest(http.MethodPost, "/api/v1/workflows", bytes.NewReader(jsonBody))
	}

	Describe("UT-WF-017-001: Valid inline workflow registration", func() {
		It("should accept valid inline schema content and not return 400", func() {
			handler := server.NewHandler(nil)
			req := makeInlineRequest(validInlineRegistrationSchemaYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).ToNot(Equal(http.StatusBadRequest),
				"Should not reject valid inline schema as bad request")
		})
	})

	Describe("UT-WF-017-002: Empty content rejected", func() {
		It("should return 400 when content is empty", func() {
			handler := server.NewHandler(nil)
			req := makeInlineRequest("")
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			var problem map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
			Expect(problem).To(HaveKey("type"))
			Expect(problem).To(HaveKey("detail"))
		})
	})

	Describe("UT-WF-017-003: Malformed YAML content returns 400", func() {
		It("should return 400 when the content is not parseable YAML", func() {
			handler := server.NewHandler(nil)
			req := makeInlineRequest("not: valid: yaml: {{{")
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			var problem map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/validation-error"))
		})
	})

	Describe("UT-WF-017-004: Content missing required spec fields returns 400", func() {
		It("should return 400 when content lacks required workflow spec fields", func() {
			handler := server.NewHandler(nil)
			req := makeInlineRequest(invalidInlineRegistrationSchemaYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			var problem map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/validation-error"))
		})
	})

	Describe("UT-WF-017-005: Invalid schema returns 400", func() {
		It("should return 400 when the schema is invalid (missing required fields)", func() {
			const minimalInvalidSchemaYAML = `apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: no-labels
spec:
  metadata:
    workflowName: no-labels
    version: "v1.0.0"
    description:
      what: Missing labels, parameters and actionType
      whenToUse: Never
  parameters:
    - name: DUMMY
      type: string
      description: dummy
`
			handler := server.NewHandler(nil)
			req := makeInlineRequest(minimalInvalidSchemaYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			var problem map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/validation-error"))
		})
	})

	Describe("UT-WF-017-006: Request body must be JSON", func() {
		It("should return 400 for invalid JSON body", func() {
			handler := server.NewHandler(nil)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/workflows",
				bytes.NewReader([]byte("not json")))
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("UT-WF-017-007: RFC 7807 compliance", func() {
		It("should return RFC 7807 Problem Details format for all errors", func() {
			handler := server.NewHandler(nil)
			req := makeInlineRequest("not a valid workflow schema")
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))
			var problem map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
			Expect(problem).To(HaveKey("type"))
			Expect(problem).To(HaveKey("title"))
			Expect(problem).To(HaveKey("status"))
			Expect(problem).To(HaveKey("detail"))
		})
	})

	// ========================================
	// GAP-4: Action-Type Taxonomy Validation (DD-WORKFLOW-016)
	// ========================================

	Describe("UT-WF-017-008: Invalid action_type rejected with 400 (BR-WORKFLOW-016-001)", func() {
		const invalidActionTypeSchemaYAML = `apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: invalid-action
spec:
  metadata:
    workflowName: invalid-action
    version: "v1.0.0"
    description:
      what: Tests invalid action type rejection
      whenToUse: When testing taxonomy validation
      whenNotToUse: N/A
      preconditions: None
  actionType: NonExistentAction
  labels:
    signalType: OOMKilled
    severity: [critical]
    component: pod
    environment: [production]
    priority: P0
  parameters:
    - name: PARAM_A
      type: string
      description: A test parameter
      required: true
  execution:
    engine: tekton
    bundle: quay.io/kubernaut/test:v1.0.0@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
`

		It("should return 400 when action_type is not in the taxonomy", func() {
			validator := &mockActionTypeValidator{
				existsFn: func(ctx context.Context, actionType string) (bool, error) {
					validTypes := map[string]bool{
						"ScaleReplicas": true, "RestartPod": true, "IncreaseCPULimits": true,
						"IncreaseMemoryLimits": true, "RollbackDeployment": true, "DrainNode": true,
						"DeletePod": true, "CordonNode": true, "ScaleHPA": true, "TaintNode": true,
					}
					return validTypes[actionType], nil
				},
			}
			handler := server.NewHandler(nil, server.WithActionTypeValidator(validator))
			req := makeInlineRequest(invalidActionTypeSchemaYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			var problem map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/validation-error"))
			Expect(problem["detail"]).To(ContainSubstring("action_type"))
			Expect(problem["detail"]).To(ContainSubstring("NonExistentAction"))
		})
	})

	Describe("UT-WF-017-009: Valid action_type accepted (BR-WORKFLOW-016-001)", func() {
		It("should accept workflow with valid action_type from taxonomy", func() {
			validator := &mockActionTypeValidator{
				existsFn: func(ctx context.Context, actionType string) (bool, error) {
					validTypes := map[string]bool{
						"ScaleReplicas": true, "RestartPod": true, "IncreaseCPULimits": true,
						"IncreaseMemoryLimits": true, "RollbackDeployment": true, "DrainNode": true,
						"DeletePod": true, "CordonNode": true, "ScaleHPA": true, "TaintNode": true,
					}
					return validTypes[actionType], nil
				},
			}
			handler := server.NewHandler(nil, server.WithActionTypeValidator(validator))
			req := makeInlineRequest(validInlineRegistrationSchemaYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).ToNot(Equal(http.StatusBadRequest),
				"Should not reject valid action_type from taxonomy")
		})
	})
})
