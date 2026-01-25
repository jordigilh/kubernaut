package datastorage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

var _ = Describe("SOC2 Gap #8: Legal Hold & Retention Integration Tests", func() {
	var (
		correlationID string
		ctx           context.Context
		auditRepo     *repository.AuditEventsRepository
	)

	BeforeEach(func() {
		// CRITICAL: API tests MUST use public schema
		// Rationale: The in-process HTTP API server (testServer) uses public schema,
		// not parallel process schemas. If tests insert/query data in test_process_X
		// schemas, the API won't find the data and tests will fail.

		ctx = context.Background()
		correlationID = "legal-hold-test-" + uuid.New().String()
		// Create audit events repository for legal hold tests
		auditRepo = repository.NewAuditEventsRepository(testDB, logger)
	})

	Describe("BR-AUDIT-006: Legal Hold Enforcement", func() {
		Context("Database-Level Trigger Enforcement", func() {
			It("should prevent deletion of events with legal hold", func() {
				// 1. Create test audit events
				event1 := &repository.AuditEvent{
					EventID:           uuid.New(),
					Version:           "1.0",
					EventType:         "test.legal_hold.created",
					EventCategory:     "gateway",
					EventAction:       "test_action",
					EventOutcome:      "success",
					CorrelationID:     correlationID,
					ResourceType:      "test_resource",
					ResourceID:        "test-123",
					ResourceNamespace: "default",
					ClusterID:         "test-cluster",
					ActorID:           "test-actor",
					ActorType:         "user",
					Severity:          "info",
					RetentionDays:     2555,
					EventData:         map[string]interface{}{"test": "data"},
				}

				createdEvent, err := auditRepo.Create(ctx, event1)
				Expect(err).ToNot(HaveOccurred())
				Expect(createdEvent).ToNot(BeNil())

				// 2. Place legal hold via direct database update (simulating API)
				query := `
					UPDATE audit_events
					SET legal_hold = TRUE,
					    legal_hold_reason = $1,
					    legal_hold_placed_by = $2,
					    legal_hold_placed_at = $3
					WHERE correlation_id = $4
				`
				_, err = testDB.ExecContext(ctx, query,
					"Litigation: Case #2026-GAP8-001",
					"legal-team@company.com",
					time.Now(),
					correlationID,
				)
				Expect(err).ToNot(HaveOccurred())

				// 3. Attempt to delete event with legal hold
				deleteQuery := `DELETE FROM audit_events WHERE correlation_id = $1`
				_, err = testDB.ExecContext(ctx, deleteQuery, correlationID)

				// 4. Verify deletion failed with legal hold error
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Cannot delete audit event with legal hold"))
				Expect(err.Error()).To(ContainSubstring(correlationID))

				// 5. Verify event still exists
				var count int
				err = testDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1", correlationID).Scan(&count)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(1))
			})

			It("should allow deletion after legal hold release", func() {
				// 1. Create test audit event with legal hold
				event1 := &repository.AuditEvent{
					EventID:           uuid.New(),
					Version:           "1.0",
					EventType:         "test.legal_hold.created",
					EventCategory:     "gateway",
					EventAction:       "test_action",
					EventOutcome:      "success",
					CorrelationID:     correlationID,
					ResourceType:      "test_resource",
					ResourceID:        "test-123",
					ResourceNamespace: "default",
					ClusterID:         "test-cluster",
					ActorID:           "test-actor",
					ActorType:         "user",
					Severity:          "info",
					RetentionDays:     2555,
					EventData:         map[string]interface{}{"test": "data"},
					LegalHold:         true,
				}

				_, err := auditRepo.Create(ctx, event1)
				Expect(err).ToNot(HaveOccurred())

				// 2. Release legal hold
				releaseQuery := `UPDATE audit_events SET legal_hold = FALSE WHERE correlation_id = $1`
				_, err = testDB.ExecContext(ctx, releaseQuery, correlationID)
				Expect(err).ToNot(HaveOccurred())

				// 3. Delete event (should succeed now)
				deleteQuery := `DELETE FROM audit_events WHERE correlation_id = $1`
				result, err := testDB.ExecContext(ctx, deleteQuery, correlationID)
				Expect(err).ToNot(HaveOccurred())

				rowsAffected, err := result.RowsAffected()
				Expect(err).ToNot(HaveOccurred())
				Expect(rowsAffected).To(BeNumerically(">", 0))

				// 4. Verify event no longer exists
				var count int
				err = testDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1", correlationID).Scan(&count)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(0))
			})
		})

		Context("POST /api/v1/audit/legal-hold", func() {
			It("should place legal hold on all events with correlation_id", func() {
				// 1. Create 5 audit events with same correlation_id
				for i := 0; i < 5; i++ {
					event := &repository.AuditEvent{
						EventID:           uuid.New(),
						Version:           "1.0",
						EventType:         fmt.Sprintf("test.legal_hold.event_%d", i),
						EventCategory:     "gateway",
						EventAction:       "test_action",
						EventOutcome:      "success",
						CorrelationID:     correlationID,
						ResourceType:      "test_resource",
						ResourceID:        fmt.Sprintf("test-%d", i),
						ResourceNamespace: "default",
						ClusterID:         "test-cluster",
						ActorID:           "test-actor",
						ActorType:         "user",
						Severity:          "info",
						RetentionDays:     2555,
						EventData:         map[string]interface{}{"test": "data"},
					}
					_, err := auditRepo.Create(ctx, event)
					Expect(err).ToNot(HaveOccurred())
				}

				// 2. Place legal hold via API
				requestBody := map[string]interface{}{
					"correlation_id": correlationID,
					"reason":         "Litigation: Case #2026-GAP8-002",
				}
				bodyBytes, err := json.Marshal(requestBody)
				Expect(err).ToNot(HaveOccurred())

				req, err := http.NewRequest("POST", dataStorageURL+"/api/v1/audit/legal-hold", bytes.NewBuffer(bodyBytes))
				Expect(err).ToNot(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Auth-Request-User", "legal-team@company.com")

				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// 3. Verify response (print body for debugging if not 200)
				if resp.StatusCode != http.StatusOK {
					bodyBytes, _ := io.ReadAll(resp.Body)
					GinkgoWriter.Printf("❌ Unexpected status %d, response body: %s\n", resp.StatusCode, string(bodyBytes))
					resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Reset body for further reading
				}
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred())
				Expect(response["correlation_id"]).To(Equal(correlationID))
				Expect(response["events_affected"]).To(BeNumerically("==", 5))
				Expect(response["placed_by"]).To(Equal("legal-team@company.com"))

				// 4. Query database to verify legal hold is set
				var count int
				query := `SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1 AND legal_hold = TRUE`
				err = testDB.QueryRowContext(ctx, query, correlationID).Scan(&count)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(5))

				// 5. Verify legal hold metadata
				var placedBy, reason string
				metadataQuery := `
					SELECT legal_hold_placed_by, legal_hold_reason
					FROM audit_events
					WHERE correlation_id = $1
					LIMIT 1
				`
				err = testDB.QueryRowContext(ctx, metadataQuery, correlationID).Scan(&placedBy, &reason)
				Expect(err).ToNot(HaveOccurred())
				Expect(placedBy).To(Equal("legal-team@company.com"))
				Expect(reason).To(Equal("Litigation: Case #2026-GAP8-002"))
			})

			It("should return 400 if correlation_id not found", func() {
				// 1. Attempt to place legal hold on non-existent correlation_id
				requestBody := map[string]interface{}{
					"correlation_id": "non-existent-correlation-id",
					"reason":         "Test: Non-existent",
				}
				bodyBytes, err := json.Marshal(requestBody)
				Expect(err).ToNot(HaveOccurred())

				req, err := http.NewRequest("POST", dataStorageURL+"/api/v1/audit/legal-hold", bytes.NewBuffer(bodyBytes))
				Expect(err).ToNot(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Auth-Request-User", "legal-team@company.com")

				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// 2. Expect 400 Bad Request or 404 Not Found
				Expect(resp.StatusCode).To(Or(Equal(http.StatusBadRequest), Equal(http.StatusNotFound)))
			})

			It("should capture X-Auth-Request-User in placed_by field", func() {
				// 1. Create test event
				event := &repository.AuditEvent{
					EventID:           uuid.New(),
					Version:           "1.0",
					EventType:         "test.legal_hold.user_capture",
					EventCategory:     "gateway",
					EventAction:       "test_action",
					EventOutcome:      "success",
					CorrelationID:     correlationID,
					ResourceType:      "test_resource",
					ResourceID:        "test-123",
					ResourceNamespace: "default",
					ClusterID:         "test-cluster",
					ActorID:           "test-actor",
					ActorType:         "user",
					Severity:          "info",
					RetentionDays:     2555,
					EventData:         map[string]interface{}{"test": "data"},
				}
				_, err := auditRepo.Create(ctx, event)
				Expect(err).ToNot(HaveOccurred())

				// 2. Place legal hold with specific X-Auth-Request-User
				requestBody := map[string]interface{}{
					"correlation_id": correlationID,
					"reason":         "Test: User ID Capture",
				}
				bodyBytes, err := json.Marshal(requestBody)
				Expect(err).ToNot(HaveOccurred())

				req, err := http.NewRequest("POST", dataStorageURL+"/api/v1/audit/legal-hold", bytes.NewBuffer(bodyBytes))
				Expect(err).ToNot(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Auth-Request-User", "compliance-officer@company.com")

				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				// 3. Verify X-Auth-Request-User was captured in database
				var placedBy string
				query := `SELECT legal_hold_placed_by FROM audit_events WHERE correlation_id = $1`
				err = testDB.QueryRowContext(ctx, query, correlationID).Scan(&placedBy)
				Expect(err).ToNot(HaveOccurred())
				Expect(placedBy).To(Equal("compliance-officer@company.com"))
			})
		})

		Context("DELETE /api/v1/audit/legal-hold/{correlation_id}", func() {
			It("should release legal hold on all events", func() {
				// 1. Create events with legal hold
				for i := 0; i < 3; i++ {
					event := &repository.AuditEvent{
						EventID:           uuid.New(),
						Version:           "1.0",
						EventType:         fmt.Sprintf("test.legal_hold.release_%d", i),
						EventCategory:     "gateway",
						EventAction:       "test_action",
						EventOutcome:      "success",
						CorrelationID:     correlationID,
						ResourceType:      "test_resource",
						ResourceID:        fmt.Sprintf("test-%d", i),
						ResourceNamespace: "default",
						ClusterID:         "test-cluster",
						ActorID:           "test-actor",
						ActorType:         "user",
						Severity:          "info",
						RetentionDays:     2555,
						EventData:         map[string]interface{}{"test": "data"},
						LegalHold:         true,
					}
					_, err := auditRepo.Create(ctx, event)
					Expect(err).ToNot(HaveOccurred())
				}

				// 2. Release legal hold via API
				requestBody := map[string]interface{}{
					"release_reason": "Case settled",
				}
				bodyBytes, err := json.Marshal(requestBody)
				Expect(err).ToNot(HaveOccurred())

				req, err := http.NewRequest("DELETE", dataStorageURL+"/api/v1/audit/legal-hold/"+correlationID, bytes.NewBuffer(bodyBytes))
				Expect(err).ToNot(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Auth-Request-User", "legal-team@company.com")

				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// 3. Verify response
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred())
				Expect(response["correlation_id"]).To(Equal(correlationID))
				Expect(response["events_released"]).To(BeNumerically("==", 3))
				Expect(response["released_by"]).To(Equal("legal-team@company.com"))

				// 4. Verify legal hold is released in database
				var count int
				query := `SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1 AND legal_hold = FALSE`
				err = testDB.QueryRowContext(ctx, query, correlationID).Scan(&count)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(3))
			})
		})

		Context("GET /api/v1/audit/legal-hold", func() {
			It("should list all active legal holds", func() {
				// 1. Place holds on 3 different correlation_ids
				correlationIDs := []string{
					"legal-hold-list-1-" + uuid.New().String(),
					"legal-hold-list-2-" + uuid.New().String(),
					"legal-hold-list-3-" + uuid.New().String(),
				}

				for _, corrID := range correlationIDs {
					event := &repository.AuditEvent{
						EventID:           uuid.New(),
						Version:           "1.0",
						EventType:         "test.legal_hold.list",
						EventCategory:     "gateway",
						EventAction:       "test_action",
						EventOutcome:      "success",
						CorrelationID:     corrID,
						ResourceType:      "test_resource",
						ResourceID:        "test-123",
						ResourceNamespace: "default",
						ClusterID:         "test-cluster",
						ActorID:           "test-actor",
						ActorType:         "user",
						Severity:          "info",
						RetentionDays:     2555,
						EventData:         map[string]interface{}{"test": "data"},
						LegalHold:         true,
						LegalHoldReason:   fmt.Sprintf("Litigation: Case #%s", corrID),
						LegalHoldPlacedBy: "legal-team@company.com",
					}
					_, err := auditRepo.Create(ctx, event)
					Expect(err).ToNot(HaveOccurred())
				}

				// 2. List legal holds via API
				req, err := http.NewRequest("GET", dataStorageURL+"/api/v1/audit/legal-hold", nil)
				Expect(err).ToNot(HaveOccurred())
				req.Header.Set("X-Auth-Request-User", "legal-team@company.com")

				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// 3. Verify response
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred())

				holds, ok := response["holds"].([]interface{})
				Expect(ok).To(BeTrue())
				Expect(len(holds)).To(BeNumerically(">=", 3))

				// Verify our test holds are present
				foundCount := 0
				for _, hold := range holds {
					holdMap := hold.(map[string]interface{})
					corrID := holdMap["correlation_id"].(string)
					for _, testCorrID := range correlationIDs {
						if corrID == testCorrID {
							foundCount++
							Expect(holdMap["placed_by"]).To(Equal("legal-team@company.com"))
							break
						}
					}
				}
				Expect(foundCount).To(Equal(3))
			})
		})
	})
})
