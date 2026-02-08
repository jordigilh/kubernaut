/*
Copyright 2025 Jordi Gil.

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
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

var _ = Describe("PrepareEventForHashing", func() {
	var original *repository.AuditEvent

	BeforeEach(func() {
		now := time.Now().UTC()
		legalHoldTime := now.Add(-1 * time.Hour)
		parentID := uuid.New()
		parentDate := now.Add(-2 * time.Hour)

		original = &repository.AuditEvent{
			EventID:           uuid.New(),
			EventTimestamp:    now,
			EventDate:         repository.DateOnly(now),
			EventType:         "test.event.type",
			Version:           "1.0",
			EventCategory:     "test",
			EventAction:       "verify",
			EventOutcome:      "success",
			CorrelationID:     "correlation-test-123",
			ParentEventID:     &parentID,
			ParentEventDate:   &parentDate,
			ResourceType:      "Pod",
			ResourceID:        "my-pod-abc",
			ResourceNamespace: "production",
			ClusterID:         "cluster-01",
			Severity:          "info",
			DurationMs:        1500,
			ErrorCode:         "ERR_TEST",
			ErrorMessage:      "test error message",
			ActorID:           "test-actor",
			ActorType:         "service",
			RetentionDays:     2555,
			IsSensitive:       true,
			EventHash:         "abc123hash",
			PreviousEventHash: "prev456hash",
			LegalHold:         true,
			LegalHoldReason:   "SOX compliance",
			LegalHoldPlacedBy: "compliance-officer",
			LegalHoldPlacedAt: &legalHoldTime,
			EventData:         map[string]interface{}{"key": "value", "nested": map[string]interface{}{"a": float64(1)}},
		}
	})

	It("should zero out excluded fields", func() {
		result := repository.PrepareEventForHashing(original)

		Expect(result.EventHash).To(BeEmpty(), "EventHash should be cleared")
		Expect(result.PreviousEventHash).To(BeEmpty(), "PreviousEventHash should be cleared")
		Expect(result.EventDate).To(Equal(repository.DateOnly{}), "EventDate should be zeroed")
		Expect(result.LegalHold).To(BeFalse(), "LegalHold should be false")
		Expect(result.LegalHoldReason).To(BeEmpty(), "LegalHoldReason should be cleared")
		Expect(result.LegalHoldPlacedBy).To(BeEmpty(), "LegalHoldPlacedBy should be cleared")
		Expect(result.LegalHoldPlacedAt).To(BeNil(), "LegalHoldPlacedAt should be nil")
	})

	It("should preserve included fields unchanged", func() {
		result := repository.PrepareEventForHashing(original)

		Expect(result.EventID).To(Equal(original.EventID))
		Expect(result.EventTimestamp).To(Equal(original.EventTimestamp))
		Expect(result.EventType).To(Equal(original.EventType))
		Expect(result.Version).To(Equal(original.Version))
		Expect(result.EventCategory).To(Equal(original.EventCategory))
		Expect(result.EventAction).To(Equal(original.EventAction))
		Expect(result.EventOutcome).To(Equal(original.EventOutcome))
		Expect(result.CorrelationID).To(Equal(original.CorrelationID))
		Expect(result.ParentEventID).To(Equal(original.ParentEventID))
		Expect(result.ParentEventDate).To(Equal(original.ParentEventDate))
		Expect(result.ResourceType).To(Equal(original.ResourceType))
		Expect(result.ResourceID).To(Equal(original.ResourceID))
		Expect(result.ResourceNamespace).To(Equal(original.ResourceNamespace))
		Expect(result.ClusterID).To(Equal(original.ClusterID))
		Expect(result.Severity).To(Equal(original.Severity))
		Expect(result.DurationMs).To(Equal(original.DurationMs))
		Expect(result.ErrorCode).To(Equal(original.ErrorCode))
		Expect(result.ErrorMessage).To(Equal(original.ErrorMessage))
		Expect(result.ActorID).To(Equal(original.ActorID))
		Expect(result.ActorType).To(Equal(original.ActorType))
		Expect(result.RetentionDays).To(Equal(original.RetentionDays))
		Expect(result.IsSensitive).To(Equal(original.IsSensitive))
		Expect(result.EventData).To(Equal(original.EventData))
	})

	It("should not mutate the original event (copy semantics)", func() {
		// Save original values before call
		originalHash := original.EventHash
		originalPrevHash := original.PreviousEventHash
		originalEventDate := original.EventDate
		originalLegalHold := original.LegalHold
		originalLegalHoldReason := original.LegalHoldReason
		originalLegalHoldPlacedBy := original.LegalHoldPlacedBy
		originalLegalHoldPlacedAt := original.LegalHoldPlacedAt

		_ = repository.PrepareEventForHashing(original)

		Expect(original.EventHash).To(Equal(originalHash), "Original EventHash should not be mutated")
		Expect(original.PreviousEventHash).To(Equal(originalPrevHash), "Original PreviousEventHash should not be mutated")
		Expect(original.EventDate).To(Equal(originalEventDate), "Original EventDate should not be mutated")
		Expect(original.LegalHold).To(Equal(originalLegalHold), "Original LegalHold should not be mutated")
		Expect(original.LegalHoldReason).To(Equal(originalLegalHoldReason), "Original LegalHoldReason should not be mutated")
		Expect(original.LegalHoldPlacedBy).To(Equal(originalLegalHoldPlacedBy), "Original LegalHoldPlacedBy should not be mutated")
		Expect(original.LegalHoldPlacedAt).To(Equal(originalLegalHoldPlacedAt), "Original LegalHoldPlacedAt should not be mutated")
	})
})
