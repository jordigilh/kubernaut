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
	"errors"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Issue #1420: Shadow Agent Verdict Escalation Fix", func() {

	// ─────────────────────────────────────────────────────────────────────
	// SI-4: Information System Monitoring
	// Security-critical evidence must survive session lifecycle events
	// ─────────────────────────────────────────────────────────────────────

	Describe("SI-4.d: Security evidence must survive session timeout/disconnect", func() {

		It("UT-KA-1420-001: CompleteUserDriving with nil preserves existing result via nil-guard", func() {
			store := session.NewStore(1 * time.Hour)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			investigationStarted := make(chan struct{})
			investigationDone := make(chan struct{})

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				close(investigationStarted)
				<-investigationDone
				return &katypes.InvestigationResult{
					RCASummary:      "OOM investigation",
					InteractiveHold: true,
					Confidence:      0.7,
					AlignmentVerdict: &katypes.AlignmentVerdictResult{
						Result:  "aligned",
						Summary: "all steps clean",
						Total:   5,
					},
				}, nil
			}, map[string]string{"remediation_id": "rr-si4-001"})
			Expect(err).NotTo(HaveOccurred())

			<-investigationStarted

			err = mgr.UpgradeToInteractive(id, "operator@example.com", []string{"sre"})
			Expect(err).NotTo(HaveOccurred())

			close(investigationDone)

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(id)
				return s.Status
			}, 2*time.Second).Should(Equal(session.StatusUserDriving),
				"non-security result with InteractiveHold should land in user_driving")

			sess, err := mgr.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Result).NotTo(BeNil(), "result from investigation must be stored")
			Expect(sess.Result.AlignmentVerdict).NotTo(BeNil())
			originalSummary := sess.Result.AlignmentVerdict.Summary

			err = mgr.CompleteUserDriving(id, nil)
			Expect(err).NotTo(HaveOccurred())

			sess, err = mgr.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusCompleted))
			Expect(sess.Result).NotTo(BeNil(), "SI-4.d: nil-guard must preserve existing result")
			Expect(sess.Result.AlignmentVerdict).NotTo(BeNil(), "SI-4.d: alignment verdict must survive nil completion")
			Expect(sess.Result.AlignmentVerdict.Summary).To(Equal(originalSummary))
		})

		It("UT-KA-1420-002: ForceCompleteByRemediationID with nil preserves existing result via nil-guard", func() {
			store := session.NewStore(1 * time.Hour)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			investigationStarted := make(chan struct{})
			investigationDone := make(chan struct{})

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				close(investigationStarted)
				<-investigationDone
				return &katypes.InvestigationResult{
					RCASummary:      "OOM investigation",
					InteractiveHold: true,
					Confidence:      0.7,
					AlignmentVerdict: &katypes.AlignmentVerdictResult{
						Result:  "aligned",
						Summary: "all steps clean",
						Total:   5,
					},
				}, nil
			}, map[string]string{"remediation_id": "rr-si4-002"})
			Expect(err).NotTo(HaveOccurred())

			<-investigationStarted

			err = mgr.UpgradeToInteractive(id, "operator@example.com", []string{"sre"})
			Expect(err).NotTo(HaveOccurred())

			close(investigationDone)

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(id)
				return s.Status
			}, 2*time.Second).Should(Equal(session.StatusUserDriving),
				"non-security InteractiveHold should land in user_driving")

			sess, err := mgr.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Result).NotTo(BeNil())
			Expect(sess.Result.AlignmentVerdict).NotTo(BeNil())

			err = mgr.ForceCompleteByRemediationID("rr-si4-002", nil)
			Expect(err).NotTo(HaveOccurred())

			sess, err = mgr.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusCompleted))
			Expect(sess.Result).NotTo(BeNil(), "SI-4.d: nil-guard must preserve existing result")
			Expect(sess.Result.AlignmentVerdict).NotTo(BeNil(), "SI-4.d: alignment verdict must survive force-completion with nil")
			Expect(sess.Result.AlignmentVerdict.Summary).To(Equal("all steps clean"))
		})

		It("UT-KA-1420-003: nil-result completion without prior findings preserves nil result", func() {
			store := session.NewStore(1 * time.Hour)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-si4-003"})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() session.Status {
				s, _ := mgr.GetSession(id)
				return s.Status
			}).Should(Equal(session.StatusRunning))

			err = mgr.TransitionToUserDriving(id, "operator@example.com", nil)
			Expect(err).NotTo(HaveOccurred())

			err = mgr.CompleteUserDriving(id, nil)
			Expect(err).NotTo(HaveOccurred())

			sess, err := mgr.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusCompleted))
			Expect(sess.Result).To(BeNil(), "regression: nil result stays nil when no prior findings exist")
		})
	})

	// ─────────────────────────────────────────────────────────────────────
	// IR-4: Incident Handling
	// Prompt injection detection must result in immediate escalation
	// ─────────────────────────────────────────────────────────────────────

	Describe("IR-4.a: Security escalation must not be deferred by interactive hold", func() {

		It("UT-KA-1420-010: alignment_check_failed forces StatusCompleted even when InteractiveHold=true", func() {
			store := session.NewStore(1 * time.Hour)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{
					RCASummary:        "partial RCA",
					InteractiveHold:   true,
					HumanReviewNeeded: true,
				HumanReviewReason: katypes.HumanReviewReasonAlignmentCheckFailed,
				AlignmentVerdict: &katypes.AlignmentVerdictResult{
					Result:                  "suspicious",
					CircuitBreakerActivated: true,
					Summary:                 "prompt injection in step 7",
					Flagged:                 1,
					Total:                   9,
				},
			}, nil
		}, map[string]string{"remediation_id": "rr-ir4-010"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(id)
				return s.Status
			}, 2*time.Second).Should(Equal(session.StatusCompleted),
				"IR-4.a: security escalation must reach terminal state immediately, not user_driving")

			sess, err := mgr.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Result).NotTo(BeNil())
			Expect(sess.Result.HumanReviewNeeded).To(BeTrue())
			Expect(sess.Result.HumanReviewReason).To(Equal(katypes.HumanReviewReasonAlignmentCheckFailed))
			Expect(sess.Result.AlignmentVerdict).NotTo(BeNil())
			Expect(sess.Result.AlignmentVerdict.CircuitBreakerActivated).To(BeTrue())
		})

		It("UT-KA-1420-011: interactiveUpgrade override skipped for alignment_check_failed", func() {
			store := session.NewStore(1 * time.Hour)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			investigationStarted := make(chan struct{})
			investigationDone := make(chan struct{})

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				close(investigationStarted)
				<-investigationDone
				return &katypes.InvestigationResult{
					RCASummary:        "partial RCA",
					HumanReviewNeeded: true,
					HumanReviewReason: katypes.HumanReviewReasonAlignmentCheckFailed,
					AlignmentVerdict: &katypes.AlignmentVerdictResult{
						Result:                  "suspicious",
						CircuitBreakerActivated: true,
						Summary:                 "prompt injection detected",
						Flagged:                 1,
						Total:                   9,
					},
				}, nil
			}, map[string]string{"remediation_id": "rr-ir4-011"})
			Expect(err).NotTo(HaveOccurred())

			<-investigationStarted

			err = mgr.UpgradeToInteractive(id, "operator@example.com", []string{"sre"})
			Expect(err).NotTo(HaveOccurred())

			close(investigationDone)

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(id)
				return s.Status
			}, 2*time.Second).Should(Equal(session.StatusCompleted),
				"IR-4.a: interactiveUpgrade must not trap security escalation in user_driving")

			sess, err := mgr.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Result).NotTo(BeNil())
			Expect(sess.Result.HumanReviewNeeded).To(BeTrue())
			Expect(sess.Result.HumanReviewReason).To(Equal(katypes.HumanReviewReasonAlignmentCheckFailed))
		})

		It("UT-KA-1420-012: non-security InteractiveHold still transitions to user_driving (regression)", func() {
			store := session.NewStore(1 * time.Hour)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{
					RCASummary:      "OOM detected in pod payments-v2",
					InteractiveHold: true,
					Confidence:      0.85,
				}, nil
			}, map[string]string{"remediation_id": "rr-ir4-012"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(id)
				return s.Status
			}, 2*time.Second).Should(Equal(session.StatusUserDriving),
				"regression: non-security InteractiveHold must still result in user_driving")

			sess, err := mgr.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Result).NotTo(BeNil())
			Expect(sess.Result.InteractiveHold).To(BeTrue())
			Expect(sess.Result.HumanReviewNeeded).To(BeFalse())
		})

		It("UT-KA-1420-013: event channel closes after security-escalation completion", func() {
			store := session.NewStore(1 * time.Hour)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{
					HumanReviewNeeded: true,
					HumanReviewReason: katypes.HumanReviewReasonAlignmentCheckFailed,
					InteractiveHold:   true,
					AlignmentVerdict: &katypes.AlignmentVerdictResult{
						Result:                  "suspicious",
						CircuitBreakerActivated: true,
					},
				}, nil
			}, map[string]string{"remediation_id": "rr-ir4-013"})
			Expect(err).NotTo(HaveOccurred())

			ch, subErr := mgr.Subscribe(context.Background(), id)
			if errors.Is(subErr, session.ErrSessionTerminal) {
				// Investigation completed before Subscribe — the security-escalation
				// forced completion (IR-4.a), so the channel closure is trivially satisfied.
				return
			}
			Expect(subErr).NotTo(HaveOccurred())

			Eventually(func() bool {
				select {
				case _, ok := <-ch:
					return !ok
				default:
					return false
				}
			}, 3*time.Second, 50*time.Millisecond).Should(BeTrue(),
				"IR-4.b: event channel must close after security-escalation session completes")
		})
	})
})
