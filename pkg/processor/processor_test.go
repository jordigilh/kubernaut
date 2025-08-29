package processor

import (
	"context"
	"fmt"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/actionhistory"
	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/executor"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// SimpleMockRepository for testing
type SimpleMockRepository struct{}

func (m *SimpleMockRepository) EnsureResourceReference(ctx context.Context, ref actionhistory.ResourceReference) (int64, error) {
	return 1, nil
}

func (m *SimpleMockRepository) GetResourceReference(ctx context.Context, namespace, kind, name string) (*actionhistory.ResourceReference, error) {
	return nil, nil
}

func (m *SimpleMockRepository) EnsureActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	return nil, nil
}

func (m *SimpleMockRepository) GetActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	return nil, nil
}

func (m *SimpleMockRepository) UpdateActionHistory(ctx context.Context, history *actionhistory.ActionHistory) error {
	return nil
}

func (m *SimpleMockRepository) StoreAction(ctx context.Context, action *actionhistory.ActionRecord) (*actionhistory.ResourceActionTrace, error) {
	return nil, nil
}

func (m *SimpleMockRepository) GetActionTraces(ctx context.Context, query actionhistory.ActionQuery) ([]actionhistory.ResourceActionTrace, error) {
	return nil, nil
}

func (m *SimpleMockRepository) GetActionTrace(ctx context.Context, actionID string) (*actionhistory.ResourceActionTrace, error) {
	return nil, nil
}

func (m *SimpleMockRepository) UpdateActionTrace(ctx context.Context, trace *actionhistory.ResourceActionTrace) error {
	return nil
}

func (m *SimpleMockRepository) GetPendingEffectivenessAssessments(ctx context.Context) ([]*actionhistory.ResourceActionTrace, error) {
	return nil, nil
}

func (m *SimpleMockRepository) GetOscillationPatterns(ctx context.Context, patternType string) ([]actionhistory.OscillationPattern, error) {
	return nil, nil
}

func (m *SimpleMockRepository) StoreOscillationDetection(ctx context.Context, detection *actionhistory.OscillationDetection) error {
	return nil
}

func (m *SimpleMockRepository) GetOscillationDetections(ctx context.Context, resourceID int64, resolved *bool) ([]actionhistory.OscillationDetection, error) {
	return nil, nil
}

func (m *SimpleMockRepository) ApplyRetention(ctx context.Context, actionHistoryID int64) error {
	return nil
}

func (m *SimpleMockRepository) GetActionHistorySummaries(ctx context.Context, since time.Duration) ([]actionhistory.ActionHistorySummary, error) {
	return nil, nil
}

// FakeSLMClient implements the slm.Client interface for testing
type FakeSLMClient struct {
	recommendation *types.ActionRecommendation
	err            error
	healthy        bool
	callCount      int
}

func NewFakeSLMClient(healthy bool) *FakeSLMClient {
	return &FakeSLMClient{
		healthy: healthy,
		recommendation: &types.ActionRecommendation{
			Action:     "notify_only",
			Confidence: 0.5,
			Reasoning:  &types.ReasoningDetails{Summary: "Fake SLM response for testing"},
			Parameters: map[string]interface{}{},
		},
	}
}

func (f *FakeSLMClient) AnalyzeAlert(ctx context.Context, alert types.Alert) (*types.ActionRecommendation, error) {
	f.callCount++
	if f.err != nil {
		return nil, f.err
	}
	return f.recommendation, nil
}

func (f *FakeSLMClient) IsHealthy() bool {
	return f.healthy
}

func (f *FakeSLMClient) SetError(err error) {
	f.err = err
}

func (f *FakeSLMClient) SetRecommendation(rec *types.ActionRecommendation) {
	f.recommendation = rec
}

func (f *FakeSLMClient) GetCallCount() int {
	return f.callCount
}

// FakeExecutor implements the executor.Executor interface for testing
type FakeExecutor struct {
	err       error
	callCount int
	healthy   bool
}

func NewFakeExecutor(healthy bool) *FakeExecutor {
	return &FakeExecutor{
		healthy: healthy,
	}
}

func (f *FakeExecutor) Execute(ctx context.Context, action *types.ActionRecommendation, alert types.Alert, actionTrace *actionhistory.ResourceActionTrace) error {
	f.callCount++
	return f.err
}

func (f *FakeExecutor) IsHealthy() bool {
	return f.healthy
}

func (f *FakeExecutor) SetError(err error) {
	f.err = err
}

func (f *FakeExecutor) GetCallCount() int {
	return f.callCount
}

func (f *FakeExecutor) GetActionRegistry() *executor.ActionRegistry {
	return executor.NewActionRegistry()
}

