/*
Copyright 2026 Jordi Gil.

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

package gateway_test

import (
	"context"
	"errors"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	gatewaypkg "github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	testmocks "github.com/jordigilh/kubernaut/test/shared/mocks"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// ========================================
// GAP-11 (Issue #1505): Gateway config hot-reload audit events
// ========================================
//
// Verifies that EmitConfigReloadAudit records 'gateway.config.reloaded' on
// success and 'gateway.config.rejected' (with the rejection reason) on
// failure, for both hot-reloadable components (log_level, ca_cert) — closing
// the SOC2 CC7.2 / FedRAMP AU-12 gap where hot-reload outcomes were only
// logged, not audited.
// ========================================

var _ = Describe("GAP-11: Gateway config reload audit events", func() {
	var (
		mockAudit *testmocks.MockAuditStore
		server    *gatewaypkg.Server
		ctx       context.Context
		scheme    *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockAudit = testmocks.NewMockAuditStore()

		Expect(os.Setenv("KUBERNAUT_CONTROLLER_NAMESPACE", "kubernaut-system")).To(Succeed())

		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		cfg := &config.ServerConfig{
			Server: config.ServerSettings{
				ListenAddr:   "127.0.0.1:0",
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
				IdleTimeout:  120 * time.Second,
			},
			Processing: config.ProcessingSettings{
				Deduplication: config.DeduplicationSettings{
					CooldownPeriod: 300 * time.Second,
				},
				Retry: config.RetrySettings{
					MaxAttempts:    3,
					InitialBackoff: 100 * time.Millisecond,
					MaxBackoff:     1 * time.Second,
				},
			},
		}

		metricsInstance := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())

		var err error
		server, err = gatewaypkg.NewServerForTesting(gatewaypkg.ServerTestDeps{
			Config: cfg, Logger: logr.Discard(), MetricsInstance: metricsInstance,
			CtrlClient: k8sClient, AuditStore: mockAudit,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	Context("when a hot-reload succeeds", func() {
		It("emits gateway.config.reloaded for the log_level component", func() {
			server.EmitConfigReloadAudit(ctx, "log_level", nil)

			Expect(mockAudit.GetStoreCalls()).To(Equal(1))
			event := mockAudit.GetLastEvent()
			Expect(event).NotTo(BeNil())
			Expect(event.EventType).To(Equal("gateway.config.reloaded"))
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeSuccess))
		})

		It("emits gateway.config.reloaded for the ca_cert component", func() {
			server.EmitConfigReloadAudit(ctx, "ca_cert", nil)

			event := mockAudit.GetLastEvent()
			Expect(event).NotTo(BeNil())
			Expect(event.EventType).To(Equal("gateway.config.reloaded"))
		})
	})

	Context("when a hot-reload is rejected", func() {
		It("emits gateway.config.rejected with the rejection reason", func() {
			reloadErr := errors.New("failed to parse config for log level reload: yaml: unexpected end of file")

			server.EmitConfigReloadAudit(ctx, "log_level", reloadErr)

			Expect(mockAudit.GetStoreCalls()).To(Equal(1))
			event := mockAudit.GetLastEvent()
			Expect(event).NotTo(BeNil())
			Expect(event.EventType).To(Equal("gateway.config.rejected"))
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeFailure))

			payload, ok := event.EventData.GetGatewayConfigRejectedPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.Component).To(Equal("log_level"))
			Expect(payload.RejectionReason).To(Equal(reloadErr.Error()))
		})
	})

	Context("when no audit store is configured", func() {
		It("does not panic and does not attempt to store an event", func() {
			serverWithoutAudit, err := gatewaypkg.NewServerForTesting(gatewaypkg.ServerTestDeps{
				Config: &config.ServerConfig{
					Server: config.ServerSettings{ListenAddr: "127.0.0.1:0"},
					Processing: config.ProcessingSettings{
						Retry: config.RetrySettings{MaxAttempts: 1, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond},
					},
				},
				Logger:          logr.Discard(),
				MetricsInstance: metrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				CtrlClient:      fake.NewClientBuilder().WithScheme(scheme).Build(),
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(func() {
				serverWithoutAudit.EmitConfigReloadAudit(ctx, "log_level", nil)
			}).NotTo(Panic())
		})
	})
})
