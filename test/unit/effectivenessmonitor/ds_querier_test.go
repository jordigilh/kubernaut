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

package effectivenessmonitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
)

// dsEnvelope mirrors the AuditEventsQueryResponse returned by DS
// (see api/openapi/data-storage-v1.yaml, Issue #575).
type dsEnvelope struct {
	Data       []map[string]interface{} `json:"data"`
	Pagination map[string]interface{}   `json:"pagination,omitempty"`
}

func serveDSEnvelope(w http.ResponseWriter, events []map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	resp := dsEnvelope{
		Data: events,
		Pagination: map[string]interface{}{
			"limit":    100,
			"offset":   0,
			"total":    len(events),
			"has_more": false,
		},
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// ========================================
// DataStorageQuerier Tests (DD-EM-002, Issue #575)
//
// All mock servers return the production-matching envelope:
//   {"data": [...], "pagination": {...}}
// ========================================
var _ = Describe("DataStorageQuerier (DD-EM-002)", func() {

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

	// ── QueryPreRemediationHash ───────────────────────────────

	Context("QueryPreRemediationHash", func() {
		It("UT-EM-DSQ-001: should retrieve pre_remediation_spec_hash from remediation.workflow_created event", func() {
			expectedHash := "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Query().Get("event_type")).To(Equal("remediation.workflow_created"))
				serveDSEnvelope(w, []map[string]interface{}{
					{
						"event_type":     "remediation.workflow_created",
						"correlation_id": "test-correlation-001",
						"event_data": map[string]interface{}{
							"pre_remediation_spec_hash": expectedHash,
						},
					},
				})
			}))

			querier := emclient.NewDataStorageHTTPQuerier(server.URL)
			hash, err := querier.QueryPreRemediationHash(ctx, "test-correlation-001")
			Expect(err).ToNot(HaveOccurred())
			Expect(hash).To(Equal(expectedHash))
		})

		It("UT-EM-DSQ-002: should return empty string when no events found", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				serveDSEnvelope(w, []map[string]interface{}{})
			}))

			querier := emclient.NewDataStorageHTTPQuerier(server.URL)
			hash, err := querier.QueryPreRemediationHash(ctx, "no-events-correlation")
			Expect(err).ToNot(HaveOccurred())
			Expect(hash).To(BeEmpty())
		})

		It("UT-EM-DSQ-003: should return empty string when event has no hash field", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				serveDSEnvelope(w, []map[string]interface{}{
					{
						"event_type":     "remediation.workflow_created",
						"correlation_id": "test-correlation-003",
						"event_data":     map[string]interface{}{},
					},
				})
			}))

			querier := emclient.NewDataStorageHTTPQuerier(server.URL)
			hash, err := querier.QueryPreRemediationHash(ctx, "test-correlation-003")
			Expect(err).ToNot(HaveOccurred())
			Expect(hash).To(BeEmpty())
		})

		It("UT-EM-DSQ-004: should return error on HTTP 500", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}))

			querier := emclient.NewDataStorageHTTPQuerier(server.URL)
			_, err := querier.QueryPreRemediationHash(ctx, "test-correlation-004")
			Expect(err).To(HaveOccurred())
		})

		It("UT-EM-DSQ-005: should return error when DS is unreachable", func() {
			querier := emclient.NewDataStorageHTTPQuerier("http://localhost:1")
			_, err := querier.QueryPreRemediationHash(ctx, "test-correlation-005")
			Expect(err).To(HaveOccurred())
		})
	})

	// ── HasWorkflowStarted (Issue #575) ──────────────────────

	Context("HasWorkflowStarted", func() {
		It("UT-EM-DSQ-006: should return true when workflow.started event exists", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Query().Get("event_type")).To(Equal("workflowexecution.workflow.started"))
				serveDSEnvelope(w, []map[string]interface{}{
					{
						"event_type":     "workflowexecution.workflow.started",
						"correlation_id": "corr-started",
					},
				})
			}))

			querier := emclient.NewDataStorageHTTPQuerier(server.URL)
			started, err := querier.HasWorkflowStarted(ctx, "corr-started")
			Expect(err).ToNot(HaveOccurred())
			Expect(started).To(BeTrue())
		})

		It("UT-EM-DSQ-007: should return false when no workflow.started event exists", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				serveDSEnvelope(w, []map[string]interface{}{})
			}))

			querier := emclient.NewDataStorageHTTPQuerier(server.URL)
			started, err := querier.HasWorkflowStarted(ctx, "corr-not-started")
			Expect(err).ToNot(HaveOccurred())
			Expect(started).To(BeFalse())
		})

		It("UT-EM-DSQ-008: should return error on HTTP 500", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}))

			querier := emclient.NewDataStorageHTTPQuerier(server.URL)
			_, err := querier.HasWorkflowStarted(ctx, "corr-error")
			Expect(err).To(HaveOccurred())
		})

		It("UT-EM-DSQ-009: should return error when DS is unreachable", func() {
			querier := emclient.NewDataStorageHTTPQuerier("http://localhost:1")
			_, err := querier.HasWorkflowStarted(ctx, "corr-unreachable")
			Expect(err).To(HaveOccurred())
		})
	})
})
