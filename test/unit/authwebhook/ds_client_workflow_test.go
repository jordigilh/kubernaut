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

package authwebhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// validWorkflowJSON returns a minimal RemediationWorkflow JSON that satisfies
// the ogen decoder's required-field validation. Used by 200/201 success tests.
func validWorkflowJSON(workflowID string) map[string]interface{} {
	return map[string]interface{}{
		"workflowId":      workflowID,
		"workflowName":    "crashloop-rollback",
		"actionType":      "RestartPod",
		"version":         "1.0.0",
		"schemaVersion":   "1.0",
		"name":            "Crashloop Rollback",
		"description":     map[string]string{"what": "Rolls back crashlooping pods", "whenToUse": "CrashLoopBackOff detected"},
		"content":         "apiVersion: kubernaut.ai/v1alpha1\nkind: RemediationWorkflow",
		"contentHash":     "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"executionEngine": "tekton",
		"labels": map[string]interface{}{
			"severity":    []string{"critical"},
			"component":   "pod",
			"environment": []string{"production"},
			"priority":    "P0",
		},
		"status": "Active",
	}
}

// rfc7807Body returns a valid RFC 7807 Problem Details JSON for error response tests.
func rfc7807Body(typeURI, title string, status int, detail string) map[string]interface{} {
	return map[string]interface{}{
		"type":   typeURI,
		"title":  title,
		"status": status,
		"detail": detail,
	}
}

// writeRFC7807Response writes an application/problem+json response to the http.ResponseWriter.
func writeRFC7807Response(w http.ResponseWriter, statusCode int, body map[string]interface{}) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(body)
}

