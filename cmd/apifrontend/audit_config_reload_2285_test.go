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

	afaudit "github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
)

type recordCAConfigReloadSpyEmitter struct {
	events []*afaudit.Event
}

func (s *recordCAConfigReloadSpyEmitter) Emit(_ context.Context, event *afaudit.Event) {
	s.events = append(s.events, event)
}

// recordCAConfigReload is apifrontend's onReload callback passed to
// tlswiring.StartCAFileWatcher (GAP-11, Issue #2285): the CA-cert
// hot-reload path had no audit trail parity with the #1450 policy-config
// watcher's config.reloaded/config.rejected events.
func TestRecordCAConfigReload_Success_EmitsChangedKeysCACert(t *testing.T) {
	emitter := &recordCAConfigReloadSpyEmitter{}

	recordCAConfigReload(context.Background(), emitter, nil)

	if len(emitter.events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(emitter.events))
	}
	ev := emitter.events[0]
	if ev.Type != afaudit.EventConfigReloaded {
		t.Errorf("Type = %q, want %q", ev.Type, afaudit.EventConfigReloaded)
	}
	if got := ev.Detail["changed_keys"]; got != "ca_cert" {
		t.Errorf("Detail[changed_keys] = %q, want %q", got, "ca_cert")
	}
}

func TestRecordCAConfigReload_Failure_EmitsRejectionReason(t *testing.T) {
	emitter := &recordCAConfigReloadSpyEmitter{}
	reloadErr := errors.New("invalid PEM content")

	recordCAConfigReload(context.Background(), emitter, reloadErr)

	if len(emitter.events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(emitter.events))
	}
	ev := emitter.events[0]
	if ev.Type != afaudit.EventConfigRejected {
		t.Errorf("Type = %q, want %q", ev.Type, afaudit.EventConfigRejected)
	}
	want := "ca_cert: invalid PEM content"
	if got := ev.Detail["rejection_reason"]; got != want {
		t.Errorf("Detail[rejection_reason] = %q, want %q", got, want)
	}
}

func TestRecordCAConfigReload_NilAuditor_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("recordCAConfigReload panicked with nil auditor: %v", r)
		}
	}()
	recordCAConfigReload(context.Background(), nil, nil)
}
