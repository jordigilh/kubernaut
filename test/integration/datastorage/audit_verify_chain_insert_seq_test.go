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

package datastorage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

// Issue #2318: POST /api/v1/audit/verify-chain must not report tampering for
// a genuinely intact hash chain written as a same-timestamp burst. Exercises
// the real write path (AuditEventsRepository.Create, migration 019's
// insert_seq) end-to-end through the real HTTP handler against real
// PostgreSQL, proving the write-path fix (insert_seq-ordered predecessor
// lookup) and the read-path fix (order-independent verifyEventChain) work
// together.
var _ = Describe("POST /api/v1/audit/verify-chain same-timestamp burst [Issue #2318]", func() {
	var (
		auditRepo *repository.AuditEventsRepository
		testID    string
	)

	BeforeEach(func() {
		auditRepo = repository.NewAuditEventsRepository(db.DB, logger)
		testID = generateTestID()
	})

	AfterEach(func() {
		_, _ = db.ExecContext(context.Background(),
			"DELETE FROM audit_events WHERE correlation_id LIKE $1",
			fmt.Sprintf("%%verify-burst-%s%%", testID))
	})

	It("IT-DS-2318-002: a same-timestamp write burst verifies as valid with zero tampered events", func() {
		corrID := fmt.Sprintf("verify-burst-%s", testID)
		sharedTimestamp := time.Now().UTC().Truncate(time.Second)

		// Deliberately give each event a UUID that sorts in the OPPOSITE
		// order from its true insertion order, so a false positive here can
		// only be explained by an ordering defect (event_id tiebreak),
		// never by coincidence.
		ids := []uuid.UUID{
			uuid.MustParse("ffffffff-ffff-ffff-ffff-fffffffffff3"),
			uuid.MustParse("ffffffff-ffff-ffff-ffff-fffffffffff2"),
			uuid.MustParse("ffffffff-ffff-ffff-ffff-fffffffffff1"),
			uuid.MustParse("ffffffff-ffff-ffff-ffff-fffffffffff0"),
		}
		for i, id := range ids {
			event := &repository.AuditEvent{
				EventID:        id,
				EventTimestamp: sharedTimestamp,
				EventType:      "test.verify_burst",
				Version:        "1.0",
				EventCategory:  "test",
				EventAction:    "verify",
				EventOutcome:   "success",
				CorrelationID:  corrID,
				ResourceType:   "test-resource",
				ResourceID:     fmt.Sprintf("res-%d", i),
				ActorID:        "test-actor",
				ActorType:      "service",
				RetentionDays:  30,
				EventData:      map[string]interface{}{"index": i},
			}
			_, err := auditRepo.Create(context.Background(), event)
			Expect(err).ToNot(HaveOccurred())
		}

		srv := server.NewMinimalAuditHandlersHTTPServer(server.MinimalAuditHandlersHTTPServerDeps{
			Logger:          logger,
			DB:              db.DB,
			AuditEventsRepo: auditRepo,
		})

		reqBody, err := json.Marshal(map[string]string{"correlation_id": corrID})
		Expect(err).ToNot(HaveOccurred())
		req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/verify-chain", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		srv.HandleVerifyChain(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))

		var resp server.VerifyChainResponse
		Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())

		Expect(resp.IsValid).To(BeTrue(),
			"a genuinely intact hash chain written in a same-timestamp burst must verify as valid (Issue #2318)")
		Expect(resp.TotalEvents).To(Equal(4))
		Expect(resp.VerifiedEvents).To(Equal(4))
		Expect(resp.TamperedEvents).To(BeEmpty())
	})
})
