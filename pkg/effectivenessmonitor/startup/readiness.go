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

// Package startup provides best-effort readiness checks for the EffectivenessMonitor.
//
// Prometheus and AlertManager are optional enrichment sources, not startup
// dependencies. CheckExternalServices verifies connectivity when enabled and
// configured, logging warnings on failure instead of terminating the process.
//
// Business Requirements:
// - BR-EM-002: Alert resolution check via AlertManager (best-effort)
// - BR-EM-003: Metric comparison via Prometheus (best-effort)
package startup

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
)

// ExternalServicesConfig holds the configuration needed for startup readiness checks.
type ExternalServicesConfig struct {
	PrometheusEnabled   bool
	PrometheusURL       string
	AlertManagerEnabled bool
	AlertManagerURL     string
}

// ReadinessResult reports the outcome of the best-effort readiness checks.
type ReadinessResult struct {
	PrometheusReachable   bool
	AlertManagerReachable bool
	Error                 error
}

// CheckExternalServices performs best-effort connectivity checks for enabled
// external services. It returns a configuration error if a service is enabled
// but its URL is empty. Connectivity failures are logged as warnings and
// reported in the result, but do not produce an error -- the reconciler
// handles unavailable services gracefully at query time.
func CheckExternalServices(
	ctx context.Context,
	logger logr.Logger,
	cfg ExternalServicesConfig,
	promClient emclient.PrometheusQuerier,
	amClient emclient.AlertManagerClient,
) ReadinessResult {
	result := ReadinessResult{}

	if cfg.PrometheusEnabled {
		reachable, err := checkPrometheusReadiness(ctx, logger, cfg, promClient)
		if err != nil {
			result.Error = err
			return result
		}
		result.PrometheusReachable = reachable
	}

	if cfg.AlertManagerEnabled {
		reachable, err := checkAlertManagerReadiness(ctx, logger, cfg, amClient)
		if err != nil {
			result.Error = err
			return result
		}
		result.AlertManagerReachable = reachable
	}

	return result
}

// checkPrometheusReadiness verifies the configured Prometheus URL is
// non-empty and, best-effort, that it is reachable. Extracted from
// CheckExternalServices (Wave 6 6a GREEN: nestif remediation) — pure code
// motion, no behavior change.
func checkPrometheusReadiness(ctx context.Context, logger logr.Logger, cfg ExternalServicesConfig, promClient emclient.PrometheusQuerier) (bool, error) {
	if cfg.PrometheusURL == "" {
		return false, fmt.Errorf("prometheus is enabled but prometheusUrl is empty")
	}
	if promClient == nil {
		return false, nil
	}
	if err := promClient.Ready(ctx); err != nil {
		logger.Info("Prometheus is enabled but unreachable at startup, metrics assessment will retry at query time",
			"url", cfg.PrometheusURL,
			"error", err.Error(),
		)
		return false, nil
	}
	logger.Info("Prometheus connectivity verified", "url", cfg.PrometheusURL)
	return true, nil
}

// checkAlertManagerReadiness verifies the configured AlertManager URL is
// non-empty and, best-effort, that it is reachable. Extracted from
// CheckExternalServices (Wave 6 6a GREEN: nestif remediation) — pure code
// motion, no behavior change.
func checkAlertManagerReadiness(ctx context.Context, logger logr.Logger, cfg ExternalServicesConfig, amClient emclient.AlertManagerClient) (bool, error) {
	if cfg.AlertManagerURL == "" {
		return false, fmt.Errorf("alertManager is enabled but alertManagerUrl is empty")
	}
	if amClient == nil {
		return false, nil
	}
	if err := amClient.Ready(ctx); err != nil {
		logger.Info("AlertManager is enabled but unreachable at startup, alert resolution will retry at query time",
			"url", cfg.AlertManagerURL,
			"error", err.Error(),
		)
		return false, nil
	}
	logger.Info("AlertManager connectivity verified", "url", cfg.AlertManagerURL)
	return true, nil
}
