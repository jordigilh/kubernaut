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
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// buildChainEvent constructs an AuditEvent with a computed EventHash chained
// off previousHash, mirroring what AuditEventsRepository.Create would
// persist. Test-only helper for verifyEventChain fixtures (Issue #2318).
func buildChainEvent(id uuid.UUID, corrID, previousHash string, index int) *repository.AuditEvent {
	event := &repository.AuditEvent{
		EventID:           id,
		EventTimestamp:    time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC), // same-second write burst
		EventType:         "test.verify_chain_order",
		EventCategory:     "test",
		EventAction:       "verify",
		EventOutcome:      "success",
		CorrelationID:     corrID,
		ResourceType:      "test-resource",
		ResourceID:        "res",
		ActorID:           "test-actor",
		ActorType:         "service",
		RetentionDays:     30,
		EventData:         map[string]interface{}{"index": index},
		PreviousEventHash: previousHash,
		HashAlgorithm:     repository.HashAlgorithmSHA256Unkeyed,
	}
	hash, err := repository.CalculateHashForVerification(nil, previousHash, event)
	Expect(err).ToNot(HaveOccurred())
	event.EventHash = hash
	return event
}

// Issue #2318 / DD-AUDIT-009: verifyEventChain must verify each event using
// its own stored previous_event_hash pointer, not a global order
// reconstructed from (event_timestamp, event_id). These tests feed
// deliberately scrambled input order to prove the verification result no
// longer depends on it.
var _ = Describe("verifyEventChain order-independence (Issue #2318, DD-AUDIT-009)", func() {
	It("UT-DS-2318-101: verifies a genuinely valid chain correctly regardless of input order", func() {
		corrID := "verify-order-valid"
		event1 := buildChainEvent(uuid.New(), corrID, "", 1)
		event2 := buildChainEvent(uuid.New(), corrID, event1.EventHash, 2)
		event3 := buildChainEvent(uuid.New(), corrID, event2.EventHash, 3)

		// Same-timestamp tiebreak ambiguity (the #2318 symptom) can hand
		// verifyEventChain events in a scrambled, non-chain order.
		scrambled := []*repository.AuditEvent{event1, event3, event2}

		response := &VerifyChainResponse{IsValid: true, TamperedEvents: []TamperedEvent{}}
		Expect(verifyEventChain(response, scrambled, nil)).To(Succeed())

		Expect(response.IsValid).To(BeTrue(),
			"a genuinely valid hash chain must verify as valid regardless of the order events are supplied in (Issue #2318)")
		Expect(response.VerifiedEvents).To(Equal(3))
		Expect(response.TamperedEvents).To(BeEmpty())
	})

	It("UT-DS-2318-102: flags an event whose event_data was tampered with, regardless of order", func() {
		corrID := "verify-order-tampered"
		event1 := buildChainEvent(uuid.New(), corrID, "", 1)
		event2 := buildChainEvent(uuid.New(), corrID, event1.EventHash, 2)
		event3 := buildChainEvent(uuid.New(), corrID, event2.EventHash, 3)

		// Tamper with event2's data AFTER its hash was computed: event2.EventHash
		// no longer matches its own content. Must surface regardless of scan order.
		event2.EventData = map[string]interface{}{"index": 999}

		response := &VerifyChainResponse{IsValid: true, TamperedEvents: []TamperedEvent{}}
		Expect(verifyEventChain(response, []*repository.AuditEvent{event3, event1, event2}, nil)).To(Succeed())

		Expect(response.IsValid).To(BeFalse())
		Expect(tamperedEventIDs(response)).To(ContainElement(event2.EventID.String()))
	})

	It("UT-DS-2318-103: flags a dangling previous_event_hash that does not resolve to any event in the set", func() {
		corrID := "verify-order-dangling"
		event1 := buildChainEvent(uuid.New(), corrID, "", 1)
		// event2 claims a previous_event_hash that doesn't belong to any
		// event actually in the retrieved set.
		event2 := buildChainEvent(uuid.New(), corrID, strings.Repeat("0", 64), 2)

		response := &VerifyChainResponse{IsValid: true, TamperedEvents: []TamperedEvent{}}
		Expect(verifyEventChain(response, []*repository.AuditEvent{event2, event1}, nil)).To(Succeed())

		Expect(response.IsValid).To(BeFalse())
		Expect(tamperedEventIDs(response)).To(ContainElement(event2.EventID.String()))
	})

	It("UT-DS-2318-104: flags a fork (two events pointing at the same previous_event_hash) distinctly from a hash mismatch", func() {
		corrID := "verify-order-fork"
		event1 := buildChainEvent(uuid.New(), corrID, "", 1)
		event2a := buildChainEvent(uuid.New(), corrID, event1.EventHash, 2)
		event2b := buildChainEvent(uuid.New(), corrID, event1.EventHash, 3) // forks off event1 too

		response := &VerifyChainResponse{IsValid: true, TamperedEvents: []TamperedEvent{}}
		Expect(verifyEventChain(response, []*repository.AuditEvent{event1, event2a, event2b}, nil)).To(Succeed())

		Expect(response.IsValid).To(BeFalse())
		Expect(tamperedMessagesContain(response, "fork")).To(BeTrue(),
			"a fork must be flagged with a message distinct from a plain hash mismatch (SOC2 CC7.2/FedRAMP AU-9)")
	})

	It("UT-DS-2318-105: allows exactly one genesis event (empty previous_event_hash) without flagging it", func() {
		corrID := "verify-order-genesis"
		event1 := buildChainEvent(uuid.New(), corrID, "", 1)
		event2 := buildChainEvent(uuid.New(), corrID, event1.EventHash, 2)

		response := &VerifyChainResponse{IsValid: true, TamperedEvents: []TamperedEvent{}}
		Expect(verifyEventChain(response, []*repository.AuditEvent{event2, event1}, nil)).To(Succeed())

		Expect(response.IsValid).To(BeTrue())
		Expect(response.VerifiedEvents).To(Equal(2))
	})
})

func tamperedEventIDs(response *VerifyChainResponse) []string {
	ids := make([]string, 0, len(response.TamperedEvents))
	for _, te := range response.TamperedEvents {
		ids = append(ids, te.EventID)
	}
	return ids
}

func tamperedMessagesContain(response *VerifyChainResponse, substr string) bool {
	for _, te := range response.TamperedEvents {
		if strings.Contains(te.Message, substr) {
			return true
		}
	}
	return false
}
