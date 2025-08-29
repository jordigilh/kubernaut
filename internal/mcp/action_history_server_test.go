package mcp

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/actionhistory"
	"github.com/jordigilh/prometheus-alerts-slm/internal/errors"
	"github.com/jordigilh/prometheus-alerts-slm/internal/oscillation"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// MockRepository for testing
type MockRepository struct {
	traces []actionhistory.ResourceActionTrace
	err    error
}

func (m *MockRepository) EnsureResourceReference(ctx context.Context, ref actionhistory.ResourceReference) (int64, error) {
	return 1, m.err
}

func (m *MockRepository) GetResourceReference(ctx context.Context, namespace, kind, name string) (*actionhistory.ResourceReference, error) {
	return nil, m.err
}

func (m *MockRepository) EnsureActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	return nil, m.err
}

func (m *MockRepository) GetActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	return nil, m.err
}

func (m *MockRepository) UpdateActionHistory(ctx context.Context, history *actionhistory.ActionHistory) error {
	return m.err
}

func (m *MockRepository) StoreAction(ctx context.Context, action *actionhistory.ActionRecord) (*actionhistory.ResourceActionTrace, error) {
	return nil, m.err
}

func (m *MockRepository) GetActionTraces(ctx context.Context, query actionhistory.ActionQuery) ([]actionhistory.ResourceActionTrace, error) {
	return m.traces, m.err
}

func (m *MockRepository) GetActionTrace(ctx context.Context, actionID string) (*actionhistory.ResourceActionTrace, error) {
	return nil, m.err
}

func (m *MockRepository) UpdateActionTrace(ctx context.Context, trace *actionhistory.ResourceActionTrace) error {
	return m.err
}

func (m *MockRepository) GetOscillationPatterns(ctx context.Context, patternType string) ([]actionhistory.OscillationPattern, error) {
	return nil, m.err
}

func (m *MockRepository) StoreOscillationDetection(ctx context.Context, detection *actionhistory.OscillationDetection) error {
	return m.err
}

func (m *MockRepository) GetOscillationDetections(ctx context.Context, resourceID int64, resolved *bool) ([]actionhistory.OscillationDetection, error) {
	return nil, m.err
}

func (m *MockRepository) ApplyRetention(ctx context.Context, actionHistoryID int64) error {
	return m.err
}

func (m *MockRepository) GetActionHistorySummaries(ctx context.Context, since time.Duration) ([]actionhistory.ActionHistorySummary, error) {
	return nil, m.err
}

func (m *MockRepository) GetPendingEffectivenessAssessments(ctx context.Context) ([]*actionhistory.ResourceActionTrace, error) {
	var traces []*actionhistory.ResourceActionTrace
	for i := range m.traces {
		traces = append(traces, &m.traces[i])
	}
	return traces, m.err
}

// MockDetector for testing
type MockDetector struct {
	result *oscillation.OscillationAnalysisResult
	err    error
}

func (m *MockDetector) AnalyzeResource(ctx context.Context, resourceRef actionhistory.ResourceReference, windowMinutes int) (*oscillation.OscillationAnalysisResult, error) {
	return m.result, m.err
}

