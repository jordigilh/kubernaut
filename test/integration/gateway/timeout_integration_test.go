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

package gateway

// Issue #673 L-3: Per-handler K8s API timeout
// BR-GATEWAY-102: Gateway must enforce timeouts for all external operations
//
// These tests validate that the gateway enforces a configurable timeout on
// ProcessSignal (K8s API operations) and returns 504 Gateway Timeout when exceeded.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

var _ = Describe("Issue #673 L-3: K8s API Timeout (BR-GATEWAY-102)", Ordered, func() {

	Context("Timeout enforcement", func() {
		var (
			testServer *httptest.Server
		)

		BeforeAll(func() {
			previousNS := os.Getenv("KUBERNAUT_CONTROLLER_NAMESPACE")
			Expect(os.Setenv("KUBERNAUT_CONTROLLER_NAMESPACE", "kubernaut-system")).To(Succeed())
			DeferCleanup(func() {
				if previousNS != "" {
					os.Setenv("KUBERNAUT_CONTROLLER_NAMESPACE", previousNS)
				} else {
					os.Unsetenv("KUBERNAUT_CONTROLLER_NAMESPACE")
				}
			})

			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())

			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "kubernaut-system"},
			}

			// Fake client with interceptor that introduces 200ms delay on List
			slowClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(ns).
				WithInterceptorFuncs(interceptor.Funcs{
					List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
						select {
						case <-ctx.Done():
							return ctx.Err()
						case <-time.After(200 * time.Millisecond):
							return c.List(ctx, list, opts...)
						}
					},
				}).
				Build()

			cfg := &config.ServerConfig{
				Server: config.ServerSettings{
					ListenAddr:       "127.0.0.1:0",
					ReadTimeout:      5 * time.Second,
					WriteTimeout:     10 * time.Second,
					IdleTimeout:      120 * time.Second,
					K8sRequestTimeout: 50 * time.Millisecond, // Very short -- will trigger before 200ms List delay
				},
				Processing: config.ProcessingSettings{
					Deduplication: config.DeduplicationSettings{
						CooldownPeriod: 300 * time.Second,
					},
					Retry: config.DefaultRetrySettings(),
				},
			}

			registry := prometheus.NewRegistry()
			metricsInstance := metrics.NewMetricsWithRegistry(registry)

			gwServer, err := gateway.NewServerForTesting(
				cfg,
				logr.Discard(),
				metricsInstance,
				slowClient,
				nil,
				&alwaysManagedScope{},
				nil,
				nil,
			)
			Expect(err).ToNot(HaveOccurred())

			prometheusAdapter := adapters.NewPrometheusAdapter(nil, nil)
			Expect(gwServer.RegisterAdapter(prometheusAdapter)).To(Succeed())

			testServer = httptest.NewServer(gwServer.Handler())
			DeferCleanup(func() {
				testServer.Close()
			})
		})

		It("IT-GW-673-008: Slow K8s API exceeding timeout returns 504 Gateway Timeout", func() {
			payload := []byte(fmt.Sprintf(`{
				"alerts": [{
					"labels": {
						"alertname": "IT-GW-673-008-Timeout",
						"namespace": "kubernaut-system",
						"pod": "test-pod-timeout",
						"severity": "warning"
					},
					"startsAt": "%s"
				}]
			}`, time.Now().Format(time.RFC3339)))

			req, err := http.NewRequest(http.MethodPost,
				testServer.URL+"/api/v1/signals/prometheus",
				bytes.NewReader(payload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusGatewayTimeout),
				"IT-GW-673-008: K8s timeout must return 504")

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			var problem map[string]interface{}
			Expect(json.Unmarshal(body, &problem)).To(Succeed())

			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/gateway-timeout"),
				"IT-GW-673-008: 504 response must use ErrorTypeGatewayTimeout")
			Expect(problem["title"]).To(Equal("Gateway Timeout"),
				"IT-GW-673-008: 504 response must use TitleGatewayTimeout")

			detail, ok := problem["detail"].(string)
			Expect(ok).To(BeTrue())
			Expect(detail).NotTo(ContainSubstring("kubernetes"),
				"IT-GW-673-008: Must not leak K8s details in timeout error")
			Expect(detail).NotTo(ContainSubstring("context deadline"),
				"IT-GW-673-008: Must not leak Go context details")
		})

		It("IT-GW-673-010: 504 detail is generic 'Request processing timed out'", func() {
			payload := []byte(fmt.Sprintf(`{
				"alerts": [{
					"labels": {
						"alertname": "IT-GW-673-010-Detail",
						"namespace": "kubernaut-system",
						"pod": "test-pod-detail",
						"severity": "critical"
					},
					"startsAt": "%s"
				}]
			}`, time.Now().Format(time.RFC3339)))

			req, err := http.NewRequest(http.MethodPost,
				testServer.URL+"/api/v1/signals/prometheus",
				bytes.NewReader(payload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusGatewayTimeout))

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			var problem map[string]interface{}
			Expect(json.Unmarshal(body, &problem)).To(Succeed())

			Expect(problem["detail"]).To(Equal("Request processing timed out"),
				"IT-GW-673-010: Timeout detail must be exactly 'Request processing timed out'")
		})
	})

	Context("Normal operation within timeout", func() {
		var (
			testServer *httptest.Server
		)

		BeforeAll(func() {
			previousNS := os.Getenv("KUBERNAUT_CONTROLLER_NAMESPACE")
			Expect(os.Setenv("KUBERNAUT_CONTROLLER_NAMESPACE", "kubernaut-system")).To(Succeed())
			DeferCleanup(func() {
				if previousNS != "" {
					os.Setenv("KUBERNAUT_CONTROLLER_NAMESPACE", previousNS)
				} else {
					os.Unsetenv("KUBERNAUT_CONTROLLER_NAMESPACE")
				}
			})

			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())

			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "kubernaut-system"},
			}

			normalClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(ns).
				Build()

			cfg := &config.ServerConfig{
				Server: config.ServerSettings{
					ListenAddr:       "127.0.0.1:0",
					ReadTimeout:      5 * time.Second,
					WriteTimeout:     10 * time.Second,
					IdleTimeout:      120 * time.Second,
					K8sRequestTimeout: 15 * time.Second, // Default production value -- plenty of headroom
				},
				Processing: config.ProcessingSettings{
					Deduplication: config.DeduplicationSettings{
						CooldownPeriod: 300 * time.Second,
					},
					Retry: config.DefaultRetrySettings(),
				},
			}

			registry := prometheus.NewRegistry()
			metricsInstance := metrics.NewMetricsWithRegistry(registry)

			gwServer, err := gateway.NewServerForTesting(
				cfg,
				logr.Discard(),
				metricsInstance,
				normalClient,
				nil,
				&alwaysManagedScope{},
				nil,
				nil,
			)
			Expect(err).ToNot(HaveOccurred())

			prometheusAdapter := adapters.NewPrometheusAdapter(nil, nil)
			Expect(gwServer.RegisterAdapter(prometheusAdapter)).To(Succeed())

			testServer = httptest.NewServer(gwServer.Handler())
			DeferCleanup(func() {
				testServer.Close()
			})
		})

		It("IT-GW-673-009: Request within timeout returns normally (not 504)", func() {
			payload := []byte(fmt.Sprintf(`{
				"alerts": [{
					"labels": {
						"alertname": "IT-GW-673-009-Normal",
						"namespace": "kubernaut-system",
						"pod": "test-pod-normal",
						"severity": "warning"
					},
					"startsAt": "%s"
				}]
			}`, time.Now().Format(time.RFC3339)))

			req, err := http.NewRequest(http.MethodPost,
				testServer.URL+"/api/v1/signals/prometheus",
				bytes.NewReader(payload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).NotTo(Equal(http.StatusGatewayTimeout),
				"IT-GW-673-009: Fast request must NOT be rejected for timeout")
		})
	})
})

// alwaysManagedScope is defined in security_integration_test.go
