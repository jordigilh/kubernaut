package slm

import (
	"strings"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

var _ = Describe("SLM Client", func() {
	var logger *logrus.Logger

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests
	})

	Describe("NewClient", func() {
		DescribeTable("creating new client",
			func(cfg config.SLMConfig, expectErr bool, errString string) {
				client, err := NewClient(cfg, logger)

				if expectErr {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(errString))
					Expect(client).To(BeNil())
				} else {
					Expect(err).ToNot(HaveOccurred())
					Expect(client).ToNot(BeNil())
					var clientInterface Client = client
					Expect(clientInterface).ToNot(BeNil())
				}
			},
			Entry("valid localai config",
				config.SLMConfig{
					Provider: "localai",
					Endpoint: "http://localhost:8080",
					Model:    "test-model",
					Timeout:  30 * time.Second,
				},
				false,
				"",
			),
			Entry("invalid provider",
				config.SLMConfig{
					Provider: "invalid",
					Endpoint: "http://localhost:8080",
					Model:    "test-model",
				},
				true,
				"only LocalAI provider supported, got: invalid",
			),
		)
	})

	Describe("Template Constants", func() {
		Describe("basicPromptTemplate", func() {
			It("should have the correct number of format placeholders", func() {
				// Count format placeholders (%s, %v)
				placeholders := strings.Count(basicPromptTemplate, "%s") + strings.Count(basicPromptTemplate, "%v")
				Expect(placeholders).To(Equal(8), "basicPromptTemplate should have exactly 8 format placeholders")
			})

			It("should not contain unescaped percentage signs", func() {
				// Check for common percentage patterns that should be escaped
				unescapedPatterns := []string{
					"90%+",
					"95% ",
					"80% ",
					"40% ",
					"20% ",
				}

				for _, pattern := range unescapedPatterns {
					if strings.Contains(basicPromptTemplate, pattern) {
						Fail("Found unescaped percentage pattern: " + pattern + " (should be escaped as %%)")
					}
				}
			})

			It("should contain essential prompt sections", func() {
				Expect(basicPromptTemplate).To(ContainSubstring("<|system|>"))
				Expect(basicPromptTemplate).To(ContainSubstring("<|user|>"))
				Expect(basicPromptTemplate).To(ContainSubstring("<|assistant|>"))
				Expect(basicPromptTemplate).To(ContainSubstring("CRITICAL DECISION RULES"))
				Expect(basicPromptTemplate).To(ContainSubstring("AVAILABLE ACTIONS"))
				Expect(basicPromptTemplate).To(ContainSubstring("confidence"))
			})
		})

		Describe("enhancedPromptTemplate", func() {
			It("should have the correct number of format placeholders", func() {
				// Count format placeholders (%s, %v) - enhanced template has 9 (8 + contextualInfo)
				placeholders := strings.Count(enhancedPromptTemplate, "%s") + strings.Count(enhancedPromptTemplate, "%v")
				Expect(placeholders).To(Equal(9), "enhancedPromptTemplate should have exactly 9 format placeholders")
			})

			It("should not contain unescaped percentage signs", func() {
				// Check for common percentage patterns that should be escaped
				unescapedPatterns := []string{
					"90%+",
					"95% ",
					"85-95% ",
					"80% ",
					"40% ",
					"20% ",
				}

				for _, pattern := range unescapedPatterns {
					if strings.Contains(enhancedPromptTemplate, pattern) {
						Fail("Found unescaped percentage pattern: " + pattern + " (should be escaped as %%)")
					}
				}
			})

			It("should contain enhanced prompt sections", func() {
				Expect(enhancedPromptTemplate).To(ContainSubstring("<|system|>"))
				Expect(enhancedPromptTemplate).To(ContainSubstring("<|user|>"))
				Expect(enhancedPromptTemplate).To(ContainSubstring("<|assistant|>"))
				Expect(enhancedPromptTemplate).To(ContainSubstring("ALERT CONTEXT ANALYSIS FRAMEWORK"))
				Expect(enhancedPromptTemplate).To(ContainSubstring("Historical Pattern Analysis"))
				Expect(enhancedPromptTemplate).To(ContainSubstring("CASCADING FAILURE PREVENTION"))
				Expect(enhancedPromptTemplate).To(ContainSubstring("confidence"))
			})
		})
	})

	Describe("Prompt Generation", func() {
		var (
			clientImpl *client
			testAlert  types.Alert
		)

		BeforeEach(func() {
			cfg := config.SLMConfig{
				Provider:       "localai",
				Endpoint:       "http://localhost:8080",
				Model:          "test-model",
				Timeout:        30 * time.Second,
				MaxContextSize: 4000,
			}

			var err error
			c, err := NewClient(cfg, logger)
			Expect(err).ToNot(HaveOccurred())
			clientImpl = c.(*client)

			testAlert = types.Alert{
				Name:        "TestAlert",
				Status:      "firing",
				Severity:    "warning",
				Description: "Test alert description",
				Namespace:   "test-namespace",
				Resource:    "test-resource",
				Labels: map[string]string{
					"alertname": "TestAlert",
					"severity":  "warning",
				},
				Annotations: map[string]string{
					"description": "Test alert annotation",
				},
			}
		})

		Describe("generatePrompt", func() {
			It("should generate a basic prompt without errors", func() {
				prompt := clientImpl.generatePrompt(testAlert)

				Expect(prompt).ToNot(BeEmpty())
				Expect(prompt).To(ContainSubstring("TestAlert"))
				Expect(prompt).To(ContainSubstring("firing"))
				Expect(prompt).To(ContainSubstring("warning"))
				Expect(prompt).To(ContainSubstring("test-namespace"))
				Expect(prompt).To(ContainSubstring("test-resource"))
			})

			It("should not contain format placeholders in output", func() {
				prompt := clientImpl.generatePrompt(testAlert)

				Expect(prompt).ToNot(ContainSubstring("%s"))
				Expect(prompt).ToNot(ContainSubstring("%v"))
				Expect(prompt).ToNot(ContainSubstring("%%"))
			})
		})

		Describe("generateEnhancedPrompt", func() {
			var mcpContext *MCPContext

			BeforeEach(func() {
				mcpContext = &MCPContext{
					ActionHistory: []ActionSummary{
						{
							ActionType:      "restart_pod",
							Confidence:      0.8,
							ExecutionStatus: "completed",
							AlertName:       "TestAlert",
							Timestamp:       time.Now().Add(-1 * time.Hour),
							Effectiveness:   floatPtr(0.9),
						},
					},
					OscillationAnalysis: &OscillationSummary{
						Severity:          "low",
						Confidence:        0.7,
						ThrashingDetected: false,
						ScaleChanges:      1,
						RiskLevel:         "low",
					},
				}
			})

			It("should generate an enhanced prompt with MCP context", func() {
				prompt := clientImpl.generateEnhancedPrompt(testAlert, mcpContext)

				Expect(prompt).ToNot(BeEmpty())
				Expect(prompt).To(ContainSubstring("TestAlert"))
				Expect(prompt).To(ContainSubstring("HISTORICAL CONTEXT"))
				Expect(prompt).To(ContainSubstring("restart_pod"))
				Expect(prompt).To(ContainSubstring("OSCILLATION RISK"))
			})

			It("should not contain format placeholders in output", func() {
				prompt := clientImpl.generateEnhancedPrompt(testAlert, mcpContext)

				Expect(prompt).ToNot(ContainSubstring("%s"))
				Expect(prompt).ToNot(ContainSubstring("%v"))
			})

			It("should handle empty MCP context gracefully", func() {
				emptyContext := &MCPContext{}
				prompt := clientImpl.generateEnhancedPrompt(testAlert, emptyContext)

				Expect(prompt).ToNot(BeEmpty())
				Expect(prompt).To(ContainSubstring("TestAlert"))
			})

			It("should contain escaped percentage signs correctly", func() {
				prompt := clientImpl.generateEnhancedPrompt(testAlert, mcpContext)

				// Enhanced template should contain properly escaped percentages
				Expect(prompt).To(ContainSubstring("90%+"))
				Expect(prompt).To(ContainSubstring("80%"))
				Expect(prompt).To(ContainSubstring("40%"))
				Expect(prompt).To(ContainSubstring("20%"))
			})
		})

		Describe("determineOptimalContextSize", func() {
			It("should use configured max context size when provided", func() {
				clientImpl.config.MaxContextSize = 8000

				size := clientImpl.determineOptimalContextSize("short prompt", nil)

				Expect(size).To(Equal(8000))
			})

			It("should calculate size based on prompt length", func() {
				clientImpl.config.MaxContextSize = 0                         // No limit configured
				longPrompt := strings.Repeat("This is a long prompt. ", 800) // ~20,000 chars = ~5000 tokens

				size := clientImpl.determineOptimalContextSize(longPrompt, nil)

				// Should be more than base size to accommodate the long prompt
				Expect(size).To(BeNumerically(">", 4000))
			})

			It("should add complexity score for MCP context", func() {
				clientImpl.config.MaxContextSize = 0 // No limit configured
				shortPrompt := "short"

				mcpContext := &MCPContext{
					ActionHistory: []ActionSummary{
						{ActionType: "restart_pod"},
						{ActionType: "scale_deployment"},
					},
					OscillationAnalysis: &OscillationSummary{
						Severity: "high",
					},
				}

				baseSize := clientImpl.determineOptimalContextSize(shortPrompt, nil)
				contextSize := clientImpl.determineOptimalContextSize(shortPrompt, mcpContext)

				Expect(contextSize).To(BeNumerically(">", baseSize))
			})

			It("should respect minimum context requirements", func() {
				clientImpl.config.MaxContextSize = 0
				veryLongPrompt := strings.Repeat("x", 20000) // Very long prompt

				size := clientImpl.determineOptimalContextSize(veryLongPrompt, nil)

				// Should accommodate the prompt plus response buffer
				expectedMin := len(veryLongPrompt)/4 + 200
				Expect(size).To(BeNumerically(">=", expectedMin))
			})

			It("should cap at maximum limits", func() {
				clientImpl.config.MaxContextSize = 0
				massivePrompt := strings.Repeat("x", 100000) // Massive prompt

				size := clientImpl.determineOptimalContextSize(massivePrompt, nil)

				Expect(size).To(BeNumerically("<=", 16000))
			})
		})
	})
})
