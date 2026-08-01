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

package mcp_test

import (
	"context"
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

// recordingAuditStore captures audit events for assertion. Local to this
// package (internal/kubernautagent/mcp) -- the mcp/tools package has its own
// copy (takeover_test.go) that isn't visible here across package boundaries.
type recordingAuditStore struct {
	mu     sync.Mutex
	events []*audit.AuditEvent
}

func (r *recordingAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

func (r *recordingAuditStore) getEvents() []*audit.AuditEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	dst := make([]*audit.AuditEvent, len(r.events))
	copy(dst, r.events)
	return dst
}

var _ = Describe("Session Resumed Audit — BR-INTERACTIVE-003 #5, audit catalog gap follow-up", func() {

	Describe("UT-KA-SESSRESUME-001: SpawnReconstruct emits aiagent.session.resumed", func() {
		It("should emit EventTypeSessionResumed with the correlation and (old) interactive session IDs", func() {
			runner := &reconSpawnRunner{}
			recon := &reconSpawnRecon{
				turns: []mcpinternal.ConversationTurn{
					{Role: "user", Content: "What caused the OOM?", ActingUser: "alice"},
				},
			}
			store := &recordingAuditStore{}

			spawner := mcpinternal.NewReconstructionSpawner(runner, recon, logr.Discard())
			spawner.SetAuditStore(store)

			entry := &mcpinternal.ReconstructionContext{
				CorrelationID: "rr-resume-001",
				SessionID:     "old-sess-resume-001",
			}
			err := spawner.SpawnReconstruct(context.Background(), entry)
			Expect(err).NotTo(HaveOccurred())

			events := store.getEvents()
			Expect(events).To(HaveLen(1))
			Expect(events[0].EventType).To(Equal(audit.EventTypeSessionResumed))
			Expect(events[0].CorrelationID).To(Equal("rr-resume-001"))
			Expect(events[0].SessionID).To(Equal("old-sess-resume-001"))
			Expect(events[0].EventAction).To(Equal(audit.ActionSessionResumed))
			Expect(events[0].EventOutcome).To(Equal(audit.OutcomeSuccess))
		})

		It("should still emit session.resumed when the reconstructor returns an error (best-effort context)", func() {
			runner := &reconSpawnRunner{}
			recon := &reconSpawnRecon{err: errAuditReconTest}
			store := &recordingAuditStore{}

			spawner := mcpinternal.NewReconstructionSpawner(runner, recon, logr.Discard())
			spawner.SetAuditStore(store)

			entry := &mcpinternal.ReconstructionContext{
				CorrelationID: "rr-resume-002",
				SessionID:     "old-sess-resume-002",
			}
			err := spawner.SpawnReconstruct(context.Background(), entry)
			Expect(err).NotTo(HaveOccurred())

			events := store.getEvents()
			Expect(events).To(HaveLen(1), "the identity transition (KA SA resuming control) happened regardless of DS reconstruction success")
			Expect(events[0].EventType).To(Equal(audit.EventTypeSessionResumed))
		})

		It("should not panic or error when no audit store is configured", func() {
			runner := &reconSpawnRunner{}
			recon := &reconSpawnRecon{}
			spawner := mcpinternal.NewReconstructionSpawner(runner, recon, logr.Discard())

			entry := &mcpinternal.ReconstructionContext{
				CorrelationID: "rr-resume-003",
				SessionID:     "old-sess-resume-003",
			}
			err := spawner.SpawnReconstruct(context.Background(), entry)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var errAuditReconTest = &reconTestError{"DS temporarily unavailable"}

type reconTestError struct{ msg string }

func (e *reconTestError) Error() string { return e.msg }