var _ = Describe("ActionHistoryMCPServer", func() {
	var (
		server       *ActionHistoryMCPServer
		mockRepo     *MockRepository
		mockDetector *MockDetector
		logger       *logrus.Logger
		ctx          context.Context
	)

	BeforeEach(func() {
		mockRepo = &MockRepository{}
		mockDetector = &MockDetector{}
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests
		server = NewActionHistoryMCPServer(mockRepo, mockDetector, logger)
		ctx = context.Background()
	})

	Describe("NewActionHistoryMCPServer", func() {
		It("should create server with capabilities", func() {
			Expect(server).NotTo(BeNil())
			Expect(server.repository).To(Equal(mockRepo))
			Expect(server.detector).To(Equal(mockDetector))
			Expect(server.logger).To(Equal(logger))

			capabilities := server.GetCapabilities()
			Expect(capabilities.Tools).To(HaveLen(4))

			toolNames := make([]string, len(capabilities.Tools))
			for i, tool := range capabilities.Tools {
				toolNames[i] = tool.Name
			}

			Expect(toolNames).To(ContainElement("get_action_history"))
			Expect(toolNames).To(ContainElement("analyze_oscillation_patterns"))
			Expect(toolNames).To(ContainElement("get_action_effectiveness"))
			Expect(toolNames).To(ContainElement("check_action_safety"))
		})
	})

	Describe("HandleToolCall", func() {
		Context("with unknown tool", func() {
			It("should return error", func() {
				request := MCPToolRequest{
					Params: MCPToolParams{
						Name: "unknown_tool",
					},
				}

				_, err := server.HandleToolCall(ctx, request)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown tool"))
			})
		})
	})

	Describe("handleGetActionHistory", func() {
		Context("with missing required parameters", func() {
			It("should return validation error for missing namespace", func() {
				args := map[string]interface{}{
					"kind": "Deployment",
					"name": "webapp",
				}

				_, err := server.handleGetActionHistory(ctx, args)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsType(err, errors.ErrorTypeValidation)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("namespace is required"))
			})

			It("should return validation error for missing kind", func() {
				args := map[string]interface{}{
					"namespace": "production",
					"name":      "webapp",
				}

				_, err := server.handleGetActionHistory(ctx, args)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsType(err, errors.ErrorTypeValidation)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("kind is required"))
			})

			It("should return validation error for missing name", func() {
				args := map[string]interface{}{
					"namespace": "production",
					"kind":      "Deployment",
				}

				_, err := server.handleGetActionHistory(ctx, args)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsType(err, errors.ErrorTypeValidation)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("name is required"))
			})
		})

		Context("with invalid resource reference", func() {
			It("should return validation error for invalid namespace", func() {
				args := map[string]interface{}{
					"namespace": "Invalid_Namespace",
					"kind":      "Deployment",
					"name":      "webapp",
				}

				_, err := server.handleGetActionHistory(ctx, args)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsType(err, errors.ErrorTypeValidation)).To(BeTrue())
			})

			It("should return validation error for invalid kind", func() {
				args := map[string]interface{}{
					"namespace": "production",
					"kind":      "deployment", // Should start with uppercase
					"name":      "webapp",
				}

				_, err := server.handleGetActionHistory(ctx, args)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsType(err, errors.ErrorTypeValidation)).To(BeTrue())
			})
		})

		Context("with valid parameters", func() {
			BeforeEach(func() {
				mockRepo.traces = []actionhistory.ResourceActionTrace{
					{
						ActionID:        "test-action-1",
						ActionType:      "scale_deployment",
						ModelUsed:       "test-model",
						ModelConfidence: 0.95,
						ExecutionStatus: "completed",
					},
				}
			})

			It("should return action history successfully", func() {
				args := map[string]interface{}{
					"namespace": "production",
					"kind":      "Deployment",
					"name":      "webapp",
				}

				response, err := server.handleGetActionHistory(ctx, args)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Content).To(HaveLen(2))

				// Check structured JSON response
				Expect(response.Content[0].Type).To(Equal("application/json"))
				Expect(response.Content[0].Data).ToNot(BeNil())

				// Check text response
				Expect(response.Content[1].Type).To(Equal("text"))
				Expect(response.Content[1].Text).To(ContainSubstring("Action History for Deployment/webapp"))
				Expect(response.Content[1].Text).To(ContainSubstring("Total actions found: 1"))
				Expect(response.Content[1].Text).To(ContainSubstring("test-action-1"))
			})

			It("should return structured JSON data for programmatic analysis", func() {
				args := map[string]interface{}{
					"namespace": "production",
					"kind":      "Deployment",
					"name":      "webapp",
				}

				response, err := server.handleGetActionHistory(ctx, args)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Content).To(HaveLen(2))

				// Validate structured JSON response can be cast to expected type
				jsonData := response.Content[0].Data
				Expect(jsonData).To(BeAssignableToTypeOf(ActionHistoryResponse{}))

				// Verify the structured data contains expected fields
				structuredData, ok := jsonData.(ActionHistoryResponse)
				Expect(ok).To(BeTrue())
				Expect(structuredData.ResourceInfo.Namespace).To(Equal("production"))
				Expect(structuredData.ResourceInfo.Kind).To(Equal("Deployment"))
				Expect(structuredData.ResourceInfo.Name).To(Equal("webapp"))
				Expect(structuredData.TotalActions).To(Equal(1))
				Expect(structuredData.Actions).To(HaveLen(1))
				Expect(structuredData.Actions[0].ID).To(Equal("test-action-1"))
				Expect(structuredData.Actions[0].ActionType).To(Equal("scale_deployment"))
			})

			It("should handle optional limit parameter", func() {
				args := map[string]interface{}{
					"namespace": "production",
					"kind":      "Deployment",
					"name":      "webapp",
					"limit":     "10",
				}

				_, err := server.handleGetActionHistory(ctx, args)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should handle optional timeRange parameter", func() {
				args := map[string]interface{}{
					"namespace": "production",
					"kind":      "Deployment",
					"name":      "webapp",
					"timeRange": "24h",
				}

				_, err := server.handleGetActionHistory(ctx, args)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when repository returns error", func() {
			BeforeEach(func() {
				mockRepo.err = errors.NewDatabaseError("query", stderrors.New("connection failed"))
			})

			It("should propagate repository error", func() {
				args := map[string]interface{}{
					"namespace": "production",
					"kind":      "Deployment",
					"name":      "webapp",
				}

				_, err := server.handleGetActionHistory(ctx, args)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get action traces"))
			})
		})
	})

	Describe("handleAnalyzeOscillationPatterns", func() {
		Context("with valid parameters", func() {
			BeforeEach(func() {
				mockDetector.result = &oscillation.OscillationAnalysisResult{
					OverallSeverity:   actionhistory.SeverityMedium,
					RecommendedAction: actionhistory.PreventionCoolingPeriod,
					Confidence:        0.85,
					ScaleOscillation: &oscillation.ScaleOscillationResult{
						DirectionChanges: 3,
						Severity:         actionhistory.SeverityMedium,
						AvgEffectiveness: 0.6,
						DurationMinutes:  45.0,
					},
				}
			})

			It("should return oscillation analysis", func() {
				args := map[string]interface{}{
					"namespace": "production",
					"kind":      "Deployment",
					"name":      "webapp",
				}

				response, err := server.handleAnalyzeOscillationPatterns(ctx, args)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Content).To(HaveLen(2))

				// Check structured JSON response
				Expect(response.Content[0].Type).To(Equal("application/json"))
				Expect(response.Content[0].Data).ToNot(BeNil())

				// Check text response
				Expect(response.Content[1].Type).To(Equal("text"))
				Expect(response.Content[1].Text).To(ContainSubstring("Oscillation Analysis"))
				Expect(response.Content[1].Text).To(ContainSubstring("Overall Severity: medium"))
				Expect(response.Content[1].Text).To(ContainSubstring("Scale Oscillation Detected"))
				Expect(response.Content[1].Text).To(ContainSubstring("Direction Changes: 3"))
			})

			It("should return structured oscillation analysis data", func() {
				args := map[string]interface{}{
					"namespace": "production",
					"kind":      "Deployment",
					"name":      "webapp",
				}

				response, err := server.handleAnalyzeOscillationPatterns(ctx, args)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Content).To(HaveLen(2))

				// Validate structured JSON response
				jsonData := response.Content[0].Data
				Expect(jsonData).To(BeAssignableToTypeOf(OscillationAnalysisResponse{}))

				structuredData, ok := jsonData.(OscillationAnalysisResponse)
				Expect(ok).To(BeTrue())
				Expect(structuredData.ResourceInfo.Namespace).To(Equal("production"))
				Expect(structuredData.OverallSeverity).To(Equal("medium"))
				Expect(structuredData.Confidence).To(Equal(0.85))
				Expect(structuredData.ScaleOscillation).NotTo(BeNil())
				Expect(structuredData.ScaleOscillation.DirectionChanges).To(Equal(3))
				Expect(structuredData.ScaleOscillation.Severity).To(Equal("medium"))
			})

			It("should handle optional windowMinutes parameter", func() {
				args := map[string]interface{}{
					"namespace":     "production",
					"kind":          "Deployment",
					"name":          "webapp",
					"windowMinutes": "60",
				}

				_, err := server.handleAnalyzeOscillationPatterns(ctx, args)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when no oscillation detected", func() {
			BeforeEach(func() {
				mockDetector.result = &oscillation.OscillationAnalysisResult{
					OverallSeverity:   actionhistory.SeverityNone,
					RecommendedAction: actionhistory.PreventionNone,
					Confidence:        0.95,
				}
			})

			It("should return safe message", func() {
				args := map[string]interface{}{
					"namespace": "production",
					"kind":      "Deployment",
					"name":      "webapp",
				}

				response, err := server.handleAnalyzeOscillationPatterns(ctx, args)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Content).To(HaveLen(2))
				Expect(response.Content[1].Text).To(ContainSubstring("No concerning oscillation patterns detected"))
			})
		})
	})

	Describe("handleCheckActionSafety", func() {
		Context("with valid parameters and safe action", func() {
			BeforeEach(func() {
				mockDetector.result = &oscillation.OscillationAnalysisResult{
					OverallSeverity:   actionhistory.SeverityNone,
					RecommendedAction: actionhistory.PreventionNone,
					Confidence:        0.95,
				}
			})

			It("should return safe assessment", func() {
				args := map[string]interface{}{
					"namespace":  "production",
					"kind":       "Deployment",
					"name":       "webapp",
					"actionType": "scale_deployment",
				}

				response, err := server.handleCheckActionSafety(ctx, args)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Content).To(HaveLen(2))

				// Check structured JSON response
				Expect(response.Content[0].Type).To(Equal("application/json"))
				Expect(response.Content[0].Data).ToNot(BeNil())

				// Check text response
				Expect(response.Content[1].Type).To(Equal("text"))
				Expect(response.Content[1].Text).To(ContainSubstring("✅ SAFE"))
				Expect(response.Content[1].Text).To(ContainSubstring("No concerning oscillation patterns detected"))
			})
		})

		Context("with unsafe action", func() {
			BeforeEach(func() {
				mockDetector.result = &oscillation.OscillationAnalysisResult{
					OverallSeverity:   actionhistory.SeverityHigh,
					RecommendedAction: actionhistory.PreventionBlock,
					Confidence:        0.88,
				}
			})

			It("should return warning assessment", func() {
				args := map[string]interface{}{
					"namespace":  "production",
					"kind":       "Deployment",
					"name":       "webapp",
					"actionType": "scale_deployment",
				}

				response, err := server.handleCheckActionSafety(ctx, args)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Content).To(HaveLen(2))

				// Check structured JSON response
				Expect(response.Content[0].Type).To(Equal("application/json"))
				Expect(response.Content[0].Data).ToNot(BeNil())

				// Check text response
				Expect(response.Content[1].Type).To(Equal("text"))
				Expect(response.Content[1].Text).To(ContainSubstring("⚠️  WARNING"))
				Expect(response.Content[1].Text).To(ContainSubstring("HIGH severity"))
				Expect(response.Content[1].Text).To(ContainSubstring("🚫 RECOMMENDATION: Block this action"))
			})
		})

		Context("with missing required parameters", func() {
			It("should return validation error for missing actionType", func() {
				args := map[string]interface{}{
					"namespace": "production",
					"kind":      "Deployment",
					"name":      "webapp",
				}

				_, err := server.handleCheckActionSafety(ctx, args)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsType(err, errors.ErrorTypeValidation)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("actionType is required"))
			})
		})
	})

	Describe("Input Validation Security", func() {
		Context("SQL injection attempts", func() {
			It("should reject SQL injection in namespace", func() {
				args := map[string]interface{}{
					"namespace": "'; DROP TABLE users; --",
					"kind":      "Deployment",
					"name":      "webapp",
				}

				_, err := server.handleGetActionHistory(ctx, args)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsType(err, errors.ErrorTypeValidation)).To(BeTrue())
			})

			It("should reject script injection in parameters", func() {
				args := map[string]interface{}{
					"namespace": "production",
					"kind":      "Deployment",
					"name":      "<script>alert('xss')</script>",
				}

				_, err := server.handleGetActionHistory(ctx, args)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsType(err, errors.ErrorTypeValidation)).To(BeTrue())
			})
		})

		Context("invalid parameter types", func() {
			It("should handle non-string parameters gracefully", func() {
				args := map[string]interface{}{
					"namespace": 123, // Should be string
					"kind":      "Deployment",
					"name":      "webapp",
				}

				_, err := server.handleGetActionHistory(ctx, args)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsType(err, errors.ErrorTypeValidation)).To(BeTrue())
			})
		})
	})
})
