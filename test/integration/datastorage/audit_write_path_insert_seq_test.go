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

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// Issue #2318 (write path): getPreviousEventHash must pick the true chain
// tip by insertion order, not by an ordering that depends on event_id (a
// client-generated UUID unrelated to insertion order). FedRAMP AU-9 / SOC2
// CC8.1: a wrong predecessor silently forks the hash chain at write time.
var _ = Describe("Audit write-path hash-chain predecessor ordering [Issue #2318]", func() {
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
			fmt.Sprintf("%%insert-seq-%s%%", testID))
	})

	It("IT-DS-2318-001: Create picks the true chain tip by insert order on a same-timestamp write burst", func() {
		corrID := fmt.Sprintf("insert-seq-%s", testID)
		sharedTimestamp := time.Now().UTC().Truncate(time.Second)

		// event1 is inserted FIRST (chronologically the older predecessor)
		// but is deliberately given a UUID that sorts HIGHER than event2's.
		// The pre-fix query (`ORDER BY event_timestamp DESC, event_id DESC`)
		// picks the row with the largest event_id among timestamp ties --
		// so with these IDs it wrongly ranks event1 above event2 regardless
		// of which was actually written last.
		event1 := &repository.AuditEvent{
			EventID:        uuid.MustParse("ffffffff-ffff-ffff-ffff-fffffffffffe"),
			EventTimestamp: sharedTimestamp,
			EventType:      "test.insert_seq",
			Version:        "1.0",
			EventCategory:  "test",
			EventAction:    "verify",
			EventOutcome:   "success",
			CorrelationID:  corrID,
			ResourceType:   "test-resource",
			ResourceID:     "res-1",
			ActorID:        "test-actor",
			ActorType:      "service",
			RetentionDays:  30,
			EventData:      map[string]interface{}{"index": 1},
		}
		created1, err := auditRepo.Create(context.Background(), event1)
		Expect(err).ToNot(HaveOccurred())

		// event2 is inserted SECOND (the true chain tip after event1) but
		// has a UUID that sorts LOWER than event1's.
		event2 := &repository.AuditEvent{
			EventID:        uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			EventTimestamp: sharedTimestamp,
			EventType:      "test.insert_seq",
			Version:        "1.0",
			EventCategory:  "test",
			EventAction:    "verify",
			EventOutcome:   "success",
			CorrelationID:  corrID,
			ResourceType:   "test-resource",
			ResourceID:     "res-2",
			ActorID:        "test-actor",
			ActorType:      "service",
			RetentionDays:  30,
			EventData:      map[string]interface{}{"index": 2},
		}
		created2, err := auditRepo.Create(context.Background(), event2)
		Expect(err).ToNot(HaveOccurred())

		// event3 is the probe: its PreviousEventHash must chain to event2
		// (the actual chain tip), not event1 (the timestamp/event_id-tiebreak
		// artifact).
		event3 := &repository.AuditEvent{
			EventID:        uuid.New(),
			EventTimestamp: sharedTimestamp,
			EventType:      "test.insert_seq",
			Version:        "1.0",
			EventCategory:  "test",
			EventAction:    "verify",
			EventOutcome:   "success",
			CorrelationID:  corrID,
			ResourceType:   "test-resource",
			ResourceID:     "res-3",
			ActorID:        "test-actor",
			ActorType:      "service",
			RetentionDays:  30,
			EventData:      map[string]interface{}{"index": 3},
		}
		created3, err := auditRepo.Create(context.Background(), event3)
		Expect(err).ToNot(HaveOccurred())

		Expect(created3.PreviousEventHash).To(Equal(created2.EventHash),
			"event3 must chain off event2 (the actual chain tip written immediately before it), "+
				"not event1 -- event_id must never be used to break event_timestamp ties (Issue #2318)")
		Expect(created3.PreviousEventHash).ToNot(Equal(created1.EventHash),
			"event3 chaining off event1 would mean the write path picked a stale predecessor "+
				"during a same-timestamp write burst (Issue #2318 write-path fork)")
	})
})
