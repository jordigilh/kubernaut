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
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/agentclient"
)

// v1.5 Feature E2E Tests
//
// Validates SSE streaming and session cancellation against the deployed
// Kubernaut Agent in a Kind cluster.
//
// Features covered:
//   - SSE streaming: GET /api/v1/incident/session/{id}/stream
//   - Session cancellation: POST /api/v1/incident/session/{id}/cancel
//
// Features intentionally skipped:
//   - Rate limiting: exhausting the limiter poisons parallel tests (covered by IT-WIRE-02)
//   - Graceful shutdown: not feasible in Kind cluster (covered by IT-WIRE-SIGTERM)

var _ = Describe("E2E-KA-V15: v1.5 Streaming and Cancellation", Label("e2e", "ka", "v15"), func() {

	// -----------------------------------------------------------------
	// SSE STREAMING
	// -----------------------------------------------------------------

	Context("SSE Streaming", func() {

		It("E2E-KA-SSE-001: SSE stream delivers events and terminates with complete", func() {
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-sse-001",
				RemediationID:     "test-rem-sse-001",
				SignalName:        "CrashLoopBackOff",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "sse-pod-001",
				ErrorMessage:      "Container restarting repeatedly",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			By("Submitting investigation")
			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(sessionID)).To(BeNumerically(">", 0),
				"SubmitInvestigation should return a non-zero-length session ID")

			By("Waiting for investigation to start")
			Eventually(func() string {
				status, pollErr := sessionClient.PollSession(ctx, sessionID)
				if pollErr != nil {
					return "error"
				}
				return status.Status
			}, 15*time.Second, 500*time.Millisecond).Should(
				SatisfyAny(Equal("investigating"), Equal("completed")),
				"session should reach investigating or completed")

			By("Connecting to SSE stream")
			streamReq, err := http.NewRequestWithContext(ctx, "GET",
				fmt.Sprintf("%s/api/v1/incident/session/%s/stream", kaURL, sessionID), nil)
			Expect(err).ToNot(HaveOccurred())
			streamReq.Header.Set("Accept", "text/event-stream")

			resp, err := authHTTPClient.Do(streamReq)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"),
				"SSE response must have text/event-stream Content-Type")
			Expect(resp.Header.Get("Cache-Control")).To(Equal("no-cache"),
				"SSE response must have Cache-Control: no-cache")

			By("Reading SSE events until stream completes")
			scanner := bufio.NewScanner(resp.Body)
			var events []string
			hasComplete := false
			deadline := time.After(60 * time.Second)

		readLoop:
			for {
				select {
				case <-deadline:
					break readLoop
				default:
					if !scanner.Scan() {
						break readLoop
					}
					line := scanner.Text()
					if strings.HasPrefix(line, "event: ") {
						eventType := strings.TrimPrefix(line, "event: ")
						events = append(events, eventType)
						if eventType == "complete" {
							hasComplete = true
							break readLoop
						}
					}
				}
			}

			By("Asserting SSE stream contents")
			Expect(len(events)).To(BeNumerically(">", 0),
				"SSE stream must deliver at least one event")
			Expect(hasComplete).To(BeTrue(),
				"SSE stream must terminate with 'complete' event")
		})
	})

	// -----------------------------------------------------------------
	// SESSION CANCELLATION
	// -----------------------------------------------------------------

	Context("Session Cancellation", func() {

		It("E2E-KA-CANCEL-001: Cancel stops a running investigation", func() {
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-cancel-001",
				RemediationID:     "test-rem-cancel-001",
				SignalName:        "CrashLoopBackOff",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "cancel-pod-001",
				ErrorMessage:      "Container restarting",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			By("Submitting investigation")
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
				SatisfyAny(Equal("investigating"), Equal("completed")),
				"session should reach investigating state")

			By("Cancelling the investigation")
			cancelReq, err := http.NewRequestWithContext(ctx, "POST",
				fmt.Sprintf("%s/api/v1/incident/session/%s/cancel", kaURL, sessionID), nil)
			Expect(err).ToNot(HaveOccurred())
			cancelResp, err := authHTTPClient.Do(cancelReq)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = cancelResp.Body.Close() }()

			// Cancel returns 200 if running, or 409 if already completed
			Expect(cancelResp.StatusCode).To(SatisfyAny(
				Equal(http.StatusOK),
				Equal(http.StatusConflict),
			), "cancel should return 200 (cancelled) or 409 (already completed)")

			if cancelResp.StatusCode == http.StatusOK {
				By("Verifying session status is cancelled")
				Eventually(func() string {
					status, pollErr := sessionClient.PollSession(ctx, sessionID)
					if pollErr != nil {
						return "error"
					}
					return status.Status
				}, 10*time.Second, 500*time.Millisecond).Should(Equal("cancelled"),
					"session status should be 'cancelled' after cancel request")
			}
		})

		It("E2E-KA-CANCEL-002: Cancel on completed session returns 409", func() {
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-cancel-002",
				RemediationID:     "test-rem-cancel-002",
				SignalName:        "CrashLoopBackOff",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "cancel-pod-002",
				ErrorMessage:      "Container restarting",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			By("Submitting and waiting for investigation to complete")
			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				status, pollErr := sessionClient.PollSession(ctx, sessionID)
				if pollErr != nil {
					return "error"
				}
				return status.Status
			}, 30*time.Second, 1*time.Second).Should(Equal("completed"))

			By("Attempting to cancel completed session")
			cancelReq, err := http.NewRequestWithContext(ctx, "POST",
				fmt.Sprintf("%s/api/v1/incident/session/%s/cancel", kaURL, sessionID), nil)
			Expect(err).ToNot(HaveOccurred())
			cancelResp, err := authHTTPClient.Do(cancelReq)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = cancelResp.Body.Close() }()

			body, _ := io.ReadAll(cancelResp.Body)
			Expect(cancelResp.StatusCode).To(Equal(http.StatusConflict),
				"cancel on completed session should return 409, got body: %s", string(body))
		})
	})

	// -----------------------------------------------------------------
	// CROSS-USER AUTHORIZATION
	// -----------------------------------------------------------------

	Context("Cross-User Authorization", func() {

		It("E2E-KA-AUTHZ-001: Different user cannot access another user's session", func() {
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-authz-001",
				RemediationID:     "test-rem-authz-001",
				SignalName:        "CrashLoopBackOff",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "authz-pod-001",
				ErrorMessage:      "Container restarting",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			By("User A submits investigation")
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

			By("User B attempts to read session status — expects 404 (authz denial)")
			statusReq, err := http.NewRequestWithContext(ctx, "GET",
				fmt.Sprintf("%s/api/v1/incident/session/%s/status", kaURL, sessionID), nil)
			Expect(err).ToNot(HaveOccurred())
			statusResp, err := authHTTPClientB.Do(statusReq)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = statusResp.Body.Close() }()
			_, _ = io.ReadAll(statusResp.Body)
			Expect(statusResp.StatusCode).To(Equal(http.StatusNotFound),
				"cross-user session status should return 404")

			By("User B attempts to cancel session — expects 404")
			cancelReq, err := http.NewRequestWithContext(ctx, "POST",
				fmt.Sprintf("%s/api/v1/incident/session/%s/cancel", kaURL, sessionID), nil)
			Expect(err).ToNot(HaveOccurred())
			cancelResp, err := authHTTPClientB.Do(cancelReq)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = cancelResp.Body.Close() }()
			_, _ = io.ReadAll(cancelResp.Body)
			Expect(cancelResp.StatusCode).To(Equal(http.StatusNotFound),
				"cross-user session cancel should return 404")

			By("User B attempts to stream session — expects 404")
			streamReq, err := http.NewRequestWithContext(ctx, "GET",
				fmt.Sprintf("%s/api/v1/incident/session/%s/stream", kaURL, sessionID), nil)
			Expect(err).ToNot(HaveOccurred())
			streamReq.Header.Set("Accept", "text/event-stream")
			streamResp, err := authHTTPClientB.Do(streamReq)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = streamResp.Body.Close() }()
			_, _ = io.ReadAll(streamResp.Body)
			Expect(streamResp.StatusCode).To(Equal(http.StatusNotFound),
				"cross-user session stream should return 404")

			By("User B attempts to snapshot session — expects 404")
			snapReq, err := http.NewRequestWithContext(ctx, "GET",
				fmt.Sprintf("%s/api/v1/incident/session/%s/snapshot", kaURL, sessionID), nil)
			Expect(err).ToNot(HaveOccurred())
			snapResp, err := authHTTPClientB.Do(snapReq)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = snapResp.Body.Close() }()
			_, _ = io.ReadAll(snapResp.Body)
			Expect(snapResp.StatusCode).To(Equal(http.StatusNotFound),
				"cross-user session snapshot should return 404")
		})
	})
})
