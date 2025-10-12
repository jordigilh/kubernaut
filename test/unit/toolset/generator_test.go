package toolset_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/toolset"
	"github.com/jordigilh/kubernaut/pkg/toolset/generator"
)

var _ = Describe("BR-TOOLSET-027: Toolset Generator", func() {
	var (
		gen generator.ToolsetGenerator
		ctx context.Context
	)

	BeforeEach(func() {
		gen = generator.NewHolmesGPTGenerator()
		ctx = context.Background()
	})

	Describe("GenerateToolset", func() {
		Context("with discovered services", func() {
			It("should generate valid HolmesGPT toolset JSON", func() {
				services := []*toolset.DiscoveredService{
					{
						Name:      "prometheus",
						Namespace: "monitoring",
						Type:      "prometheus",
						Endpoint:  "http://prometheus.monitoring:9090",
						Annotations: map[string]string{
							"app": "prometheus",
						},
						Metadata: map[string]string{
							"version": "2.45.0",
						},
						DiscoveredAt: time.Now(),
					},
					{
						Name:      "grafana",
						Namespace: "monitoring",
						Type:      "grafana",
						Endpoint:  "http://grafana.monitoring:3000",
						Annotations: map[string]string{
							"app": "grafana",
						},
						Metadata:     map[string]string{},
						DiscoveredAt: time.Now(),
					},
				}

				toolsetJSON, err := gen.GenerateToolset(ctx, services)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolsetJSON).ToNot(BeEmpty())

				// Validate it's valid JSON
				var result map[string]interface{}
				err = json.Unmarshal([]byte(toolsetJSON), &result)
				Expect(err).ToNot(HaveOccurred())

				// Should have tools array
				tools, ok := result["tools"].([]interface{})
				Expect(ok).To(BeTrue())
				Expect(tools).To(HaveLen(2))
			})

			It("should include service metadata in toolset", func() {
				services := []*toolset.DiscoveredService{
					{
						Name:      "elasticsearch",
						Namespace: "logging",
						Type:      "elasticsearch",
						Endpoint:  "http://elasticsearch.logging:9200",
						Metadata: map[string]string{
							"cluster_name": "prod-logs",
						},
						DiscoveredAt: time.Now(),
					},
				}

				toolsetJSON, err := gen.GenerateToolset(ctx, services)

				Expect(err).ToNot(HaveOccurred())

				var result map[string]interface{}
				json.Unmarshal([]byte(toolsetJSON), &result)

				tools := result["tools"].([]interface{})
				Expect(tools).To(HaveLen(1))

				tool := tools[0].(map[string]interface{})
				Expect(tool["name"]).To(Equal("elasticsearch"))
				Expect(tool["endpoint"]).To(Equal("http://elasticsearch.logging:9200"))
			})

			It("should handle empty service list", func() {
				services := []*toolset.DiscoveredService{}

				toolsetJSON, err := gen.GenerateToolset(ctx, services)

				Expect(err).ToNot(HaveOccurred())

				var result map[string]interface{}
				json.Unmarshal([]byte(toolsetJSON), &result)

				tools := result["tools"].([]interface{})
				Expect(tools).To(BeEmpty())
			})

			It("should deduplicate services by name and namespace", func() {
				services := []*toolset.DiscoveredService{
					{
						Name:         "prometheus",
						Namespace:    "monitoring",
						Type:         "prometheus",
						Endpoint:     "http://prometheus.monitoring:9090",
						DiscoveredAt: time.Now(),
					},
					{
						Name:         "prometheus",
						Namespace:    "monitoring",
						Type:         "prometheus",
						Endpoint:     "http://prometheus.monitoring:9090",
						DiscoveredAt: time.Now().Add(1 * time.Minute),
					},
				}

				toolsetJSON, err := gen.GenerateToolset(ctx, services)

				Expect(err).ToNot(HaveOccurred())

				var result map[string]interface{}
				json.Unmarshal([]byte(toolsetJSON), &result)

				tools := result["tools"].([]interface{})
				Expect(tools).To(HaveLen(1))
			})
		})

		Context("BR-TOOLSET-028: with HolmesGPT format requirements", func() {
			It("should generate correct tool structure", func() {
				services := []*toolset.DiscoveredService{
					{
						Name:      "jaeger",
						Namespace: "tracing",
						Type:      "jaeger",
						Endpoint:  "http://jaeger.tracing:16686",
						Metadata: map[string]string{
							"ui_path": "/search",
						},
						DiscoveredAt: time.Now(),
					},
				}

				toolsetJSON, err := gen.GenerateToolset(ctx, services)

				Expect(err).ToNot(HaveOccurred())

				var result map[string]interface{}
				json.Unmarshal([]byte(toolsetJSON), &result)

				tools := result["tools"].([]interface{})
				tool := tools[0].(map[string]interface{})

				// HolmesGPT required fields
				Expect(tool).To(HaveKey("name"))
				Expect(tool).To(HaveKey("type"))
				Expect(tool).To(HaveKey("endpoint"))
				Expect(tool).To(HaveKey("description"))
			})

			It("should use service type for tool type", func() {
				services := []*toolset.DiscoveredService{
					{
						Name:         "custom-api",
						Namespace:    "default",
						Type:         "custom-api",
						Endpoint:     "http://custom-api:8080",
						DiscoveredAt: time.Now(),
					},
				}

				toolsetJSON, err := gen.GenerateToolset(ctx, services)

				Expect(err).ToNot(HaveOccurred())

				var result map[string]interface{}
				json.Unmarshal([]byte(toolsetJSON), &result)

				tools := result["tools"].([]interface{})
				tool := tools[0].(map[string]interface{})

				Expect(tool["type"]).To(Equal("custom-api"))
			})

			It("should generate human-readable descriptions", func() {
				services := []*toolset.DiscoveredService{
					{
						Name:         "prometheus",
						Namespace:    "monitoring",
						Type:         "prometheus",
						Endpoint:     "http://prometheus.monitoring:9090",
						DiscoveredAt: time.Now(),
					},
				}

				toolsetJSON, err := gen.GenerateToolset(ctx, services)

				Expect(err).ToNot(HaveOccurred())

				var result map[string]interface{}
				json.Unmarshal([]byte(toolsetJSON), &result)

				tools := result["tools"].([]interface{})
				tool := tools[0].(map[string]interface{})

				description := tool["description"].(string)
				Expect(description).ToNot(BeEmpty())
				Expect(description).To(ContainSubstring("prometheus"))
				Expect(description).To(ContainSubstring("monitoring"))
			})
		})

		Context("with metadata preservation", func() {
			It("should preserve all service metadata", func() {
				services := []*toolset.DiscoveredService{
					{
						Name:      "service-with-metadata",
						Namespace: "default",
						Type:      "custom",
						Endpoint:  "http://service:8080",
						Metadata: map[string]string{
							"health_path": "/healthz",
							"version":     "1.2.3",
							"custom_key":  "custom_value",
						},
						DiscoveredAt: time.Now(),
					},
				}

				toolsetJSON, err := gen.GenerateToolset(ctx, services)

				Expect(err).ToNot(HaveOccurred())

				var result map[string]interface{}
				json.Unmarshal([]byte(toolsetJSON), &result)

				tools := result["tools"].([]interface{})
				tool := tools[0].(map[string]interface{})

				metadata := tool["metadata"].(map[string]interface{})
				Expect(metadata["health_path"]).To(Equal("/healthz"))
				Expect(metadata["version"]).To(Equal("1.2.3"))
				Expect(metadata["custom_key"]).To(Equal("custom_value"))
			})

			It("should handle services without metadata", func() {
				services := []*toolset.DiscoveredService{
					{
						Name:         "service-no-metadata",
						Namespace:    "default",
						Type:         "custom",
						Endpoint:     "http://service:8080",
						Metadata:     map[string]string{},
						DiscoveredAt: time.Now(),
					},
				}

				toolsetJSON, err := gen.GenerateToolset(ctx, services)

				Expect(err).ToNot(HaveOccurred())

				var result map[string]interface{}
				json.Unmarshal([]byte(toolsetJSON), &result)

				tools := result["tools"].([]interface{})
				tool := tools[0].(map[string]interface{})

				// Metadata should be present but empty
				metadata, ok := tool["metadata"].(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(metadata).To(BeEmpty())
			})
		})
	})

	Describe("ValidateToolset", func() {
		It("should validate correct toolset JSON", func() {
			validJSON := `{
				"tools": [
					{
						"name": "prometheus",
						"type": "prometheus",
						"endpoint": "http://prometheus:9090",
						"description": "Prometheus monitoring",
						"metadata": {}
					}
				]
			}`

			err := gen.ValidateToolset(ctx, validJSON)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject invalid JSON", func() {
			invalidJSON := `{invalid json`

			err := gen.ValidateToolset(ctx, invalidJSON)
			Expect(err).To(HaveOccurred())
		})

		It("should reject toolset without tools array", func() {
			noToolsJSON := `{
				"services": []
			}`

			err := gen.ValidateToolset(ctx, noToolsJSON)
			Expect(err).To(HaveOccurred())
		})

		It("should reject tool without required fields", func() {
			missingFieldsJSON := `{
				"tools": [
					{
						"name": "test"
					}
				]
			}`

			err := gen.ValidateToolset(ctx, missingFieldsJSON)
			Expect(err).To(HaveOccurred())
		})
	})
})
