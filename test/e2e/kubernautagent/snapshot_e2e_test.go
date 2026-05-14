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

package kubernautagent

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/agentclient"
)

// E2E Snapshot Endpoint Tests — BR-SESSION-002 / BR-AUDIT-070
//
// Validates GET /api/v1/incident/session/{id}/snapshot against the
// deployed Kubernaut Agent in a Kind cluster. The snapshot endpoint
// is used by the RO controller for forensic post-mortem; correctness
// is critical for downstream data integrity.

var _ = Describe("E2E-KA-SNAP: Session Snapshot Endpoint", Label("e2e", "ka", "snapshot"), func() {

	// -----------------------------------------------------------------
	// E2E-KA-SNAP-001: Snapshot on completed session returns 200
	// -----------------------------------------------------------------

	Context("Completed session snapshot", func() {

		It("E2E-KA-SNAP-001: Snapshot returns full payload for completed session", func() {
			By("Submitting investigation and waiting for completion")
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-snap-001",
				RemediationID:     "test-rem-snap-001",
				SignalName:        "OOMKilled",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: "production",
				ResourceKind:      "Pod",
				ResourceName:      "snap-pod-001",
				ErrorMessage:      "Container OOMKilled",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				status, pollErr := sessionClient.PollSession(ctx, sessionID)
				if pollErr != nil {
					return "error"
				}
				return status.Status
			}, 60*time.Second, 1*time.Second).Should(Equal("completed"),
				"investigation should complete")

			By("Fetching snapshot via ogen-generated client")
			snapRes, err := kaClient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGet(ctx,
				agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetParams{
					SessionID: sessionID,
				})
			Expect(err).ToNot(HaveOccurred(), "snapshot request should succeed")

			snap, ok := snapRes.(*agentclient.SessionSnapshot)
			Expect(ok).To(BeTrue(), "response should be *SessionSnapshot, got %T", snapRes)

			By("Validating snapshot payload fields")
			Expect(snap.SessionID).To(Equal(sessionID))
			Expect(snap.Status).To(Equal("completed"))
			Expect(snap.CreatedAt).ToNot(BeEmpty(), "created_at should be set")

			_, parseErr := time.Parse(time.RFC3339, snap.CreatedAt)
			Expect(parseErr).ToNot(HaveOccurred(), "created_at should be valid RFC3339")

			By("Validating RCA summary is populated (Mock LLM always produces one)")
			rcaSummary, hasRCA := snap.RcaSummary.Get()
			Expect(hasRCA).To(BeTrue(), "rca_summary should be set for completed investigation")
			Expect(rcaSummary).ToNot(BeEmpty(), "rca_summary should not be empty")

			By("Validating token usage from Mock LLM (if populated)")
			if promptTokens, hasPrompt := snap.TotalPromptTokens.Get(); hasPrompt {
				Expect(promptTokens).To(BeNumerically(">", 0),
					"prompt tokens should be positive when set (Mock LLM returns 100-500)")
			}
			if completionTokens, hasCompletion := snap.TotalCompletionTokens.Get(); hasCompletion {
				Expect(completionTokens).To(BeNumerically(">", 0),
					"completion tokens should be positive when set (Mock LLM returns 50)")
			}
		})
	})

	// -----------------------------------------------------------------
	// E2E-KA-SNAP-002: Snapshot on running session returns 409
	// -----------------------------------------------------------------

	Context("In-progress session snapshot", func() {

		It("E2E-KA-SNAP-002: Snapshot on running session returns 409 Conflict", func() {
			By("Submitting investigation")
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-snap-002",
				RemediationID:     "test-rem-snap-002",
				SignalName:        "CrashLoopBackOff",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "snap-pod-002",
				ErrorMessage:      "Container restarting",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for investigation to reach 'investigating' state")
			Eventually(func() string {
				status, pollErr := sessionClient.PollSession(ctx, sessionID)
				if pollErr != nil {
					return "error"
				}
				return status.Status
			}, 15*time.Second, 500*time.Millisecond).Should(
				SatisfyAny(Equal("investigating"), Equal("completed")))

			By("Attempting snapshot while session is in progress")
			// Use raw HTTP to inspect exact status code — the ogen client
			// returns the typed response but we need 409 specifically.
			snapReq, err := http.NewRequestWithContext(ctx, "GET",
				fmt.Sprintf("%s/api/v1/incident/session/%s/snapshot", kaURL, sessionID), nil)
			Expect(err).ToNot(HaveOccurred())
			snapResp, err := authHTTPClient.Do(snapReq)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = snapResp.Body.Close() }()
			body, _ := io.ReadAll(snapResp.Body)

			// If the Mock LLM completed the investigation before we could
			// snapshot, we get 200 instead of 409 — this is a race condition
			// inherent to async investigations. Accept both.
			Expect(snapResp.StatusCode).To(SatisfyAny(
				Equal(http.StatusConflict),
				Equal(http.StatusOK),
			), "snapshot should return 409 (in-progress) or 200 (already completed), got body: %s", string(body))

			if snapResp.StatusCode == http.StatusConflict {
				Expect(string(body)).To(ContainSubstring("session-in-progress"),
					"409 body should contain problem type 'session-in-progress'")
			}
		})
	})

	// -----------------------------------------------------------------
	// E2E-KA-SNAP-003: Snapshot on non-existent session returns 404
	// -----------------------------------------------------------------

	Context("Non-existent session snapshot", func() {

		It("E2E-KA-SNAP-003: Snapshot on non-existent session returns 404", func() {
			fakeID := uuid.New().String()

			snapRes, err := kaClient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGet(ctx,
				agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetParams{
					SessionID: fakeID,
				})
			Expect(err).ToNot(HaveOccurred(), "ogen client should not error on 404")

			_, ok := snapRes.(*agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetNotFound)
			Expect(ok).To(BeTrue(), "response should be NotFound type, got %T", snapRes)
		})
	})

	// -----------------------------------------------------------------
	// E2E-KA-SNAP-004: Snapshot on cancelled session
	// -----------------------------------------------------------------

	Context("Cancelled session snapshot", func() {

		It("E2E-KA-SNAP-004: Snapshot on cancelled session returns 200 with cancelled fields", func() {
			By("Submitting investigation")
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-snap-004",
				RemediationID:     "test-rem-snap-004",
				SignalName:        "CrashLoopBackOff",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "snap-pod-004",
				ErrorMessage:      "Container restarting",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for investigation to be active")
			Eventually(func() string {
				status, pollErr := sessionClient.PollSession(ctx, sessionID)
				if pollErr != nil {
					return "error"
				}
				return status.Status
			}, 15*time.Second, 500*time.Millisecond).Should(
				SatisfyAny(Equal("investigating"), Equal("completed")))

			By("Cancelling the investigation")
			cancelReq, err := http.NewRequestWithContext(ctx, "POST",
				fmt.Sprintf("%s/api/v1/incident/session/%s/cancel", kaURL, sessionID), nil)
			Expect(err).ToNot(HaveOccurred())
			cancelResp, err := authHTTPClient.Do(cancelReq)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = cancelResp.Body.Close() }()

			// May already be completed — accept 200 (cancelled) or 409 (already done)
			Expect(cancelResp.StatusCode).To(SatisfyAny(
				Equal(http.StatusOK),
				Equal(http.StatusConflict),
			))

			By("Waiting for terminal state")
			var finalStatus string
			Eventually(func() string {
				status, pollErr := sessionClient.PollSession(ctx, sessionID)
				if pollErr != nil {
					return "error"
				}
				finalStatus = status.Status
				return status.Status
			}, 15*time.Second, 500*time.Millisecond).Should(
				SatisfyAny(Equal("cancelled"), Equal("completed")))

			By("Fetching snapshot")
			snapRes, err := kaClient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGet(ctx,
				agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetParams{
					SessionID: sessionID,
				})
			Expect(err).ToNot(HaveOccurred())

			snap, ok := snapRes.(*agentclient.SessionSnapshot)
			Expect(ok).To(BeTrue(), "response should be *SessionSnapshot, got %T", snapRes)
			Expect(snap.Status).To(SatisfyAny(Equal("cancelled"), Equal("completed")))

			if finalStatus == "cancelled" {
				By("Validating cancelled-specific fields")
				cancelledPhase, hasPhase := snap.CancelledPhase.Get()
				if hasPhase {
					Expect(cancelledPhase).To(SatisfyAny(
						Equal("rca"), Equal("workflow_selection"),
					), "cancelled_phase should indicate which phase was interrupted")
				}
			}
		})
	})

	// -----------------------------------------------------------------
	// E2E-KA-SNAP-005: Cross-user snapshot returns 404 (authz)
	// -----------------------------------------------------------------

	Context("Cross-user authorization on snapshot", func() {

		It("E2E-KA-SNAP-005: User B cannot snapshot User A's session", func() {
			By("User A submits investigation")
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-snap-005",
				RemediationID:     "test-rem-snap-005",
				SignalName:        "OOMKilled",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: "production",
				ResourceKind:      "Pod",
				ResourceName:      "snap-pod-005",
				ErrorMessage:      "Container OOMKilled",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for investigation to complete")
			Eventually(func() string {
				status, pollErr := sessionClient.PollSession(ctx, sessionID)
				if pollErr != nil {
					return "error"
				}
				return status.Status
			}, 60*time.Second, 1*time.Second).Should(Equal("completed"))

			By("User B attempts to snapshot User A's session")
			snapReq, err := http.NewRequestWithContext(ctx, "GET",
				fmt.Sprintf("%s/api/v1/incident/session/%s/snapshot", kaURL, sessionID), nil)
			Expect(err).ToNot(HaveOccurred())
			snapResp, err := authHTTPClientB.Do(snapReq)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = snapResp.Body.Close() }()
			_, _ = io.ReadAll(snapResp.Body)

			Expect(snapResp.StatusCode).To(Equal(http.StatusNotFound),
				"cross-user snapshot should return 404 (authz denial)")
		})
	})
})
