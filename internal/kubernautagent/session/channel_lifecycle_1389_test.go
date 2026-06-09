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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// trySend attempts a non-blocking send on the channel. Returns true if the
// event was accepted, false if dropped (buffer full or nil sink), and panics
// if the channel is closed — which is the exact production symptom of #1389.
func trySend(ch chan<- session.InvestigationEvent, evt session.InvestigationEvent) (sent bool) {
	defer func() {
		if r := recover(); r != nil {
			panic(r)
		}
	}()
	if ch == nil {
		return false
	}
	select {
	case ch <- evt:
		return true
	default:
		return false
	}
}

var _ = Describe("Event channel lifecycle (#1389)", func() {

	// UT-KA-1389-001: The core regression test.
	// With the buggy defer, the investigation goroutine closes the channel
	// on exit, so sending on it panics. With the fix, the channel stays
	// open until session completion and workflow_discovery can emit events.
	It("UT-KA-1389-001: event channel stays open after investigation goroutine exits", func() {
		store := session.NewStore(30 * time.Minute)
		mgr := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)

		subscribed := make(chan struct{})
		id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
			<-subscribed
			return &katypes.InvestigationResult{
				RCASummary:      "done",
				InteractiveHold: true,
			}, nil
		}, map[string]string{"remediation_id": "rr-1389-001"})
		Expect(err).NotTo(HaveOccurred())

		ch, subErr := mgr.Subscribe(context.Background(), id)
		Expect(subErr).NotTo(HaveOccurred())
		Expect(ch).NotTo(BeNil())
		close(subscribed)

		// Wait for the goroutine to transition the session to UserDriving.
		Eventually(func() session.Status {
			s, _ := mgr.GetSession(id)
			if s == nil {
				return ""
			}
			return s.Status
		}, 5*time.Second).Should(Equal(session.StatusUserDriving))

		// Allow defers to fully complete.
		time.Sleep(100 * time.Millisecond)

		// Drain any buffered events (including EventTypeComplete).
	drainLoop:
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					break drainLoop
				}
			default:
				break drainLoop
			}
		}

		// The LazySink must still be writable so workflow_discovery can
		// emit events. With the buggy defer, the channel is closed and
		// this send panics.
		ls, found := mgr.GetSessionLazySink(id)
		Expect(found).To(BeTrue(), "LazySink must still exist")
		activeSink := ls.Get()

		Expect(func() {
			trySend(activeSink, session.InvestigationEvent{
				Type:  session.EventTypeTokenDelta,
				Phase: "workflow_discovery",
			})
		}).NotTo(Panic(),
			"sending on event channel after investigation goroutine exits must not panic — "+
				"if it panics, the defer closeEventChan bug (#1389) is present")
	})

	// UT-KA-1389-002: Verifies that CompleteUserDriving closes the channel.
	It("UT-KA-1389-002: CompleteUserDriving closes the event channel", func() {
		store := session.NewStore(30 * time.Minute)
		mgr := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)

		subscribed := make(chan struct{})
		id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
			<-subscribed
			return &katypes.InvestigationResult{
				RCASummary:      "done",
				InteractiveHold: true,
			}, nil
		}, map[string]string{"remediation_id": "rr-1389-002"})
		Expect(err).NotTo(HaveOccurred())

		ch, subErr := mgr.Subscribe(context.Background(), id)
		Expect(subErr).NotTo(HaveOccurred())
		close(subscribed)

		// Wait for UserDriving status.
		Eventually(func() session.Status {
			s, _ := mgr.GetSession(id)
			if s == nil {
				return ""
			}
			return s.Status
		}, 5*time.Second).Should(Equal(session.StatusUserDriving))

		// Drain buffered events.
	drainLoop:
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					break drainLoop
				}
			default:
				break drainLoop
			}
		}

		// Complete the session — this should close the channel.
		err = mgr.CompleteUserDriving(id, &katypes.InvestigationResult{WorkflowID: "wf-1"})
		Expect(err).NotTo(HaveOccurred())

		// Channel must be closed.
		Eventually(func() bool {
			select {
			case _, ok := <-ch:
				return !ok
			default:
				return false
			}
		}, 2*time.Second).Should(BeTrue(),
			"event channel must be closed after CompleteUserDriving")
	})
})
