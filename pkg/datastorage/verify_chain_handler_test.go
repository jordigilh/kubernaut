/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.

BR-STORAGE-033 / BR-AUDIT-007 / BR-STORAGE-024: SOC2 Gap #9 handler contracts.
Authority: UT-DS-1088-P9-* (audit verify-chain & export handlers).
*/

package datastorage_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/cert"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

func rfc7807TypeSubstring(body []byte) string {
	var problem map[string]any
	if err := json.Unmarshal(body, &problem); err != nil {
		return ""
	}
	raw, ok := problem["type"].(string)
	if !ok {
		return ""
	}
	return raw
}

var _ = Describe("Audit HTTP handlers — verify-chain & export (UT-DS-1088-P9)", func() {
	var testLogger logr.Logger

	BeforeEach(func() {
		// testr.New requires *testing.T; kubelog matches other test/unit/datastorage suites.
		testLogger = kubelog.NewLogger(kubelog.DefaultOptions())
	})

	Context("HandleVerifyChain", func() {
		verifyURL := "/api/v1/audit/verify-chain"

		It("UT-DS-1088-P9-001: POST with empty body returns 400", func() {
			srv := server.NewMinimalAuditHandlersHTTPServer(server.MinimalAuditHandlersHTTPServerDeps{
				Logger: testLogger,
			})
			req := httptest.NewRequest(http.MethodPost, verifyURL, strings.NewReader(""))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			srv.HandleVerifyChain(rec, req)

			Expect(rec.Code).To(Equal(http.StatusBadRequest))
			Expect(rfc7807TypeSubstring(rec.Body.Bytes())).To(ContainSubstring("invalid-request-body"))
		})

		It("UT-DS-1088-P9-002: POST with missing correlation_id returns 400 with missing-correlation-id", func() {
			srv := server.NewMinimalAuditHandlersHTTPServer(server.MinimalAuditHandlersHTTPServerDeps{
				Logger: testLogger,
			})
			req := httptest.NewRequest(http.MethodPost, verifyURL, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			srv.HandleVerifyChain(rec, req)

			Expect(rec.Code).To(Equal(http.StatusBadRequest))
			Expect(rfc7807TypeSubstring(rec.Body.Bytes())).To(ContainSubstring("missing-correlation-id"))
		})

		It("UT-DS-1088-P9-003: POST with oversized correlation_id (257 chars) returns 400 with invalid-correlation-id", func() {
			srv := server.NewMinimalAuditHandlersHTTPServer(server.MinimalAuditHandlersHTTPServerDeps{
				Logger: testLogger,
			})
			longID := strings.Repeat("x", 257)
			body := fmt.Sprintf(`{"correlation_id":"%s"}`, longID)
			req := httptest.NewRequest(http.MethodPost, verifyURL, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			srv.HandleVerifyChain(rec, req)

			Expect(rec.Code).To(Equal(http.StatusBadRequest))
			Expect(rfc7807TypeSubstring(rec.Body.Bytes())).To(ContainSubstring("invalid-correlation-id"))
		})

		It("UT-DS-1088-P9-004: GET returns 405 Method Not Allowed", func() {
			srv := server.NewMinimalAuditHandlersHTTPServer(server.MinimalAuditHandlersHTTPServerDeps{
				Logger: testLogger,
			})
			req := httptest.NewRequest(http.MethodGet, verifyURL, nil)
			rec := httptest.NewRecorder()

			srv.HandleVerifyChain(rec, req)

			Expect(rec.Code).To(Equal(http.StatusMethodNotAllowed))
			Expect(rfc7807TypeSubstring(rec.Body.Bytes())).To(ContainSubstring("method-not-allowed"))
		})

		It("UT-DS-1088-P9-005: POST with valid correlation_id passes validation then fails verification (RFC7807)", func() {
			mockDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = mockDB.Close() }()

			corr := "corr-accepted-past-validation"
			mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_events WHERE correlation_id`).
				WithArgs(corr).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			mock.ExpectQuery(`FROM audit_events`).
				WithArgs(corr, maxVerifyChainQueryLimit()).WillReturnError(errors.New("expected verify-chain DB failure"))

			srv := server.NewMinimalAuditHandlersHTTPServer(server.MinimalAuditHandlersHTTPServerDeps{
				Logger: testLogger,
				DB:     mockDB,
			})
			body := fmt.Sprintf(`{"correlation_id":"%s"}`, corr)
			req := httptest.NewRequest(http.MethodPost, verifyURL, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			srv.HandleVerifyChain(rec, req)

			Expect(rec.Code).To(Equal(http.StatusInternalServerError))
			Expect(rfc7807TypeSubstring(rec.Body.Bytes())).To(ContainSubstring("verify-chain/internal-error"))

			Expect(mock.ExpectationsWereMet()).To(Succeed())
		})
	})

	Context("HandleExportAuditEvents", func() {
		exportURL := "/api/v1/audit/export"

		It("UT-DS-1088-P9-010: GET without X-Auth-Request-User returns 401", func() {
			srv := server.NewMinimalAuditHandlersHTTPServer(server.MinimalAuditHandlersHTTPServerDeps{
				Logger: testLogger,
			})
			req := httptest.NewRequest(http.MethodGet, exportURL, nil)
			rec := httptest.NewRecorder()

			srv.HandleExportAuditEvents(rec, req)

			Expect(rec.Code).To(Equal(http.StatusUnauthorized))
			Expect(rfc7807TypeSubstring(rec.Body.Bytes())).To(ContainSubstring("export/unauthorized"))
		})

		It("UT-DS-1088-P9-011: POST returns 405 Method Not Allowed", func() {
			srv := server.NewMinimalAuditHandlersHTTPServer(server.MinimalAuditHandlersHTTPServerDeps{
				Logger: testLogger,
			})
			req := httptest.NewRequest(http.MethodPost, exportURL, nil)
			req.Header.Set("X-Auth-Request-User", "alice")
			rec := httptest.NewRecorder()

			srv.HandleExportAuditEvents(rec, req)

			Expect(rec.Code).To(Equal(http.StatusMethodNotAllowed))
			Expect(rfc7807TypeSubstring(rec.Body.Bytes())).To(ContainSubstring("method-not-allowed"))
		})

		It("UT-DS-1088-P9-012: GET with X-Auth-Request-User but limit > 10000 returns 413", func() {
			srv := server.NewMinimalAuditHandlersHTTPServer(server.MinimalAuditHandlersHTTPServerDeps{
				Logger: testLogger,
			})
			req := httptest.NewRequest(http.MethodGet,
				exportURL+"?limit=10001",
				nil,
			)
			req.Header.Set("X-Auth-Request-User", "alice")
			rec := httptest.NewRecorder()

			srv.HandleExportAuditEvents(rec, req)

			Expect(rec.Code).To(Equal(http.StatusRequestEntityTooLarge))
			Expect(rfc7807TypeSubstring(rec.Body.Bytes())).To(ContainSubstring("export/limit-exceeded"))
		})

		It("UT-DS-1088-P9-013: GET with invalid start_time format returns 400", func() {
			srv := server.NewMinimalAuditHandlersHTTPServer(server.MinimalAuditHandlersHTTPServerDeps{
				Logger: testLogger,
			})
			req := httptest.NewRequest(http.MethodGet,
				exportURL+"?start_time=not-rfc3339",
				nil,
			)
			req.Header.Set("X-Auth-Request-User", "alice")
			rec := httptest.NewRecorder()

			srv.HandleExportAuditEvents(rec, req)

			Expect(rec.Code).To(Equal(http.StatusBadRequest))
			Expect(rfc7807TypeSubstring(rec.Body.Bytes())).To(ContainSubstring("export/invalid-parameters"))
			Expect(rec.Body.String()).To(ContainSubstring("invalid query parameters"))
		})

		It("UT-DS-1088-P9-014: GET with negative offset returns 400", func() {
			srv := server.NewMinimalAuditHandlersHTTPServer(server.MinimalAuditHandlersHTTPServerDeps{
				Logger: testLogger,
			})
			req := httptest.NewRequest(http.MethodGet,
				exportURL+"?offset=-5",
				nil,
			)
			req.Header.Set("X-Auth-Request-User", "alice")
			rec := httptest.NewRecorder()

			srv.HandleExportAuditEvents(rec, req)

			Expect(rec.Code).To(Equal(http.StatusBadRequest))
			Expect(rfc7807TypeSubstring(rec.Body.Bytes())).To(ContainSubstring("export/invalid-parameters"))
			Expect(rec.Body.String()).To(ContainSubstring("invalid query parameters"))
		})

		It("UT-DS-1088-P9-015: indirect parseExportFilters — export query reaches repository with decoded filters", func() {
			mockDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = mockDB.Close() }()

			startParsed := mustRFC3339("2026-01-08T01:02:03Z")
			endParsed := mustRFC3339("2026-01-09T09:09:09Z")
			corrID := "chain-corr"
			category := "security"
			limit := 99
			offset := 15

			exportCols := []string{
				"event_id", "event_version", "event_type", "event_timestamp",
				"event_category", "event_action", "event_outcome", "correlation_id",
				"parent_event_id", "parent_event_date", "resource_type", "resource_id",
				"namespace", "cluster_id", "actor_id", "actor_type",
				"severity", "duration_ms", "error_code", "error_message",
				"retention_days", "is_sensitive", "event_data",
				"event_hash", "previous_event_hash", "legal_hold",
			}
			mock.ExpectQuery(`FROM audit_events`).
				WithArgs(startParsed, endParsed, corrID, category, limit, offset).
				WillReturnRows(sqlmock.NewRows(exportCols))

			pair, err := cert.GenerateSelfSigned(cert.CertificateOptions{
				CommonName:       "ut-audit-export-parse",
				Organization:     "Kubernaut",
				DNSNames:         []string{"localhost"},
				ValidityDuration: time.Hour,
				KeySize:          2048,
			})
			Expect(err).ToNot(HaveOccurred())
			signerInst, err := cert.NewSignerFromPEM(pair.CertPEM, pair.KeyPEM)
			Expect(err).ToNot(HaveOccurred())

			repo := repository.NewAuditEventsRepository(mockDB, testLogger)

			srv := server.NewMinimalAuditHandlersHTTPServer(server.MinimalAuditHandlersHTTPServerDeps{
				Logger:          testLogger,
				DB:              mockDB,
				AuditEventsRepo: repo,
				Signer:          signerInst,
			})

			q := exportURL + "?start_time=2026-01-08T01:02:03Z" +
				"&end_time=2026-01-09T09:09:09Z" +
				"&correlation_id=" + corrID +
				"&event_category=" + category +
				fmt.Sprintf("&limit=%d&offset=%d&redact_pii=true", limit, offset)

			req := httptest.NewRequest(http.MethodGet, q, nil)
			req.Header.Set("X-Auth-Request-User", "bob-principal")
			rec := httptest.NewRecorder()

			srv.HandleExportAuditEvents(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(mock.ExpectationsWereMet()).To(Succeed())

			var envelope map[string]any
			Expect(json.Unmarshal(rec.Body.Bytes(), &envelope)).To(Succeed())
			meta := envelope["export_metadata"].(map[string]any)
			qfilters := meta["query_filters"].(map[string]any)
			Expect(fmt.Sprint(qfilters["limit"])).To(Equal(fmt.Sprint(float64(limit))))
			Expect(fmt.Sprint(qfilters["offset"])).To(Equal(fmt.Sprint(float64(offset))))
			Expect(fmt.Sprint(qfilters["correlation_id"])).To(Equal(corrID))
			Expect(fmt.Sprint(qfilters["event_category"])).To(Equal(category))
			Expect(fmt.Sprint(qfilters["start_time"])).To(ContainSubstring("2026-01-08T01:02:03"))
			Expect(fmt.Sprint(qfilters["end_time"])).To(ContainSubstring("2026-01-09T09:09:09"))

			expBy, ok := meta["exported_by"].(string)
			Expect(ok).To(BeTrue(), "exported_by should echo the authenticated caller")
			Expect(expBy).To(Equal("bob-principal"))

			sig, ok := meta["signature"].(string)
			Expect(ok).To(BeTrue())
			Expect(len(sig)).To(BeNumerically(">", 32))
		})
	})
})

func mustRFC3339(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	Expect(err).ToNot(HaveOccurred())
	return t
}

// maxVerifyChainQueryLimit returns the production LIMIT binding (MaxVerifyChainEvents+1).
func maxVerifyChainQueryLimit() int { return server.MaxVerifyChainEvents + 1 }
