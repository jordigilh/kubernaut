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
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/jordigilh/kubernaut/pkg/toolset/discovery"
)

var _ = Describe("BR-TOOLSET-010: Prometheus Detector", func() {
	var (
		detector discovery.ServiceDetector
		ctx      context.Context
	)

	BeforeEach(func() {
		detector = discovery.NewPrometheusDetector()
		ctx = context.Background()
	})

	Describe("ServiceType", func() {
		It("should return 'prometheus' as service type", func() {
			Expect(detector.ServiceType()).To(Equal("prometheus"))
		})
	})

	Describe("Detect", func() {
		// Table-driven tests for successful detection scenarios
		DescribeTable("should detect Prometheus services",
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
				Expect(discovered.Type).To(Equal("prometheus"))
				if expectedEndpoint != "" {
					Expect(discovered.Endpoint).To(Equal(expectedEndpoint))
				}
			},
			Entry("with app=prometheus label",
				"prometheus-server", "monitoring",
				map[string]string{"app": "prometheus"},
				[]corev1.ServicePort{{Name: "web", Port: 9090, TargetPort: intstr.FromInt(9090)}},
				"http://prometheus-server.monitoring.svc.cluster.local:9090",
			),
			Entry("with app.kubernetes.io/name=prometheus label",
				"prometheus", "default",
				map[string]string{"app.kubernetes.io/name": "prometheus"},
				[]corev1.ServicePort{{Name: "web", Port: 9090, TargetPort: intstr.FromInt(9090)}},
				"http://prometheus.default.svc.cluster.local:9090",
			),
			Entry("named 'prometheus'",
				"prometheus", "monitoring",
				nil,
				[]corev1.ServicePort{{Name: "web", Port: 9090, TargetPort: intstr.FromInt(9090)}},
				"http://prometheus.monitoring.svc.cluster.local:9090",
			),
			Entry("named 'prometheus-server'",
				"prometheus-server", "monitoring",
				nil,
				[]corev1.ServicePort{{Port: 9090}},
				"http://prometheus-server.monitoring.svc.cluster.local:9090",
			),
			Entry("with port named 'web' on 9090",
				"my-prometheus", "monitoring",
				nil,
				[]corev1.ServicePort{{Name: "web", Port: 9090}},
				"http://my-prometheus.monitoring.svc.cluster.local:9090",
			),
			Entry("with custom port number",
				"prometheus-custom", "monitoring",
				map[string]string{"app": "prometheus"},
				[]corev1.ServicePort{{Name: "web", Port: 9091}},
				"http://prometheus-custom.monitoring.svc.cluster.local:9091",
			),
		)

		Context("BR-TOOLSET-011: endpoint URL construction", func() {
			It("should use first port if multiple ports exist", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "prometheus",
						Namespace: "default",
						Labels:    map[string]string{"app": "prometheus"},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "web", Port: 9090},
							{Name: "admin", Port: 9091},
						},
					},
				}

				discovered, err := detector.Detect(ctx, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(discovered.Endpoint).To(Equal("http://prometheus.default.svc.cluster.local:9090"))
			})
		})

		// Table-driven tests for services that should NOT be detected
		DescribeTable("should NOT detect non-Prometheus services",
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
			Entry("for non-Prometheus service (grafana)",
				"grafana", "monitoring",
				map[string]string{"app": "grafana"},
				[]corev1.ServicePort{{Port: 3000}},
			),
			Entry("for service without ports",
				"prometheus-headless", "monitoring",
				map[string]string{"app": "prometheus"},
				[]corev1.ServicePort{},
			),
		)

		Context("when populating metadata", func() {
			It("should populate labels correctly", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "prometheus",
						Namespace: "monitoring",
						Labels: map[string]string{
							"app":     "prometheus",
							"version": "v2.40.0",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 9090}},
					},
				}

				discovered, err := detector.Detect(ctx, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(discovered.Labels).To(HaveKeyWithValue("app", "prometheus"))
				Expect(discovered.Labels).To(HaveKeyWithValue("version", "v2.40.0"))
			})

			It("should set DiscoveredAt timestamp", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "prometheus",
						Namespace: "monitoring",
						Labels:    map[string]string{"app": "prometheus"},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 9090}},
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

	Describe("BR-TOOLSET-012: HealthCheck", func() {
		// Table-driven tests for healthy scenarios
		DescribeTable("should pass health check for healthy Prometheus",
			func(statusCode int, body string) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/-/healthy"))
					w.WriteHeader(statusCode)
					if body != "" {
						w.Write([]byte(body))
					}
				}))
				defer server.Close()

				err := detector.HealthCheck(ctx, server.URL)
				Expect(err).ToNot(HaveOccurred())
			},
			Entry("with 200 OK status", http.StatusOK, ""),
			Entry("with 204 No Content status", http.StatusNoContent, ""),
		)

		// Table-driven tests for unhealthy scenarios
		DescribeTable("should fail health check for unhealthy Prometheus",
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
				// Sleep with context awareness to allow clean shutdown
				select {
				case <-time.After(10 * time.Second):
				case <-r.Context().Done():
					return
				}
			}))
			defer server.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			err := detector.HealthCheck(ctx, server.URL)
			Expect(err).To(HaveOccurred())
		})
	})
})
