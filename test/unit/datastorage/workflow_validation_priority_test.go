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
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// ========================================
// Issue #1070: Validation Error Priority Contract Tests
// ========================================
// Authority: BR-STORAGE-014, BR-WORKFLOW-006, Issue #1070
// Purpose: Lock the sequential error priority of HandleCreateWorkflow so
// parallelization (Issue #1070) preserves the same RFC 7807 error type
// for callers regardless of which check finishes first.
//
// Validation order (as defined in HandleCreateWorkflow):
//   Step 3:  Schema validation         → type = validation-error
//   Step 5a: Action-type taxonomy      → type = validation-error
//   Step 5b: Bundle-exists (OCI)       → type = bundle-not-found
//   Step 5c: Dependency validation     → type = dependency-validation-error
//
// These tests wire all validators so every step *would* fail,
// then assert only the highest-priority error is returned.
// ========================================

// mockDependencyValidator implements validation.DependencyValidator for testing.
type mockDependencyValidator struct {
	err error
}

func (m *mockDependencyValidator) ValidateDependencies(_ context.Context, _ string, _ *models.WorkflowDependencies) error {
	return m.err
}

// validSchemaForPriorityTests returns a well-formed schema YAML with dependencies
// and a known action type, used as the baseline for priority tests.
func validSchemaForPriorityTests() string {
	crd := testutil.NewTestWorkflowCRD("priority-test-wf", "RestartPod", "job")
	crd.Spec.Description = sharedtypes.StructuredDescription{
		What:      "Priority test workflow",
		WhenToUse: "When testing validation priority (#1070)",
	}
	crd.Spec.Execution.Bundle = "quay.io/kubernaut/priority-test:v1.0.0@sha256:f313b9632f3a8d0ffd41150b12715a43a41c6c8e7871bb830fd82c09b5988cc4"
	crd.Spec.Dependencies = &models.WorkflowDependencies{
		Secrets: []models.ResourceDependency{{Name: "missing-secret"}},
	}
	return testutil.MarshalWorkflowCRD(crd)
}

// rejectingActionTypeValidator returns a validator where ActionTypeExists
// always returns false (action type not in taxonomy).
func rejectingActionTypeValidator() *mockActionTypeValidator {
	return &mockActionTypeValidator{
		existsFn: func(_ context.Context, _ string) (bool, error) {
			return false, nil
		},
	}
}

