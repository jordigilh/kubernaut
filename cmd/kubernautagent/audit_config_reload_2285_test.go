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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kaaudit "github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
)

type recordConfigReloadSpyStore struct {
	events []*kaaudit.AuditEvent
}

func (s *recordConfigReloadSpyStore) StoreAudit(_ context.Context, event *kaaudit.AuditEvent) error {
	s.events = append(s.events, event)
	return nil
}

// recordConfigReload is KubernautAgent's onReload callback passed to
// sharedtls.StartCAFileWatcher (GAP-11, Issue #2285): the CA-cert hot-reload
// path had no audit trail parity with every other hot-reloadable component.
var _ = Describe("cmd/kubernautagent recordConfigReload — CA hot-reload audit parity (#2285, CM-3/AU-2/AU-12)", func() {

	Describe("UT-KA-2285-001: emits aiagent.config.reloaded on successful reload", func() {
		It("stores a success event with the ca_cert component", func() {
			store := &recordConfigReloadSpyStore{}

			recordConfigReload(context.Background(), store, nil, logr.Discard())

			Expect(store.events).To(HaveLen(1))
			ev := store.events[0]
			Expect(ev.EventType).To(Equal(kaaudit.EventTypeConfigReloaded))
			Expect(ev.EventAction).To(Equal(kaaudit.ActionConfigReloaded))
			Expect(ev.EventOutcome).To(Equal(kaaudit.OutcomeSuccess))
			Expect(ev.Data["component"]).To(Equal("ca_cert"))
			// Regression (live must-gather-e2e repro against PR #2288): DataStorage's
			// OpenAPI schema requires correlation_id minLength=1. A blank ID was
			// silently dropping this event server-side (best-effort store swallowed
			// the validation error), invisible to this spy-store unit test until
			// asserted explicitly.
			Expect(ev.CorrelationID).NotTo(BeEmpty())
		})
	})

	Describe("UT-KA-2285-002: emits aiagent.config.rejected with reason on failed reload", func() {
		It("stores a failure event carrying the rejection reason", func() {
			store := &recordConfigReloadSpyStore{}
			reloadErr := errors.New("invalid PEM content")

			recordConfigReload(context.Background(), store, reloadErr, logr.Discard())

			Expect(store.events).To(HaveLen(1))
			ev := store.events[0]
			Expect(ev.EventType).To(Equal(kaaudit.EventTypeConfigRejected))
			Expect(ev.EventAction).To(Equal(kaaudit.ActionConfigRejected))
			Expect(ev.EventOutcome).To(Equal(kaaudit.OutcomeFailure))
			Expect(ev.Data["component"]).To(Equal("ca_cert"))
			Expect(ev.Data["rejection_reason"]).To(Equal("invalid PEM content"))
			Expect(ev.CorrelationID).NotTo(BeEmpty())
		})
	})

	Describe("UT-KA-2285-003: does not panic and does not store when audit store is nil", func() {
		It("no-ops safely", func() {
			Expect(func() {
				recordConfigReload(context.Background(), nil, nil, logr.Discard())
			}).NotTo(Panic())
		})
	})
})
