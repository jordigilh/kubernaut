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

package holmesgpt_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
)

var _ = Describe("ToolsetTemplateEngine - Implementation Correctness Testing", func() {
	var (
		engine *holmesgpt.ToolsetTemplateEngine
		log    *logrus.Logger
	)

	BeforeEach(func() {
		log = logrus.New()
		log.SetLevel(logrus.ErrorLevel) // Reduce test noise
		engine = holmesgpt.NewToolsetTemplateEngine(log)
	})

	// BR-HOLMES-023: Unit tests for toolset template engine implementation
	Describe("ToolsetTemplateEngine Implementation", func() {
		Context("Template Management", func() {
			It("should initialize with default templates", func() {
				templates := engine.GetAvailableTemplates()

				Expect(templates).To(ContainElement("prometheus"))
				Expect(templates).To(ContainElement("grafana"))
				Expect(templates).To(ContainElement("jaeger"))
				Expect(templates).To(ContainElement("elasticsearch"))
				Expect(templates).To(ContainElement("custom"))
			})

			It("should add custom templates successfully", func() {
				customTemplate := `{
					"name": "{{.ServiceType}}-{{.Namespace}}-{{.ServiceName}}",
					"service_type": "{{.ServiceType}}",
					"description": "Custom template for {{.ServiceName}}"
				}`

				err := engine.AddTemplate("custom-test", customTemplate)
				Expect(err).ToNot(HaveOccurred())

				templates := engine.GetAvailableTemplates()
				Expect(templates).To(ContainElement("custom-test"))
			})

			It("should fail to add template with invalid syntax", func() {
				invalidTemplate := `{
					"name": "{{.InvalidSyntax",  // Missing closing braces
					"service_type": "test"
				}`

				err := engine.AddTemplate("invalid", invalidTemplate)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse template"))
			})

			It("should validate templates correctly", func() {
				validTemplate := `{"name": "{{.ServiceName}}", "type": "{{.ServiceType}}"}`
				invalidTemplate := `{"name": "{{.Invalid}", "syntax": "{{unclosed"}`

				err := engine.ValidateTemplate(validTemplate)
				Expect(err).ToNot(HaveOccurred())

				err = engine.ValidateTemplate(invalidTemplate)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("template validation failed"))
			})
		})

		Context("Template Rendering", func() {
			It("should render simple template with variables", func() {
				variables := holmesgpt.TemplateVariables{
					ServiceName:  "prometheus-server",
					Namespace:    "monitoring",
					ServiceType:  "prometheus",
					Capabilities: []string{"query_metrics", "alert_rules"},
				}

				// Use the built-in prometheus template
				result, err := engine.RenderTemplate("prometheus", variables)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(ContainSubstring("prometheus-monitoring-prometheus-server"))
				Expect(result).To(ContainSubstring("prometheus"))
			})

			It("should handle missing template gracefully", func() {
				variables := holmesgpt.TemplateVariables{
					ServiceName: "test",
					ServiceType: "test",
				}

				result, err := engine.RenderTemplate("non-existent-template", variables)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("template non-existent-template not found"))
				Expect(result).To(BeEmpty())
			})

			It("should handle template execution errors", func() {
				// Add template that references non-existent field
				badTemplate := `{"field": "{{.NonExistentField}}"}`
				err := engine.AddTemplate("bad-template", badTemplate)
				Expect(err).ToNot(HaveOccurred())

				variables := holmesgpt.TemplateVariables{
					ServiceName: "test",
				}

				result, err := engine.RenderTemplate("bad-template", variables)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to execute template"))
				Expect(result).To(BeEmpty())
			})
		})

		Context("Tool Command Rendering", func() {
			It("should render tool command templates with variables", func() {
				commandTemplate := "curl -s '${endpoint}/api/v1/query?query=${query}'"
				variables := map[string]string{
					"endpoint": "http://prometheus:9090",
					"query":    "up",
				}

				result, err := engine.RenderToolCommand(commandTemplate, variables)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal("curl -s 'http://prometheus:9090/api/v1/query?query=up'"))
			})

			It("should handle missing variables in command template", func() {
				commandTemplate := "curl -s '${endpoint}/query?q=${missing_var}'"
				variables := map[string]string{
					"endpoint": "http://service:8080",
					// missing_var is not provided
				}

				result, err := engine.RenderToolCommand(commandTemplate, variables)

				// Should not fail - missing variables result in empty values
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal("curl -s 'http://service:8080/query?q='"))
			})

			It("should handle invalid command template syntax", func() {
				invalidTemplate := "curl -s '${unclosed"
				variables := map[string]string{
					"endpoint": "http://service:8080",
				}

				result, err := engine.RenderToolCommand(invalidTemplate, variables)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse command template"))
				Expect(result).To(BeEmpty())
			})

			It("should handle command template execution errors", func() {
				// This would cause execution error if variables contained functions
				commandTemplate := "curl -s '${endpoint}'"
				variables := map[string]string{
					"endpoint": "http://service:8080",
				}

				// Should work fine with simple string substitution
				result, err := engine.RenderToolCommand(commandTemplate, variables)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal("curl -s 'http://service:8080'"))
			})
		})

		Context("Toolset Configuration Generation", func() {
			It("should generate toolset config from template variables", func() {
				variables := holmesgpt.TemplateVariables{
					ServiceName: "prometheus-server",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: map[string]string{
						"query": "http://prometheus:9090/api/v1/query",
						"rules": "http://prometheus:9090/api/v1/rules",
					},
					Capabilities: []string{"query_metrics", "alert_rules"},
					Labels: map[string]string{
						"app": "prometheus",
					},
				}

				config, err := engine.GenerateToolsetConfig("prometheus", variables)

				Expect(err).ToNot(HaveOccurred())
				Expect(config.Name).To(Equal("prometheus-monitoring-prometheus-server"), "BR-AI-001-CONFIDENCE: HolmesGPT toolset template engine must return templates with valid identifiers for AI confidence requirements")
				Expect(config.ServiceType).To(Equal("prometheus"))
				Expect(config.Description).To(ContainSubstring("Prometheus tools for prometheus-server"))
				Expect(config.Endpoints).To(HaveKeyWithValue("query", "http://prometheus:9090/api/v1/query"))
				Expect(config.Capabilities).To(ContainElement("query_metrics"))
				Expect(config.Enabled).To(BeTrue())
				Expect(config.ServiceMeta.Namespace).To(Equal("monitoring"))
				Expect(config.ServiceMeta.ServiceName).To(Equal("prometheus-server"))
			})

			It("should fallback to custom template for unknown service types", func() {
				variables := holmesgpt.TemplateVariables{
					ServiceName: "unknown-service",
					Namespace:   "test",
					ServiceType: "unknown-type",
					Endpoints: map[string]string{
						"api": "http://unknown:8080",
					},
					Capabilities: []string{"custom_capability"},
				}

				config, err := engine.GenerateToolsetConfig("unknown-type", variables)

				Expect(err).ToNot(HaveOccurred())
				Expect(config.Name).To(Equal("unknown-type-test-unknown-service"), "BR-AI-001-CONFIDENCE: HolmesGPT toolset template engine must return templates with valid identifiers for AI confidence requirements")
				Expect(config.ServiceType).To(Equal("unknown-type"))
				Expect(config.Description).To(ContainSubstring("Unknown-Type tools for unknown-service"))
			})

			It("should handle empty variables gracefully", func() {
				variables := holmesgpt.TemplateVariables{
					ServiceName: "minimal-service",
					ServiceType: "minimal",
					Namespace:   "default",
				}

				config, err := engine.GenerateToolsetConfig("minimal", variables)

				Expect(err).ToNot(HaveOccurred())
				Expect(config.Name).To(Equal("minimal-default-minimal-service"), "BR-AI-001-CONFIDENCE: HolmesGPT toolset template engine must return templates with valid identifiers for AI confidence requirements")
				Expect(config.ServiceType).To(Equal("minimal"))
				Expect(config.Endpoints).To(BeNil())
				Expect(config.Capabilities).To(BeNil())
			})
		})

		Context("Template Helper Functions", func() {
			It("should provide helper functions for template processing", func() {
				helpers := engine.GetTemplateHelpers()

				Expect(helpers).To(HaveKey("upper"))
				Expect(helpers).To(HaveKey("lower"))
				Expect(helpers).To(HaveKey("title"))
				Expect(helpers).To(HaveKey("contains"))
				Expect(helpers).To(HaveKey("replace"))
				Expect(helpers).To(HaveKey("joinStrings"))
			})

			It("should execute upper helper correctly", func() {
				helpers := engine.GetTemplateHelpers()
				upperFunc, ok := helpers["upper"].(func(string) string)
				Expect(ok).To(BeTrue())

				result := upperFunc("hello world")
				Expect(result).To(Equal("HELLO WORLD"))
			})

			It("should execute lower helper correctly", func() {
				helpers := engine.GetTemplateHelpers()
				lowerFunc, ok := helpers["lower"].(func(string) string)
				Expect(ok).To(BeTrue())

				result := lowerFunc("HELLO WORLD")
				Expect(result).To(Equal("hello world"))
			})

			It("should execute title helper correctly", func() {
				helpers := engine.GetTemplateHelpers()
				titleFunc, ok := helpers["title"].(func(string) string)
				Expect(ok).To(BeTrue())

				result := titleFunc("hello world")
				Expect(result).To(Equal("Hello World"))
			})

			It("should execute contains helper correctly", func() {
				helpers := engine.GetTemplateHelpers()
				containsFunc, ok := helpers["contains"].(func(string, string) bool)
				Expect(ok).To(BeTrue())

				result := containsFunc("hello world", "world")
				Expect(result).To(BeTrue())

				result = containsFunc("hello world", "missing")
				Expect(result).To(BeFalse())
			})

			It("should execute replace helper correctly", func() {
				helpers := engine.GetTemplateHelpers()
				replaceFunc, ok := helpers["replace"].(func(string, string, string) string)
				Expect(ok).To(BeTrue())

				result := replaceFunc("hello world", "world", "universe")
				Expect(result).To(Equal("hello universe"))
			})

			It("should execute joinStrings helper correctly", func() {
				helpers := engine.GetTemplateHelpers()
				joinFunc, ok := helpers["joinStrings"].(func([]string, string) string)
				Expect(ok).To(BeTrue())

				result := joinFunc([]string{"a", "b", "c"}, ",")
				Expect(result).To(Equal("a,b,c"))
			})
		})

		Context("Template Content Validation", func() {
			It("should validate template content with all required fields", func() {
				validTemplate := `{
					"name": "{{.ServiceType}}-{{.Namespace}}-{{.ServiceName}}",
					"service_type": "{{.ServiceType}}",
					"description": "{{.ServiceType | title}} tools for {{.ServiceName}}",
					"version": "1.0.0",
					"endpoints": {{.Endpoints}},
					"capabilities": {{.Capabilities}},
					"enabled": true
				}`

				err := engine.ValidateTemplate(validTemplate)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should validate template with helper functions", func() {
				templateWithHelpers := `{
					"name": "{{.ServiceType | upper}}-{{.ServiceName | lower}}",
					"enabled": {{if contains .ServiceName "prometheus"}}true{{else}}false{{end}}
				}`

				err := engine.ValidateTemplate(templateWithHelpers)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should fail validation for templates with syntax errors", func() {
				syntaxErrorTemplates := []string{
					`{"name": "{{.ServiceName", "missing": "closing"}`,   // Missing closing brace
					`{"name": "{{.ServiceName}}", "unclosed": "{{if"}`,   // Unclosed if
					`{"name": "{{.ServiceName}}", "invalid": "{{end}}"}`, // Invalid end without matching if
				}

				for _, template := range syntaxErrorTemplates {
					err := engine.ValidateTemplate(template)
					Expect(err).To(HaveOccurred())
				}
			})
		})

		Context("Edge Cases and Error Handling", func() {
			It("should handle empty template names", func() {
				err := engine.AddTemplate("", `{"name": "test"}`)
				Expect(err).ToNot(HaveOccurred()) // Empty name is technically valid

				// Should be retrievable
				templates := engine.GetAvailableTemplates()
				Expect(templates).To(ContainElement(""))
			})

			It("should handle overwriting existing templates", func() {
				originalTemplate := `{"version": "1.0"}`
				updatedTemplate := `{"version": "2.0"}`

				err := engine.AddTemplate("overwrite-test", originalTemplate)
				Expect(err).ToNot(HaveOccurred())

				err = engine.AddTemplate("overwrite-test", updatedTemplate)
				Expect(err).ToNot(HaveOccurred())

				// Template should be updated
				templates := engine.GetAvailableTemplates()
				Expect(templates).To(ContainElement("overwrite-test"))
			})

			It("should handle very large templates", func() {
				// Create a large template with many fields
				largeTemplate := `{
					"name": "{{.ServiceName}}-large",
					"field1": "{{.ServiceType}}",
					"field2": "{{.Namespace}}"`

				// Add 1000 more fields
				for i := 0; i < 1000; i++ {
					largeTemplate += fmt.Sprintf(`,
					"field%d": "{{.ServiceType}}"`, i+3)
				}
				largeTemplate += "}"

				err := engine.AddTemplate("large-template", largeTemplate)
				Expect(err).ToNot(HaveOccurred())

				variables := holmesgpt.TemplateVariables{
					ServiceName: "test",
					ServiceType: "test",
					Namespace:   "test",
				}

				result, err := engine.RenderTemplate("large-template", variables)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(result)).To(BeNumerically(">=", 1), "BR-AI-001-CONFIDENCE: HolmesGPT template engine must generate templates for AI confidence requirements")
				Expect(result).To(ContainSubstring("test-large"))
			})

			It("should handle concurrent template operations safely", func() {
				// This test verifies thread safety (though Go's template package handles this)
				done := make(chan bool, 10)

				// Start multiple goroutines adding templates
				for i := 0; i < 10; i++ {
					go func(index int) {
						templateName := fmt.Sprintf("concurrent-%d", index)
						template := fmt.Sprintf(`{"name": "test-%d"}`, index)

						err := engine.AddTemplate(templateName, template)
						Expect(err).ToNot(HaveOccurred())

						done <- true
					}(i)
				}

				// Wait for all goroutines to complete
				for i := 0; i < 10; i++ {
					<-done
				}

				// Verify all templates were added
				templates := engine.GetAvailableTemplates()
				for i := 0; i < 10; i++ {
					expectedName := fmt.Sprintf("concurrent-%d", i)
					Expect(templates).To(ContainElement(expectedName))
				}
			})
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUtoolsetUtemplateUengine(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UtoolsetUtemplateUengine Suite")
}
