package toolset

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/pkg/toolset/discovery"
)

var _ = Describe("BR-TOOLSET-022: Custom Detector", func() {
	var (
		detector discovery.ServiceDetector
		ctx      context.Context
	)

	BeforeEach(func() {
		detector = discovery.NewCustomDetector()
		ctx = context.Background()
	})

	Describe("ServiceType", func() {
		It("should return 'custom' as service type", func() {
			Expect(detector.ServiceType()).To(Equal("custom"))
		})
	})

	Describe("Detect", func() {
		// Table-driven tests for successful detection scenarios
		DescribeTable("should detect services with kubernaut.io/toolset annotation",
			func(name, namespace string, annotations map[string]string, ports []corev1.ServicePort, expectedType, expectedEndpoint string) {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        name,
						Namespace:   namespace,
						Annotations: annotations,
					},
					Spec: corev1.ServiceSpec{
						Ports: ports,
					},
				}

				discovered, err := detector.Detect(ctx, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(discovered).ToNot(BeNil())
				Expect(discovered.Name).To(Equal(name))
				Expect(discovered.Namespace).To(Equal(namespace))
				Expect(discovered.Type).To(Equal(expectedType))
				if expectedEndpoint != "" {
					Expect(discovered.Endpoint).To(Equal(expectedEndpoint))
				}
			},
			Entry("with toolset=enabled annotation and custom type",
				"custom-api", "default",
				map[string]string{
					"kubernaut.io/toolset":      "enabled",
					"kubernaut.io/toolset-type": "custom-api",
				},
				[]corev1.ServicePort{{Name: "http", Port: 8080}},
				"custom-api",
				"http://custom-api.default.svc.cluster.local:8080",
			),
			Entry("with toolset=true annotation and custom type",
				"my-service", "apps",
				map[string]string{
					"kubernaut.io/toolset":      "true",
					"kubernaut.io/toolset-type": "my-service",
				},
				[]corev1.ServicePort{{Name: "api", Port: 9000}},
				"my-service",
				"http://my-service.apps.svc.cluster.local:9000",
			),
			Entry("with custom endpoint override",
				"external-service", "integrations",
				map[string]string{
					"kubernaut.io/toolset":          "enabled",
					"kubernaut.io/toolset-type":     "external-api",
					"kubernaut.io/toolset-endpoint": "https://api.external.com:443",
				},
				[]corev1.ServicePort{{Port: 8080}},
				"external-api",
				"https://api.external.com:443",
			),
			Entry("with custom health endpoint override",
				"legacy-service", "default",
				map[string]string{
					"kubernaut.io/toolset":             "enabled",
					"kubernaut.io/toolset-type":        "legacy",
					"kubernaut.io/toolset-health-path": "/healthz",
				},
				[]corev1.ServicePort{{Port: 8080}},
				"legacy",
				"http://legacy-service.default.svc.cluster.local:8080",
			),
		)

		Context("BR-TOOLSET-023: when building endpoint URL", func() {
			It("should use default cluster.local format if no override", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-service",
						Namespace: "default",
						Annotations: map[string]string{
							"kubernaut.io/toolset":      "enabled",
							"kubernaut.io/toolset-type": "custom",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 8080}},
					},
				}

				discovered, err := detector.Detect(ctx, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(discovered.Endpoint).To(Equal("http://my-service.default.svc.cluster.local:8080"))
			})

			It("should respect endpoint override annotation", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "external",
						Namespace: "default",
						Annotations: map[string]string{
							"kubernaut.io/toolset":          "enabled",
							"kubernaut.io/toolset-type":     "external",
							"kubernaut.io/toolset-endpoint": "http://external.example.com:9090",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 8080}},
					},
				}

				discovered, err := detector.Detect(ctx, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(discovered.Endpoint).To(Equal("http://external.example.com:9090"))
			})

			It("should use first port if multiple ports exist", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multi-port",
						Namespace: "default",
						Annotations: map[string]string{
							"kubernaut.io/toolset":      "enabled",
							"kubernaut.io/toolset-type": "custom",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "http", Port: 8080},
							{Name: "admin", Port: 8081},
						},
					},
				}

				discovered, err := detector.Detect(ctx, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(discovered.Endpoint).To(Equal("http://multi-port.default.svc.cluster.local:8080"))
			})
		})

		// Table-driven tests for services that should NOT be detected
		DescribeTable("should NOT detect services without proper annotations",
			func(name string, annotations map[string]string, ports []corev1.ServicePort) {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:        name,
						Namespace:   "default",
						Annotations: annotations,
					},
					Spec: corev1.ServiceSpec{
						Ports: ports,
					},
				}

				discovered, err := detector.Detect(ctx, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(discovered).To(BeNil())
			},
			Entry("without kubernaut.io/toolset annotation",
				"no-annotation",
				map[string]string{"app": "myapp"},
				[]corev1.ServicePort{{Port: 8080}},
			),
			Entry("with toolset=disabled",
				"disabled-service",
				map[string]string{"kubernaut.io/toolset": "disabled"},
				[]corev1.ServicePort{{Port: 8080}},
			),
			Entry("with toolset=false",
				"false-service",
				map[string]string{"kubernaut.io/toolset": "false"},
				[]corev1.ServicePort{{Port: 8080}},
			),
			Entry("with toolset enabled but missing type",
				"no-type",
				map[string]string{"kubernaut.io/toolset": "enabled"},
				[]corev1.ServicePort{{Port: 8080}},
			),
			Entry("with toolset enabled but no ports",
				"no-ports",
				map[string]string{
					"kubernaut.io/toolset":      "enabled",
					"kubernaut.io/toolset-type": "custom",
				},
				[]corev1.ServicePort{},
			),
		)

		Context("when populating metadata", func() {
			It("should populate annotations correctly", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "annotated-service",
						Namespace: "default",
						Annotations: map[string]string{
							"kubernaut.io/toolset":      "enabled",
							"kubernaut.io/toolset-type": "custom",
							"app.version":               "1.2.3",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 8080}},
					},
				}

				discovered, err := detector.Detect(ctx, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(discovered.Annotations).To(HaveKeyWithValue("kubernaut.io/toolset", "enabled"))
				Expect(discovered.Annotations).To(HaveKeyWithValue("kubernaut.io/toolset-type", "custom"))
				Expect(discovered.Annotations).To(HaveKeyWithValue("app.version", "1.2.3"))
			})

			It("should store custom health path in metadata", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "custom-health",
						Namespace: "default",
						Annotations: map[string]string{
							"kubernaut.io/toolset":             "enabled",
							"kubernaut.io/toolset-type":        "custom",
							"kubernaut.io/toolset-health-path": "/health/check",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 8080}},
					},
				}

				discovered, err := detector.Detect(ctx, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(discovered.Metadata).To(HaveKeyWithValue("health_path", "/health/check"))
			})

			It("should set DiscoveredAt timestamp", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "timestamped",
						Namespace: "default",
						Annotations: map[string]string{
							"kubernaut.io/toolset":      "enabled",
							"kubernaut.io/toolset-type": "custom",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 8080}},
					},
				}

				before := time.Now()
				discovered, err := detector.Detect(ctx, service)
				after := time.Now()

				Expect(err).ToNot(HaveOccurred())
				Expect(discovered.DiscoveredAt).To(BeTemporally(">=", before))
				Expect(discovered.DiscoveredAt).To(BeTemporally("<=", after))
			})
		})
	})

	Describe("BR-TOOLSET-024: HealthCheck", func() {
		// Table-driven tests for healthy scenarios
		DescribeTable("should pass health check for healthy service",
			func(statusCode int, healthPath string) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal(healthPath))
					w.WriteHeader(statusCode)
				}))
				defer server.Close()

				err := detector.HealthCheck(ctx, server.URL+healthPath)
				Expect(err).ToNot(HaveOccurred())
			},
			Entry("with 200 OK at /health", http.StatusOK, "/health"),
			Entry("with 204 No Content at /healthz", http.StatusNoContent, "/healthz"),
			Entry("with 200 OK at custom path", http.StatusOK, "/api/v1/health"),
		)

		// Table-driven tests for unhealthy scenarios
		DescribeTable("should fail health check for unhealthy service",
			func(setupServer func() string) {
				endpoint := setupServer()
				err := detector.HealthCheck(ctx, endpoint)
				Expect(err).To(HaveOccurred())
			},
			Entry("for 503 Service Unavailable", func() string {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
				}))
				DeferCleanup(server.Close)
				return server.URL + "/health"
			}),
			Entry("for connection refused", func() string {
				return "http://localhost:9999/health"
			}),
			Entry("for invalid URL", func() string {
				return "not-a-valid-url"
			}),
		)

		It("should timeout after configured duration", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(10 * time.Second) // Longer than health check timeout
			}))
			defer server.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			err := detector.HealthCheck(ctx, server.URL+"/health")
			Expect(err).To(HaveOccurred())
		})
	})
})
