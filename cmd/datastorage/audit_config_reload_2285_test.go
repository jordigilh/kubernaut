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
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

type recordConfigReloadSpyStore struct {
	events []*ogenclient.AuditEventRequest
}

func (s *recordConfigReloadSpyStore) StoreAudit(_ context.Context, event *ogenclient.AuditEventRequest) error {
	s.events = append(s.events, event)
	return nil
}

func (s *recordConfigReloadSpyStore) Flush(_ context.Context) error { return nil }

func (s *recordConfigReloadSpyStore) Close() error { return nil }

// recordConfigReload is Data Storage's onReload callback passed to
// sharedtls.StartCAFileWatcher (GAP-11, Issue #2285): the CA-cert
// hot-reload path had no audit trail parity with every other
// hot-reloadable component.
func TestRecordConfigReload_Success_EmitsConfigReloaded(t *testing.T) {
	store := &recordConfigReloadSpyStore{}

	recordConfigReload(context.Background(), store, nil, logr.Discard())

	if len(store.events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(store.events))
	}
	ev := store.events[0]
	if ev.EventType != "datastorage.config.reloaded" {
		t.Errorf("EventType = %q, want %q", ev.EventType, "datastorage.config.reloaded")
	}
	if !ev.EventData.IsDatastorageConfigReloadedPayload() {
		t.Fatalf("EventData is not DatastorageConfigReloadedPayload")
	}
	if got := ev.EventData.DatastorageConfigReloadedPayload.Component; got != "ca_cert" {
		t.Errorf("Component = %q, want %q", got, "ca_cert")
	}
}

func TestRecordConfigReload_Failure_EmitsConfigRejected(t *testing.T) {
	store := &recordConfigReloadSpyStore{}
	reloadErr := errors.New("invalid PEM content")

	recordConfigReload(context.Background(), store, reloadErr, logr.Discard())

	if len(store.events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(store.events))
	}
	ev := store.events[0]
	if ev.EventType != "datastorage.config.rejected" {
		t.Errorf("EventType = %q, want %q", ev.EventType, "datastorage.config.rejected")
	}
	if !ev.EventData.IsDatastorageConfigRejectedPayload() {
		t.Fatalf("EventData is not DatastorageConfigRejectedPayload")
	}
	payload := ev.EventData.DatastorageConfigRejectedPayload
	if payload.Component != "ca_cert" {
		t.Errorf("Component = %q, want %q", payload.Component, "ca_cert")
	}
	if payload.RejectionReason != "invalid PEM content" {
		t.Errorf("RejectionReason = %q, want %q", payload.RejectionReason, "invalid PEM content")
	}
}

func TestRecordConfigReload_NilAuditStore_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("recordConfigReload panicked with nil auditStore: %v", r)
		}
	}()
	recordConfigReload(context.Background(), nil, nil, logr.Discard())
}
