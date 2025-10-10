<<<<<<< HEAD
package k8s_test

import (
	"testing"
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

package k8s_test

import (
>>>>>>> crd_implementation
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
<<<<<<< HEAD
=======
	"testing"
>>>>>>> crd_implementation

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
)

var _ = Describe("ServiceValidators - Implementation Correctness Testing", func() {
	var (
		fakeClient *fake.Clientset
		log        *logrus.Logger
		ctx        context.Context
	)

	BeforeEach(func() {
		fakeClient = fake.NewSimpleClientset()
		log = logrus.New()
		log.SetLevel(logrus.ErrorLevel) // Reduce test noise
		ctx = context.Background()
	})

	// BR-HOLMES-019: Unit tests for Health validator implementation
	Describe("HealthValidator Implementation", func() {
		var validator *k8s.HealthValidator

		BeforeEach(func() {
			validator = k8s.NewHealthValidator(fakeClient, log)
		})

		Context("Service Health Validation", func() {
			It("should validate service with healthy endpoint", func() {
				// Create test HTTP server that returns 200 OK
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					if _, err := w.Write([]byte("OK")); err != nil {
						// Test server write failure is not critical for this test
					}
				}))
				defer server.Close()

				service := &k8s.DetectedService{
					Name:        "healthy-service",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{
							Name:     "web",
							URL:      server.URL,
							Port:     9090,
							Protocol: "http",
						},
					},
				}

				err := validator.Validate(ctx, service)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should handle service with no endpoints gracefully", func() {
				service := &k8s.DetectedService{
					Name:        "no-endpoints",
					Namespace:   "monitoring",
					ServiceType: "grafana",
					Endpoints:   []k8s.ServiceEndpoint{},
				}

				err := validator.Validate(ctx, service)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no endpoints"))
			})

			It("should handle unreachable endpoint gracefully", func() {
				service := &k8s.DetectedService{
					Name:        "unreachable-service",
					Namespace:   "monitoring",
					ServiceType: "jaeger",
					Endpoints: []k8s.ServiceEndpoint{
						{
							Name:     "query",
							URL:      "http://nonexistent-host:16686",
							Port:     16686,
							Protocol: "http",
						},
					},
				}

				// Should not return error - health validator is lenient for network issues
				err := validator.Validate(ctx, service)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should handle HTTP error responses gracefully", func() {
				// Create test HTTP server that returns 500 error
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					if _, err := w.Write([]byte("Internal Server Error")); err != nil {
						// Test server write failure is not critical for this test
					}
				}))
				defer server.Close()

				service := &k8s.DetectedService{
					Name:        "error-service",
					Namespace:   "monitoring",
					ServiceType: "elasticsearch",
					Endpoints: []k8s.ServiceEndpoint{
						{
							Name:     "http",
							URL:      server.URL,
							Port:     9200,
							Protocol: "http",
						},
					},
				}

				// Should not return error - validation is lenient
				err := validator.Validate(ctx, service)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should handle invalid URL gracefully", func() {
				service := &k8s.DetectedService{
					Name:        "invalid-url-service",
					Namespace:   "monitoring",
					ServiceType: "custom",
					Endpoints: []k8s.ServiceEndpoint{
						{
							Name:     "invalid",
							URL:      "not-a-valid-url",
							Port:     8080,
							Protocol: "http",
						},
					},
				}

				// Should not return error - validation is lenient for malformed URLs
				err := validator.Validate(ctx, service)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	// BR-HOLMES-019: Unit tests for Endpoint validator implementation
	Describe("EndpointValidator Implementation", func() {
		var validator *k8s.EndpointValidator

		BeforeEach(func() {
			validator = k8s.NewEndpointValidator(fakeClient, log)
		})

		Context("Kubernetes Service Validation", func() {
			It("should validate service that exists in Kubernetes", func() {
				// Create service in fake Kubernetes
				k8sService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-service",
						Namespace: "monitoring",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "web", Port: 9090},
						},
					},
				}
				_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, k8sService, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				detectedService := &k8s.DetectedService{
					Name:        "existing-service",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "web", Port: 9090},
					},
				}

				err = validator.Validate(ctx, detectedService)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should fail validation for non-existent service", func() {
				detectedService := &k8s.DetectedService{
					Name:        "nonexistent-service",
					Namespace:   "monitoring",
					ServiceType: "grafana",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "web", Port: 3000},
					},
				}

				err := validator.Validate(ctx, detectedService)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("service not found"))
			})
		})

		Context("Endpoint Validation", func() {
			It("should validate service with ready endpoints", func() {
				// Create service
				k8sService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-with-endpoints",
						Namespace: "monitoring",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "web", Port: 9090},
						},
					},
				}
				_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, k8sService, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				// Create endpoints with ready addresses
				endpoints := &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-with-endpoints",
						Namespace: "monitoring",
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{
								{IP: "10.0.0.1"},
								{IP: "10.0.0.2"},
							},
							Ports: []corev1.EndpointPort{
								{Name: "web", Port: 9090},
							},
						},
					},
				}
				_, err = fakeClient.CoreV1().Endpoints("monitoring").Create(ctx, endpoints, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				detectedService := &k8s.DetectedService{
					Name:        "service-with-endpoints",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "web", Port: 9090},
					},
				}

				err = validator.Validate(ctx, detectedService)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should handle service without endpoints gracefully", func() {
				// Create service but no endpoints
				k8sService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-no-endpoints",
						Namespace: "monitoring",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "web", Port: 9090},
						},
					},
				}
				_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, k8sService, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				detectedService := &k8s.DetectedService{
					Name:        "service-no-endpoints",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "web", Port: 9090},
					},
				}

				// Should not fail validation - endpoints might not exist yet
				err = validator.Validate(ctx, detectedService)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should handle service with unready endpoints gracefully", func() {
				// Create service
				k8sService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-unready-endpoints",
						Namespace: "monitoring",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "web", Port: 9090},
						},
					},
				}
				_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, k8sService, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				// Create endpoints with only unready addresses
				endpoints := &corev1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-unready-endpoints",
						Namespace: "monitoring",
					},
					Subsets: []corev1.EndpointSubset{
						{
							NotReadyAddresses: []corev1.EndpointAddress{
								{IP: "10.0.0.1"},
							},
							Ports: []corev1.EndpointPort{
								{Name: "web", Port: 9090},
							},
						},
					},
				}
				_, err = fakeClient.CoreV1().Endpoints("monitoring").Create(ctx, endpoints, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				detectedService := &k8s.DetectedService{
					Name:        "service-unready-endpoints",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "web", Port: 9090},
					},
				}

				// Should not fail validation - services might be starting up
				err = validator.Validate(ctx, detectedService)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("Service Port Validation", func() {
			It("should validate detected endpoints match service ports", func() {
				// Create service with specific ports
				k8sService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "port-validation-service",
						Namespace: "monitoring",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "web", Port: 9090},
							{Name: "admin", Port: 9091},
						},
					},
				}
				_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, k8sService, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				detectedService := &k8s.DetectedService{
					Name:        "port-validation-service",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "web", Port: 9090},   // Matches service port
						{Name: "admin", Port: 9091}, // Matches service port
					},
				}

				err = validator.Validate(ctx, detectedService)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should handle detected endpoints that don't match service ports gracefully", func() {
				k8sService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mismatched-ports-service",
						Namespace: "monitoring",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "web", Port: 9090},
						},
					},
				}
				_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, k8sService, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				detectedService := &k8s.DetectedService{
					Name:        "mismatched-ports-service",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "different", Port: 8080}, // Doesn't match service port
					},
				}

				// Should not fail validation - might be port mapping or ingress
				err = validator.Validate(ctx, detectedService)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should fail validation for detected service with no endpoints", func() {
				k8sService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "no-endpoints-service",
						Namespace: "monitoring",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "web", Port: 9090},
						},
					},
				}
				_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, k8sService, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				detectedService := &k8s.DetectedService{
					Name:        "no-endpoints-service",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints:   []k8s.ServiceEndpoint{}, // No endpoints
				}

				err = validator.Validate(ctx, detectedService)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no endpoints"))
			})
		})
	})

	// BR-HOLMES-027: Unit tests for RBAC validator implementation
	Describe("RBACValidator Implementation", func() {
		var validator *k8s.RBACValidator

		BeforeEach(func() {
			validator = k8s.NewRBACValidator(fakeClient, log)
		})

		Context("Namespace Access Validation", func() {
			It("should validate access to existing namespace", func() {
				// Create namespace in fake Kubernetes
				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "accessible-namespace",
					},
				}
				_, err := fakeClient.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				detectedService := &k8s.DetectedService{
					Name:        "test-service",
					Namespace:   "accessible-namespace",
					ServiceType: "prometheus",
				}

				err = validator.Validate(ctx, detectedService)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should fail validation for non-accessible namespace", func() {
				// Don't create namespace - simulates RBAC restriction
				detectedService := &k8s.DetectedService{
					Name:        "test-service",
					Namespace:   "restricted-namespace",
					ServiceType: "grafana",
				}

				err := validator.Validate(ctx, detectedService)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("cannot access service namespace"))
			})
		})

		Context("Service Access Validation", func() {
			It("should validate access to service in namespace", func() {
				// Create namespace and service
				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-namespace",
					},
				}
				_, err := fakeClient.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				k8sService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "accessible-service",
						Namespace: "test-namespace",
					},
				}
				_, err = fakeClient.CoreV1().Services("test-namespace").Create(ctx, k8sService, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				detectedService := &k8s.DetectedService{
					Name:        "accessible-service",
					Namespace:   "test-namespace",
					ServiceType: "jaeger",
				}

				err = validator.Validate(ctx, detectedService)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should fail validation when service list is restricted", func() {
				// Create namespace but simulate RBAC restriction on service listing
				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "restricted-services-namespace",
					},
				}
				_, err := fakeClient.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				// Add reactor to fake client to simulate RBAC denial
				fakeClient.PrependReactor("list", "services", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
					listAction := action.(ktesting.ListAction)
					if listAction.GetNamespace() == "restricted-services-namespace" {
						return true, nil, fmt.Errorf("services is forbidden: User cannot list services in namespace restricted-services-namespace")
					}
					return false, nil, nil
				})

				detectedService := &k8s.DetectedService{
					Name:        "restricted-service",
					Namespace:   "restricted-services-namespace",
					ServiceType: "elasticsearch",
				}

				err = validator.Validate(ctx, detectedService)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("insufficient permissions"))
			})
		})
	})

	// BR-HOLMES-022: Unit tests for ServiceType validator implementation
	Describe("ServiceTypeValidator Implementation", func() {
		var validator *k8s.ServiceTypeValidator

		BeforeEach(func() {
			validator = k8s.NewServiceTypeValidator(log)
		})

		Context("Basic Validation", func() {
			It("should fail validation for service with empty service type", func() {
				service := &k8s.DetectedService{
					Name:        "test-service",
					Namespace:   "monitoring",
					ServiceType: "", // Empty service type
				}

				err := validator.Validate(ctx, service)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("service type cannot be empty"))
			})

			It("should validate service with valid service type", func() {
				service := &k8s.DetectedService{
					Name:        "test-service",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "web", Port: 9090},
					},
					Capabilities: []string{"query_metrics"},
				}

				err := validator.Validate(ctx, service)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("Prometheus Service Validation", func() {
			It("should validate Prometheus service with required fields", func() {
				service := &k8s.DetectedService{
					Name:        "prometheus-server",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "web", Port: 9090},
					},
					Capabilities: []string{
						"query_metrics", "alert_rules", "time_series",
					},
				}

				err := validator.Validate(ctx, service)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should fail validation for Prometheus service without endpoints", func() {
				service := &k8s.DetectedService{
					Name:         "prometheus-no-endpoints",
					Namespace:    "monitoring",
					ServiceType:  "prometheus",
					Endpoints:    []k8s.ServiceEndpoint{}, // No endpoints
					Capabilities: []string{"query_metrics"},
				}

				err := validator.Validate(ctx, service)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("prometheus service must have at least one endpoint"))
			})

			It("should pass validation for Prometheus service with missing expected capabilities", func() {
				service := &k8s.DetectedService{
					Name:        "prometheus-minimal",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "web", Port: 9090},
					},
					Capabilities: []string{}, // Missing expected capabilities
				}

				// Should not fail - missing capabilities logged as debug message only
				err := validator.Validate(ctx, service)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("Grafana Service Validation", func() {
			It("should validate Grafana service with required fields", func() {
				service := &k8s.DetectedService{
					Name:        "grafana",
					Namespace:   "monitoring",
					ServiceType: "grafana",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "service", Port: 3000},
					},
					Capabilities: []string{
						"get_dashboards", "query_datasource",
					},
				}

				err := validator.Validate(ctx, service)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should fail validation for Grafana service without endpoints", func() {
				service := &k8s.DetectedService{
					Name:        "grafana-no-endpoints",
					Namespace:   "monitoring",
					ServiceType: "grafana",
					Endpoints:   []k8s.ServiceEndpoint{},
				}

				err := validator.Validate(ctx, service)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("grafana service must have at least one endpoint"))
			})
		})

		Context("Custom Service Validation", func() {
			It("should validate custom service with toolset annotation", func() {
				service := &k8s.DetectedService{
					Name:        "custom-service",
					Namespace:   "monitoring",
					ServiceType: "vector-database",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "api", Port: 8080},
					},
					Annotations: map[string]string{
						"kubernaut.io/toolset": "vector-database",
					},
				}

				err := validator.Validate(ctx, service)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should fail validation for custom service without annotations", func() {
				service := &k8s.DetectedService{
					Name:        "custom-no-annotations",
					Namespace:   "monitoring",
					ServiceType: "custom-type",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "api", Port: 8080},
					},
					Annotations: nil, // No annotations
				}

				err := validator.Validate(ctx, service)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("custom service must have annotations"))
			})

			It("should fail validation for custom service without toolset annotation", func() {
				service := &k8s.DetectedService{
					Name:        "custom-no-toolset",
					Namespace:   "monitoring",
					ServiceType: "custom-type",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "api", Port: 8080},
					},
					Annotations: map[string]string{
						"other.annotation": "value",
					}, // Has annotations but no toolset
				}

				err := validator.Validate(ctx, service)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("custom service must have kubernaut.io/toolset annotation"))
			})
		})

		Context("Capability Validation Logic", func() {
			It("should correctly identify when service has all expected capabilities", func() {
				service := &k8s.DetectedService{
					Name:        "complete-prometheus",
					Namespace:   "monitoring",
					ServiceType: "prometheus",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "web", Port: 9090},
					},
					Capabilities: []string{
						"query_metrics", "alert_rules", "time_series", "extra_capability",
					},
				}

				err := validator.Validate(ctx, service)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should handle empty expected capabilities list", func() {
				service := &k8s.DetectedService{
					Name:        "unknown-service-type",
					Namespace:   "monitoring",
					ServiceType: "unknown",
					Endpoints: []k8s.ServiceEndpoint{
						{Name: "api", Port: 8080},
					},
					Capabilities: []string{"any_capability"},
				}

				// Should pass validation for unknown service types
				err := validator.Validate(ctx, service)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUserviceUvalidators(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UserviceUvalidators Suite")
}
