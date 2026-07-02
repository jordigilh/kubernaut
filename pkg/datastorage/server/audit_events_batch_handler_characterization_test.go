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

package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	dsmetrics "github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

// ========================================
// CHARACTERIZATION TESTS: (*Server).handleCreateAuditEventsBatch
// 📋 Business Requirement: BR-AUDIT-001 (Complete audit trail, no data loss)
// 📋 Design Decision: DD-AUDIT-002 (StoreBatch interface must accept arrays)
// 📋 Compliance: SOC2 CC8.1, AU-2/AU-3 (audit event completeness/content)
//
// Written per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3 (Phase 6, DataStorage)
// coverage-before-refactor mandate: handleCreateAuditEventsBatch (cyclomatic 24)
// previously had only integration-level coverage (test/integration/datastorage/
// audit_events_batch_write_api_test.go, batch_size_limit_test.go,
// dlq_fallback_http_test.go). These tests invoke the handler directly (in-process,
// no HTTP transport) with a sqlmock-backed repository and a miniredis-backed DLQ
// client, pinning down its request-parsing, validation, and DB/DLQ fallback
// control flow so the planned decomposition is provably behavior-preserving.
// ========================================
var _ = Describe("(*Server).handleCreateAuditEventsBatch", func() {
	var (
		mockDB      *sql.DB
		mock        sqlmock.Sqlmock
		logger      logr.Logger
		srv         *Server
		miniRedis   *miniredis.Miniredis
		redisClient *redis.Client
	)

	BeforeEach(func() {
		var err error
		mockDB, mock, err = sqlmock.New()
		Expect(err).ToNot(HaveOccurred())

		logger = kubelog.NewLogger(kubelog.DefaultOptions())

		miniRedis = miniredis.RunT(GinkgoT())
		redisClient = redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
		dlqClient, dlqErr := dlq.NewClient(redisClient, logger, 1000)
		Expect(dlqErr).ToNot(HaveOccurred())

		srv = &Server{
			logger:          logger,
			db:              mockDB,
			auditEventsRepo: repository.NewAuditEventsRepository(mockDB, logger),
			dlqClient:       dlqClient,
			metrics:         dsmetrics.NewMetrics("", ""),
			maxBatchSize:    500,
		}
	})

	AfterEach(func() {
		_ = mockDB.Close()
		_ = redisClient.Close()
		miniRedis.Close()
	})

	postBatch := func(body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/events/batch", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		srv.handleCreateAuditEventsBatch(rr, req)
		return rr
	}

	validEvent := func(correlationID string) map[string]interface{} {
		return map[string]interface{}{
			"version":         "1.0",
			"event_type":      "gateway.signal.received",
			"event_category":  "gateway",
			"event_action":    "signal_received",
			"event_outcome":   "success",
			"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			"correlation_id":  correlationID,
			"event_data": map[string]interface{}{
				"event_type":    "gateway.signal.received",
				"signal_name":   "test-signal",
				"signal_type":   "alert",
				"fingerprint":   "fp-" + correlationID,
				"resource_kind": "Deployment",
				"resource_name": "test-deploy",
				"namespace":     "default",
			},
		}
	}

	expectSuccessfulCreateBatch := func(correlationID string) {
		mock.ExpectBegin()
		mock.ExpectPrepare("INSERT INTO audit_events")
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(correlationID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT event_hash").WithArgs(correlationID).WillReturnRows(sqlmock.NewRows([]string{"event_hash"}))
		mock.ExpectQuery("INSERT INTO audit_events").
			WillReturnRows(sqlmock.NewRows([]string{"event_timestamp"}).AddRow(time.Now()))
		mock.ExpectCommit()
	}

	Context("request parsing", func() {
		It("returns 400 when the payload is a single object instead of a JSON array", func() {
			body, err := json.Marshal(validEvent("single-object"))
			Expect(err).ToNot(HaveOccurred())

			rr := postBatch(body)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			Expect(rr.Body.String()).To(ContainSubstring("must be a JSON array"))
		})

		It("returns 400 when the payload is not valid JSON", func() {
			rr := postBatch([]byte("not json"))

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			Expect(rr.Body.String()).To(ContainSubstring("valid JSON array"))
		})

		It("returns 400 when the batch is an empty array", func() {
			rr := postBatch([]byte(`[]`))

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			Expect(rr.Body.String()).To(ContainSubstring("batch cannot be empty"))
		})

		It("returns 400 when the batch exceeds maxBatchSize", func() {
			srv.maxBatchSize = 1
			batch := []map[string]interface{}{validEvent("over-limit-1"), validEvent("over-limit-2")}
			body, err := json.Marshal(batch)
			Expect(err).ToNot(HaveOccurred())

			rr := postBatch(body)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			Expect(rr.Body.String()).To(ContainSubstring("batch-size-exceeded"))
		})
	})

	Context("per-event validation (atomic batch: fail before any persistence)", func() {
		It("returns 400 without touching the database when an event has an invalid timestamp", func() {
			event := validEvent("bad-timestamp")
			event["event_timestamp"] = "not-a-timestamp"
			body, err := json.Marshal([]map[string]interface{}{event})
			Expect(err).ToNot(HaveOccurred())

			rr := postBatch(body)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			Expect(mock.ExpectationsWereMet()).To(Succeed(), "no DB interaction expected before validation passes")
		})
	})

	Context("successful persistence", func() {
		It("returns 201 with one event_id per created event", func() {
			expectSuccessfulCreateBatch("batch-success")
			body, err := json.Marshal([]map[string]interface{}{validEvent("batch-success")})
			Expect(err).ToNot(HaveOccurred())

			rr := postBatch(body)

			Expect(rr.Code).To(Equal(http.StatusCreated))
			var resp BatchAuditEventCreatedResponse
			Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp.EventIDs).To(HaveLen(1))
			Expect(mock.ExpectationsWereMet()).To(Succeed())
		})
	})

	Context("database failure with DLQ fallback (DD-009)", func() {
		It("returns 202 accepted when the DB write fails but DLQ enqueue succeeds", func() {
			mock.ExpectBegin().WillReturnError(fmt.Errorf("simulated db outage"))
			body, err := json.Marshal([]map[string]interface{}{validEvent("dlq-fallback")})
			Expect(err).ToNot(HaveOccurred())

			rr := postBatch(body)

			Expect(rr.Code).To(Equal(http.StatusAccepted))
			var resp BatchAuditEventAcceptedResponse
			Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp.Status).To(Equal("accepted"))
			Expect(resp.Count).To(Equal(1))
		})

		It("returns 500 when the DB write fails and DLQ enqueue also fails", func() {
			mock.ExpectBegin().WillReturnError(fmt.Errorf("simulated db outage"))
			// Force DLQ enqueue to fail by closing the redis connection before the handler call.
			miniRedis.Close()

			body, err := json.Marshal([]map[string]interface{}{validEvent("dlq-double-failure")})
			Expect(err).ToNot(HaveOccurred())

			rr := postBatch(body)

			Expect(rr.Code).To(Equal(http.StatusInternalServerError))
			Expect(rr.Body.String()).To(ContainSubstring("database-error"))
		})
	})
})
