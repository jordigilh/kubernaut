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

var _ = Describe("BR-TOOLSET-013: Grafana Detector", func() {
	var (
		detector discovery.ServiceDetector
		ctx      context.Context
	)

	BeforeEach(func() {
		detector = discovery.NewGrafanaDetector()
		ctx = context.Background()
	})

	Describe("ServiceType", func() {
		It("should return 'grafana' as service type", func() {
			Expect(detector.ServiceType()).To(Equal("grafana"))
		})
	})

	Describe("Detect", func() {
		// Table-driven tests for successful detection scenarios
		DescribeTable("should detect Grafana services",
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
				Expect(discovered.Type).To(Equal("grafana"))
				if expectedEndpoint != "" {
					Expect(discovered.Endpoint).To(Equal(expectedEndpoint))
				}
			},
			Entry("with app=grafana label",
				"grafana", "monitoring",
				map[string]string{"app": "grafana"},
				[]corev1.ServicePort{{Name: "service", Port: 3000, TargetPort: intstr.FromInt(3000)}},
				"http://grafana.monitoring.svc.cluster.local:3000",
			),
			Entry("with app.kubernetes.io/name=grafana label",
				"grafana-server", "default",
				map[string]string{"app.kubernetes.io/name": "grafana"},
				[]corev1.ServicePort{{Name: "service", Port: 3000, TargetPort: intstr.FromInt(3000)}},
				"http://grafana-server.default.svc.cluster.local:3000",
			),
			Entry("named 'grafana'",
				"grafana", "monitoring",
				nil,
				[]corev1.ServicePort{{Name: "service", Port: 3000, TargetPort: intstr.FromInt(3000)}},
				"http://grafana.monitoring.svc.cluster.local:3000",
			),
			Entry("named 'grafana-server'",
				"grafana-server", "monitoring",
				nil,
				[]corev1.ServicePort{{Port: 3000}},
				"http://grafana-server.monitoring.svc.cluster.local:3000",
			),
			Entry("with port named 'service' on 3000",
				"my-grafana", "monitoring",
				nil,
				[]corev1.ServicePort{{Name: "service", Port: 3000}},
				"http://my-grafana.monitoring.svc.cluster.local:3000",
			),
			Entry("with custom port number",
				"grafana-custom", "monitoring",
				map[string]string{"app": "grafana"},
				[]corev1.ServicePort{{Name: "http", Port: 8080}},
				"http://grafana-custom.monitoring.svc.cluster.local:8080",
			),
		)

		Context("BR-TOOLSET-014: endpoint URL construction", func() {
			It("should use first port if multiple ports exist", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "grafana",
						Namespace: "default",
						Labels:    map[string]string{"app": "grafana"},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "http", Port: 3000},
							{Name: "admin", Port: 3001},
						},
					},
				}

				discovered, err := detector.Detect(ctx, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(discovered.Endpoint).To(Equal("http://grafana.default.svc.cluster.local:3000"))
			})
		})

		// Table-driven tests for services that should NOT be detected
		DescribeTable("should NOT detect non-Grafana services",
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
			Entry("for non-Grafana service (prometheus)",
				"prometheus", "monitoring",
				map[string]string{"app": "prometheus"},
				[]corev1.ServicePort{{Port: 9090}},
			),
			Entry("for service without ports",
				"grafana-headless", "monitoring",
				map[string]string{"app": "grafana"},
				[]corev1.ServicePort{},
			),
		)

		Context("when populating metadata", func() {
			It("should populate labels correctly", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "grafana",
						Namespace: "monitoring",
						Labels: map[string]string{
							"app":     "grafana",
							"version": "v9.5.0",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3000}},
					},
				}

				discovered, err := detector.Detect(ctx, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(discovered.Labels).To(HaveKeyWithValue("app", "grafana"))
				Expect(discovered.Labels).To(HaveKeyWithValue("version", "v9.5.0"))
			})

			It("should set DiscoveredAt timestamp", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "grafana",
						Namespace: "monitoring",
						Labels:    map[string]string{"app": "grafana"},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3000}},
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

	Describe("BR-TOOLSET-015: HealthCheck", func() {
		// Table-driven tests for healthy scenarios
		DescribeTable("should pass health check for healthy Grafana",
			func(statusCode int, body string) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/api/health"))
					w.WriteHeader(statusCode)
					if body != "" {
						w.Write([]byte(body))
					}
				}))
				defer server.Close()

				err := detector.HealthCheck(ctx, server.URL)
				Expect(err).ToNot(HaveOccurred())
			},
			Entry("with 200 OK status", http.StatusOK, `{"database":"ok","version":"9.5.0"}`),
			Entry("with 204 No Content status", http.StatusNoContent, ""),
		)

		// Table-driven tests for unhealthy scenarios
		DescribeTable("should fail health check for unhealthy Grafana",
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
