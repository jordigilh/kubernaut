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
	"encoding/json"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Fix #1390: UpgradeToInteractive Integration — BR-INTERACTIVE-004", func() {

	Describe("IT-KA-1390-W01 [SC-24]: Full upgrade flow: autonomous -> upgrade -> InteractiveHold -> user_driving with result", func() {
		It("should complete the full upgrade lifecycle through real Manager/Store wiring", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)

			upgradeDone := make(chan struct{})
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-upgradeDone
				return &katypes.InvestigationResult{
					RCASummary:      "Autonomous RCA with late upgrade",
					Confidence:      0.92,
					InteractiveHold: session.InteractiveUpgradeFromContext(ctx),
				}, nil
			}, map[string]string{"remediation_id": "rr-it-w01", "incident_id": "inc-it-w01"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusRunning))

			err = manager.UpgradeToInteractive(id, "sre-user@example.com", []string{"sre"})
			Expect(err).NotTo(HaveOccurred())
			close(upgradeDone)

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusUserDriving))

			sess, err := manager.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Result).NotTo(BeNil())
			Expect(sess.Result.InteractiveHold).To(BeTrue())
			Expect(sess.Result.RCASummary).To(Equal("Autonomous RCA with late upgrade"))
			Expect(sess.Metadata["acting_user"]).To(Equal("sre-user@example.com"))
		})
	})

	Describe("IT-KA-1390-W03 [SI-4]: EventLogBridge wired after upgrade; events stream to subscriber", func() {
		It("should deliver investigation events through LazySink after Subscribe on an upgraded session", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)

			eventReceived := make(chan struct{})
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				sink := session.EventSinkFromContext(ctx)
				for sink == nil {
					time.Sleep(10 * time.Millisecond)
					sink = session.EventSinkFromContext(ctx)
				}
				sink <- session.InvestigationEvent{
					Type: session.EventTypeReasoningDelta,
					Data: json.RawMessage(`"upgrade-event-stream"`),
				}
				close(eventReceived)

				return &katypes.InvestigationResult{
					RCASummary:      "streamed via bridge",
					InteractiveHold: session.InteractiveUpgradeFromContext(ctx),
				}, nil
			}, map[string]string{"remediation_id": "rr-it-w03"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusRunning))

			Expect(manager.UpgradeToInteractive(id, "testuser", nil)).To(Succeed())

			ch, subErr := manager.Subscribe(context.Background(), id)
			Expect(subErr).NotTo(HaveOccurred())
			Expect(ch).NotTo(BeNil())

			Eventually(eventReceived, 2*time.Second).Should(BeClosed())
			Eventually(ch, 500*time.Millisecond).Should(Receive(HaveField("Data", json.RawMessage(`"upgrade-event-stream"`))))
		})
	})

	Describe("IT-KA-1390-W05 [SC-24]: MCP disconnect during upgrade window -> ForceComplete handles gracefully", func() {
		It("should handle ForceCompleteByRemediationID without panic or orphan goroutine", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)

			goroutineExited := make(chan struct{})
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				defer close(goroutineExited)
				<-ctx.Done()
				return &katypes.InvestigationResult{RCASummary: "aborted"}, ctx.Err()
			}, map[string]string{"remediation_id": "rr-it-w05"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusRunning))

			Expect(manager.UpgradeToInteractive(id, "user-who-disconnects", nil)).To(Succeed())

			err = manager.ForceCompleteByRemediationID("rr-it-w05", &katypes.InvestigationResult{
				RCASummary: "MCP disconnect cleanup",
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(goroutineExited, 2*time.Second).Should(BeClosed(),
				"goroutine must exit after ForceComplete cancels context")

			sess, err := manager.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(SatisfyAny(
				Equal(session.StatusCompleted),
				Equal(session.StatusUserDriving),
			))
		})
	})
})
