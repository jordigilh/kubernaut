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
	"testing"

	config "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
)

// TestBuildAuditStore_ValidConfig is a characterization test for
// buildAuditStore, extracted from main() in GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 0a. Pins the contract that a default (no live DataStorage connection
// required at construction time) config builds a usable, closeable audit
// store. cmd/remediationorchestrator had zero test coverage before this
// extraction.
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
// any network I/O, surfaced by buildAuditStore as a wrapped error (not a
// panic or silent nil store).
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
