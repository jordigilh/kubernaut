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

// Issue #673 C-ADV-2: Verify handleProcessingError returns generic error details.
// BR-GATEWAY-182: Defense-in-depth -- K8s API errors must not leak internal details.

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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	sharedscope "github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// alwaysManagedScope is a mock ScopeChecker that always returns managed=true.
type alwaysManagedScope struct{}

func (a *alwaysManagedScope) IsManaged(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}

var _ sharedscope.ScopeChecker = &alwaysManagedScope{}

var _ = Describe("Issue #673 C-ADV-2: Generic Processing Error (BR-GATEWAY-182)", Ordered, func() {

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

		// Build a K8s client WITHOUT RemediationRequest in the scheme.
		// This causes ShouldDeduplicate's List call to fail with a K8s error,
		// which flows through handleProcessingError.
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "kubernaut-system"},
		}

		brokenClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(ns).
			Build()

		cfg := &config.ServerConfig{
			Server: config.ServerSettings{
				ListenAddr:   "127.0.0.1:0",
				ReadTimeout:  5 * time.Second,
				WriteTimeout: 10 * time.Second,
				IdleTimeout:  120 * time.Second,
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
			brokenClient,
			nil,                     // audit store
			&alwaysManagedScope{},   // scope always returns managed
			nil,                     // no auth
			nil,                     // no authz
		)
		Expect(err).ToNot(HaveOccurred())

		prometheusAdapter := adapters.NewPrometheusAdapter(nil, nil)
		Expect(gwServer.RegisterAdapter(prometheusAdapter)).To(Succeed())

		testServer = httptest.NewServer(gwServer.Handler())
		DeferCleanup(func() {
			testServer.Close()
		})
	})

	It("IT-GW-673-007: K8s API error returns generic detail without internal info", func() {
		payload := []byte(fmt.Sprintf(`{
			"alerts": [{
				"labels": {
					"alertname": "IT-GW-673-007-Test",
					"namespace": "kubernaut-system",
					"pod": "test-pod-security",
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

		Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError),
			"IT-GW-673-007: K8s error must return 500")

		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		var problem map[string]interface{}
		Expect(json.Unmarshal(body, &problem)).To(Succeed())

		detail, ok := problem["detail"].(string)
		Expect(ok).To(BeTrue(), "IT-GW-673-007: Response must have detail field")

		Expect(detail).To(Equal("Internal server error"),
			"IT-GW-673-007: Error detail must be generic")
		Expect(detail).NotTo(ContainSubstring("kubernetes"),
			"IT-GW-673-007: Must not leak 'kubernetes' in error")
		Expect(detail).NotTo(ContainSubstring("deduplication"),
			"IT-GW-673-007: Must not leak 'deduplication' in error")
		Expect(detail).NotTo(ContainSubstring("RemediationRequest"),
			"IT-GW-673-007: Must not leak CRD type in error")
	})
})
