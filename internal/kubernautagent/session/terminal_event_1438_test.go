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

package session_test

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Terminal event before session release (#1438)", func() {

	Describe("UT-KA-1438-001 (SI-4): EmitSessionEndedByRR sends terminal event for user_driving session", func() {
		It("should emit InvestigationEvent{Type: session_ended, Phase: reason} to subscriber", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{InteractiveHold: true}, nil
			}, map[string]string{"remediation_id": "rr-1438-001"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(id)
				return s.Status
			}, 5*time.Second).Should(Equal(session.StatusUserDriving))

			ch, err := mgr.Subscribe(context.Background(), id)
			Expect(err).NotTo(HaveOccurred())

			mgr.EmitSessionEndedByRR("rr-1438-001", "inactivity_timeout")

			var evt session.InvestigationEvent
			Eventually(ch, 2*time.Second).Should(Receive(&evt))
			Expect(evt.Type).To(Equal(session.EventTypeSessionEnded),
				"AU-3: terminal event type must be session_ended for traceability")
			Expect(evt.Phase).To(Equal("inactivity_timeout"),
				"SI-4: phase must carry the release reason for lifecycle monitoring")
		})
	})

	Describe("UT-KA-1438-002 (SI-4): EmitSessionEndedByRR is no-op when no user_driving session matches", func() {
		It("should not panic and not send events for non-existent RR ID", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			Expect(func() {
				mgr.EmitSessionEndedByRR("rr-nonexistent", "disconnect")
			}).NotTo(Panic())
		})

		It("should not send events when session is completed (not user_driving)", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{InteractiveHold: true}, nil
			}, map[string]string{"remediation_id": "rr-1438-002"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(id)
				return s.Status
			}, 5*time.Second).Should(Equal(session.StatusUserDriving))

			ch, err := mgr.Subscribe(context.Background(), id)
			Expect(err).NotTo(HaveOccurred())

			err = mgr.CompleteUserDriving(id, &katypes.InvestigationResult{WorkflowID: "wf-done"})
			Expect(err).NotTo(HaveOccurred())

			mgr.EmitSessionEndedByRR("rr-1438-002", "inactivity_timeout")

			Consistently(ch, 200*time.Millisecond).ShouldNot(Receive(),
				"no event should be emitted for a completed session")
		})
	})

	Describe("UT-KA-1438-003 (SI-4): EmitSessionEndedByRR is safe after session release", func() {
		It("should not panic when called after session transitions away from user_driving", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			_, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{InteractiveHold: true}, nil
			}, map[string]string{"remediation_id": "rr-1438-003"})
			Expect(err).NotTo(HaveOccurred())

			Expect(func() {
				mgr.EmitSessionEndedByRR("rr-1438-003", "ttl_expired")
			}).NotTo(Panic(),
				"must be safe to call even during state transition")
		})
	})

	Describe("UT-KA-1438-004 (SI-4): emitTerminalEvent logs when event is dropped on full channel", func() {
		It("should log a warning when the event channel is full", func() {
			var mu sync.Mutex
			var logLines []string
			logger := funcr.New(func(prefix, args string) {
				mu.Lock()
				defer mu.Unlock()
				logLines = append(logLines, prefix+" "+args)
			}, funcr.Options{Verbosity: 10})

			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logger, nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{InteractiveHold: true}, nil
			}, map[string]string{"remediation_id": "rr-1438-004"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(id)
				return s.Status
			}, 5*time.Second).Should(Equal(session.StatusUserDriving))

			_, err = mgr.Subscribe(context.Background(), id)
			Expect(err).NotTo(HaveOccurred())

			// Fill the channel to capacity (64 slots).
			for i := 0; i < 64; i++ {
				mgr.EmitSessionEndedByRR("rr-1438-004", "filler")
			}

			// This call should hit the default branch and log the drop.
			mgr.EmitSessionEndedByRR("rr-1438-004", "inactivity_timeout")

			mu.Lock()
			defer mu.Unlock()
			found := false
			for _, line := range logLines {
				if strings.Contains(line, "terminal event dropped") {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(),
				"SI-4: must log when terminal event is dropped due to full channel")
		})
	})
})
