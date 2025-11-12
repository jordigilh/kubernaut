package toolset

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/pkg/toolset"
	"github.com/jordigilh/kubernaut/pkg/toolset/discovery"
)

// mockDetector is a test mock for ServiceDetector
type mockDetector struct {
	serviceType string
	detectFunc  func(ctx context.Context, svc *corev1.Service) (*toolset.DiscoveredService, error)
	healthFunc  func(ctx context.Context, endpoint string) error
}

func (m *mockDetector) ServiceType() string {
	return m.serviceType
}

func (m *mockDetector) Detect(ctx context.Context, svc *corev1.Service) (*toolset.DiscoveredService, error) {
	if m.detectFunc != nil {
		return m.detectFunc(ctx, svc)
	}
	return nil, nil
}

func (m *mockDetector) HealthCheck(ctx context.Context, endpoint string) error {
	if m.healthFunc != nil {
		return m.healthFunc(ctx, endpoint)
	}
	return nil
}

var _ = Describe("BR-TOOLSET-025: Service Discoverer", func() {
	var (
		discoverer discovery.ServiceDiscoverer
		ctx        context.Context
		fakeClient *fake.Clientset
		cancelFunc context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancelFunc = context.WithCancel(context.Background())
		fakeClient = fake.NewSimpleClientset()
	})

	AfterEach(func() {
		if cancelFunc != nil {
			cancelFunc()
		}
		if discoverer != nil {
			_ = discoverer.Stop()
		}
	})

	Describe("DiscoverServices", func() {
		Context("with multiple registered detectors", func() {
			It("should discover services using all detectors", func() {
				// Create test services in fake client
				promSvc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "prometheus",
						Namespace: "monitoring",
						Labels:    map[string]string{"app": "prometheus"},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Name: "web", Port: 9090}},
					},
				}
				grafanaSvc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "grafana",
						Namespace: "monitoring",
						Labels:    map[string]string{"app": "grafana"},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Name: "http", Port: 3000}},
					},
				}

				_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, promSvc, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
				_, err = fakeClient.CoreV1().Services("monitoring").Create(ctx, grafanaSvc, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				// Create mock detectors
				prometheusDetector := &mockDetector{
					serviceType: "prometheus",
					detectFunc: func(ctx context.Context, svc *corev1.Service) (*toolset.DiscoveredService, error) {
						if svc.Name == "prometheus" {
							return &toolset.DiscoveredService{
								Name:         svc.Name,
								Namespace:    svc.Namespace,
								Type:         "prometheus",
								Endpoint:     "http://prometheus.monitoring:9090",
								DiscoveredAt: time.Now(),
							}, nil
						}
						return nil, nil
					},
					healthFunc: func(ctx context.Context, endpoint string) error {
						return nil // Healthy
					},
				}

				grafanaDetector := &mockDetector{
					serviceType: "grafana",
					detectFunc: func(ctx context.Context, svc *corev1.Service) (*toolset.DiscoveredService, error) {
						if svc.Name == "grafana" {
							return &toolset.DiscoveredService{
								Name:         svc.Name,
								Namespace:    svc.Namespace,
								Type:         "grafana",
								Endpoint:     "http://grafana.monitoring:3000",
								DiscoveredAt: time.Now(),
							}, nil
						}
						return nil, nil
					},
					healthFunc: func(ctx context.Context, endpoint string) error {
						return nil // Healthy
					},
				}

				// Create discoverer and register detectors
				discoverer = discovery.NewServiceDiscoverer(fakeClient, 10*time.Second)
				discoverer.RegisterDetector(prometheusDetector)
				discoverer.RegisterDetector(grafanaDetector)

				// Discover services
				discovered, err := discoverer.DiscoverServices(ctx)

				Expect(err).ToNot(HaveOccurred())
				Expect(discovered).To(HaveLen(2))

				// Check that both service types were discovered
				types := []string{discovered[0].Type, discovered[1].Type}
				Expect(types).To(ContainElements("prometheus", "grafana"))
			})

			It("should handle detector errors", func() {
				// Create test service
				svc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "error-service",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 8080}},
					},
				}
				_, err := fakeClient.CoreV1().Services("default").Create(ctx, svc, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				// Create detector that returns error
				errorDetector := &mockDetector{
					serviceType: "error",
					detectFunc: func(ctx context.Context, svc *corev1.Service) (*toolset.DiscoveredService, error) {
						return nil, errors.New("detector error")
					},
				}

				discoverer = discovery.NewServiceDiscoverer(fakeClient, 10*time.Second)
				discoverer.RegisterDetector(errorDetector)

				// Discover should handle error gracefully
				_, err = discoverer.DiscoverServices(ctx)

				// Should still succeed but log error
				Expect(err).ToNot(HaveOccurred())
			})

			It("should skip services that no detector matches", func() {
				// Create unmatched service
				svc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unknown-service",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 8080}},
					},
				}
				_, err := fakeClient.CoreV1().Services("default").Create(ctx, svc, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				// Create detector that never matches
				neverMatchDetector := &mockDetector{
					serviceType: "never",
					detectFunc: func(ctx context.Context, svc *corev1.Service) (*toolset.DiscoveredService, error) {
						return nil, nil // Never matches
					},
				}

				discoverer = discovery.NewServiceDiscoverer(fakeClient, 10*time.Second)
				discoverer.RegisterDetector(neverMatchDetector)

				discovered, err := discoverer.DiscoverServices(ctx)

				Expect(err).ToNot(HaveOccurred())
				Expect(discovered).To(BeEmpty())
			})

			It("should filter out unhealthy services", func() {
				// Create test services
				healthySvc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "healthy",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 8080}},
					},
				}
				unhealthySvc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unhealthy",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 8080}},
					},
				}

				_, err := fakeClient.CoreV1().Services("default").Create(ctx, healthySvc, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
				_, err = fakeClient.CoreV1().Services("default").Create(ctx, unhealthySvc, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				// Create detector with health checks
				detector := &mockDetector{
					serviceType: "test",
					detectFunc: func(ctx context.Context, svc *corev1.Service) (*toolset.DiscoveredService, error) {
						return &toolset.DiscoveredService{
							Name:         svc.Name,
							Namespace:    svc.Namespace,
							Type:         "test",
							Endpoint:     "http://" + svc.Name + ":8080",
							DiscoveredAt: time.Now(),
						}, nil
					},
					healthFunc: func(ctx context.Context, endpoint string) error {
						// Only "healthy" service passes health check
						if endpoint == "http://healthy:8080" {
							return nil
						}
						return errors.New("unhealthy")
					},
				}

				discoverer = discovery.NewServiceDiscoverer(fakeClient, 10*time.Second)
				discoverer.RegisterDetector(detector)

				discovered, err := discoverer.DiscoverServices(ctx)

				Expect(err).ToNot(HaveOccurred())
				Expect(discovered).To(HaveLen(1))
				Expect(discovered[0].Name).To(Equal("healthy"))
			})
		})

		Context("with empty cluster", func() {
			It("should return empty list", func() {
				discoverer = discovery.NewServiceDiscoverer(fakeClient, 10*time.Second)
				discoverer.RegisterDetector(&mockDetector{serviceType: "test"})

				discovered, err := discoverer.DiscoverServices(ctx)

				Expect(err).ToNot(HaveOccurred())
				Expect(discovered).To(BeEmpty())
			})
		})
	})

	Describe("RegisterDetector", func() {
		It("should allow registering multiple detectors", func() {
			discoverer = discovery.NewServiceDiscoverer(fakeClient, 10*time.Second)

			detector1 := &mockDetector{serviceType: "test1"}
			detector2 := &mockDetector{serviceType: "test2"}

			// Should not panic
			Expect(func() {
				discoverer.RegisterDetector(detector1)
				discoverer.RegisterDetector(detector2)
			}).ToNot(Panic())
		})
	})

	Describe("BR-TOOLSET-026: Start/Stop Discovery Loop", func() {
		It("should start and stop gracefully", func() {
			discoverer = discovery.NewServiceDiscoverer(fakeClient, 10*time.Second)
			discoverer.RegisterDetector(&mockDetector{serviceType: "test"})

			// Start in background
			go func() {
				_ = discoverer.Start(ctx)
			}()

			// Give it time to start
			time.Sleep(100 * time.Millisecond)

			// Stop should not error
			err := discoverer.Stop()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should respect context cancellation", func() {
			discoverer = discovery.NewServiceDiscoverer(fakeClient, 10*time.Second)
			discoverer.RegisterDetector(&mockDetector{serviceType: "test"})

			done := make(chan error)
			go func() {
				done <- discoverer.Start(ctx)
			}()

			// Cancel context
			cancelFunc()

			// Should stop within reasonable time
			Eventually(done, "2s").Should(Receive(Not(BeNil())))
		})
	})
})
