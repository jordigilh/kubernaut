// Copyright 2025 Jordi Gil.
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/audit"
)

func TestAuditEventBuilder(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Storage Audit Event Builder Suite")
}

// ========================================
// TDD RED PHASE: Base Event Builder Tests
// BR-STORAGE-033: Event Data Helpers
// ========================================
//
// These tests define the contract for the base event builder.
// Tests are written FIRST, implementation SECOND.
//
// Business Requirements:
// - BR-STORAGE-033-001: Standardized event_data JSONB structure
// - BR-STORAGE-033-002: Type-safe event building API
// - BR-STORAGE-033-003: Consistent field naming across services
//
// Testing Principle: Behavior + Correctness
// ========================================

var _ = Describe("BaseEventBuilder", func() {
	Context("BR-STORAGE-033-001: Standardized event_data JSONB structure", func() {
		// BEHAVIOR: Builder creates event with correct version field
		// CORRECTNESS: Version is exactly "1.0"
		It("should create event with version 1.0", func() {
			// TDD RED: This test will FAIL until we implement BaseEventBuilder
			builder := audit.NewEventBuilder("test-service", "test.event")
			Expect(builder).ToNot(BeNil())

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData).To(HaveKey("version"))
			Expect(eventData["version"]).To(Equal("1.0"))
		})

		It("should include service name in event data", func() {
			builder := audit.NewEventBuilder("gateway", "signal.received")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData).To(HaveKey("service"))
			Expect(eventData["service"]).To(Equal("gateway"))
		})

		It("should include event type in event data", func() {
			builder := audit.NewEventBuilder("aianalysis", "analysis.completed")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData).To(HaveKey("event_type"))
			Expect(eventData["event_type"]).To(Equal("analysis.completed"))
		})

		It("should include timestamp in event data", func() {
			builder := audit.NewEventBuilder("workflow", "workflow.started")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData).To(HaveKey("timestamp"))

			// Validate timestamp is a valid RFC3339 string
			timestampStr, ok := eventData["timestamp"].(string)
			Expect(ok).To(BeTrue(), "timestamp should be a string")

			_, err = time.Parse(time.RFC3339, timestampStr)
			Expect(err).ToNot(HaveOccurred(), "timestamp should be valid RFC3339")
		})

		It("should include empty data object by default", func() {
			builder := audit.NewEventBuilder("test-service", "test.event")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData).To(HaveKey("data"))

			data, ok := eventData["data"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "data should be a map")
			Expect(data).To(BeEmpty(), "data should be empty by default")
		})
	})

	Context("BR-STORAGE-033-002: Type-safe event building API", func() {
		It("should support fluent API with custom fields", func() {
			builder := audit.NewEventBuilder("test-service", "test.event").
				WithCustomField("key1", "value1").
				WithCustomField("key2", 42).
				WithCustomField("key3", true)

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, ok := eventData["data"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(data).To(HaveKeyWithValue("key1", "value1"))
			Expect(data).To(HaveKeyWithValue("key2", float64(42))) // JSON numbers are float64
			Expect(data).To(HaveKeyWithValue("key3", true))
		})

		It("should support nested objects in custom fields", func() {
			nestedData := map[string]interface{}{
				"nested_key": "nested_value",
				"nested_num": 123,
			}

			builder := audit.NewEventBuilder("test-service", "test.event").
				WithCustomField("nested_object", nestedData)

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, ok := eventData["data"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(data).To(HaveKey("nested_object"))

			nested, ok := data["nested_object"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(nested).To(HaveKeyWithValue("nested_key", "nested_value"))
			Expect(nested).To(HaveKeyWithValue("nested_num", float64(123)))
		})

		It("should support array values in custom fields", func() {
			arrayData := []string{"item1", "item2", "item3"}

			builder := audit.NewEventBuilder("test-service", "test.event").
				WithCustomField("array_field", arrayData)

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, ok := eventData["data"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(data).To(HaveKey("array_field"))

			array, ok := data["array_field"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(array).To(HaveLen(3))
			Expect(array[0]).To(Equal("item1"))
			Expect(array[1]).To(Equal("item2"))
			Expect(array[2]).To(Equal("item3"))
		})

		It("should be chainable (fluent API)", func() {
			// Test that WithCustomField returns the builder for chaining
			builder := audit.NewEventBuilder("test-service", "test.event")
			result := builder.WithCustomField("key1", "value1")
			Expect(result).To(BeIdenticalTo(builder), "WithCustomField should return the same builder for chaining")
		})
	})

	Context("BR-STORAGE-033-003: Valid JSONB output", func() {
		It("should produce valid JSON that can be marshaled/unmarshaled", func() {
			builder := audit.NewEventBuilder("test-service", "test.event").
				WithCustomField("test_key", "test_value")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Marshal to JSON
			jsonBytes, err := json.Marshal(eventData)
			Expect(err).ToNot(HaveOccurred())

			// Unmarshal back
			var unmarshaled map[string]interface{}
			err = json.Unmarshal(jsonBytes, &unmarshaled)
			Expect(err).ToNot(HaveOccurred())

			// Verify structure is preserved
			Expect(unmarshaled).To(HaveKey("version"))
			Expect(unmarshaled).To(HaveKey("service"))
			Expect(unmarshaled).To(HaveKey("event_type"))
			Expect(unmarshaled).To(HaveKey("timestamp"))
			Expect(unmarshaled).To(HaveKey("data"))
		})

		It("should handle nil values gracefully", func() {
			builder := audit.NewEventBuilder("test-service", "test.event").
				WithCustomField("nil_field", nil)

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, ok := eventData["data"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(data).To(HaveKey("nil_field"))
			Expect(data["nil_field"]).To(BeNil())
		})

		It("should produce consistent JSONB structure across multiple builds", func() {
			builder1 := audit.NewEventBuilder("service1", "event1").
				WithCustomField("key1", "value1")

			builder2 := audit.NewEventBuilder("service2", "event2").
				WithCustomField("key2", "value2")

			eventData1, err := builder1.Build()
			Expect(err).ToNot(HaveOccurred())

			eventData2, err := builder2.Build()
			Expect(err).ToNot(HaveOccurred())

			// Both should have the same structure (keys)
			Expect(eventData1).To(HaveKey("version"))
			Expect(eventData1).To(HaveKey("service"))
			Expect(eventData1).To(HaveKey("event_type"))
			Expect(eventData1).To(HaveKey("timestamp"))
			Expect(eventData1).To(HaveKey("data"))

			Expect(eventData2).To(HaveKey("version"))
			Expect(eventData2).To(HaveKey("service"))
			Expect(eventData2).To(HaveKey("event_type"))
			Expect(eventData2).To(HaveKey("timestamp"))
			Expect(eventData2).To(HaveKey("data"))
		})
	})

	Context("Edge Cases", func() {
		It("should handle empty service name", func() {
			builder := audit.NewEventBuilder("", "test.event")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData["service"]).To(Equal(""))
		})

		It("should handle empty event type", func() {
			builder := audit.NewEventBuilder("test-service", "")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData["event_type"]).To(Equal(""))
		})

		It("should overwrite custom field if set multiple times", func() {
			builder := audit.NewEventBuilder("test-service", "test.event").
				WithCustomField("key1", "original").
				WithCustomField("key1", "updated")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, ok := eventData["data"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(data).To(HaveKeyWithValue("key1", "updated"))
		})
	})
})

