package llm

import (
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
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
			func(cfg config.LLMConfig, expectErr bool, errString string) {
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
				config.LLMConfig{
					Provider: "localai",
					Endpoint: "http://localhost:8080",
					Model:    "test-model",
					Timeout:  30 * time.Second,
				},
				false,
				"",
			),
			Entry("invalid provider",
				config.LLMConfig{
					Provider: "invalid",
					Endpoint: "http://localhost:8080",
					Model:    "test-model",
				},
				true,
				"unsupported provider: invalid",
			),
		)
	})

	Describe("Template Constants", func() {
		Describe("promptTemplate", func() {
			It("should have the correct number of format placeholders", func() {
				// Count format placeholders (%s, %v)
				placeholders := strings.Count(promptTemplate, "%s") + strings.Count(promptTemplate, "%v")
				Expect(placeholders).To(Equal(8), "promptTemplate should have exactly 8 format placeholders")
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
					if strings.Contains(promptTemplate, pattern) {
						Fail("Found unescaped percentage pattern: " + pattern + " (should be escaped as %%)")
					}
				}
			})

			It("should contain essential prompt sections", func() {
				Expect(promptTemplate).To(ContainSubstring("<|system|>"))
				Expect(promptTemplate).To(ContainSubstring("<|user|>"))
				Expect(promptTemplate).To(ContainSubstring("<|assistant|>"))
				Expect(promptTemplate).To(ContainSubstring("CRITICAL DECISION RULES"))
				Expect(promptTemplate).To(ContainSubstring("AVAILABLE ACTIONS"))
				Expect(promptTemplate).To(ContainSubstring("confidence"))
			})
		})
	})

	Describe("Prompt Generation", func() {
		var (
			clientImpl *client
			testAlert  types.Alert
		)

		BeforeEach(func() {
			cfg := config.LLMConfig{
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

	})
})
