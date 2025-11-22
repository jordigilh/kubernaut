package audit

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	audit "github.com/jordigilh/kubernaut/pkg/audit"
)

var _ = Describe("AuditEvent", func() {
	Describe("audit.NewAuditEvent", func() {
		It("should create event with defaults", func() {
			event := audit.NewAuditEvent()

			Expect(event.EventID).NotTo(Equal(uuid.Nil))
			Expect(event.EventVersion).To(Equal("1.0"))
			Expect(event.EventTimestamp).To(BeTemporally("~", time.Now(), time.Second))
			Expect(event.RetentionDays).To(Equal(2555)) // 7 years
			Expect(event.IsSensitive).To(BeFalse())
		})
	})

	Describe("Validate", func() {
		var event *audit.AuditEvent

		BeforeEach(func() {
			event = audit.NewAuditEvent()
			event.EventType = "test.event.created"
			event.EventCategory = "test"
			event.EventAction = "created"
			event.EventOutcome = "success"
			event.ActorType = "service"
			event.ActorID = "test-service"
			event.ResourceType = "TestResource"
			event.ResourceID = "test-123"
			event.CorrelationID = "corr-123"
			event.EventData = []byte(`{"test": "data"}`)
		})

		It("should validate a complete event", func() {
			err := event.Validate()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should require event_type", func() {
			event.EventType = ""
			err := event.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("event_type is required"))
		})

		It("should require event_category", func() {
			event.EventCategory = ""
			err := event.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("event_category is required"))
		})

		It("should require event_action", func() {
			event.EventAction = ""
			err := event.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("event_action is required"))
		})

		It("should require event_outcome", func() {
			event.EventOutcome = ""
			err := event.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("event_outcome is required"))
		})

		It("should require actor_type", func() {
			event.ActorType = ""
			err := event.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("actor_type is required"))
		})

		It("should require actor_id", func() {
			event.ActorID = ""
			err := event.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("actor_id is required"))
		})

		It("should require resource_type", func() {
			event.ResourceType = ""
			err := event.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("resource_type is required"))
		})

		It("should require resource_id", func() {
			event.ResourceID = ""
			err := event.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("resource_id is required"))
		})

		It("should require correlation_id", func() {
			event.CorrelationID = ""
			err := event.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("correlation_id is required"))
		})

		It("should require event_data", func() {
			event.EventData = nil
			err := event.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("event_data is required"))
		})

		It("should require positive retention_days", func() {
			event.RetentionDays = 0
			err := event.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("retention_days must be positive"))
		})
	})
})

var _ = Describe("audit.CommonEnvelope", func() {
	Describe("audit.NewEventData", func() {
		It("should create envelope with defaults", func() {
			payload := map[string]interface{}{
				"key1": "value1",
				"key2": 123,
			}

			envelope := audit.NewEventData("gateway", "signal_received", "success", payload)

			Expect(envelope.Version).To(Equal("1.0"))
			Expect(envelope.Service).To(Equal("gateway"))
			Expect(envelope.Operation).To(Equal("signal_received"))
			Expect(envelope.Status).To(Equal("success"))
			Expect(envelope.Payload).To(Equal(payload))
			Expect(envelope.SourcePayload).To(BeNil())
		})
	})

	Describe("WithSourcePayload", func() {
		It("should add source payload", func() {
			payload := map[string]interface{}{"key": "value"}
			sourcePayload := map[string]interface{}{"original": "data"}

			envelope := audit.NewEventData("gateway", "signal_received", "success", payload)
			envelope.WithSourcePayload(sourcePayload)

			Expect(envelope.SourcePayload).To(Equal(sourcePayload))
		})

		It("should support method chaining", func() {
			payload := map[string]interface{}{"key": "value"}
			sourcePayload := map[string]interface{}{"original": "data"}

			envelope := audit.NewEventData("gateway", "signal_received", "success", payload).
				WithSourcePayload(sourcePayload)

			Expect(envelope.SourcePayload).To(Equal(sourcePayload))
		})
	})

	Describe("ToJSON", func() {
		It("should marshal to JSON", func() {
			payload := map[string]interface{}{
				"key1": "value1",
				"key2": 123,
			}

			envelope := audit.NewEventData("gateway", "signal_received", "success", payload)
			jsonBytes, err := envelope.ToJSON()

			Expect(err).ToNot(HaveOccurred())
			Expect(jsonBytes).ToNot(BeEmpty())
			Expect(string(jsonBytes)).To(ContainSubstring(`"service":"gateway"`))
			Expect(string(jsonBytes)).To(ContainSubstring(`"operation":"signal_received"`))
			Expect(string(jsonBytes)).To(ContainSubstring(`"status":"success"`))
		})
	})

	Describe("audit.FromJSON", func() {
		It("should unmarshal from JSON", func() {
			jsonData := []byte(`{
				"version": "1.0",
				"service": "gateway",
				"operation": "signal_received",
				"status": "success",
				"payload": {"key": "value"}
			}`)

			envelope, err := audit.FromJSON(jsonData)

			Expect(err).ToNot(HaveOccurred())
			Expect(envelope.Version).To(Equal("1.0"))
			Expect(envelope.Service).To(Equal("gateway"))
			Expect(envelope.Operation).To(Equal("signal_received"))
			Expect(envelope.Status).To(Equal("success"))
			Expect(envelope.Payload).To(HaveKey("key"))
		})

		It("should return error for invalid JSON", func() {
			jsonData := []byte(`{invalid json}`)

			_, err := audit.FromJSON(jsonData)

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Validate", func() {
		var envelope *audit.CommonEnvelope

		BeforeEach(func() {
			envelope = audit.NewEventData("gateway", "signal_received", "success", map[string]interface{}{
				"key": "value",
			})
		})

		It("should validate a complete envelope", func() {
			err := envelope.Validate()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should require version", func() {
			envelope.Version = ""
			err := envelope.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("version is required"))
		})

		It("should require service", func() {
			envelope.Service = ""
			err := envelope.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("service is required"))
		})

		It("should require operation", func() {
			envelope.Operation = ""
			err := envelope.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("operation is required"))
		})

		It("should require status", func() {
			envelope.Status = ""
			err := envelope.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("status is required"))
		})

		It("should require payload", func() {
			envelope.Payload = nil
			err := envelope.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("payload is required"))
		})
	})
})
