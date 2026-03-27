/*
Copyright 2026 Jordi Gil.

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
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

// ========================================
// BR-WE-015: AWXHTTPClient Credential API Unit Tests
// ========================================
// Authority: BR-WE-015 (Ansible Execution Engine)
// These tests validate the AWXHTTPClient's credential management
// methods using httptest to mock the AWX REST API.
// ========================================

var _ = Describe("AWXHTTPClient Credential API (BR-WE-015)", func() {
	var (
		ctx    context.Context
		server *httptest.Server
		client *executor.AWXHTTPClient
		mux    *http.ServeMux
	)

	BeforeEach(func() {
		ctx = context.Background()
		mux = http.NewServeMux()
		server = httptest.NewServer(mux)
		client = executor.NewAWXHTTPClient(server.URL, "test-token", false)
	})

	AfterEach(func() {
		server.Close()
	})

	Context("CreateCredentialType", func() {
		It("UT-WE-015-010: should create a credential type and return its ID", func() {
			mux.HandleFunc("/api/v2/credential_types/", func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodPost))
				Expect(r.Header.Get("Authorization")).To(Equal("Bearer test-token"))
				Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))

				body, err := io.ReadAll(r.Body)
				Expect(err).ToNot(HaveOccurred())

				var payload map[string]interface{}
				Expect(json.Unmarshal(body, &payload)).To(Succeed())
				Expect(payload["name"]).To(Equal("kubernaut-secret-gitea-repo-creds"))
				Expect(payload["kind"]).To(Equal("cloud"))
				Expect(payload).To(HaveKey("inputs"))
				Expect(payload).To(HaveKey("injectors"))

				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id": 15}`))
			})

			inputs := executor.CredentialTypeInputs{
				Fields: []executor.CredentialTypeField{
					{ID: "username", Label: "username", Type: "string", Secret: true},
					{ID: "password", Label: "password", Type: "string", Secret: true},
				},
			}
			injectors := executor.CredentialTypeInjectors{
				Env: map[string]string{
					"KUBERNAUT_SECRET_GITEA_REPO_CREDS_USERNAME": "{{username}}",
					"KUBERNAUT_SECRET_GITEA_REPO_CREDS_PASSWORD": "{{password}}",
				},
			}

			id, err := client.CreateCredentialType(ctx, "kubernaut-secret-gitea-repo-creds", inputs, injectors)
			Expect(err).ToNot(HaveOccurred())
			Expect(id).To(Equal(15))
		})

		It("UT-WE-015-011: should return error on non-201 response", func() {
			mux.HandleFunc("/api/v2/credential_types/", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"detail": "name already exists"}`))
			})

			_, err := client.CreateCredentialType(ctx, "dup", executor.CredentialTypeInputs{}, executor.CredentialTypeInjectors{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("400"))
		})
	})

	Context("FindCredentialTypeByName", func() {
		It("UT-WE-015-012: should find an existing credential type by name", func() {
			mux.HandleFunc("/api/v2/credential_types/", func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodGet))
				Expect(r.URL.Query().Get("name")).To(Equal("kubernaut-secret-gitea-repo-creds"))

				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"count": 1, "results": [{"id": 15}]}`))
			})

			id, err := client.FindCredentialTypeByName(ctx, "kubernaut-secret-gitea-repo-creds")
			Expect(err).ToNot(HaveOccurred())
			Expect(id).To(Equal(15))
		})

		It("UT-WE-015-013: should return error when credential type not found", func() {
			mux.HandleFunc("/api/v2/credential_types/", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"count": 0, "results": []}`))
			})

			_, err := client.FindCredentialTypeByName(ctx, "nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Context("CreateCredential", func() {
		It("UT-WE-015-014: should create a credential and return its ID", func() {
			mux.HandleFunc("/api/v2/credentials/", func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodPost))

				body, err := io.ReadAll(r.Body)
				Expect(err).ToNot(HaveOccurred())

				var payload map[string]interface{}
				Expect(json.Unmarshal(body, &payload)).To(Succeed())
				Expect(payload["name"]).To(Equal("kubernaut-ephemeral-gitea-repo-creds"))
				Expect(payload["credential_type"]).To(BeNumerically("==", 15))
				Expect(payload["organization"]).To(BeNumerically("==", 1))
				Expect(payload).To(HaveKey("inputs"))

				inputs, ok := payload["inputs"].(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(inputs["username"]).To(Equal("admin"))
				Expect(inputs["password"]).To(Equal("secret123"))

				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id": 42}`))
			})

			id, err := client.CreateCredential(ctx, "kubernaut-ephemeral-gitea-repo-creds", 15, 1, map[string]string{
				"username": "admin",
				"password": "secret123",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(id).To(Equal(42))
		})

		It("UT-WE-015-015: should return error on AWX API failure", func() {
			mux.HandleFunc("/api/v2/credentials/", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"detail": "insufficient permissions"}`))
			})

			_, err := client.CreateCredential(ctx, "test", 15, 1, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("403"))
		})
	})

	Context("DeleteCredential", func() {
		It("UT-WE-015-016: should delete a credential by ID", func() {
			mux.HandleFunc("/api/v2/credentials/42/", func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodDelete))
				Expect(r.Header.Get("Authorization")).To(Equal("Bearer test-token"))
				w.WriteHeader(http.StatusNoContent)
			})

			err := client.DeleteCredential(ctx, 42)
			Expect(err).ToNot(HaveOccurred())
		})

		It("UT-WE-015-017: should return error on non-204 response", func() {
			mux.HandleFunc("/api/v2/credentials/99/", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"detail": "not found"}`))
			})

			err := client.DeleteCredential(ctx, 99)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("404"))
		})
	})

	Context("LaunchJobTemplateWithCreds", func() {
		It("UT-WE-015-018: should launch with extra_vars and credential IDs", func() {
			mux.HandleFunc("/api/v2/job_templates/10/launch/", func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodPost))

				body, err := io.ReadAll(r.Body)
				Expect(err).ToNot(HaveOccurred())

				var payload map[string]interface{}
				Expect(json.Unmarshal(body, &payload)).To(Succeed())

				Expect(payload).To(HaveKey("extra_vars"))
				extraVars, ok := payload["extra_vars"].(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(extraVars["TARGET_NAMESPACE"]).To(Equal("demo-ns"))

				Expect(payload).To(HaveKey("credentials"))
				creds, ok := payload["credentials"].([]interface{})
				Expect(ok).To(BeTrue())
				Expect(creds).To(HaveLen(2))
				Expect(creds[0]).To(BeNumerically("==", 42))
				Expect(creds[1]).To(BeNumerically("==", 55))

				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id": 77}`))
			})

			extraVars := map[string]interface{}{
				"TARGET_NAMESPACE": "demo-ns",
			}

			jobID, err := client.LaunchJobTemplateWithCreds(ctx, 10, extraVars, []int{42, 55})
			Expect(err).ToNot(HaveOccurred())
			Expect(jobID).To(Equal(77))
		})

		It("UT-WE-015-019: should work with empty credential list", func() {
			mux.HandleFunc("/api/v2/job_templates/10/launch/", func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				Expect(err).ToNot(HaveOccurred())

				var payload map[string]interface{}
				Expect(json.Unmarshal(body, &payload)).To(Succeed())
				Expect(payload).ToNot(HaveKey("credentials"))

				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id": 78}`))
			})

			jobID, err := client.LaunchJobTemplateWithCreds(ctx, 10, nil, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(jobID).To(Equal(78))
		})

		It("UT-WE-015-020: should return error on launch failure", func() {
			mux.HandleFunc("/api/v2/job_templates/10/launch/", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"detail": "invalid credentials"}`))
			})

			_, err := client.LaunchJobTemplateWithCreds(ctx, 10, nil, []int{999})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("400"))
		})
	})
})
