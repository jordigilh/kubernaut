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

var _ = Describe("BR-TOOLSET-019: Elasticsearch Detector", func() {
	var (
		detector discovery.ServiceDetector
		ctx      context.Context
	)

	BeforeEach(func() {
		detector = discovery.NewElasticsearchDetector()
		ctx = context.Background()
	})

	Describe("ServiceType", func() {
		It("should return 'elasticsearch' as service type", func() {
			Expect(detector.ServiceType()).To(Equal("elasticsearch"))
		})
	})

	Describe("Detect", func() {
		// Table-driven tests for successful detection scenarios
		DescribeTable("should detect Elasticsearch services",
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
				Expect(discovered.Type).To(Equal("elasticsearch"))
				if expectedEndpoint != "" {
					Expect(discovered.Endpoint).To(Equal(expectedEndpoint))
				}
			},
			Entry("with app=elasticsearch label",
				"elasticsearch", "logging",
				map[string]string{"app": "elasticsearch"},
				[]corev1.ServicePort{{Name: "http", Port: 9200, TargetPort: intstr.FromInt(9200)}},
				"http://elasticsearch.logging.svc.cluster.local:9200",
			),
			Entry("with app.kubernetes.io/name=elasticsearch label",
				"elasticsearch", "default",
				map[string]string{"app.kubernetes.io/name": "elasticsearch"},
				[]corev1.ServicePort{{Name: "http", Port: 9200, TargetPort: intstr.FromInt(9200)}},
				"http://elasticsearch.default.svc.cluster.local:9200",
			),
			Entry("named 'elasticsearch'",
				"elasticsearch", "logging",
				nil,
				[]corev1.ServicePort{{Name: "http", Port: 9200, TargetPort: intstr.FromInt(9200)}},
				"http://elasticsearch.logging.svc.cluster.local:9200",
			),
			Entry("named 'elasticsearch-master'",
				"elasticsearch-master", "logging",
				nil,
				[]corev1.ServicePort{{Port: 9200}},
				"http://elasticsearch-master.logging.svc.cluster.local:9200",
			),
			Entry("with port 9200",
				"my-elasticsearch", "logging",
				nil,
				[]corev1.ServicePort{{Name: "http", Port: 9200}},
				"http://my-elasticsearch.logging.svc.cluster.local:9200",
			),
			Entry("with custom port number",
				"elasticsearch-custom", "logging",
				map[string]string{"app": "elasticsearch"},
				[]corev1.ServicePort{{Name: "http", Port: 9201}},
				"http://elasticsearch-custom.logging.svc.cluster.local:9201",
			),
		)

		Context("BR-TOOLSET-020: endpoint URL construction", func() {
			It("should use first port if multiple ports exist", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "elasticsearch",
						Namespace: "default",
						Labels:    map[string]string{"app": "elasticsearch"},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "http", Port: 9200},
							{Name: "transport", Port: 9300},
						},
					},
				}

				discovered, err := detector.Detect(ctx, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(discovered.Endpoint).To(Equal("http://elasticsearch.default.svc.cluster.local:9200"))
			})
		})

		// Table-driven tests for services that should NOT be detected
		DescribeTable("should NOT detect non-Elasticsearch services",
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
			Entry("for non-Elasticsearch service (prometheus)",
				"prometheus", "monitoring",
				map[string]string{"app": "prometheus"},
				[]corev1.ServicePort{{Port: 9090}},
			),
			Entry("for service without ports",
				"elasticsearch-headless", "logging",
				map[string]string{"app": "elasticsearch"},
				[]corev1.ServicePort{},
			),
		)

		Context("when populating metadata", func() {
			It("should populate labels correctly", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "elasticsearch",
						Namespace: "logging",
						Labels: map[string]string{
							"app":     "elasticsearch",
							"version": "v8.11.0",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 9200}},
					},
				}

				discovered, err := detector.Detect(ctx, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(discovered.Labels).To(HaveKeyWithValue("app", "elasticsearch"))
				Expect(discovered.Labels).To(HaveKeyWithValue("version", "v8.11.0"))
			})

			It("should set DiscoveredAt timestamp", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "elasticsearch",
						Namespace: "logging",
						Labels:    map[string]string{"app": "elasticsearch"},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 9200}},
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

	Describe("BR-TOOLSET-021: HealthCheck", func() {
		// Table-driven tests for healthy scenarios
		DescribeTable("should pass health check for healthy Elasticsearch",
			func(statusCode int, body string) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/_cluster/health"))
					w.WriteHeader(statusCode)
					if body != "" {
						w.Write([]byte(body))
					}
				}))
				defer server.Close()

				err := detector.HealthCheck(ctx, server.URL)
				Expect(err).ToNot(HaveOccurred())
			},
			Entry("with green cluster status", http.StatusOK, `{"status":"green","cluster_name":"elasticsearch"}`),
			Entry("with yellow cluster status", http.StatusOK, `{"status":"yellow","cluster_name":"elasticsearch"}`),
			Entry("with 204 No Content status", http.StatusNoContent, ""),
		)

		// Table-driven tests for unhealthy scenarios
		DescribeTable("should fail health check for unhealthy Elasticsearch",
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
