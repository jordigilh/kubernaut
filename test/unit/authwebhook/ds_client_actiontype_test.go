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
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// UT-AT-300-012: DS Client Adapter Unit Tests
// BR-WORKFLOW-007.1, BR-WORKFLOW-007.2, BR-WORKFLOW-007.3, BR-WORKFLOW-007.5
//
// Tests that DSClientAdapter correctly maps ogen HTTP responses
// to domain result types for all ActionType operations.
// Uses httptest servers to simulate DS API responses.
// ========================================

var _ = Describe("UT-AT-300-012: DSClientAdapter ActionType operations", Label("unit", "actiontype", "ds-client"), func() {
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

	// Helper: build DSClientAdapter from httptest server
	buildAdapter := func(handler http.Handler) *authwebhook.DSClientAdapter {
		server = httptest.NewServer(handler)
		client, err := ogenclient.NewClient(server.URL)
		Expect(err).ToNot(HaveOccurred())
		return authwebhook.NewDSClientAdapterFromClient(client, logr.Discard())
	}

	// ========================================
	// CreateActionType
	// ========================================
	Describe("CreateActionType", func() {
		It("should map 201 Created response to ActionTypeRegistrationResult with status=created", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v1/action-types", func(w http.ResponseWriter, r *http.Request) {
				var req map[string]interface{}
				body, _ := io.ReadAll(r.Body)
				Expect(json.Unmarshal(body, &req)).To(Succeed())
				Expect(req["name"]).To(Equal("RestartPod"))
				Expect(req["registeredBy"]).To(Equal("admin@example.com"))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				resp := map[string]interface{}{
					"actionType":   "RestartPod",
					"status":       "created",
					"wasReenabled": false,
				}
				json.NewEncoder(w).Encode(resp)
			})

			adapter := buildAdapter(mux)
			desc := ogenclient.ActionTypeDescription{
				What:      "Kill and recreate pods",
				WhenToUse: "When pod is stuck",
			}

			result, err := adapter.CreateActionType(ctx, "RestartPod", desc, "admin@example.com")

			Expect(err).ToNot(HaveOccurred())
			Expect(result.ActionType).To(Equal("RestartPod"))
			Expect(result.Status).To(Equal("created"))
			Expect(result.WasReenabled).To(BeFalse())
		})

		It("should map 200 OK response to ActionTypeRegistrationResult with status=exists", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v1/action-types", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				resp := map[string]interface{}{
					"actionType":   "RestartPod",
					"status":       "exists",
					"wasReenabled": false,
				}
				json.NewEncoder(w).Encode(resp)
			})

			adapter := buildAdapter(mux)
			desc := ogenclient.ActionTypeDescription{What: "Kill pods", WhenToUse: "Stuck pods"}

			result, err := adapter.CreateActionType(ctx, "RestartPod", desc, "admin@example.com")

			Expect(err).ToNot(HaveOccurred())
			Expect(result.ActionType).To(Equal("RestartPod"))
			Expect(result.Status).To(Equal("exists"))
			Expect(result.WasReenabled).To(BeFalse())
		})

		It("should map 200 OK response for re-enabled action type", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v1/action-types", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				resp := map[string]interface{}{
					"actionType":   "RestartPod",
					"status":       "reenabled",
					"wasReenabled": true,
				}
				json.NewEncoder(w).Encode(resp)
			})

			adapter := buildAdapter(mux)
			desc := ogenclient.ActionTypeDescription{What: "Kill pods", WhenToUse: "Stuck pods"}

			result, err := adapter.CreateActionType(ctx, "RestartPod", desc, "admin@example.com")

			Expect(err).ToNot(HaveOccurred())
			Expect(result.ActionType).To(Equal("RestartPod"))
			Expect(result.Status).To(Equal("reenabled"))
			Expect(result.WasReenabled).To(BeTrue())
		})

		It("should return error when DS returns 500", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v1/action-types", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			})

			adapter := buildAdapter(mux)
			desc := ogenclient.ActionTypeDescription{What: "Kill pods", WhenToUse: "Stuck pods"}

			_, err := adapter.CreateActionType(ctx, "RestartPod", desc, "admin@example.com")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("CreateActionType failed"))
		})
	})

	// ========================================
	// UpdateActionType
	// ========================================
	Describe("UpdateActionType", func() {
		It("should map 200 response to ActionTypeUpdateResult with updated fields", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("PATCH /api/v1/action-types/{name}", func(w http.ResponseWriter, r *http.Request) {
				name := r.PathValue("name")
				Expect(name).To(Equal("RestartPod"))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				resp := map[string]interface{}{
					"actionType": "RestartPod",
					"oldDescription": map[string]string{
						"what":      "Old description",
						"whenToUse": "Old when",
					},
					"newDescription": map[string]string{
						"what":      "New description",
						"whenToUse": "New when",
					},
					"updatedFields": []string{"what", "whenToUse"},
				}
				json.NewEncoder(w).Encode(resp)
			})

			adapter := buildAdapter(mux)
			desc := ogenclient.ActionTypeDescription{What: "New description", WhenToUse: "New when"}

			result, err := adapter.UpdateActionType(ctx, "RestartPod", desc, "admin@example.com")

			Expect(err).ToNot(HaveOccurred())
			Expect(result.ActionType).To(Equal("RestartPod"))
			Expect(result.UpdatedFields).To(ConsistOf("what", "whenToUse"))
		})
	})

	// ========================================
	// DisableActionType
	// ========================================
	Describe("DisableActionType", func() {
		It("should map 200 response to ActionTypeDisableResult with Disabled=true", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("PATCH /api/v1/action-types/{name}/disable", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				resp := map[string]interface{}{
					"actionType": "RestartPod",
					"status":     "disabled",
				}
				json.NewEncoder(w).Encode(resp)
			})

			adapter := buildAdapter(mux)

			result, err := adapter.DisableActionType(ctx, "RestartPod", "admin@example.com")

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Disabled).To(BeTrue())
		})

		It("should map 409 Conflict response to ActionTypeDisableResult with dependency info", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("PATCH /api/v1/action-types/{name}/disable", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				resp := map[string]interface{}{
					"actionType":             "RestartPod",
					"dependentWorkflowCount": 3,
					"dependentWorkflows":     []string{"wf-alpha", "wf-beta", "wf-gamma"},
				}
				json.NewEncoder(w).Encode(resp)
			})

			adapter := buildAdapter(mux)

			result, err := adapter.DisableActionType(ctx, "RestartPod", "admin@example.com")

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Disabled).To(BeFalse())
			Expect(result.DependentWorkflowCount).To(Equal(3))
			Expect(result.DependentWorkflows).To(ConsistOf("wf-alpha", "wf-beta", "wf-gamma"))
		})
	})

	// ========================================
	// GetActiveWorkflowCount
	// ========================================
	Describe("GetActiveWorkflowCount", func() {
		It("should return the count from DS workflow-count endpoint", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v1/action-types/{name}/workflow-count", func(w http.ResponseWriter, r *http.Request) {
				name := r.PathValue("name")
				Expect(name).To(Equal("RestartPod"))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]int{"count": 7})
			})

			adapter := buildAdapter(mux)

			count, err := adapter.GetActiveWorkflowCount(ctx, "RestartPod")

			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(7))
		})

		It("should return zero for an action type with no workflows", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v1/action-types/{name}/workflow-count", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]int{"count": 0})
			})

			adapter := buildAdapter(mux)

			count, err := adapter.GetActiveWorkflowCount(ctx, "RestartPod")

			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(0))
		})
	})
})
