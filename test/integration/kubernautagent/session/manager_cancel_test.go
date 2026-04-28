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
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
)

var _ = Describe("Kubernaut Agent Session Manager Cancellation — #823 PR3", func() {

	var (
		store   *session.Store
		manager *session.Manager
	)

	BeforeEach(func() {
		store = session.NewStore(5 * time.Minute)
		manager = session.NewManager(store, slog.Default(), audit.NopAuditStore{}, nil)
	})

	Describe("IT-KA-823-C01: Cancelled session stores partial result for snapshot", func() {
		It("session status is cancelled AND partial InvestigationResult is stored", func() {
			proceed := make(chan struct{})

			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				close(proceed)
				<-ctx.Done()
				return &katypes.InvestigationResult{
					Cancelled:      true,
					CancelledPhase: "rca",
					RCASummary:     "partial RCA before cancel",
				}, nil
			}, map[string]string{"remediation_id": "rem-cancel-001"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				select {
				case <-proceed:
					return true
				default:
					return false
				}
			}, 2*time.Second, 10*time.Millisecond).Should(BeTrue(), "investigation function should start")

			err = manager.CancelInvestigation(id)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() interface{} {
				sess, sErr := manager.GetSession(id)
				if sErr != nil {
					return nil
				}
				return sess.Result
			}, 2*time.Second, 10*time.Millisecond).ShouldNot(BeNil(),
				"partial result should be stored on the cancelled session (BR-SESSION-002)")

			sess, err := manager.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusCancelled))

			result, ok := sess.Result.(*katypes.InvestigationResult)
			Expect(ok).To(BeTrue(), "result should be *InvestigationResult")
			Expect(result.Cancelled).To(BeTrue())
			Expect(result.RCASummary).To(Equal("partial RCA before cancel"))
		})
	})

	Describe("IT-KA-823-C02: Cancel during multi-turn with event channel", func() {
		It("event channel is eventually closed and partial result is stored", func() {
			proceed := make(chan struct{})

			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				close(proceed)
				<-ctx.Done()
				return &katypes.InvestigationResult{
					Cancelled:      true,
					CancelledPhase: "workflow_discovery",
					RCASummary:     "full RCA completed, workflow cancelled",
				}, nil
			}, map[string]string{"remediation_id": "rem-cancel-002"})
			Expect(err).NotTo(HaveOccurred())

			<-proceed

			ch, subErr := manager.Subscribe(context.Background(), id)
			Expect(subErr).NotTo(HaveOccurred())
			Expect(ch).NotTo(BeNil())

			err = manager.CancelInvestigation(id)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				_, open := <-ch
				return !open
			}, 2*time.Second, 10*time.Millisecond).Should(BeTrue(),
				"event channel should be closed after investigation goroutine exits")

			sess, gErr := manager.GetSession(id)
			Expect(gErr).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusCancelled))
			Expect(sess.Result).NotTo(BeNil(), "partial result must be stored")
		})
	})

	Describe("IT-KA-823-C03: Non-cancelled investigation regression guard", func() {
		It("produces identical result to v1.4 behavior — completed with full result", func() {
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				return &katypes.InvestigationResult{
					RCASummary: "full investigation completed",
					WorkflowID: "oom-increase-memory",
					Confidence: 0.95,
					Cancelled:  false,
				}, nil
			}, map[string]string{"remediation_id": "rem-normal-001"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, sErr := manager.GetSession(id)
				if sErr != nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			sess, err := manager.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusCompleted))

			result, ok := sess.Result.(*katypes.InvestigationResult)
			Expect(ok).To(BeTrue())
			Expect(result.Cancelled).To(BeFalse(), "non-cancelled investigation should have Cancelled=false")
			Expect(result.RCASummary).To(Equal("full investigation completed"))
			Expect(result.WorkflowID).To(Equal("oom-increase-memory"))
		})
	})
})
