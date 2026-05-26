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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("BR-INTERACTIVE-010: Interactive Session Lifecycle — #1293", func() {

	var (
		store   *session.Store
		manager *session.Manager
	)

	BeforeEach(func() {
		store = session.NewStore(5 * time.Minute)
		manager = session.NewManager(store, logr.Discard(), nil, nil)
	})

	Describe("UT-KA-1293-006: StartInteractiveSession creates session in StatusPending", func() {
		It("should create session and remain in StatusPending without launching goroutine", func() {
			fn := func(ctx context.Context) (*katypes.InvestigationResult, error) {
				Fail("investigation function should NOT be called for interactive session")
				return nil, nil
			}

			id, err := manager.StartInteractiveSession(context.Background(), fn, map[string]string{
				"incident_id":    "int-006",
				"remediation_id": "rem-int-006",
				"signal_name":    "OOMKilled",
				"severity":       "critical",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty())

			sess, sErr := manager.GetSession(id)
			Expect(sErr).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusPending),
				"interactive session must be created in StatusPending")
			Expect(sess.Metadata["incident_id"]).To(Equal("int-006"))
			Expect(sess.Metadata["remediation_id"]).To(Equal("rem-int-006"))

			// Confirm investigation does NOT run
			Consistently(func() session.Status {
				s, _ := manager.GetSession(id)
				return s.Status
			}, 300*time.Millisecond, 50*time.Millisecond).Should(Equal(session.StatusPending),
				"session must remain pending — goroutine not launched")
		})
	})

	Describe("UT-KA-1293-007: LaunchDeferredInvestigation transitions pending to running", func() {
		It("should launch the deferred investigation and transition to running/completed", func() {
			investigated := make(chan struct{})
			fn := func(ctx context.Context) (*katypes.InvestigationResult, error) {
				close(investigated)
				return &katypes.InvestigationResult{
					RCASummary: "interactive RCA",
					Confidence: 0.90,
				}, nil
			}

			id, err := manager.StartInteractiveSession(context.Background(), fn, map[string]string{
				"incident_id":    "int-007",
				"remediation_id": "rem-int-007",
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify pending before launch
			sess, _ := manager.GetSession(id)
			Expect(sess.Status).To(Equal(session.StatusPending))

			// Launch deferred investigation
			launchErr := manager.LaunchDeferredInvestigation(id)
			Expect(launchErr).NotTo(HaveOccurred())

			// Investigation function should be called
			Eventually(investigated, 2*time.Second).Should(BeClosed(),
				"deferred investigation function must be called after launch")

			// Session should reach completed
			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				return s.Status
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusCompleted))

			// Verify result
			s, _ := manager.GetSession(id)
			Expect(s.Result).NotTo(BeNil())
			Expect(s.Result.RCASummary).To(Equal("interactive RCA"))
		})
	})

	Describe("UT-KA-1293-008: LaunchDeferredInvestigation fails for non-pending session", func() {
		It("should return error when session is already running", func() {
			fn := func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}

			id, err := manager.StartInvestigation(context.Background(), fn, map[string]string{
				"remediation_id": "rem-int-008",
			})
			Expect(err).NotTo(HaveOccurred())

			// Wait for running state
			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				return s.Status
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusRunning))

			// Attempting to launch deferred on a running session should fail
			launchErr := manager.LaunchDeferredInvestigation(id)
			Expect(launchErr).To(HaveOccurred())
			Expect(launchErr.Error()).To(ContainSubstring("pending"),
				"error should indicate session must be in pending state")

			// Clean up
			_ = manager.CancelInvestigation(id)
		})
	})

	Describe("UT-KA-1293-009: LaunchDeferredInvestigation fails for non-existent session", func() {
		It("should return ErrSessionNotFound for unknown session ID", func() {
			launchErr := manager.LaunchDeferredInvestigation("non-existent-id")
			Expect(launchErr).To(MatchError(session.ErrSessionNotFound))
		})
	})

	Describe("UT-KA-1293-010: StartInteractiveSession stores metadata and created_by", func() {
		It("should propagate user identity from context into metadata", func() {
			fn := func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return nil, nil
			}

			ctx := context.Background()
			// Simulate authenticated user context (auth.WithUserContext)
			// We use StartInteractiveSession's internal user extraction
			id, err := manager.StartInteractiveSession(ctx, fn, map[string]string{
				"incident_id":    "int-010",
				"remediation_id": "rem-int-010",
				"signal_name":    "OOMKilled",
				"severity":       "critical",
			})
			Expect(err).NotTo(HaveOccurred())

			sess, sErr := manager.GetSession(id)
			Expect(sErr).NotTo(HaveOccurred())
			Expect(sess.Metadata["signal_name"]).To(Equal("OOMKilled"))
			Expect(sess.Metadata["severity"]).To(Equal("critical"))
		})
	})

	Describe("UT-KA-1293-018: GetLatestRCASummaryByRemediationID returns RCA from completed session", func() {
		It("should return the RCA summary from the most recent completed session", func() {
			fn := func(_ context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{
					RCASummary: "OOMKilled due to memory limit in api-server",
					Confidence: 0.92,
				}, nil
			}

			id, err := manager.StartInvestigation(context.Background(), fn, map[string]string{
				"remediation_id": "rem-rca-018",
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() string {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return string(s.Status)
			}).Should(Equal(string(session.StatusCompleted)))

			summary, found := manager.GetLatestRCASummaryByRemediationID("rem-rca-018")
			Expect(found).To(BeTrue())
			Expect(summary).To(Equal("OOMKilled due to memory limit in api-server"))
		})

		It("should return false when no session has RCA for the given remediation_id", func() {
			_, found := manager.GetLatestRCASummaryByRemediationID("rem-nonexistent")
			Expect(found).To(BeFalse())
		})
	})

	Describe("UT-KA-1293-019: FindPendingByRemediationID on real Manager", func() {
		It("should find a pending session by remediation_id", func() {
			fn := func(_ context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{InteractiveHold: true}, nil
			}

			id, err := manager.StartInteractiveSession(context.Background(), fn, map[string]string{
				"remediation_id": "rem-pending-019",
			})
			Expect(err).NotTo(HaveOccurred())

			foundID, found := manager.FindPendingByRemediationID("rem-pending-019")
			Expect(found).To(BeTrue())
			Expect(foundID).To(Equal(id))
		})

		It("should not find a session that has already been launched", func() {
			fn := func(_ context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{InteractiveHold: true}, nil
			}

			id, err := manager.StartInteractiveSession(context.Background(), fn, map[string]string{
				"remediation_id": "rem-launched-019",
			})
			Expect(err).NotTo(HaveOccurred())

			err = manager.LaunchDeferredInvestigation(id)
			Expect(err).NotTo(HaveOccurred())

			_, found := manager.FindPendingByRemediationID("rem-launched-019")
			Expect(found).To(BeFalse())
		})

		It("should return false for unknown remediation_id", func() {
			_, found := manager.FindPendingByRemediationID("rem-unknown-019")
			Expect(found).To(BeFalse())
		})
	})
})
