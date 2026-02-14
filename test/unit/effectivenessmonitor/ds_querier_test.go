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
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
)

// ========================================
// DataStorageQuerier Tests (DD-EM-002, Phase 5)
//
// Contract: DataStorageQuerier.QueryPreRemediationHash(ctx, correlationID)
//   - Queries DS audit events for correlation_id
//   - Finds remediation.workflow_created event
//   - Extracts pre_remediation_spec_hash from event_data
//   - Returns empty string when no events found
//   - Returns empty string when event has no hash field
//   - Returns error on HTTP 500 or network errors
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

	It("UT-EM-DSQ-001: should retrieve pre_remediation_spec_hash from remediation.workflow_created event", func() {
		expectedHash := "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Return audit events with a remediation.workflow_created event containing the hash
			events := []map[string]interface{}{
				{
					"event_type":    "remediation.workflow_created",
					"correlation_id": "test-correlation-001",
					"event_data": map[string]interface{}{
						"event_type":                  "remediation.workflow_created",
						"pre_remediation_spec_hash":   expectedHash,
						"target_resource":             "default/Deployment/nginx",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(events)
		}))

		querier := emclient.NewDataStorageHTTPQuerier(server.URL)
		hash, err := querier.QueryPreRemediationHash(ctx, "test-correlation-001")
		Expect(err).ToNot(HaveOccurred())
		Expect(hash).To(Equal(expectedHash))
	})

	It("UT-EM-DSQ-002: should return empty string when no events found", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{})
		}))

		querier := emclient.NewDataStorageHTTPQuerier(server.URL)
		hash, err := querier.QueryPreRemediationHash(ctx, "no-events-correlation")
		Expect(err).ToNot(HaveOccurred())
		Expect(hash).To(BeEmpty())
	})

	It("UT-EM-DSQ-003: should return empty string when event has no hash field", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			events := []map[string]interface{}{
				{
					"event_type":    "remediation.workflow_created",
					"correlation_id": "test-correlation-003",
					"event_data": map[string]interface{}{
						"event_type": "remediation.workflow_created",
						// no pre_remediation_spec_hash field
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(events)
		}))

		querier := emclient.NewDataStorageHTTPQuerier(server.URL)
		hash, err := querier.QueryPreRemediationHash(ctx, "test-correlation-003")
		Expect(err).ToNot(HaveOccurred())
		Expect(hash).To(BeEmpty())
	})

	It("UT-EM-DSQ-004: should return error on HTTP 500", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, "internal server error")
		}))

		querier := emclient.NewDataStorageHTTPQuerier(server.URL)
		_, err := querier.QueryPreRemediationHash(ctx, "test-correlation-004")
		Expect(err).To(HaveOccurred())
	})

	It("UT-EM-DSQ-005: should return error when DS is unreachable", func() {
		querier := emclient.NewDataStorageHTTPQuerier("http://localhost:1") // unreachable port
		_, err := querier.QueryPreRemediationHash(ctx, "test-correlation-005")
		Expect(err).To(HaveOccurred())
	})
})
