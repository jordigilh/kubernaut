//go:build unit
// +build unit

package alert_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/alert"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/sirupsen/logrus"
)

// TDD GREEN PHASE: Alert Service Implementation Tests
// Tests pass with the new alert service implementation

var _ = Describe("Alert Service GREEN Phase Tests", func() {
	var (
		alertService alert.AlertService
		config       *alert.Config
		logger       *logrus.Logger
		ctx          context.Context
		cancel       context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel)

		config = &alert.Config{
			ServicePort:            8081,
			MaxConcurrentAlerts:    200,
			AlertProcessingTimeout: 30 * time.Second,
			DeduplicationWindow:    5 * time.Minute,
			EnrichmentTimeout:      10 * time.Second,
			AI: alert.AIConfig{
				Provider:            "holmesgpt",
				Endpoint:            "http://ai-service:8082",
				Model:               "test-model",
				Timeout:             30 * time.Second,
				MaxRetries:          2,
				ConfidenceThreshold: 0.6,
			},
		}

		// Create alert service with nil LLM client for testing
		alertService = alert.NewAlertService(nil, config, logger)
	})

	AfterEach(func() {
		cancel()
	})

	Context("Alert Validation", func() {
		It("should validate correct alerts", func() {
			alert := types.Alert{
				Name:      "TestAlert",
				Severity:  "critical",
				Status:    "firing",
				Namespace: "production",
			}

			validation := alertService.ValidateAlert(alert)
			Expect(validation).To(HaveKey("valid"))
			Expect(validation["valid"]).To(BeTrue())
		})

		It("should reject invalid alerts", func() {
			alert := types.Alert{
				Name: "", // Missing required field
			}

			validation := alertService.ValidateAlert(alert)
			Expect(validation).To(HaveKey("valid"))
			Expect(validation["valid"]).To(BeFalse())
			Expect(validation).To(HaveKey("errors"))
		})
	})

	Context("Alert Processing", func() {
		It("should process valid alerts successfully", func() {
			alert := types.Alert{
				Name:      "ProcessAlert",
				Severity:  "critical",
				Status:    "firing",
				Namespace: "production",
			}

			result, err := alertService.ProcessAlert(ctx, alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Success).To(BeTrue())
		})

		It("should skip invalid alerts", func() {
			alert := types.Alert{
				Name: "", // Invalid alert
			}

			result, err := alertService.ProcessAlert(ctx, alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Success).To(BeFalse())
			Expect(result.Reason).To(Equal("validation failed"))
		})
	})

	Context("Alert Routing", func() {
		It("should route critical alerts to AI service per approved architecture", func() {
			// ARCHITECTURE FIX: Updated test to match approved microservices architecture
			// Alert Processor → AI Analysis Service → Workflow Orchestrator
			alert := types.Alert{
				Name:     "CriticalAlert",
				Severity: "critical",
				Status:   "firing",
			}

			routing := alertService.RouteAlert(ctx, alert)
			Expect(routing).To(HaveKey("routed"))
			Expect(routing["routed"]).To(BeTrue())
			Expect(routing["destination"]).To(Equal("ai-service")) // ARCHITECTURE FIX: Route to AI service first
		})

		It("should filter low priority alerts", func() {
			alert := types.Alert{
				Name:     "InfoAlert",
				Severity: "info",
				Status:   "firing",
			}

			routing := alertService.RouteAlert(ctx, alert)
			Expect(routing).To(HaveKey("routed"))
			Expect(routing["routed"]).To(BeFalse())
		})
	})

	Context("Service Health", func() {
		It("should report healthy status", func() {
			health := alertService.Health()
			Expect(health).To(HaveKey("status"))
			Expect(health["status"]).To(Equal("healthy"))
			Expect(health["service"]).To(Equal("alert-service"))
		})
	})

	Context("Service Metrics", func() {
		It("should provide processing metrics", func() {
			metrics := alertService.GetAlertMetrics()
			Expect(metrics).To(HaveKey("alerts_ingested"))
			Expect(metrics).To(HaveKey("processing_rate"))
		})

		It("should provide deduplication statistics", func() {
			stats := alertService.GetDeduplicationStats()
			Expect(stats).To(HaveKey("total_alerts"))
			Expect(stats).To(HaveKey("deduplication_rate"))
		})
	})
})

func TestAlertServiceGreen(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Alert Service GREEN Phase Suite")
}
