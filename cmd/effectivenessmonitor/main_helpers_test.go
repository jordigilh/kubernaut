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

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"

	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	config "github.com/jordigilh/kubernaut/internal/config/effectivenessmonitor"
)

// TestLoadEffectivenessMonitorConfig_DefaultsWhenPathEmpty is a
// characterization test for loadEffectivenessMonitorConfig, extracted from
// main() in GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0a. Pins the contract that
// an empty configPath yields the built-in defaults and does not error.
// cmd/effectivenessmonitor had zero test coverage before this extraction.
func TestLoadEffectivenessMonitorConfig_DefaultsWhenPathEmpty(t *testing.T) {
	atomicLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()

	cfg, err := loadEffectivenessMonitorConfig("", atomicLevel)
	if err != nil {
		t.Fatalf("loadEffectivenessMonitorConfig returned unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("loadEffectivenessMonitorConfig returned a nil config with a nil error")
	}

	want := config.DefaultConfig()
	if cfg.Controller.MetricsAddr != want.Controller.MetricsAddr {
		t.Errorf("Controller.MetricsAddr = %q, want %q", cfg.Controller.MetricsAddr, want.Controller.MetricsAddr)
	}
	if cfg.Assessment.ValidityWindow != want.Assessment.ValidityWindow {
		t.Errorf("Assessment.ValidityWindow = %v, want %v", cfg.Assessment.ValidityWindow, want.Assessment.ValidityWindow)
	}
}

// TestLoadEffectivenessMonitorConfig_MissingFile pins the guard-clause
// contract that a non-empty configPath pointing to a nonexistent file is
// surfaced as a wrapped error (not a panic, not a silent fall-back).
func TestLoadEffectivenessMonitorConfig_MissingFile(t *testing.T) {
	atomicLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()

	cfg, err := loadEffectivenessMonitorConfig(filepath.Join(t.TempDir(), "does-not-exist.yaml"), atomicLevel)
	if err == nil {
		t.Fatal("expected an error for a missing config file, got nil")
	}
	if cfg != nil {
		t.Fatalf("expected a nil config alongside the error, got %+v", cfg)
	}
}

// TestLoadEffectivenessMonitorConfig_AppliesLogLevel pins the Issue #875
// contract that the config-driven log level is applied to atomicLevel.
func TestLoadEffectivenessMonitorConfig_AppliesLogLevel(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Logging.Level = "DEBUG"
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeEffectivenessMonitorConfig(t, path, cfg)

	atomicLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()
	if _, err := loadEffectivenessMonitorConfig(path, atomicLevel); err != nil {
		t.Fatalf("loadEffectivenessMonitorConfig returned unexpected error: %v", err)
	}
	if got := atomicLevel.Level().String(); got != "debug" {
		t.Errorf("atomicLevel.Level() = %q, want %q", got, "debug")
	}
}

func writeEffectivenessMonitorConfig(t *testing.T, path string, cfg *config.Config) {
	t.Helper()
	data := []byte("assessment:\n" +
		"  stabilizationWindow: " + cfg.Assessment.StabilizationWindow.String() + "\n" +
		"  validityWindow: " + cfg.Assessment.ValidityWindow.String() + "\n" +
		"  maxConcurrentReconciles: 10\n" +
		"datastorage:\n" +
		"  url: \"" + cfg.DataStorage.URL + "\"\n" +
		"controller:\n" +
		"  metricsAddr: \"" + cfg.Controller.MetricsAddr + "\"\n" +
		"  healthProbeAddr: \"" + cfg.Controller.HealthProbeAddr + "\"\n" +
		"external:\n" +
		"  prometheusEnabled: false\n" +
		"  alertManagerEnabled: false\n" +
		"  connectionTimeout: 10s\n" +
		"  prometheusLookback: 30m\n" +
		"  scrapeInterval: 60s\n" +
		"logging:\n" +
		"  level: \"" + cfg.Logging.Level + "\"\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write temp config file: %v", err)
	}
}

