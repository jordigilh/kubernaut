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
	"time"

	"github.com/go-logr/logr"

	notificationconfig "github.com/jordigilh/kubernaut/pkg/notification/config"
)

// TestBuildAuditStore_ValidConfig is a characterization test for
// buildAuditStore, extracted from main() in GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 0a. Pins the contract that a default config builds a usable,
// closeable audit store without a live DataStorage connection.
// cmd/notification had zero test coverage before this extraction.
func TestBuildAuditStore_ValidConfig(t *testing.T) {
	cfg := testNotificationConfig(t)

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

// TestBuildDeliveryServices_Defaults pins the contract that console
// delivery is always constructed, and that file/log delivery are only
// constructed when explicitly enabled via config (both disabled by
// zero-value config here).
func TestBuildDeliveryServices_Defaults(t *testing.T) {
	cfg := testNotificationConfig(t)

	ds, err := buildDeliveryServices(cfg, logr.Discard())
	if err != nil {
		t.Fatalf("buildDeliveryServices returned unexpected error: %v", err)
	}
	if ds.console == nil {
		t.Error("expected console delivery service to always be constructed")
	}
	if ds.file != nil {
		t.Error("expected nil file delivery service when Delivery.File.OutputDir is empty")
	}
	if ds.log != nil {
		t.Error("expected nil log delivery service when Delivery.Log.Enabled is false")
	}
}

// TestBuildDeliveryServices_FileDeliveryEnabled pins the file-delivery
// branch: a writable OutputDir constructs a non-nil file delivery service.
func TestBuildDeliveryServices_FileDeliveryEnabled(t *testing.T) {
	cfg := testNotificationConfig(t)
	cfg.Delivery.File.OutputDir = t.TempDir()
	cfg.Delivery.File.Format = "json"
	cfg.Delivery.File.Timeout = 5 * time.Second

	ds, err := buildDeliveryServices(cfg, logr.Discard())
	if err != nil {
		t.Fatalf("buildDeliveryServices returned unexpected error: %v", err)
	}
	if ds.file == nil {
		t.Error("expected non-nil file delivery service when OutputDir is set and writable")
	}
}

// TestBuildDeliveryServices_FileDeliveryUnwritableDir pins the fail-fast
// guard: an OutputDir that cannot be created (parent path is a file, not a
// directory) surfaces as an error rather than a panic or silent skip.
func TestBuildDeliveryServices_FileDeliveryUnwritableDir(t *testing.T) {
	cfg := testNotificationConfig(t)
	// Create a file, then try to use a path *under* that file as OutputDir —
	// os.MkdirAll fails because a path component is not a directory.
	blockingFile := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blockingFile, []byte("blocking"), 0o644); err != nil {
		t.Fatalf("failed to set up blocking file: %v", err)
	}
	cfg.Delivery.File.OutputDir = filepath.Join(blockingFile, "output")

	_, err := buildDeliveryServices(cfg, logr.Discard())
	if err == nil {
		t.Fatal("expected an error when the file output directory cannot be created, got nil")
	}
}

func testNotificationConfig(t *testing.T) *notificationconfig.Config {
	t.Helper()
	cfg := &notificationconfig.Config{}
	cfg.DataStorage.URL = "http://localhost:8095"
	cfg.DataStorage.Timeout = 5 * time.Second
	cfg.DataStorage.Buffer.BufferSize = 100
	cfg.DataStorage.Buffer.BatchSize = 10
	cfg.DataStorage.Buffer.FlushInterval = time.Second
	cfg.DataStorage.Buffer.MaxRetries = 3
	return cfg
}