var _ = Describe("Issue #1070: HandleCreateWorkflow Validation Error Priority", Label("unit", "issue-1070"), func() {

	makeRequest := func(content string) *http.Request {
		body := map[string]string{"content": content}
		jsonBody, err := json.Marshal(body)
		Expect(err).ToNot(HaveOccurred())
		return httptest.NewRequest(http.MethodPost, "/api/v1/workflows", bytes.NewReader(jsonBody))
	}

	parseRFC7807 := func(rr *httptest.ResponseRecorder) map[string]interface{} {
		var problem map[string]interface{}
		Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
		return problem
	}

	Context("Sequential error priority (pre-parallelization baseline)", func() {

		It("UT-WF-1070-001: schema validation error beats all downstream failures", func() {
			schemaYAML := validSchemaForPriorityTests()
			failingPuller := oci.NewMockImagePullerWithFailingExists(schemaYAML, fmt.Errorf("bundle not found"))
			extractor := oci.NewSchemaExtractor(failingPuller, schema.NewParser())

			handler := server.NewHandler(nil,
				server.WithActionTypeValidator(rejectingActionTypeValidator()),
				server.WithSchemaExtractor(extractor),
				server.WithDependencyValidator(
					&mockDependencyValidator{err: fmt.Errorf("secret missing-secret not found")},
					"test-ns",
				),
			)

			req := makeRequest("invalid yaml {{{")
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			problem := parseRFC7807(rr)
			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/validation-error"),
				"Schema validation must be the first error, regardless of downstream failures")
		})

		It("UT-WF-1070-002: action-type error beats bundle-not-found and dependency errors", func() {
			schemaYAML := validSchemaForPriorityTests()
			failingPuller := oci.NewMockImagePullerWithFailingExists(schemaYAML, fmt.Errorf("bundle not found"))
			extractor := oci.NewSchemaExtractor(failingPuller, schema.NewParser())

			handler := server.NewHandler(nil,
				server.WithActionTypeValidator(rejectingActionTypeValidator()),
				server.WithSchemaExtractor(extractor),
				server.WithDependencyValidator(
					&mockDependencyValidator{err: fmt.Errorf("secret missing-secret not found")},
					"test-ns",
				),
			)

			req := makeRequest(schemaYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			problem := parseRFC7807(rr)
			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/validation-error"),
				"Action-type rejection must take priority over bundle and dependency errors")
			Expect(problem["detail"]).To(ContainSubstring("action_type"),
				"Error detail should mention action_type")
		})

		It("UT-WF-1070-003: bundle-not-found beats dependency-validation-error", func() {
			schemaYAML := validSchemaForPriorityTests()
			failingPuller := oci.NewMockImagePullerWithFailingExists(schemaYAML, fmt.Errorf("bundle image not in registry"))
			extractor := oci.NewSchemaExtractor(failingPuller, schema.NewParser())

			acceptingValidator := &mockActionTypeValidator{
				existsFn: func(_ context.Context, _ string) (bool, error) {
					return true, nil
				},
			}

			handler := server.NewHandler(nil,
				server.WithActionTypeValidator(acceptingValidator),
				server.WithSchemaExtractor(extractor),
				server.WithDependencyValidator(
					&mockDependencyValidator{err: fmt.Errorf("secret missing-secret not found")},
					"test-ns",
				),
			)

			req := makeRequest(schemaYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			problem := parseRFC7807(rr)
			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/bundle-not-found"),
				"Bundle-not-found must take priority over dependency validation error")
		})

		It("UT-WF-1070-004: dependency-validation-error returned when no higher-priority failures", func() {
			schemaYAML := validSchemaForPriorityTests()
			mockPuller := oci.NewMockImagePuller(schemaYAML)
			extractor := oci.NewSchemaExtractor(mockPuller, schema.NewParser())

			acceptingValidator := &mockActionTypeValidator{
				existsFn: func(_ context.Context, _ string) (bool, error) {
					return true, nil
				},
			}

			handler := server.NewHandler(nil,
				server.WithActionTypeValidator(acceptingValidator),
				server.WithSchemaExtractor(extractor),
				server.WithDependencyValidator(
					&mockDependencyValidator{err: fmt.Errorf("secret missing-secret not found")},
					"test-ns",
				),
			)

			req := makeRequest(schemaYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			problem := parseRFC7807(rr)
			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/dependency-validation-error"),
				"Dependency error should surface when no higher-priority validation fails")
		})

		It("UT-WF-1070-005: all validations pass — no error returned", func() {
			schemaYAML := validSchemaForPriorityTests()
			mockPuller := oci.NewMockImagePuller(schemaYAML)
			extractor := oci.NewSchemaExtractor(mockPuller, schema.NewParser())

			acceptingValidator := &mockActionTypeValidator{
				existsFn: func(_ context.Context, _ string) (bool, error) {
					return true, nil
				},
			}

			handler := server.NewHandler(nil,
				server.WithActionTypeValidator(acceptingValidator),
				server.WithSchemaExtractor(extractor),
				server.WithDependencyValidator(
					&mockDependencyValidator{err: nil},
					"test-ns",
				),
			)

			req := makeRequest(schemaYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).ToNot(Equal(http.StatusBadRequest),
				"No validation error should occur when all checks pass")
		})
	})
})

// Compile-time assertion that mockDependencyValidator satisfies the interface.
var _ validation.DependencyValidator = &mockDependencyValidator{}
