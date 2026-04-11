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
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sync/errgroup"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// Issue #667: BR-STORAGE-040 — Concurrent batch writes must not deadlock
var _ = Describe("CreateBatch Lock Ordering [BR-STORAGE-040]", func() {
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
			fmt.Sprintf("%%lock-order-%s%%", testID))
	})

	It("IT-DS-040-001: concurrent CreateBatch calls with overlapping correlation IDs complete without deadlock", func() {
		corrA := fmt.Sprintf("lock-order-%s-corr-a", testID)
		corrB := fmt.Sprintf("lock-order-%s-corr-b", testID)

		makeBatch := func(suffix string) []*repository.AuditEvent {
			events := make([]*repository.AuditEvent, 0, 10)
			for i := 0; i < 5; i++ {
				events = append(events, &repository.AuditEvent{
					EventID:       uuid.New(),
					EventType:     "test.lock_ordering",
					Version:       "1.0",
					EventCategory: "test",
					EventAction:   "verify",
					EventOutcome:  "success",
					CorrelationID: corrA,
					ResourceType:  "test-resource",
					ResourceID:    fmt.Sprintf("res-%s-%d", suffix, i),
					ActorID:       "test-actor",
					ActorType:     "system",
					RetentionDays: 30,
					EventData:     map[string]interface{}{"batch": suffix, "index": i},
				})
				events = append(events, &repository.AuditEvent{
					EventID:       uuid.New(),
					EventType:     "test.lock_ordering",
					Version:       "1.0",
					EventCategory: "test",
					EventAction:   "verify",
					EventOutcome:  "success",
					CorrelationID: corrB,
					ResourceType:  "test-resource",
					ResourceID:    fmt.Sprintf("res-%s-%d", suffix, i),
					ActorID:       "test-actor",
					ActorType:     "system",
					RetentionDays: 30,
					EventData:     map[string]interface{}{"batch": suffix, "index": i},
				})
			}
			return events
		}

		batchA := makeBatch("A")
		batchB := makeBatch("B")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		g, gCtx := errgroup.WithContext(ctx)
		_ = gCtx

		g.Go(func() error {
			_, err := auditRepo.CreateBatch(ctx, batchA)
			return err
		})
		g.Go(func() error {
			_, err := auditRepo.CreateBatch(ctx, batchB)
			return err
		})

		err := g.Wait()
		Expect(err).ToNot(HaveOccurred(),
			"both concurrent CreateBatch calls must complete without deadlock (40P01)")

		var count int
		err = db.QueryRowContext(context.Background(),
			"SELECT COUNT(*) FROM audit_events WHERE correlation_id IN ($1, $2)",
			corrA, corrB).Scan(&count)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(Equal(20),
			"all 20 events (10 per batch) must be persisted")
	})

	It("IT-DS-040-002: CreateBatch with single correlation ID produces correct hash chain", func() {
		corrID := fmt.Sprintf("lock-order-%s-single", testID)

		events := make([]*repository.AuditEvent, 3)
		for i := 0; i < 3; i++ {
			events[i] = &repository.AuditEvent{
				EventID:       uuid.New(),
				EventType:     "test.hash_chain",
				Version:       "1.0",
				EventCategory: "test",
				EventAction:   "verify",
				EventOutcome:  "success",
				CorrelationID: corrID,
				ResourceType:  "test-resource",
				ResourceID:    fmt.Sprintf("res-%d", i),
				ActorID:       "test-actor",
				ActorType:     "system",
				RetentionDays: 30,
				EventData:     map[string]interface{}{"index": i},
			}
		}

		created, err := auditRepo.CreateBatch(context.Background(), events)
		Expect(err).ToNot(HaveOccurred())
		Expect(created).To(HaveLen(3), "all 3 events must be created")

		for i, evt := range created {
			Expect(evt.EventHash).ToNot(BeEmpty(),
				fmt.Sprintf("event %d must have a computed hash", i))
		}

		filters := repository.ExportFilters{CorrelationID: corrID}
		result, err := auditRepo.Export(context.Background(), filters)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Events).To(HaveLen(3))

		Expect(result.Events[0].AuditEvent.PreviousEventHash).To(BeEmpty(),
			"first event in chain has no previous hash")
		for i := 1; i < len(result.Events); i++ {
			Expect(result.Events[i].AuditEvent.PreviousEventHash).To(
				Equal(result.Events[i-1].AuditEvent.EventHash),
				fmt.Sprintf("event %d previous_hash must chain to event %d hash", i, i-1))
		}
	})
})
