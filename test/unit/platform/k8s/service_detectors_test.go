package k8s_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
)

var _ = Describe("ServiceDetectors - Implementation Correctness Testing", func() {
	var (
		log *logrus.Logger
		ctx context.Context
	)

	BeforeEach(func() {
		log = logrus.New()
		log.SetLevel(logrus.ErrorLevel) // Reduce test noise
		ctx = context.Background()
	})

	// BR-HOLMES-017: Unit tests for Prometheus detector implementation
	Describe("PrometheusDetector Implementation", func() {
		var (
			detector *k8s.PrometheusDetector
			pattern  k8s.ServicePattern
		)

		BeforeEach(func() {
			pattern = k8s.ServicePattern{
				Enabled:  true,
				Priority: 80,
				Selectors: []map[string]string{
					{"app.kubernetes.io/name": "prometheus"},
					{"app": "prometheus"},
				},
				ServiceNames:  []string{"prometheus", "prometheus-server"},
				RequiredPorts: []int32{9090},
				Capabilities: []string{
					"query_metrics", "alert_rules", "time_series",
				},
			}
			detector = k8s.NewPrometheusDetector(pattern, log)
		})

		Context("Service Name Detection", func() {
			It("should detect Prometheus by exact service name match", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "prometheus",
						Namespace: "monitoring",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 8080}}, // Non-standard port
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
				Expect(detected.ServiceType).To(Equal("prometheus"))
				Expect(detected.Name).To(Equal("prometheus"))
				Expect(detected.Priority).To(Equal(80))
			})

			It("should detect Prometheus by partial service name match", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "prometheus-server-monitoring",
						Namespace: "monitoring",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3000}},
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected.ServiceType).To(Equal("prometheus"), "BR-MON-001-UPTIME: Prometheus service detection must support partial name matching for monitoring flexibility")
			})

			It("should not detect non-Prometheus service names", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "grafana-dashboard",
						Namespace: "monitoring",
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected).To(BeNil())
			})
		})

		Context("Label Selector Detection", func() {
			It("should detect Prometheus by app.kubernetes.io/name label", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "metrics-service",
						Namespace: "monitoring",
						Labels: map[string]string{
							"app.kubernetes.io/name": "prometheus",
						},
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
				Expect(detected.ServiceType).To(Equal("prometheus"))
			})

			It("should detect Prometheus by app label", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "metrics-service",
						Namespace: "monitoring",
						Labels: map[string]string{
							"app": "prometheus",
						},
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
			})

			It("should not detect service with partial label match", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "metrics-service",
						Namespace: "monitoring",
						Labels: map[string]string{
							"app": "prometheus-operator", // Contains prometheus but not exact match
						},
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected).To(BeNil())
			})

			It("should handle services without labels gracefully", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unlabeled-service",
						Namespace: "monitoring",
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected).To(BeNil())
			})
		})

		Context("Port-Based Detection", func() {
			It("should detect Prometheus by default port 9090", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unknown-metrics",
						Namespace: "monitoring",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "web", Port: 9090},
						},
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
				Expect(detected.ServiceType).To(Equal("prometheus"))
			})

			It("should not detect service without required ports", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-service",
						Namespace: "monitoring",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "http", Port: 8080},
						},
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected).To(BeNil())
			})

			It("should detect service with multiple ports including required port", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multi-port-service",
						Namespace: "monitoring",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "admin", Port: 8080},
							{Name: "metrics", Port: 9090},
							{Name: "health", Port: 8081},
						},
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
			})
		})

		Context("Endpoint Generation Logic", func() {
			It("should create correct HTTP endpoints for standard ports", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "prometheus",
						Namespace: "monitoring",
						Labels:    map[string]string{"app": "prometheus"},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "web", Port: 9090},
						},
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected.Endpoints).To(HaveLen(1))
				Expect(detected.Endpoints[0].URL).To(Equal("http://prometheus.monitoring.svc.cluster.local:9090"))
				Expect(detected.Endpoints[0].Port).To(Equal(int32(9090)))
				Expect(detected.Endpoints[0].Protocol).To(Equal("http"))
			})

			It("should create HTTPS endpoints for port 443", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secure-prometheus",
						Namespace: "monitoring",
						Labels:    map[string]string{"app": "prometheus"},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "https", Port: 443},
						},
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected.Endpoints[0].Protocol).To(Equal("https"))
				Expect(detected.Endpoints[0].URL).To(ContainSubstring("https://"))
			})

			It("should create HTTPS endpoints for ports with https in name", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secure-prometheus",
						Namespace: "monitoring",
						Labels:    map[string]string{"app": "prometheus"},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "web-https", Port: 9443},
						},
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected.Endpoints[0].Protocol).To(Equal("https"))
			})
		})

		Context("Detector Configuration", func() {
			It("should return correct service type", func() {
				Expect(detector.GetServiceType()).To(Equal("prometheus"))
			})

			It("should return configured priority", func() {
				Expect(detector.GetPriority()).To(Equal(80))
			})

			It("should not detect when disabled", func() {
				disabledPattern := pattern
				disabledPattern.Enabled = false
				disabledDetector := k8s.NewPrometheusDetector(disabledPattern, log)

				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "prometheus",
						Labels: map[string]string{"app": "prometheus"},
					},
				}

				detected, err := disabledDetector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected).To(BeNil())
			})
		})
	})

	// BR-HOLMES-017: Unit tests for Grafana detector implementation
	Describe("GrafanaDetector Implementation", func() {
		var (
			detector *k8s.GrafanaDetector
			pattern  k8s.ServicePattern
		)

		BeforeEach(func() {
			pattern = k8s.ServicePattern{
				Enabled:  true,
				Priority: 70,
				Selectors: []map[string]string{
					{"app.kubernetes.io/name": "grafana"},
				},
				ServiceNames:  []string{"grafana"},
				RequiredPorts: []int32{3000},
				Capabilities: []string{
					"get_dashboards", "query_datasource", "visualization",
				},
			}
			detector = k8s.NewGrafanaDetector(pattern, log)
		})

		It("should detect Grafana by service name", func() {
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "grafana",
					Namespace: "monitoring",
				},
			}

			detected, err := detector.Detect(ctx, service)

			Expect(err).ToNot(HaveOccurred())
			Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
			Expect(detected.ServiceType).To(Equal("grafana"))
			Expect(detected.Priority).To(Equal(70))
		})

		It("should detect Grafana by label", func() {
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dashboard-service",
					Namespace: "monitoring",
					Labels: map[string]string{
						"app.kubernetes.io/name": "grafana",
					},
				},
			}

			detected, err := detector.Detect(ctx, service)

			Expect(err).ToNot(HaveOccurred())
			Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
		})

		It("should detect Grafana by port 3000", func() {
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dashboard",
					Namespace: "monitoring",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 3000}},
				},
			}

			detected, err := detector.Detect(ctx, service)

			Expect(err).ToNot(HaveOccurred())
			Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
		})

		It("should return correct service type and priority", func() {
			Expect(detector.GetServiceType()).To(Equal("grafana"))
			Expect(detector.GetPriority()).To(Equal(70))
		})
	})

	// BR-HOLMES-017: Unit tests for Jaeger detector implementation
	Describe("JaegerDetector Implementation", func() {
		var (
			detector *k8s.JaegerDetector
			pattern  k8s.ServicePattern
		)

		BeforeEach(func() {
			pattern = k8s.ServicePattern{
				Enabled:  true,
				Priority: 60,
				Selectors: []map[string]string{
					{"app.kubernetes.io/name": "jaeger"},
					{"app.kubernetes.io/component": "query"},
				},
				ServiceNames:  []string{"jaeger-query", "jaeger"},
				RequiredPorts: []int32{16686},
				Capabilities: []string{
					"search_traces", "get_services", "distributed_tracing",
				},
			}
			detector = k8s.NewJaegerDetector(pattern, log)
		})

		It("should detect Jaeger by service name", func() {
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "jaeger-query",
					Namespace: "observability",
				},
			}

			detected, err := detector.Detect(ctx, service)

			Expect(err).ToNot(HaveOccurred())
			Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
			Expect(detected.ServiceType).To(Equal("jaeger"))
		})

		It("should detect Jaeger by component label", func() {
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tracing-query",
					Namespace: "observability",
					Labels: map[string]string{
						"app.kubernetes.io/component": "query",
					},
				},
			}

			detected, err := detector.Detect(ctx, service)

			Expect(err).ToNot(HaveOccurred())
			Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
		})

		It("should detect Jaeger by port 16686", func() {
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tracing-ui",
					Namespace: "observability",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 16686}},
				},
			}

			detected, err := detector.Detect(ctx, service)

			Expect(err).ToNot(HaveOccurred())
			Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
		})
	})

	// BR-HOLMES-018: Unit tests for Custom detector implementation
	Describe("CustomServiceDetector Implementation", func() {
		var (
			detector *k8s.CustomServiceDetector
			pattern  k8s.ServicePattern
		)

		BeforeEach(func() {
			pattern = k8s.ServicePattern{
				Enabled:  true,
				Priority: 30,
			}
			detector = k8s.NewCustomServiceDetector(pattern, log)
		})

		Context("Annotation-Based Detection", func() {
			It("should detect custom service by toolset annotation", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "custom-monitoring",
						Namespace: "monitoring",
						Annotations: map[string]string{
							"kubernaut.io/toolset": "vector-database",
						},
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
				Expect(detected.ServiceType).To(Equal("vector-database"))
				Expect(detected.Name).To(Equal("custom-monitoring"))
			})

			It("should not detect service without toolset annotation", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "regular-service",
						Namespace: "monitoring",
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected).To(BeNil())
			})

			It("should not detect service without annotations", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "no-annotations",
						Namespace: "monitoring",
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected).To(BeNil())
			})
		})

		Context("Custom Endpoint Parsing", func() {
			It("should parse custom endpoints from annotation", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "custom-service",
						Namespace: "monitoring",
						Annotations: map[string]string{
							"kubernaut.io/toolset":   "custom-metrics",
							"kubernaut.io/endpoints": "metrics:8080,logs:3100,health:8081",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "metrics", Port: 8080},
							{Name: "logs", Port: 3100},
							{Name: "health", Port: 8081},
						},
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
				Expect(detected.Endpoints).To(HaveLen(3))

				// Verify specific endpoints
				endpointsByName := make(map[string]k8s.ServiceEndpoint)
				for _, ep := range detected.Endpoints {
					endpointsByName[ep.Name] = ep
				}

				Expect(endpointsByName["metrics"].Port).To(Equal(int32(8080)))
				Expect(endpointsByName["logs"].Port).To(Equal(int32(3100)))
				Expect(endpointsByName["health"].Port).To(Equal(int32(8081)))
			})

			It("should handle malformed endpoint annotations gracefully", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "malformed-endpoints",
						Namespace: "monitoring",
						Annotations: map[string]string{
							"kubernaut.io/toolset":   "custom",
							"kubernaut.io/endpoints": "invalid,metrics:notanumber,incomplete:",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Name: "default", Port: 8080}},
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
				// Should fallback to service ports when custom endpoints are invalid
				Expect(detected.Endpoints).To(HaveLen(1))
				Expect(detected.Endpoints[0].Port).To(Equal(int32(8080)))
			})

			It("should fallback to service ports when no custom endpoints specified", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "no-custom-endpoints",
						Namespace: "monitoring",
						Annotations: map[string]string{
							"kubernaut.io/toolset": "custom",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "api", Port: 8080},
							{Name: "metrics", Port: 9090},
						},
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected.Endpoints).To(HaveLen(2))
			})
		})

		Context("Custom Capabilities Parsing", func() {
			It("should parse capabilities from annotation", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "custom-service",
						Namespace: "monitoring",
						Annotations: map[string]string{
							"kubernaut.io/toolset":      "analytics",
							"kubernaut.io/capabilities": "data_processing,ml_inference,pattern_detection",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Name: "api", Port: 8080}},
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected.Capabilities).To(HaveLen(3))
				Expect(detected.Capabilities).To(ContainElement("data_processing"))
				Expect(detected.Capabilities).To(ContainElement("ml_inference"))
				Expect(detected.Capabilities).To(ContainElement("pattern_detection"))
			})

			It("should handle empty capabilities gracefully", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "no-capabilities",
						Namespace: "monitoring",
						Annotations: map[string]string{
							"kubernaut.io/toolset": "basic-service",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Name: "api", Port: 8080}},
					},
				}

				detected, err := detector.Detect(ctx, service)

				Expect(err).ToNot(HaveOccurred())
				Expect(detected.Capabilities).To(BeEmpty())
			})
		})

		It("should return correct service type and priority", func() {
			Expect(detector.GetServiceType()).To(Equal("custom"))
			Expect(detector.GetPriority()).To(Equal(30))
		})
	})

	// BR-HOLMES-017: Unit tests for Elasticsearch detector implementation
	Describe("ElasticsearchDetector Implementation", func() {
		var (
			detector *k8s.ElasticsearchDetector
			pattern  k8s.ServicePattern
		)

		BeforeEach(func() {
			pattern = k8s.ServicePattern{
				Enabled:  true,
				Priority: 50,
				Selectors: []map[string]string{
					{"app.kubernetes.io/name": "elasticsearch"},
				},
				ServiceNames:  []string{"elasticsearch", "elasticsearch-master"},
				RequiredPorts: []int32{9200},
				Capabilities: []string{
					"search_logs", "analyze_patterns", "log_analysis",
				},
			}
			detector = k8s.NewElasticsearchDetector(pattern, log)
		})

		It("should detect Elasticsearch by service name", func() {
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "elasticsearch",
					Namespace: "logging",
				},
			}

			detected, err := detector.Detect(ctx, service)

			Expect(err).ToNot(HaveOccurred())
			Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
			Expect(detected.ServiceType).To(Equal("elasticsearch"))
		})

		It("should detect Elasticsearch by master service name", func() {
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "elasticsearch-master-headless",
					Namespace: "logging",
				},
			}

			detected, err := detector.Detect(ctx, service)

			Expect(err).ToNot(HaveOccurred())
			Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
		})

		It("should detect Elasticsearch by port 9200", func() {
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "search-engine",
					Namespace: "logging",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 9200}},
				},
			}

			detected, err := detector.Detect(ctx, service)

			Expect(err).ToNot(HaveOccurred())
			Expect(detected.ServiceType).ToNot(BeEmpty(), "BR-MON-001-UPTIME: Service detection must return valid service type for monitoring configuration")
		})

		It("should return correct service type and priority", func() {
			Expect(detector.GetServiceType()).To(Equal("elasticsearch"))
			Expect(detector.GetPriority()).To(Equal(50))
		})
	})
})