// TestBuildAuditStore_ValidConfig is a characterization test for
// buildAuditStore. Pins the contract that a default config (no live
// DataStorage connection required at construction time) builds a usable,
// closeable audit store.
func TestBuildAuditStore_ValidConfig(t *testing.T) {
	cfg := config.DefaultConfig()

	store, err := buildAuditStore(cfg)
	if err != nil {
		t.Fatalf("buildAuditStore returned unexpected error: %v", err)
	}
	if store == nil {
		t.Fatal("buildAuditStore returned a nil store with a nil error")
	}

	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() returned unexpected error: %v", err)
	}
}

// TestBuildAuditStore_EmptyDataStorageURL pins the guard-clause contract of
// audit.NewOpenAPIClientAdapter: an empty DataStorage URL is rejected before
// any network I/O, surfaced by buildAuditStore as a wrapped error.
func TestBuildAuditStore_EmptyDataStorageURL(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DataStorage.URL = ""

	store, err := buildAuditStore(cfg)
	if err == nil {
		if store != nil {
			_ = store.Close()
		}
		t.Fatal("expected an error for an empty DataStorage URL, got nil")
	}
	if store != nil {
		t.Fatalf("expected a nil store alongside the error, got %v", store)
	}
}

// TestWaitForCAFile_ReturnsImmediatelyWhenPresent pins the happy-path
// contract of waitForCAFile: when the file already exists and is non-empty,
// it returns the file contents without waiting for the retry timeout.
func TestWaitForCAFile_ReturnsImmediatelyWhenPresent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ca.pem")
	want := []byte("fake-pem-bytes")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatalf("failed to write temp CA file: %v", err)
	}

	start := time.Now()
	got, err := waitForCAFile(path, 2*time.Second, 30*time.Second, logr.Discard())
	if err != nil {
		t.Fatalf("waitForCAFile returned unexpected error: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("waitForCAFile() = %q, want %q", got, want)
	}
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Errorf("waitForCAFile took %v, expected an immediate return for a pre-existing file", elapsed)
	}
}

// TestWaitForCAFile_TimesOutWhenMissing pins the Issue #484 guard-clause
// contract: when the CA file never appears, waitForCAFile returns a wrapped
// error once the retry timeout elapses (not a panic, not an infinite loop).
func TestWaitForCAFile_TimesOutWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never-appears.pem")

	_, err := waitForCAFile(path, 20*time.Millisecond, 100*time.Millisecond, logr.Discard())
	if err == nil {
		t.Fatal("expected a timeout error for a CA file that never appears, got nil")
	}
}

// TestBuildExternalClients_DisabledReturnsNilClients pins the guard-clause
// contract that disabled external services yield nil clients (metric
// comparison / alert resolution checks are skipped) rather than clients
// pointed at empty URLs.
func TestBuildExternalClients_DisabledReturnsNilClients(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.External.PrometheusEnabled = false
	cfg.External.AlertManagerEnabled = false

	promClient, amClient := buildExternalClients(cfg, &http.Client{}, logr.Discard())
	if promClient != nil {
		t.Errorf("expected a nil Prometheus client when disabled, got %v", promClient)
	}
	if amClient != nil {
		t.Errorf("expected a nil AlertManager client when disabled, got %v", amClient)
	}
}

// TestBuildExternalClients_EnabledReturnsNonNilClients pins the contract
// that enabled external services yield non-nil, ready-to-use clients wired
// to the provided HTTP client.
func TestBuildExternalClients_EnabledReturnsNonNilClients(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.External.PrometheusEnabled = true
	cfg.External.AlertManagerEnabled = true

	promClient, amClient := buildExternalClients(cfg, &http.Client{}, logr.Discard())
	if promClient == nil {
		t.Error("expected a non-nil Prometheus client when enabled")
	}
	if amClient == nil {
		t.Error("expected a non-nil AlertManager client when enabled")
	}
}
