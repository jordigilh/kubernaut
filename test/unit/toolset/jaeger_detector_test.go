package toolset_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/jordigilh/kubernaut/pkg/toolset/discovery"
)

var _ = Describe("BR-TOOLSET-016: Jaeger Detector", func() {
	var (
		detector discovery.ServiceDetector
		ctx      context.Context
	)

	BeforeEach(func() {
		detector = discovery.NewJaegerDetector()
		ctx = context.Background()
	})

	Describe("ServiceType", func() {
		It("should return 'jaeger' as service type", func() {
			Expect(detector.ServiceType()).To(Equal("jaeger"))
		})
	})

	Describe("Detect", func() {
		// Table-driven tests for successful detection scenarios
		DescribeTable("should detect Jaeger services",
			func(name, namespace string, labels map[string]string, ports []corev1.ServicePort, expectedEndpoint string) {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
						Labels:    labels,
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
				Expect(discovered.Type).To(Equal("jaeger"))
				if expectedEndpoint != "" {
					Expect(discovered.Endpoint).To(Equal(expectedEndpoint))
				}
			},
			Entry("with app=jaeger label",
				"jaeger-query", "observability",
				map[string]string{"app": "jaeger"},
				[]corev1.ServicePort{{Name: "query", Port: 16686, TargetPort: intstr.FromInt(16686)}},
				"http://jaeger-query.observability.svc.cluster.local:16686",
			),
			Entry("with app.kubernetes.io/name=jaeger label",
				"jaeger", "default",
				map[string]string{"app.kubernetes.io/name": "jaeger"},
				[]corev1.ServicePort{{Name: "query", Port: 16686, TargetPort: intstr.FromInt(16686)}},
				"http://jaeger.default.svc.cluster.local:16686",
			),
			Entry("named 'jaeger'",
				"jaeger", "observability",
				nil,
				[]corev1.ServicePort{{Name: "query", Port: 16686, TargetPort: intstr.FromInt(16686)}},
				"http://jaeger.observability.svc.cluster.local:16686",
			),
			Entry("named 'jaeger-query'",
				"jaeger-query", "observability",
				nil,
				[]corev1.ServicePort{{Port: 16686}},
				"http://jaeger-query.observability.svc.cluster.local:16686",
			),
			Entry("with port named 'query' on 16686",
				"my-jaeger", "observability",
				nil,
				[]corev1.ServicePort{{Name: "query", Port: 16686}},
				"http://my-jaeger.observability.svc.cluster.local:16686",
			),
			Entry("with custom port number",
				"jaeger-custom", "observability",
				map[string]string{"app": "jaeger"},
				[]corev1.ServicePort{{Name: "ui", Port: 8080}},
				"http://jaeger-custom.observability.svc.cluster.local:8080",
			),
		)

		Context("BR-TOOLSET-017: endpoint URL construction", func() {
			It("should use first port if multiple ports exist", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "jaeger",
						Namespace: "default",
						Labels:    map[string]string{"app": "jaeger"},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "query", Port: 16686},
							{Name: "admin", Port: 14269},
						},
					},
				}

				discovered, err := detector.Detect(ctx, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(discovered.Endpoint).To(Equal("http://jaeger.default.svc.cluster.local:16686"))
			})
		})

		// Table-driven tests for services that should NOT be detected
		DescribeTable("should NOT detect non-Jaeger services",
			func(name, namespace string, labels map[string]string, ports []corev1.ServicePort) {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
						Labels:    labels,
					},
					Spec: corev1.ServiceSpec{
						Ports: ports,
					},
				}

				discovered, err := detector.Detect(ctx, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(discovered).To(BeNil())
			},
			Entry("for non-Jaeger service (prometheus)",
				"prometheus", "monitoring",
				map[string]string{"app": "prometheus"},
				[]corev1.ServicePort{{Port: 9090}},
			),
			Entry("for service without ports",
				"jaeger-headless", "observability",
				map[string]string{"app": "jaeger"},
				[]corev1.ServicePort{},
			),
		)

		Context("when populating metadata", func() {
			It("should populate labels correctly", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "jaeger",
						Namespace: "observability",
						Labels: map[string]string{
							"app":     "jaeger",
							"version": "v1.49.0",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 16686}},
					},
				}

				discovered, err := detector.Detect(ctx, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(discovered.Labels).To(HaveKeyWithValue("app", "jaeger"))
				Expect(discovered.Labels).To(HaveKeyWithValue("version", "v1.49.0"))
			})

			It("should set DiscoveredAt timestamp", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "jaeger",
						Namespace: "observability",
						Labels:    map[string]string{"app": "jaeger"},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 16686}},
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

	Describe("BR-TOOLSET-018: HealthCheck", func() {
		// Table-driven tests for healthy scenarios
		DescribeTable("should pass health check for healthy Jaeger",
			func(statusCode int, body string) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/"))
					w.WriteHeader(statusCode)
					if body != "" {
						w.Write([]byte(body))
					}
				}))
				defer server.Close()

				err := detector.HealthCheck(ctx, server.URL)
				Expect(err).ToNot(HaveOccurred())
			},
			Entry("with 200 OK status", http.StatusOK, "<html><title>Jaeger UI</title></html>"),
			Entry("with 204 No Content status", http.StatusNoContent, ""),
		)

		// Table-driven tests for unhealthy scenarios
		DescribeTable("should fail health check for unhealthy Jaeger",
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
				return server.URL
			}),
			Entry("for connection refused", func() string {
				return "http://localhost:9999"
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

			err := detector.HealthCheck(ctx, server.URL)
			Expect(err).To(HaveOccurred())
		})
	})
})
