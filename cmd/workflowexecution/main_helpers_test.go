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
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"

	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	weconfig "github.com/jordigilh/kubernaut/pkg/workflowexecution/config"
	weexecutor "github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

// TestLoadWorkflowExecutionConfig_DefaultsWhenPathEmpty is a characterization
// test for loadWorkflowExecutionConfig, extracted from main() in
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0a. Pins the contract that an empty
// configPath yields the built-in defaults (weconfig.DefaultConfig) and does
// not error, mirroring the pre-refactor inline branch in main().
// cmd/workflowexecution had zero test coverage before this extraction.
func TestLoadWorkflowExecutionConfig_DefaultsWhenPathEmpty(t *testing.T) {
	atomicLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()

	cfg, err := loadWorkflowExecutionConfig("", atomicLevel)
	if err != nil {
		t.Fatalf("loadWorkflowExecutionConfig returned unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("loadWorkflowExecutionConfig returned a nil config with a nil error")
	}

	want := weconfig.DefaultConfig()
	if cfg.Execution.Namespace != want.Execution.Namespace { //nolint:staticcheck // SA5011 false positive: cfg is guaranteed non-nil by the preceding t.Fatal guard
		t.Errorf("Execution.Namespace = %q, want %q", cfg.Execution.Namespace, want.Execution.Namespace)
	}
	if cfg.Controller.MetricsAddr != want.Controller.MetricsAddr {
		t.Errorf("Controller.MetricsAddr = %q, want %q", cfg.Controller.MetricsAddr, want.Controller.MetricsAddr)
	}
}

// TestLoadWorkflowExecutionConfig_MissingFile pins the guard-clause contract
// that a non-empty configPath pointing to a nonexistent file is surfaced as a
// wrapped error (not a panic, not a silent fall-back to defaults).
func TestLoadWorkflowExecutionConfig_MissingFile(t *testing.T) {
	atomicLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()

	cfg, err := loadWorkflowExecutionConfig(filepath.Join(t.TempDir(), "does-not-exist.yaml"), atomicLevel)
	if err == nil {
		t.Fatal("expected an error for a missing config file, got nil")
	}
	if cfg != nil {
		t.Fatalf("expected a nil config alongside the error, got %+v", cfg)
	}
}

// TestLoadWorkflowExecutionConfig_AppliesLogLevelAndValidates pins two
// behaviors bundled into the extracted helper: (1) the config-driven log
// level (Issue #875) is applied to atomicLevel, and (2) cfg.Validate() is
// invoked, surfacing validation failures (e.g. a missing required field) as
// an error instead of a partially-initialized config.
func TestLoadWorkflowExecutionConfig_AppliesLogLevelAndValidates(t *testing.T) {
	valid := weconfig.DefaultConfig()
	valid.Logging.Level = "DEBUG"
	validPath := writeWorkflowExecutionConfig(t, valid)

	atomicLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()
	cfg, err := loadWorkflowExecutionConfig(validPath, atomicLevel)
	if err != nil {
		t.Fatalf("loadWorkflowExecutionConfig returned unexpected error: %v", err)
	}
	if got := atomicLevel.Level().String(); got != "debug" {
		t.Errorf("atomicLevel.Level() = %q, want %q", got, "debug")
	}
	if cfg.Logging.Level != "DEBUG" {
		t.Errorf("cfg.Logging.Level = %q, want %q", cfg.Logging.Level, "DEBUG")
	}

	invalid := weconfig.DefaultConfig()
	invalid.Execution.Namespace = "" // validate:"required"
	invalidPath := writeWorkflowExecutionConfig(t, invalid)

	if _, err := loadWorkflowExecutionConfig(invalidPath, atomicLevel); err == nil {
		t.Fatal("expected a validation error for an empty required field, got nil")
	}
}

// writeWorkflowExecutionConfig marshals cfg to a temp YAML file and returns
// its path, for use with weconfig.LoadFromFile via loadWorkflowExecutionConfig.
func writeWorkflowExecutionConfig(t *testing.T, cfg *weconfig.Config) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("execution:\n" +
		"  namespace: \"" + cfg.Execution.Namespace + "\"\n" +
		"  cooldownPeriod: " + cfg.Execution.CooldownPeriod.String() + "\n" +
		"datastorage:\n" +
		"  url: \"" + cfg.DataStorage.URL + "\"\n" +
		"controller:\n" +
		"  metricsAddr: \"" + cfg.Controller.MetricsAddr + "\"\n" +
		"  healthProbeAddr: \"" + cfg.Controller.HealthProbeAddr + "\"\n" +
		"  leaderElectionId: \"" + cfg.Controller.LeaderElectionID + "\"\n" +
		"logging:\n" +
		"  level: \"" + cfg.Logging.Level + "\"\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write temp config file: %v", err)
	}
	return path
}

// TestBuildAuditStore_ValidConfig is a characterization test for
// buildAuditStore, extracted from main() in GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 0a. Pins the contract that a default config (no live DataStorage
// connection required at construction time) builds a usable, closeable
// audit store.
func TestBuildAuditStore_ValidConfig(t *testing.T) {
	cfg := weconfig.DefaultConfig()

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
// any network I/O, surfaced by buildAuditStore as a wrapped error (not a
// panic or silent nil store).
func TestBuildAuditStore_EmptyDataStorageURL(t *testing.T) {
	cfg := weconfig.DefaultConfig()
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

// TestRegisterAnsibleExecutor_NilConfigIsNoop pins the guard-clause contract
// that registerAnsibleExecutor is a no-op (no registration, no panic) when
// cfg.Ansible is nil, matching the pre-refactor `else if cfg.Ansible != nil`
// branch structure in main().
func TestRegisterAnsibleExecutor_NilConfigIsNoop(t *testing.T) {
	cfg := weconfig.DefaultConfig()
	cfg.Ansible = nil

	registry := weexecutor.NewRegistry()
	registerAnsibleExecutor(cfg, nil, "kubernaut-system", registry, logr.Discard())

	if len(registry.Engines()) != 0 {
		t.Errorf("expected no engines registered, got %v", registry.Engines())
	}
}

// TestRegisterAnsibleExecutor_MissingTokenSecretRefIsNoop pins the
// guard-clause contract that an Ansible config without a TokenSecretRef logs
// an informational message and does not register the ansible executor
// (matching the pre-refactor `cfg.Ansible.TokenSecretRef != nil` guard).
func TestRegisterAnsibleExecutor_MissingTokenSecretRefIsNoop(t *testing.T) {
	cfg := weconfig.DefaultConfig()
	cfg.Ansible = &weconfig.AnsibleConfig{APIURL: "https://awx.example.com"}

	registry := weexecutor.NewRegistry()
	registerAnsibleExecutor(cfg, nil, "kubernaut-system", registry, logr.Discard())

	if len(registry.Engines()) != 0 {
		t.Errorf("expected no engines registered, got %v", registry.Engines())
	}
}
