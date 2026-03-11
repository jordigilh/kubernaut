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

// Startup Readiness Unit Tests (Issue #331)
//
// Validates that the EffectivenessMonitor starts successfully when Prometheus
// and/or AlertManager are enabled but unreachable. These are best-effort
// enrichment sources, not mandatory startup dependencies.
//
// Business Requirements:
// - BR-EM-002: Alert resolution check (best-effort at startup)
// - BR-EM-003: Metric comparison (best-effort at startup)
package effectivenessmonitor

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/startup"
)

// stubPrometheusClient implements emclient.PrometheusQuerier for testing.
type stubPrometheusClient struct {
	readyErr error
}

func (s *stubPrometheusClient) Query(_ context.Context, _ string, _ time.Time) (*emclient.QueryResult, error) {
	return nil, nil
}
func (s *stubPrometheusClient) QueryRange(_ context.Context, _ string, _, _ time.Time, _ time.Duration) (*emclient.QueryResult, error) {
	return nil, nil
}
func (s *stubPrometheusClient) Ready(_ context.Context) error {
	return s.readyErr
}

// stubAlertManagerClient implements emclient.AlertManagerClient for testing.
type stubAlertManagerClient struct {
	readyErr error
}

func (s *stubAlertManagerClient) GetAlerts(_ context.Context, _ emclient.AlertFilters) ([]emclient.Alert, error) {
	return nil, nil
}
func (s *stubAlertManagerClient) Ready(_ context.Context) error {
	return s.readyErr
}

var _ = Describe("Startup Readiness Check (Issue #331, BR-EM-002, BR-EM-003)", func() {

	var (
		ctx    context.Context
		logger logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logr.Discard()
	})

	// UT-EM-STARTUP-001: Prometheus enabled but unreachable -- should not error
	It("UT-EM-STARTUP-001: should return no error when Prometheus is enabled but unreachable", func() {
		promClient := &stubPrometheusClient{readyErr: fmt.Errorf("connection refused")}

		result := startup.CheckExternalServices(ctx, logger, startup.ExternalServicesConfig{
			PrometheusEnabled: true,
			PrometheusURL:     "http://prometheus:9090",
		}, promClient, nil)

		Expect(result.PrometheusReachable).To(BeFalse(),
			"prometheus should be reported as unreachable")
		Expect(result.Error).NotTo(HaveOccurred(),
			"startup must not fail when Prometheus is unreachable")
	})

	// UT-EM-STARTUP-002: AlertManager enabled but unreachable -- should not error
	It("UT-EM-STARTUP-002: should return no error when AlertManager is enabled but unreachable", func() {
		amClient := &stubAlertManagerClient{readyErr: fmt.Errorf("connection refused")}

		result := startup.CheckExternalServices(ctx, logger, startup.ExternalServicesConfig{
			AlertManagerEnabled: true,
			AlertManagerURL:     "http://alertmanager:9093",
		}, nil, amClient)

		Expect(result.AlertManagerReachable).To(BeFalse(),
			"alertmanager should be reported as unreachable")
		Expect(result.Error).NotTo(HaveOccurred(),
			"startup must not fail when AlertManager is unreachable")
	})

	// UT-EM-STARTUP-003: Both enabled but unreachable -- should not error
	It("UT-EM-STARTUP-003: should return no error when both services are enabled but unreachable", func() {
		promClient := &stubPrometheusClient{readyErr: fmt.Errorf("connection refused")}
		amClient := &stubAlertManagerClient{readyErr: fmt.Errorf("connection refused")}

		result := startup.CheckExternalServices(ctx, logger, startup.ExternalServicesConfig{
			PrometheusEnabled:   true,
			PrometheusURL:       "http://prometheus:9090",
			AlertManagerEnabled: true,
			AlertManagerURL:     "http://alertmanager:9093",
		}, promClient, amClient)

		Expect(result.PrometheusReachable).To(BeFalse())
		Expect(result.AlertManagerReachable).To(BeFalse())
		Expect(result.Error).NotTo(HaveOccurred(),
			"startup must not fail when both services are unreachable")
	})

	// UT-EM-STARTUP-004: Both enabled and reachable -- should report reachable
	It("UT-EM-STARTUP-004: should report both reachable when connectivity succeeds", func() {
		promClient := &stubPrometheusClient{readyErr: nil}
		amClient := &stubAlertManagerClient{readyErr: nil}

		result := startup.CheckExternalServices(ctx, logger, startup.ExternalServicesConfig{
			PrometheusEnabled:   true,
			PrometheusURL:       "http://prometheus:9090",
			AlertManagerEnabled: true,
			AlertManagerURL:     "http://alertmanager:9093",
		}, promClient, amClient)

		Expect(result.PrometheusReachable).To(BeTrue())
		Expect(result.AlertManagerReachable).To(BeTrue())
		Expect(result.Error).NotTo(HaveOccurred())
	})

	// UT-EM-STARTUP-005: Prometheus disabled -- should skip check entirely
	It("UT-EM-STARTUP-005: should skip Prometheus check when disabled", func() {
		result := startup.CheckExternalServices(ctx, logger, startup.ExternalServicesConfig{
			PrometheusEnabled:   false,
			AlertManagerEnabled: false,
		}, nil, nil)

		Expect(result.PrometheusReachable).To(BeFalse(),
			"disabled service should not be reported as reachable")
		Expect(result.AlertManagerReachable).To(BeFalse(),
			"disabled service should not be reported as reachable")
		Expect(result.Error).NotTo(HaveOccurred())
	})

	// UT-EM-STARTUP-006: Enabled but nil client -- should not panic
	It("UT-EM-STARTUP-006: should not panic when enabled but client is nil", func() {
		Expect(func() {
			startup.CheckExternalServices(ctx, logger, startup.ExternalServicesConfig{
				PrometheusEnabled:   true,
				PrometheusURL:       "http://prometheus:9090",
				AlertManagerEnabled: true,
				AlertManagerURL:     "http://alertmanager:9093",
			}, nil, nil)
		}).NotTo(Panic(), "nil clients with enabled config must not panic")
	})

	// UT-EM-STARTUP-007: Enabled but empty URL -- should return config error
	It("UT-EM-STARTUP-007: should return error when Prometheus enabled but URL is empty", func() {
		result := startup.CheckExternalServices(ctx, logger, startup.ExternalServicesConfig{
			PrometheusEnabled: true,
			PrometheusURL:     "",
		}, nil, nil)

		Expect(result.Error).To(HaveOccurred(),
			"empty URL with enabled service is a configuration error")
	})

	// UT-EM-STARTUP-008: AlertManager enabled but empty URL -- should return config error
	It("UT-EM-STARTUP-008: should return error when AlertManager enabled but URL is empty", func() {
		result := startup.CheckExternalServices(ctx, logger, startup.ExternalServicesConfig{
			AlertManagerEnabled: true,
			AlertManagerURL:     "",
		}, nil, nil)

		Expect(result.Error).To(HaveOccurred(),
			"empty URL with enabled service is a configuration error")
	})
})
