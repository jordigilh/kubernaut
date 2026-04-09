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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
)

// testAuditEvent builds a schema-compliant AuditEvent JSON map with all 8 required
// fields populated. Callers supply the event_type, correlation_id, and an eventData
// map that is merged into the required event_data structure.
//
// Required by ogen's AuditEvent.Decode() bitmask validation (F9).
func testAuditEvent(eventType, correlationID string, eventData map[string]interface{}) map[string]interface{} {
	ed := map[string]interface{}{
		"event_type": eventType,
	}
	for k, v := range eventData {
		ed[k] = v
	}

	return map[string]interface{}{
		"version":         "1.0",
		"event_type":      eventType,
		"event_timestamp": "2026-01-01T00:00:00Z",
		"event_category":  categoryForEventType(eventType),
		"event_action":    "test_action",
		"event_outcome":   "success",
		"correlation_id":  correlationID,
		"event_data":      ed,
	}
}

// categoryForEventType derives event_category from the event_type prefix.
func categoryForEventType(eventType string) string {
	switch {
	case len(eventType) > 12 && eventType[:12] == "remediation.":
		return "orchestration"
	case len(eventType) > 18 && eventType[:18] == "workflowexecution.":
		return "workflowexecution"
	default:
		return "unknown"
	}
}

// serveOgenCompliantResponse writes a schema-compliant AuditEventsQueryResponse
// JSON envelope that satisfies ogen's strict decoder.
func serveOgenCompliantResponse(w http.ResponseWriter, events []map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"data": events,
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// ========================================
// DataStorageQuerier Tests (DD-EM-002, DD-API-001, Issue #236)
//
// All mock servers return ogen-compliant AuditEvent JSON with the 8
// required fields per F9 and event_type discriminator inside event_data per F4.
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
				serveOgenCompliantResponse(w, []map[string]interface{}{
					testAuditEvent("remediation.workflow_created", "test-correlation-001", map[string]interface{}{
						"rr_name":                   "test-rr",
						"namespace":                 "test-ns",
						"pre_remediation_spec_hash": expectedHash,
					}),
				})
			}))

			querier, err := emclient.NewOgenDataStorageQuerier(server.URL, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())
			hash, err := querier.QueryPreRemediationHash(ctx, "test-correlation-001")
			Expect(err).ToNot(HaveOccurred())
			Expect(hash).To(Equal(expectedHash))
		})

		It("UT-EM-DSQ-002: should return empty string when no events found", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				serveOgenCompliantResponse(w, []map[string]interface{}{})
			}))

			querier, err := emclient.NewOgenDataStorageQuerier(server.URL, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())
			hash, err := querier.QueryPreRemediationHash(ctx, "no-events-correlation")
			Expect(err).ToNot(HaveOccurred())
			Expect(hash).To(BeEmpty())
		})

		It("UT-EM-DSQ-003: should return empty string when event has no hash field", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				serveOgenCompliantResponse(w, []map[string]interface{}{
					testAuditEvent("remediation.workflow_created", "test-correlation-003", map[string]interface{}{
						"rr_name":   "test-rr",
						"namespace": "test-ns",
					}),
				})
			}))

			querier, err := emclient.NewOgenDataStorageQuerier(server.URL, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())
			hash, err := querier.QueryPreRemediationHash(ctx, "test-correlation-003")
			Expect(err).ToNot(HaveOccurred())
			Expect(hash).To(BeEmpty())
		})

		It("UT-EM-DSQ-004: should return error on HTTP 500", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}))

			querier, err := emclient.NewOgenDataStorageQuerier(server.URL, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())
			_, err = querier.QueryPreRemediationHash(ctx, "test-correlation-004")
			Expect(err).To(HaveOccurred())
		})

		It("UT-EM-DSQ-005: should return error when DS is unreachable", func() {
			querier, err := emclient.NewOgenDataStorageQuerier("http://localhost:1", 10*time.Second)
			Expect(err).ToNot(HaveOccurred())
			_, err = querier.QueryPreRemediationHash(ctx, "test-correlation-005")
			Expect(err).To(HaveOccurred())
		})
	})

	// ── HasWorkflowStarted (Issue #575) ──────────────────────

	Context("HasWorkflowStarted", func() {
		It("UT-EM-DSQ-006: should return true when execution.started event exists", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Query().Get("event_type")).To(Equal("workflowexecution.execution.started"))
				serveOgenCompliantResponse(w, []map[string]interface{}{
					testAuditEvent("workflowexecution.execution.started", "corr-started", map[string]interface{}{
						"workflow_id":      "test-wf",
						"workflow_version": "1.0.0",
						"target_resource":  "test-ns/Deployment/test",
						"phase":            "Running",
						"container_image":  "test:latest",
						"execution_name":   "test-exec",
					}),
				})
			}))

			querier, err := emclient.NewOgenDataStorageQuerier(server.URL, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())
			started, err := querier.HasWorkflowStarted(ctx, "corr-started")
			Expect(err).ToNot(HaveOccurred())
			Expect(started).To(BeTrue())
		})

		It("UT-EM-DSQ-007: should return false when no execution.started event exists", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				serveOgenCompliantResponse(w, []map[string]interface{}{})
			}))

			querier, err := emclient.NewOgenDataStorageQuerier(server.URL, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())
			started, err := querier.HasWorkflowStarted(ctx, "corr-not-started")
			Expect(err).ToNot(HaveOccurred())
			Expect(started).To(BeFalse())
		})

		It("UT-EM-DSQ-008: should return error on HTTP 500", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}))

			querier, err := emclient.NewOgenDataStorageQuerier(server.URL, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())
			_, err = querier.HasWorkflowStarted(ctx, "corr-error")
			Expect(err).To(HaveOccurred())
		})

		It("UT-EM-DSQ-009: should return error when DS is unreachable", func() {
			querier, err := emclient.NewOgenDataStorageQuerier("http://localhost:1", 10*time.Second)
			Expect(err).ToNot(HaveOccurred())
			_, err = querier.HasWorkflowStarted(ctx, "corr-unreachable")
			Expect(err).To(HaveOccurred())
		})
	})

	// UT-EM-573-009: HasWorkflowCompleted (ADR-EM-001 section 5)
	Describe("HasWorkflowCompleted (#573)", func() {
		It("UT-EM-573-009: should return true when workflow.completed event exists", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Query().Get("event_type")).To(Equal("workflowexecution.workflow.completed"))
				serveOgenCompliantResponse(w, []map[string]interface{}{
					testAuditEvent("workflowexecution.workflow.completed", "rr-completed", map[string]interface{}{
						"workflow_id":      "test-wf",
						"workflow_version": "1.0.0",
						"target_resource":  "test-ns/Deployment/test",
						"phase":            "Completed",
						"container_image":  "test:latest",
						"execution_name":   "test-exec",
					}),
				})
			}))

			querier, err := emclient.NewOgenDataStorageQuerier(server.URL, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())
			completed, err := querier.HasWorkflowCompleted(ctx, "rr-completed")
			Expect(err).ToNot(HaveOccurred())
			Expect(completed).To(BeTrue(),
				"UT-EM-573-009: should return true when completed event exists")
		})

		It("UT-EM-573-009: should return false when only started event exists", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				serveOgenCompliantResponse(w, []map[string]interface{}{})
			}))

			querier, err := emclient.NewOgenDataStorageQuerier(server.URL, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())
			completed, err := querier.HasWorkflowCompleted(ctx, "rr-only-started")
			Expect(err).ToNot(HaveOccurred())
			Expect(completed).To(BeFalse(),
				"UT-EM-573-009: should return false when no completed event exists")
		})
	})
})