var _ = Describe("Processor", func() {
	var (
		logger    *logrus.Logger
		slmClient *FakeSLMClient
		executor  *FakeExecutor
		mockRepo  *SimpleMockRepository
		processor Processor
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel)
		slmClient = NewFakeSLMClient(true)
		executor = NewFakeExecutor(true)
		mockRepo = &SimpleMockRepository{}
		processor = NewProcessor(slmClient, executor, []config.FilterConfig{}, mockRepo, logger)
	})

	Describe("NewProcessor", func() {
		It("should create a new processor", func() {
			processor := NewProcessor(slmClient, executor, []config.FilterConfig{}, mockRepo, logger)
			Expect(processor).ToNot(BeNil())
		})
	})

	Describe("ProcessAlert", func() {
		var (
			ctx   context.Context
			alert types.Alert
		)

		BeforeEach(func() {
			ctx = context.Background()
			alert = types.Alert{
				Name:      "HighCPUUsage",
				Status:    "firing",
				Severity:  "warning",
				Namespace: "production",
			}
		})

		Context("when processing a firing alert successfully", func() {
			BeforeEach(func() {
				recommendation := &types.ActionRecommendation{
					Action:     "scale_deployment",
					Confidence: 0.8,
					Reasoning:  &types.ReasoningDetails{Summary: "High CPU usage detected"},
					Parameters: map[string]interface{}{
						"replicas": 5,
					},
				}
				slmClient.SetRecommendation(recommendation)
			})

			It("should process the alert successfully", func() {
				err := processor.ProcessAlert(ctx, alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(slmClient.GetCallCount()).To(Equal(1))
				Expect(executor.GetCallCount()).To(Equal(1))
			})
		})

		Context("when alert status is not firing", func() {
			BeforeEach(func() {
				alert.Status = "resolved"
			})

			It("should not call SLM or executor", func() {
				err := processor.ProcessAlert(ctx, alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(slmClient.GetCallCount()).To(Equal(0))
				Expect(executor.GetCallCount()).To(Equal(0))
			})
		})

		Context("when SLM analysis fails", func() {
			BeforeEach(func() {
				slmClient.SetError(fmt.Errorf("SLM analysis failed"))
			})

			It("should return an error and not execute", func() {
				err := processor.ProcessAlert(ctx, alert)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to analyze alert with SLM"))
				Expect(slmClient.GetCallCount()).To(Equal(1))
				Expect(executor.GetCallCount()).To(Equal(0))
			})
		})

		Context("when executor fails", func() {
			BeforeEach(func() {
				executor.SetError(fmt.Errorf("execution failed"))
			})

			It("should return an error after calling both SLM and executor", func() {
				err := processor.ProcessAlert(ctx, alert)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to execute action"))
				Expect(slmClient.GetCallCount()).To(Equal(1))
				Expect(executor.GetCallCount()).To(Equal(1))
			})
		})

		Context("when alert is filtered out", func() {
			BeforeEach(func() {
				filters := []config.FilterConfig{
					{
						Name: "production-only",
						Conditions: map[string][]string{
							"namespace": {"production"},
						},
					},
				}
				processor = NewProcessor(slmClient, executor, filters, mockRepo, logger)
				alert.Namespace = "development" // Doesn't match filter
			})

			It("should not process the alert", func() {
				err := processor.ProcessAlert(ctx, alert)
				Expect(err).ToNot(HaveOccurred())
				Expect(slmClient.GetCallCount()).To(Equal(0))
				Expect(executor.GetCallCount()).To(Equal(0))
			})
		})
	})

	Describe("ShouldProcess", func() {
		Context("when no filters are configured", func() {
			It("should process all alerts", func() {
				alert := types.Alert{
					Name:      "TestAlert",
					Namespace: "any-namespace",
					Severity:  "critical",
				}
				Expect(processor.ShouldProcess(alert)).To(BeTrue())
			})
		})

		Context("when filters are configured", func() {
			BeforeEach(func() {
				filters := []config.FilterConfig{
					{
						Name: "production-filter",
						Conditions: map[string][]string{
							"namespace": {"production", "staging"},
							"severity":  {"critical", "warning"},
						},
					},
					{
						Name: "critical-only",
						Conditions: map[string][]string{
							"severity": {"critical"},
						},
					},
				}
				processor = NewProcessor(slmClient, executor, filters, mockRepo, logger)
			})

			DescribeTable("filter matching scenarios",
				func(alert types.Alert, expected bool) {
					Expect(processor.ShouldProcess(alert)).To(Equal(expected))
				},
				Entry("matches first filter", types.Alert{
					Name:      "ProdAlert",
					Namespace: "production",
					Severity:  "critical",
				}, true),
				Entry("matches second filter", types.Alert{
					Name:      "CriticalAlert",
					Namespace: "development",
					Severity:  "critical",
				}, true),
				Entry("doesn't match any filter", types.Alert{
					Name:      "InfoAlert",
					Namespace: "development",
					Severity:  "info",
				}, false),
				Entry("partial match on first filter", types.Alert{
					Name:      "ProdAlert",
					Namespace: "production",
					Severity:  "info", // Wrong severity
				}, false),
			)
		})
	})

})