var _ = Describe("UT-AW-446: DSClientAdapter Workflow operations", Label("unit", "workflow", "ds-client", "446"), func() {
	var (
		ctx    context.Context
		server *httptest.Server
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	buildAdapter := func(handler http.Handler) *authwebhook.DSClientAdapter {
		server = httptest.NewServer(handler)
		client, err := ogenclient.NewClient(server.URL)
		Expect(err).ToNot(HaveOccurred())
		return authwebhook.NewDSClientAdapterFromClient(client, logr.Discard())
	}

	// ========================================
	// CreateWorkflowInline
	// ========================================
	Describe("CreateWorkflowInline", func() {

		// --- Error response handling (new behavior) ---

		It("UT-AW-446-001: should surface RFC 7807 details when DS returns 400 Bad Request", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v1/workflows", func(w http.ResponseWriter, _ *http.Request) {
				writeRFC7807Response(w, http.StatusBadRequest, rfc7807Body(
					"https://kubernaut.ai/problems/workflow-validation-failed",
					"Workflow Validation Failed",
					400,
					"Execution bundle image not found in registry",
				))
			})

			adapter := buildAdapter(mux)
			result, err := adapter.CreateWorkflowInline(ctx, "content", "crd", "admin@example.com")

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("workflow registration rejected"))
			Expect(err.Error()).To(ContainSubstring("Workflow Validation Failed"))
			Expect(err.Error()).To(ContainSubstring("Execution bundle image not found in registry"))
		})

		It("UT-AW-446-002: should surface RFC 7807 details when DS returns 409 Conflict", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v1/workflows", func(w http.ResponseWriter, _ *http.Request) {
				writeRFC7807Response(w, http.StatusConflict, rfc7807Body(
					"https://kubernaut.ai/problems/workflow-conflict",
					"Workflow Conflict",
					409,
					"Workflow crashloop-rollback v1.0.0 already exists",
				))
			})

			adapter := buildAdapter(mux)
			result, err := adapter.CreateWorkflowInline(ctx, "content", "crd", "admin@example.com")

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("workflow already exists"))
			Expect(err.Error()).To(ContainSubstring("Workflow Conflict"))
			Expect(err.Error()).To(ContainSubstring("crashloop-rollback v1.0.0 already exists"))
		})

		It("UT-AW-446-003: should surface RFC 7807 details when DS returns 403 Forbidden", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v1/workflows", func(w http.ResponseWriter, _ *http.Request) {
				writeRFC7807Response(w, http.StatusForbidden, rfc7807Body(
					"https://kubernaut.ai/problems/forbidden",
					"Access Denied",
					403,
					"Insufficient permissions to register workflows",
				))
			})

			adapter := buildAdapter(mux)
			result, err := adapter.CreateWorkflowInline(ctx, "content", "crd", "viewer@example.com")

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("workflow registration forbidden"))
			Expect(err.Error()).To(ContainSubstring("Access Denied"))
			Expect(err.Error()).To(ContainSubstring("Insufficient permissions"))
		})

		It("UT-AW-446-004: should surface RFC 7807 details when DS returns 401 Unauthorized", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v1/workflows", func(w http.ResponseWriter, _ *http.Request) {
				writeRFC7807Response(w, http.StatusUnauthorized, rfc7807Body(
					"https://kubernaut.ai/problems/unauthorized",
					"Authentication Required",
					401,
					"Missing or invalid service account token",
				))
			})

			adapter := buildAdapter(mux)
			result, err := adapter.CreateWorkflowInline(ctx, "content", "crd", "anonymous")

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("workflow registration unauthorized"))
			Expect(err.Error()).To(ContainSubstring("Authentication Required"))
			Expect(err.Error()).To(ContainSubstring("Missing or invalid service account token"))
		})

		It("UT-AW-446-005: should surface RFC 7807 details when DS returns 500 Internal Server Error", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v1/workflows", func(w http.ResponseWriter, _ *http.Request) {
				writeRFC7807Response(w, http.StatusInternalServerError, rfc7807Body(
					"https://kubernaut.ai/problems/internal-error",
					"Internal Server Error",
					500,
					"Database connection pool exhausted",
				))
			})

			adapter := buildAdapter(mux)
			result, err := adapter.CreateWorkflowInline(ctx, "content", "crd", "admin@example.com")

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("workflow registration server error"))
			Expect(err.Error()).To(ContainSubstring("Internal Server Error"))
			Expect(err.Error()).To(ContainSubstring("Database connection pool exhausted"))
		})

		// --- Success response handling (no regression) ---

		It("UT-AW-446-006: should map 201 Created response to WorkflowRegistrationResult", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v1/workflows", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(validWorkflowJSON("550e8400-e29b-41d4-a716-446655440000"))
			})

			adapter := buildAdapter(mux)
			result, err := adapter.CreateWorkflowInline(ctx, "content", "crd", "admin@example.com")

			Expect(err).ToNot(HaveOccurred())
			Expect(result.WorkflowID).To(Equal("550e8400-e29b-41d4-a716-446655440000"))
			Expect(result.WorkflowName).To(Equal("crashloop-rollback"))
			Expect(result.Version).To(Equal("1.0.0"))
			Expect(result.Status).To(Equal("Active"))
			Expect(result.PreviouslyExisted).To(BeFalse())
		})

		It("UT-AW-446-007: should map 200 OK response to WorkflowRegistrationResult with PreviouslyExisted=true", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v1/workflows", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(validWorkflowJSON("550e8400-e29b-41d4-a716-446655440000"))
			})

			adapter := buildAdapter(mux)
			result, err := adapter.CreateWorkflowInline(ctx, "content", "crd", "admin@example.com")

			Expect(err).ToNot(HaveOccurred())
			Expect(result.PreviouslyExisted).To(BeTrue())
		})
	})

	// ========================================
	// DisableWorkflow
	// ========================================
	Describe("DisableWorkflow", func() {

		It("UT-AW-446-008: should surface RFC 7807 details when DS returns 400 Bad Request on disable", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("PATCH /api/v1/workflows/{workflow_id}/disable", func(w http.ResponseWriter, _ *http.Request) {
				writeRFC7807Response(w, http.StatusBadRequest, rfc7807Body(
					"https://kubernaut.ai/problems/bad-request",
					"Invalid Request",
					400,
					"Workflow is already disabled",
				))
			})

			adapter := buildAdapter(mux)
			err := adapter.DisableWorkflow(ctx, "550e8400-e29b-41d4-a716-446655440000", "CRD deleted", "admin@example.com")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("disable workflow"))
			Expect(err.Error()).To(ContainSubstring("bad request"))
			Expect(err.Error()).To(ContainSubstring("Invalid Request"))
			Expect(err.Error()).To(ContainSubstring("Workflow is already disabled"))
		})

		It("UT-AW-446-009: should surface RFC 7807 details when DS returns 404 Not Found on disable", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("PATCH /api/v1/workflows/{workflow_id}/disable", func(w http.ResponseWriter, _ *http.Request) {
				writeRFC7807Response(w, http.StatusNotFound, rfc7807Body(
					"https://kubernaut.ai/problems/not-found",
					"Workflow Not Found",
					404,
					"No workflow with ID 550e8400-e29b-41d4-a716-446655440000",
				))
			})

			adapter := buildAdapter(mux)
			err := adapter.DisableWorkflow(ctx, "550e8400-e29b-41d4-a716-446655440000", "CRD deleted", "admin@example.com")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("disable workflow"))
			Expect(err.Error()).To(ContainSubstring("not found"))
			Expect(err.Error()).To(ContainSubstring("Workflow Not Found"))
			Expect(err.Error()).To(ContainSubstring("No workflow with ID"))
		})

		It("UT-AW-446-010: should return nil error when DS returns 200 OK on disable", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("PATCH /api/v1/workflows/{workflow_id}/disable", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(validWorkflowJSON("550e8400-e29b-41d4-a716-446655440000"))
			})

			adapter := buildAdapter(mux)
			err := adapter.DisableWorkflow(ctx, "550e8400-e29b-41d4-a716-446655440000", "CRD deleted", "admin@example.com")

			Expect(err).ToNot(HaveOccurred())
		})
	})
})
