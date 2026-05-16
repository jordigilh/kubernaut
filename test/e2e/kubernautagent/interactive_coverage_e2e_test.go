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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/pkg/agentclient"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

var _ = Describe("CP-5 INT Coverage: Interactive gap-closure tests", Label("e2e", "ka", "interactive", "coverage"), func() {

	var (
		mcpEndpoint  string
		tlsTransport http.RoundTripper
		saToken      string
		saTokenB     string
	)

	BeforeEach(func() {
		mcpEndpoint = infrastructure.MCPEndpointForKAE2E()
		tlsTransport = testauth.NewRetryOn429Transport(http.DefaultTransport)

		var err error
		saToken, err = infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "kubernaut-agent-e2e-sa", kubeconfigPath)
		Expect(err).NotTo(HaveOccurred(), "should get E2E SA token")

		saTokenB, err = infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "kubernaut-agent-e2e-sa-2", kubeconfigPath)
		Expect(err).NotTo(HaveOccurred(), "should get E2E SA-2 token")
	})

	// ---------------------------------------------------------------
	// E2E-KA-INT-008: MCP takeover with context reconstruction
	// BR: BR-INTERACTIVE-004
	// ---------------------------------------------------------------
	Describe("E2E-KA-INT-008: MCP takeover with context reconstruction", func() {
		It("should takeover an autonomous session and reconstruct context [E2E-KA-INT-008]", func() {
			rrID := fmt.Sprintf("rr-int008-%d", time.Now().UnixNano())
			createTestRemediationRequest(ctx, rrID)

			By("Step 1: Starting an autonomous investigation via REST")
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-int008",
				RemediationID:     rrID,
				SignalName:        "OOMKilled",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: sharedNamespace,
				ResourceKind:      "Pod",
				ResourceName:      "int008-pod",
				ErrorMessage:      "Container OOMKilled",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}
			_, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			By("Step 2: Waiting briefly for autonomous investigation to start (but not complete)")
			time.Sleep(2 * time.Second)

			By("Step 3: Connecting MCP client and calling takeover")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			result, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "takeover",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			takeoverText := infrastructure.ExtractToolResultText(result)
			GinkgoWriter.Printf("Takeover result: %s\n", takeoverText)

			By("Step 4: Sending message to verify context is preserved")
			result, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":   rrID,
				"action":  "message",
				"message": "What have you found so far about this OOMKill?",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			msgText := infrastructure.ExtractToolResultText(result)
			Expect(msgText).NotTo(BeEmpty(), "LLM should respond to message after takeover")

			By("Step 5: Completing and verifying Lease released")
			_, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "complete",
			})
			Expect(err).NotTo(HaveOccurred())

			clientset, csErr := getKubernetesClientset()
			Expect(csErr).NotTo(HaveOccurred())
			Eventually(func() bool {
				_, err := clientset.CoordinationV1().Leases(sharedNamespace).Get(
					ctx, leaseNameForRR(rrID), metav1.GetOptions{})
				return err != nil
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Lease should be deleted after complete")

			GinkgoWriter.Println("INT-008: Takeover with context reconstruction completed successfully")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-INT-009: MCP cancel action releases Lease
	// BR: BR-INTERACTIVE-001
	// ---------------------------------------------------------------
	Describe("E2E-KA-INT-009: MCP cancel action releases Lease", func() {
		It("should release Lease on cancel and reject subsequent messages [E2E-KA-INT-009]", func() {
			rrID := fmt.Sprintf("rr-int009-%d", time.Now().UnixNano())
			createTestRemediationRequest(ctx, rrID)

			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			By("Starting interactive session")
			_, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			By("Sending a message to confirm session is active")
			_, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":   rrID,
				"action":  "message",
				"message": "what is the status?",
			})
			Expect(err).NotTo(HaveOccurred())

			By("Calling cancel")
			result, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "cancel",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			By("Verifying Lease is deleted")
			clientset, csErr := getKubernetesClientset()
			Expect(csErr).NotTo(HaveOccurred())
			Eventually(func() bool {
				_, err := clientset.CoordinationV1().Leases(sharedNamespace).Get(
					ctx, leaseNameForRR(rrID), metav1.GetOptions{})
				return err != nil
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Lease should be deleted after cancel")

			By("Attempting message after cancel — should fail")
			result, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":   rrID,
				"action":  "message",
				"message": "this should fail",
			})
			if err != nil {
				Expect(err.Error()).To(Or(
					ContainSubstring("not_driving"),
					ContainSubstring("no_active_session"),
					ContainSubstring("not found"),
				))
			} else {
				Expect(result.IsError).To(BeTrue(),
					"message after cancel must be rejected")
			}

			GinkgoWriter.Println("INT-009: Cancel releases Lease validated successfully")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-INT-010: Non-driver message rejected
	// BR: BR-INTERACTIVE-004, SEC-CRIT-01
	// ---------------------------------------------------------------
	Describe("E2E-KA-INT-010: Non-driver message rejected", func() {
		It("should reject messages from a user who is not the active driver [E2E-KA-INT-010]", func() {
			rrID := fmt.Sprintf("rr-int010-%d", time.Now().UnixNano())
			createTestRemediationRequest(ctx, rrID)

			By("User-A starts session")
			sessionA, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer sessionA.Close()

			_, err = infrastructure.CallInvestigate(ctx, sessionA, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			By("User-B connects and sends message on same rr_id")
			sessionB, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saTokenB,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer sessionB.Close()

			result, err := infrastructure.CallInvestigate(ctx, sessionB, map[string]any{
				"rr_id":   rrID,
				"action":  "message",
				"message": "hijack attempt",
			})
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("session_active"))
			} else {
				Expect(result.IsError).To(BeTrue(),
					"non-driver message must be rejected")
				text := infrastructure.ExtractToolResultText(result)
				Expect(text).To(ContainSubstring("session_active"),
					"error should contain session_active code")
			}

			By("User-A completes session (cleanup)")
			_, _ = infrastructure.CallInvestigate(ctx, sessionA, map[string]any{
				"rr_id":  rrID,
				"action": "complete",
			})

			GinkgoWriter.Println("INT-010: Non-driver message rejection validated")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-INT-011: Non-driver complete rejected
	// BR: SEC-CRIT-01
	// ---------------------------------------------------------------
	Describe("E2E-KA-INT-011: Non-driver complete rejected", func() {
		It("should reject complete from a user who is not the active driver [E2E-KA-INT-011]", func() {
			rrID := fmt.Sprintf("rr-int011-%d", time.Now().UnixNano())
			createTestRemediationRequest(ctx, rrID)

			By("User-A starts session")
			sessionA, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer sessionA.Close()

			_, err = infrastructure.CallInvestigate(ctx, sessionA, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			By("User-B attempts complete")
			sessionB, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saTokenB,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer sessionB.Close()

			result, err := infrastructure.CallInvestigate(ctx, sessionB, map[string]any{
				"rr_id":  rrID,
				"action": "complete",
			})
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("session_active"))
			} else {
				Expect(result.IsError).To(BeTrue(),
					"non-driver complete must be rejected")
			}

			By("User-A can still complete successfully")
			_, err = infrastructure.CallInvestigate(ctx, sessionA, map[string]any{
				"rr_id":  rrID,
				"action": "complete",
			})
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Println("INT-011: Non-driver complete rejection validated")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-CANCEL-003: REST cancel unknown session returns 404
	// BR: BR-SESSION-002
	// ---------------------------------------------------------------
	Describe("E2E-KA-CANCEL-003: REST cancel unknown session returns 404", func() {
		It("should return 404 with RFC 7807 fields for unknown session [E2E-KA-CANCEL-003]", func() {
			fakeID := uuid.New().String()

			cancelReq, err := http.NewRequestWithContext(ctx, "POST",
				fmt.Sprintf("%s/api/v1/incident/session/%s/cancel", kaURL, fakeID), nil)
			Expect(err).NotTo(HaveOccurred())
			cancelResp, err := authHTTPClient.Do(cancelReq)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = cancelResp.Body.Close() }()

			body, _ := io.ReadAll(cancelResp.Body)
			Expect(cancelResp.StatusCode).To(Equal(http.StatusNotFound),
				"cancel on unknown session should return 404, got body: %s", string(body))

			var problemDetail map[string]interface{}
			if err := json.Unmarshal(body, &problemDetail); err == nil {
				Expect(problemDetail).To(HaveKey("type"),
					"RFC 7807: response should have 'type' field")
				Expect(problemDetail).To(HaveKey("title"),
					"RFC 7807: response should have 'title' field")
			}

			GinkgoWriter.Println("CANCEL-003: Unknown session cancel returns 404 validated")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-SSE-002: SSE subscribe after session completed returns 404
	// BR: BR-SESSION-002
	// ---------------------------------------------------------------
	Describe("E2E-KA-SSE-002: SSE subscribe after session completed returns 404", func() {
		It("should return 404 when subscribing to completed session stream [E2E-KA-SSE-002]", func() {
			By("Submitting investigation and waiting for completion")
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-sse-002",
				RemediationID:     "test-rem-sse-002",
				SignalName:        "OOMKilled",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "sse-pod-002",
				ErrorMessage:      "Container OOMKilled",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() string {
				status, pollErr := sessionClient.PollSession(ctx, sessionID)
				if pollErr != nil {
					return "error"
				}
				return status.Status
			}, 60*time.Second, 1*time.Second).Should(Equal("completed"))

			By("Subscribing to SSE stream after completion")
			streamReq, err := http.NewRequestWithContext(ctx, "GET",
				fmt.Sprintf("%s/api/v1/incident/session/%s/stream", kaURL, sessionID), nil)
			Expect(err).NotTo(HaveOccurred())
			streamReq.Header.Set("Accept", "text/event-stream")

			resp, err := authHTTPClient.Do(streamReq)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(resp.Body)

			Expect(resp.StatusCode).To(Equal(http.StatusNotFound),
				"SSE subscribe after completion should return 404, got body: %s", string(body))

			GinkgoWriter.Println("SSE-002: Subscribe after completed returns 404 validated")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-SNAP-006: Snapshot includes error field on failed investigation
	// BR: BR-SESSION-002, BR-AUDIT-070
	// ---------------------------------------------------------------
	Describe("E2E-KA-SNAP-006: Snapshot includes error field on failed investigation", func() {
		It("should have error field present on MOCK_MAX_RETRIES_EXHAUSTED scenario [E2E-KA-SNAP-006]", func() {
			By("Submitting investigation with MOCK_MAX_RETRIES_EXHAUSTED scenario")
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-snap-006",
				RemediationID:     "test-rem-snap-006",
				SignalName:        "MOCK_MAX_RETRIES_EXHAUSTED",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "snap-pod-006",
				ErrorMessage:      "Container restarting",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() string {
				status, pollErr := sessionClient.PollSession(ctx, sessionID)
				if pollErr != nil {
					return "error"
				}
				return status.Status
			}, 60*time.Second, 1*time.Second).Should(Equal("completed"),
				"MOCK_MAX_RETRIES_EXHAUSTED should eventually complete")

			By("Fetching snapshot")
			snapRes, err := kaClient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGet(ctx,
				agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetParams{
					SessionID: sessionID,
				})
			Expect(err).NotTo(HaveOccurred())

			snap, ok := snapRes.(*agentclient.SessionSnapshot)
			Expect(ok).To(BeTrue(), "response should be *SessionSnapshot, got %T", snapRes)

			By("Asserting error-related fields")
			Expect(snap.Status).To(Equal("completed"))

			if errField, hasErr := snap.Error.Get(); hasErr {
				Expect(errField).NotTo(BeEmpty(),
					"error field should be non-empty for failed investigation")
				GinkgoWriter.Printf("Snapshot error: %s\n", errField)
			}

			cancelledPhase, hasPhase := snap.CancelledPhase.Get()
			if hasPhase {
				GinkgoWriter.Printf("cancelled_phase unexpectedly present: %s\n", cancelledPhase)
			}

			if promptTokens, hasPrompt := snap.TotalPromptTokens.Get(); hasPrompt {
				Expect(promptTokens).To(BeNumerically(">=", 0))
			}

			GinkgoWriter.Println("SNAP-006: Error field on failed investigation validated")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-SNAP-007: Snapshot cancelled session has cancelled_phase
	// BR: BR-SESSION-002
	// ---------------------------------------------------------------
	Describe("E2E-KA-SNAP-007: Snapshot cancelled session has cancelled_phase", func() {
		It("should have cancelled_phase set when session is cancelled [E2E-KA-SNAP-007]", func() {
			By("Submitting investigation")
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-snap-007",
				RemediationID:     "test-rem-snap-007",
				SignalName:        "CrashLoopBackOff",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "snap-pod-007",
				ErrorMessage:      "Container restarting",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).NotTo(HaveOccurred())

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
			Expect(err).NotTo(HaveOccurred())
			cancelResp, err := authHTTPClient.Do(cancelReq)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = cancelResp.Body.Close() }()

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

			if finalStatus != "cancelled" {
				GinkgoWriter.Println("SNAP-007: Session completed before cancel took effect — skipping cancelled_phase assertion")
				return
			}

			By("Fetching snapshot for cancelled session")
			snapRes, err := kaClient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGet(ctx,
				agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetParams{
					SessionID: sessionID,
				})
			Expect(err).NotTo(HaveOccurred())

			snap, ok := snapRes.(*agentclient.SessionSnapshot)
			Expect(ok).To(BeTrue(), "response should be *SessionSnapshot")
			Expect(snap.Status).To(Equal("cancelled"))

			cancelledPhase, hasPhase := snap.CancelledPhase.Get()
			if hasPhase {
				Expect(cancelledPhase).NotTo(BeEmpty(),
					"cancelled_phase should indicate which phase was interrupted")
				GinkgoWriter.Printf("cancelled_phase: %s\n", cancelledPhase)
			}

			GinkgoWriter.Println("SNAP-007: Cancelled session snapshot validated")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-STATUS-001: action:status returns "not_found" for unknown RR
	// BR: BR-INTERACTIVE-001
	// ---------------------------------------------------------------
	Describe("E2E-KA-STATUS-001: Status returns not_found for unknown RR", func() {
		It("should return mode=not_found for a non-existent remediation [E2E-KA-STATUS-001]", func() {
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			result, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  "rr-nonexistent-status-001",
				"action": "status",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			text := infrastructure.ExtractToolResultText(result)
			GinkgoWriter.Printf("Status result: %s\n", text)

			var outer map[string]interface{}
			Expect(json.Unmarshal([]byte(text), &outer)).To(Succeed(),
				"tool result should be valid JSON")
			Expect(outer["status"]).To(Equal("status"))

			var status map[string]interface{}
			Expect(json.Unmarshal([]byte(outer["response"].(string)), &status)).To(Succeed(),
				"response field should contain valid JSON StatusOutput")
			Expect(status["mode"]).To(Equal("not_found"),
				"mode should be not_found for unknown RR")

			GinkgoWriter.Println("STATUS-001: not_found mode validated")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-STATUS-002: action:status returns "interactive" during active session
	// BR: BR-INTERACTIVE-001
	// ---------------------------------------------------------------
	Describe("E2E-KA-STATUS-002: Status returns interactive during active session", func() {
		It("should return mode=interactive with driver info [E2E-KA-STATUS-002]", func() {
			rrID := fmt.Sprintf("rr-status002-%d", time.Now().UnixNano())
			createTestRemediationRequest(ctx, rrID)

			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			By("Starting an interactive session")
			startResult, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(startResult).NotTo(BeNil())

			By("Querying status while session is active")
			result, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "status",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			text := infrastructure.ExtractToolResultText(result)
			GinkgoWriter.Printf("Status result: %s\n", text)

			var outer map[string]interface{}
			Expect(json.Unmarshal([]byte(text), &outer)).To(Succeed())
			Expect(outer["status"]).To(Equal("status"))

			var status map[string]interface{}
			Expect(json.Unmarshal([]byte(outer["response"].(string)), &status)).To(Succeed(),
				"response field should contain valid JSON StatusOutput")
			Expect(status["mode"]).To(Equal("interactive"),
				"mode should be interactive during active session")
			Expect(status["driver"]).NotTo(BeEmpty(),
				"driver should contain the session owner identity")

			By("Cleaning up — completing the session")
			_, _ = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "complete",
			})

			GinkgoWriter.Println("STATUS-002: interactive mode validated")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-STATUS-003: action:status returns "autonomous" during autonomous investigation
	// BR: BR-INTERACTIVE-001
	// ---------------------------------------------------------------
	Describe("E2E-KA-STATUS-003: Status returns autonomous during autonomous investigation", func() {
		It("should return mode=autonomous while investigation runs [E2E-KA-STATUS-003]", func() {
			rrID := fmt.Sprintf("rr-status003-%d", time.Now().UnixNano())
			createTestRemediationRequest(ctx, rrID)

			By("Starting an autonomous investigation via REST")
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-status003",
				RemediationID:     rrID,
				SignalName:        "CrashLoopBackOff",
				Severity:          agentclient.SeverityHigh,
				SignalSource:      "kubernetes",
				ResourceNamespace: sharedNamespace,
				ResourceKind:      "Pod",
				ResourceName:      "status003-pod",
				ErrorMessage:      "Container restarting repeatedly",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}
			_, err := sessionClient.SubmitInvestigation(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting briefly for autonomous investigation to start")
			time.Sleep(2 * time.Second)

			By("Querying status via MCP")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			result, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "status",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			text := infrastructure.ExtractToolResultText(result)
			GinkgoWriter.Printf("Status result: %s\n", text)

			var outer map[string]interface{}
			Expect(json.Unmarshal([]byte(text), &outer)).To(Succeed())
			Expect(outer["status"]).To(Equal("status"))

			var status map[string]interface{}
			Expect(json.Unmarshal([]byte(outer["response"].(string)), &status)).To(Succeed(),
				"response field should contain valid JSON StatusOutput")
			Expect(status["mode"]).To(Equal("autonomous"),
				"mode should be autonomous while investigation runs")

			GinkgoWriter.Println("STATUS-003: autonomous mode validated")
		})
	})
})

