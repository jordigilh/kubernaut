<<<<<<< HEAD
package holmesgpt_test

import (
	"testing"
	"context"
=======
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
	"context"
	"testing"
>>>>>>> crd_implementation

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
)

var _ = Describe("ToolsetGenerators - Implementation Correctness Testing", func() {
	var (
		log *logrus.Logger
		ctx context.Context
	)

	BeforeEach(func() {
		log = logrus.New()
		log.SetLevel(logrus.ErrorLevel) // Reduce test noise
		ctx = context.Background()
	})

	// BR-HOLMES-023: Unit tests for Prometheus toolset generator implementation
	Describe("PrometheusToolsetGenerator Implementation", func() {
		var generator *holmesgpt.PrometheusToolsetGenerator

		BeforeEach(func() {
			generator = holmesgpt.NewPrometheusToolsetGenerator(log)
		})

		Context("Toolset Generation Logic", func() {
			It("should generate complete Prometheus toolset configuration", func() {
				service := &k8s.DetectedService{
					Name:        "prometheus-server",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{
							Name:     "web",
							URL:      "http://prometheus-server.monitoring.svc.cluster.local:9090",
							Port:     9090,
							Protocol: "http",
						},
					},
					Labels: map[string]string{
						"app.kubernetes.io/name": "prometheus",
					},
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
					},
					Priority: 80,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolset.Name).To(Equal("prometheus-monitoring-prometheus-server"), "BR-AI-001-CONFIDENCE: Prometheus toolset generation must provide functional service identifier for AI confidence requirements")
				Expect(toolset.ServiceType).To(Equal("prometheus"))
				Expect(toolset.Description).To(ContainSubstring("prometheus-server"))
				Expect(toolset.Version).To(Equal("1.0.0"))
				Expect(toolset.Enabled).To(BeTrue())
				Expect(toolset.Priority).To(Equal(80))
			})

			It("should generate correct endpoint mappings", func() {
				service := &k8s.DetectedService{
					Name:        "prometheus",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{
							Name:     "web",
							URL:      "http://prometheus.monitoring.svc.cluster.local:9090",
							Port:     9090,
							Protocol: "http",
						},
					},
					Priority: 80,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolset.Endpoints).To(HaveKey("query"))
				Expect(toolset.Endpoints).To(HaveKey("query_range"))
				Expect(toolset.Endpoints).To(HaveKey("targets"))
				Expect(toolset.Endpoints).To(HaveKey("rules"))

				Expect(toolset.Endpoints["query"]).To(Equal("http://prometheus.monitoring.svc.cluster.local:9090/api/v1/query"))
				Expect(toolset.Endpoints["query_range"]).To(Equal("http://prometheus.monitoring.svc.cluster.local:9090/api/v1/query_range"))
			})

			It("should generate expected capabilities", func() {
				service := &k8s.DetectedService{
					Name:        "prometheus",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{URL: "http://prometheus:9090", Port: 9090},
					},
					Priority: 80,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolset.Capabilities).To(ContainElement("query_metrics"))
				Expect(toolset.Capabilities).To(ContainElement("alert_rules"))
				Expect(toolset.Capabilities).To(ContainElement("time_series"))
				Expect(toolset.Capabilities).To(ContainElement("resource_usage_analysis"))
				Expect(toolset.Capabilities).To(ContainElement("threshold_analysis"))
			})

			It("should generate Prometheus-specific tools", func() {
				service := &k8s.DetectedService{
					Name:        "prometheus",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{URL: "http://prometheus:9090", Port: 9090},
					},
					Priority: 80,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(len(toolset.Tools)).To(BeNumerically(">=", 1), "BR-AI-001-CONFIDENCE: HolmesGPT toolset generators must provide tools for AI confidence requirements")

				// Check for specific tools
				toolNames := make([]string, len(toolset.Tools))
				for i, tool := range toolset.Tools {
					toolNames[i] = tool.Name
				}

				Expect(toolNames).To(ContainElement("prometheus_query"))
				Expect(toolNames).To(ContainElement("prometheus_range_query"))
				Expect(toolNames).To(ContainElement("prometheus_targets"))
				Expect(toolNames).To(ContainElement("prometheus_rules"))
			})

			It("should generate tool with correct parameters", func() {
				service := &k8s.DetectedService{
					Name:        "prometheus",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{URL: "http://prometheus:9090", Port: 9090},
					},
					Priority: 80,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())

				// Find the prometheus_query tool
				var queryTool *holmesgpt.HolmesGPTTool
				for _, tool := range toolset.Tools {
					if tool.Name == "prometheus_query" {
						queryTool = &tool
						break
					}
				}

				Expect(queryTool.Name).To(Equal("prometheus_query"), "BR-AI-001-CONFIDENCE: Query tool generation must provide functional Prometheus query capability for AI confidence requirements")
				Expect(queryTool.Parameters).To(HaveLen(1))
				Expect(queryTool.Parameters[0].Name).To(Equal("query"))
				Expect(queryTool.Parameters[0].Required).To(BeTrue())
				Expect(queryTool.Parameters[0].Type).To(Equal("string"))
			})

			It("should fail generation for service without endpoints", func() {
				service := &k8s.DetectedService{
					Name:        "prometheus-no-endpoints",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints:   []k8s.ServiceEndpoint{}, // No endpoints
					Priority:    80,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("prometheus service has no endpoints"))
				Expect(toolset).To(BeNil())
			})

			It("should include service metadata", func() {
				service := &k8s.DetectedService{
					Name:        "prometheus",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{URL: "http://prometheus:9090", Port: 9090},
					},
					Labels: map[string]string{
						"app": "prometheus",
					},
					Annotations: map[string]string{
						"monitoring.io/enabled": "true",
					},
					Priority: 80,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolset.ServiceMeta.Namespace).To(Equal("monitoring"))
				Expect(toolset.ServiceMeta.ServiceName).To(Equal("prometheus"))
				Expect(toolset.ServiceMeta.Labels).To(HaveKeyWithValue("app", "prometheus"))
				Expect(toolset.ServiceMeta.Annotations).To(HaveKeyWithValue("monitoring.io/enabled", "true"))
			})
		})

		Context("Generator Properties", func() {
			It("should return correct service type", func() {
				Expect(generator.GetServiceType()).To(Equal("prometheus"))
			})

			It("should return correct priority", func() {
				Expect(generator.GetPriority()).To(Equal(80))
			})
		})
	})

	// BR-HOLMES-023: Unit tests for Grafana toolset generator implementation
	Describe("GrafanaToolsetGenerator Implementation", func() {
		var generator *holmesgpt.GrafanaToolsetGenerator

		BeforeEach(func() {
			generator = holmesgpt.NewGrafanaToolsetGenerator(log)
		})

		Context("Toolset Generation Logic", func() {
			It("should generate complete Grafana toolset configuration", func() {
				service := &k8s.DetectedService{
					Name:        "grafana",
					Namespace:   "monitoring",
					ServiceType: "grafana",
					Endpoints: []k8s.ServiceEndpoint{
						{
							Name:     "service",
							URL:      "http://grafana.monitoring.svc.cluster.local:3000",
							Port:     3000,
							Protocol: "http",
						},
					},
					Priority: 70,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolset.ServiceType).To(Equal("grafana"), "BR-AI-001-CONFIDENCE: Grafana toolset generation must provide functional service type for AI confidence requirements")
				Expect(toolset.Name).To(Equal("grafana-monitoring-grafana"))
				Expect(toolset.Priority).To(Equal(70))
				Expect(toolset.Enabled).To(BeTrue())
			})

			It("should generate correct Grafana endpoints", func() {
				service := &k8s.DetectedService{
					Name:        "grafana",
					Namespace:   "monitoring",
					ServiceType: "grafana",
					Endpoints: []k8s.ServiceEndpoint{
						{URL: "http://grafana:3000", Port: 3000},
					},
					Priority: 70,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolset.Endpoints).To(HaveKey("api"))
				Expect(toolset.Endpoints).To(HaveKey("dashboards"))
				Expect(toolset.Endpoints).To(HaveKey("datasources"))

				Expect(toolset.Endpoints["api"]).To(Equal("http://grafana:3000/api"))
				Expect(toolset.Endpoints["dashboards"]).To(Equal("http://grafana:3000/api/search"))
				Expect(toolset.Endpoints["datasources"]).To(Equal("http://grafana:3000/api/datasources"))
			})

			It("should generate Grafana-specific capabilities", func() {
				service := &k8s.DetectedService{
					Name:        "grafana",
					Namespace:   "monitoring",
					ServiceType: "grafana",
					Endpoints: []k8s.ServiceEndpoint{
						{URL: "http://grafana:3000", Port: 3000},
					},
					Priority: 70,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolset.Capabilities).To(ContainElement("get_dashboards"))
				Expect(toolset.Capabilities).To(ContainElement("query_datasource"))
				Expect(toolset.Capabilities).To(ContainElement("get_alerts"))
				Expect(toolset.Capabilities).To(ContainElement("visualization"))
				Expect(toolset.Capabilities).To(ContainElement("dashboard_analysis"))
			})

			It("should generate Grafana-specific tools", func() {
				service := &k8s.DetectedService{
					Name:        "grafana",
					Namespace:   "monitoring",
					ServiceType: "grafana",
					Endpoints: []k8s.ServiceEndpoint{
						{URL: "http://grafana:3000", Port: 3000},
					},
					Priority: 70,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())

				toolNames := make([]string, len(toolset.Tools))
				for i, tool := range toolset.Tools {
					toolNames[i] = tool.Name
				}

				Expect(toolNames).To(ContainElement("grafana_dashboards"))
				Expect(toolNames).To(ContainElement("grafana_datasources"))
				Expect(toolNames).To(ContainElement("grafana_dashboard_info"))
			})

			It("should fail generation for service without endpoints", func() {
				service := &k8s.DetectedService{
					Name:        "grafana-no-endpoints",
					Namespace:   "monitoring",
					ServiceType: "grafana",
					Endpoints:   []k8s.ServiceEndpoint{},
					Priority:    70,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("grafana service has no endpoints"))
				Expect(toolset).To(BeNil())
			})
		})

		It("should return correct service type and priority", func() {
			Expect(generator.GetServiceType()).To(Equal("grafana"))
			Expect(generator.GetPriority()).To(Equal(70))
		})
	})

	// BR-HOLMES-023: Unit tests for Jaeger toolset generator implementation
	Describe("JaegerToolsetGenerator Implementation", func() {
		var generator *holmesgpt.JaegerToolsetGenerator

		BeforeEach(func() {
			generator = holmesgpt.NewJaegerToolsetGenerator(log)
		})

		Context("Toolset Generation Logic", func() {
			It("should generate complete Jaeger toolset configuration", func() {
				service := &k8s.DetectedService{
					Name:        "jaeger-query",
					Namespace:   "observability",
					ServiceType: "jaeger",
					Endpoints: []k8s.ServiceEndpoint{
						{
							Name:     "query-http",
							URL:      "http://jaeger-query.observability.svc.cluster.local:16686",
							Port:     16686,
							Protocol: "http",
						},
					},
					Priority: 60,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolset.ServiceType).To(Equal("jaeger"), "BR-AI-001-CONFIDENCE: Jaeger toolset generation must provide functional service type for AI confidence requirements")
				Expect(toolset.Name).To(Equal("jaeger-observability-jaeger-query"))
				Expect(toolset.Priority).To(Equal(60))
			})

			It("should generate Jaeger-specific capabilities", func() {
				service := &k8s.DetectedService{
					Name:        "jaeger-query",
					Namespace:   "observability",
					ServiceType: "jaeger",
					Endpoints: []k8s.ServiceEndpoint{
						{URL: "http://jaeger:16686", Port: 16686},
					},
					Priority: 60,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolset.Capabilities).To(ContainElement("search_traces"))
				Expect(toolset.Capabilities).To(ContainElement("get_services"))
				Expect(toolset.Capabilities).To(ContainElement("analyze_latency"))
				Expect(toolset.Capabilities).To(ContainElement("distributed_tracing"))
				Expect(toolset.Capabilities).To(ContainElement("service_dependencies"))
			})

			It("should generate Jaeger-specific tools with parameters", func() {
				service := &k8s.DetectedService{
					Name:        "jaeger-query",
					Namespace:   "observability",
					ServiceType: "jaeger",
					Endpoints: []k8s.ServiceEndpoint{
						{URL: "http://jaeger:16686", Port: 16686},
					},
					Priority: 60,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())

				// Find jaeger_traces tool
				var tracesTool *holmesgpt.HolmesGPTTool
				for _, tool := range toolset.Tools {
					if tool.Name == "jaeger_traces" {
						tracesTool = &tool
						break
					}
				}

				Expect(tracesTool.Name).To(Equal("jaeger_traces"), "BR-AI-001-CONFIDENCE: Traces tool generation must provide functional tracing capability for AI confidence requirements")
				Expect(len(tracesTool.Parameters)).To(BeNumerically(">=", 1), "BR-AI-001-CONFIDENCE: HolmesGPT toolset generators must provide tool parameters for AI confidence requirements")

				// Check required service parameter
				var serviceParam *holmesgpt.ToolParameter
				for _, param := range tracesTool.Parameters {
					if param.Name == "service" {
						serviceParam = &param
						break
					}
				}

				Expect(serviceParam.Name).To(Equal("service"), "BR-AI-001-CONFIDENCE: Service parameter generation must provide functional service identification for AI confidence requirements")
				Expect(serviceParam.Required).To(BeTrue())
				Expect(serviceParam.Type).To(Equal("string"))
			})
		})

		It("should return correct service type and priority", func() {
			Expect(generator.GetServiceType()).To(Equal("jaeger"))
			Expect(generator.GetPriority()).To(Equal(60))
		})
	})

	// BR-HOLMES-023: Unit tests for Elasticsearch toolset generator implementation
	Describe("ElasticsearchToolsetGenerator Implementation", func() {
		var generator *holmesgpt.ElasticsearchToolsetGenerator

		BeforeEach(func() {
			generator = holmesgpt.NewElasticsearchToolsetGenerator(log)
		})

		Context("Toolset Generation Logic", func() {
			It("should generate complete Elasticsearch toolset configuration", func() {
				service := &k8s.DetectedService{
					Name:        "elasticsearch",
					Namespace:   "logging",
					ServiceType: "elasticsearch",
					Endpoints: []k8s.ServiceEndpoint{
						{
							Name:     "http",
							URL:      "http://elasticsearch.logging.svc.cluster.local:9200",
							Port:     9200,
							Protocol: "http",
						},
					},
					Priority: 50,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolset.ServiceType).To(Equal("elasticsearch"), "BR-AI-001-CONFIDENCE: Elasticsearch toolset generation must provide functional service type for AI confidence requirements")
				Expect(toolset.Name).To(Equal("elasticsearch-logging-elasticsearch"))
				Expect(toolset.Priority).To(Equal(50))
			})

			It("should generate Elasticsearch-specific capabilities", func() {
				service := &k8s.DetectedService{
					Name:        "elasticsearch",
					Namespace:   "logging",
					ServiceType: "elasticsearch",
					Endpoints: []k8s.ServiceEndpoint{
						{URL: "http://elasticsearch:9200", Port: 9200},
					},
					Priority: 50,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolset.Capabilities).To(ContainElement("search_logs"))
				Expect(toolset.Capabilities).To(ContainElement("analyze_patterns"))
				Expect(toolset.Capabilities).To(ContainElement("aggregation"))
				Expect(toolset.Capabilities).To(ContainElement("log_analysis"))
				Expect(toolset.Capabilities).To(ContainElement("full_text_search"))
			})

			It("should generate Elasticsearch-specific tools", func() {
				service := &k8s.DetectedService{
					Name:        "elasticsearch",
					Namespace:   "logging",
					ServiceType: "elasticsearch",
					Endpoints: []k8s.ServiceEndpoint{
						{URL: "http://elasticsearch:9200", Port: 9200},
					},
					Priority: 50,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())

				toolNames := make([]string, len(toolset.Tools))
				for i, tool := range toolset.Tools {
					toolNames[i] = tool.Name
				}

				Expect(toolNames).To(ContainElement("elasticsearch_search"))
				Expect(toolNames).To(ContainElement("elasticsearch_indices"))
				Expect(toolNames).To(ContainElement("elasticsearch_cluster_health"))
			})
		})

		It("should return correct service type and priority", func() {
			Expect(generator.GetServiceType()).To(Equal("elasticsearch"))
			Expect(generator.GetPriority()).To(Equal(50))
		})
	})

	// BR-HOLMES-018: Unit tests for Custom toolset generator implementation
	Describe("CustomToolsetGenerator Implementation", func() {
		var generator *holmesgpt.CustomToolsetGenerator

		BeforeEach(func() {
			generator = holmesgpt.NewCustomToolsetGenerator(log)
		})

		Context("Toolset Generation Logic", func() {
			It("should generate toolset from custom service annotations", func() {
				service := &k8s.DetectedService{
					Name:        "vector-db",
					Namespace:   "ai",
					ServiceType: "vector-database",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "query", URL: "http://vector-db:8080", Port: 8080},
						{Name: "admin", URL: "http://vector-db:8081", Port: 8081},
					},
					Capabilities: []string{
						"vector_search", "similarity_analysis", "embeddings",
					},
					Annotations: map[string]string{
						"kubernaut.io/toolset":         "vector-database",
						"kubernaut.io/health-endpoint": "/healthz",
					},
					Priority: 40,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolset.ServiceType).To(Equal("vector-database"), "BR-AI-001-CONFIDENCE: Vector database toolset generation must provide functional service type for AI confidence requirements")
				Expect(toolset.Name).To(Equal("vector-database-ai-vector-db"))
				Expect(toolset.Priority).To(Equal(40))
			})

			It("should use custom service type from annotations", func() {
				service := &k8s.DetectedService{
					Name:        "ml-inference",
					Namespace:   "ai",
					ServiceType: "custom",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "api", URL: "http://ml-inference:8000", Port: 8000},
					},
					Annotations: map[string]string{
						"kubernaut.io/toolset": "ml-inference-engine",
					},
					Priority: 35,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolset.ServiceType).To(Equal("ml-inference-engine"))
				Expect(toolset.Name).To(Equal("ml-inference-engine-ai-ml-inference"))
			})

			It("should generate basic tools for each endpoint", func() {
				service := &k8s.DetectedService{
					Name:        "multi-endpoint-service",
					Namespace:   "services",
					ServiceType: "custom-api",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "api", URL: "http://service:8080", Port: 8080},
						{Name: "metrics", URL: "http://service:9090", Port: 9090},
						{Name: "health", URL: "http://service:8081", Port: 8081},
					},
					Priority: 30,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolset.Tools).To(HaveLen(3)) // One tool per endpoint

				toolNames := make([]string, len(toolset.Tools))
				for i, tool := range toolset.Tools {
					toolNames[i] = tool.Name
				}

				Expect(toolNames).To(ContainElement("api_api_call"))
				Expect(toolNames).To(ContainElement("metrics_api_call"))
				Expect(toolNames).To(ContainElement("health_api_call"))
			})

			It("should generate capability-specific tools", func() {
				service := &k8s.DetectedService{
					Name:        "analytics-service",
					Namespace:   "analytics",
					ServiceType: "analytics",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "api", URL: "http://analytics:8080", Port: 8080},
					},
					Capabilities: []string{
						"query_logs", "analyze_patterns", "custom_metric",
					},
					Priority: 30,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())

				// Should have basic API tool + capability-specific tools
				Expect(len(toolset.Tools)).To(BeNumerically(">=", 4))

				toolNames := make([]string, len(toolset.Tools))
				for i, tool := range toolset.Tools {
					toolNames[i] = tool.Name
				}

				Expect(toolNames).To(ContainElement("query_logs"))
				Expect(toolNames).To(ContainElement("analyze_patterns"))
				Expect(toolNames).To(ContainElement("get_metrics"))
			})

			It("should use custom health endpoint from annotation", func() {
				service := &k8s.DetectedService{
					Name:        "custom-health-service",
					Namespace:   "services",
					ServiceType: "custom",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "api", URL: "http://service:8080", Port: 8080},
					},
					Annotations: map[string]string{
						"kubernaut.io/toolset":         "custom",
						"kubernaut.io/health-endpoint": "/custom-health",
					},
					Priority: 30,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolset.HealthCheck.Endpoint).To(Equal("/custom-health"))
			})

			It("should fail generation for service without endpoints", func() {
				service := &k8s.DetectedService{
					Name:        "no-endpoints-custom",
					Namespace:   "services",
					ServiceType: "custom",
					Endpoints:   []k8s.ServiceEndpoint{},
					Priority:    30,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("custom service has no endpoints"))
				Expect(toolset).To(BeNil())
			})
		})

		Context("Endpoint Mapping Logic", func() {
			It("should create endpoint map from service endpoints", func() {
				service := &k8s.DetectedService{
					Name:        "endpoint-mapping-test",
					Namespace:   "test",
					ServiceType: "custom",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "primary", URL: "http://service:8080", Port: 8080},
						{Name: "secondary", URL: "http://service:8081", Port: 8081},
					},
					Priority: 30,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(toolset.Endpoints).To(HaveKeyWithValue("primary", "http://service:8080"))
				Expect(toolset.Endpoints).To(HaveKeyWithValue("secondary", "http://service:8081"))
			})
		})

		Context("Tool Generation Logic", func() {
			It("should generate query_logs tool with correct parameters", func() {
				service := &k8s.DetectedService{
					Name:        "log-service",
					Namespace:   "logging",
					ServiceType: "log-analytics",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "api", URL: "http://logs:8080", Port: 8080},
					},
					Capabilities: []string{"query_logs"},
					Priority:     30,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())

				// Find the query_logs tool
				var queryLogsTool *holmesgpt.HolmesGPTTool
				for _, tool := range toolset.Tools {
					if tool.Name == "query_logs" {
						queryLogsTool = &tool
						break
					}
				}

				Expect(queryLogsTool.Name).To(Equal("query_logs"), "BR-AI-001-CONFIDENCE: Query logs tool generation must provide functional log querying capability for AI confidence requirements")
				Expect(queryLogsTool.Parameters).To(HaveLen(2)) // query and limit

				// Check query parameter
				var queryParam *holmesgpt.ToolParameter
				for _, param := range queryLogsTool.Parameters {
					if param.Name == "query" {
						queryParam = &param
						break
					}
				}

				Expect(queryParam.Name).To(Equal("query"), "BR-AI-001-CONFIDENCE: Query parameter generation must provide functional query specification for AI confidence requirements")
				Expect(queryParam.Required).To(BeTrue())
			})

			It("should handle unknown capabilities gracefully", func() {
				service := &k8s.DetectedService{
					Name:        "unknown-capability-service",
					Namespace:   "test",
					ServiceType: "unknown",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "api", URL: "http://service:8080", Port: 8080},
					},
					Capabilities: []string{"unknown_capability", "another_unknown"},
					Priority:     30,
				}

				toolset, err := generator.Generate(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				// Should still generate basic API tools even with unknown capabilities
				Expect(len(toolset.Tools)).To(BeNumerically(">=", 1))
			})
		})

		It("should return correct service type and priority", func() {
			Expect(generator.GetServiceType()).To(Equal("custom"))
			Expect(generator.GetPriority()).To(Equal(30))
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUtoolsetUgenerators(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UtoolsetUgenerators Suite")
}
