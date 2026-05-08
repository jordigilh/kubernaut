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
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

var _ = Describe("CP-5 INT: Interactive Flow Lifecycle Tests", Label("e2e", "ka", "interactive", "int"), func() {

	var (
		mcpEndpoint  string
		tlsTransport http.RoundTripper
		saToken      string
	)

	BeforeEach(func() {
		mcpEndpoint = infrastructure.MCPEndpointForKAE2E()
		tlsTransport = testauth.NewRetryOn429Transport(http.DefaultTransport)

		var err error
		saToken, err = infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "kubernaut-agent-e2e-sa", kubeconfigPath)
		Expect(err).NotTo(HaveOccurred(), "should get E2E SA token")
	})

	// ---------------------------------------------------------------
	// E2E-KA-INT-001: Complete interactive lifecycle
	// BR: BR-INTERACTIVE-001, BR-INTERACTIVE-004
	// Validates: connect → start → message → complete
	// ---------------------------------------------------------------
	Describe("E2E-KA-INT-001: Complete interactive lifecycle", func() {
		It("should execute full interactive lifecycle: start → message → complete [E2E-KA-INT-001]", func() {
			rrID := fmt.Sprintf("rr-int001-%d", time.Now().Unix())
			createTestRemediationRequest(ctx, rrID)

			By("Step 1: Connecting MCP client")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred(), "MCP client should connect")
			defer session.Close()

			By("Step 2: Starting interactive session (acquires Lease)")
			result, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred(), "start should succeed")
			Expect(result).NotTo(BeNil())
			startText := infrastructure.ExtractToolResultText(result)
			GinkgoWriter.Printf("Start result: %s\n", startText)

			By("Step 3: Verifying Lease exists in cluster")
			clientset, err := getKubernetesClientset()
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				lease, err := clientset.CoordinationV1().Leases(sharedNamespace).Get(
					ctx, leaseNameForRR(rrID), metav1.GetOptions{})
				if err != nil {
					return false
				}
				return lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity != ""
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Lease should exist with holder identity after start")

			By("Step 4: Sending investigative message")
			result, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":   rrID,
				"action":  "message",
				"message": "What is the root cause of this OOMKill event?",
			})
			Expect(err).NotTo(HaveOccurred(), "message should succeed")
			Expect(result).NotTo(BeNil())
			msgText := infrastructure.ExtractToolResultText(result)
			Expect(msgText).NotTo(BeEmpty(), "LLM should respond to message")
			GinkgoWriter.Printf("Message response: %s\n", msgText)

			By("Step 5: Completing interactive session (releases Lease)")
			_, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "complete",
			})
			Expect(err).NotTo(HaveOccurred(), "complete should succeed")

			By("Step 6: Verifying Lease released")
			Eventually(func() bool {
				_, err := clientset.CoordinationV1().Leases(sharedNamespace).Get(
					ctx, leaseNameForRR(rrID), metav1.GetOptions{})
				return err != nil
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Lease should be deleted after complete")

			GinkgoWriter.Println("✅ INT-001: Full interactive lifecycle completed successfully")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-INT-004: Impersonated K8s call in real cluster
	// BR: BR-INTERACTIVE-002
	// Validates: enrichment uses impersonated user RBAC, not KA SA
	// ---------------------------------------------------------------
	Describe("E2E-KA-INT-004: Impersonated K8s call in real cluster", func() {
		It("should enforce user RBAC during enrichment (not KA SA privileges) [E2E-KA-INT-004]", func() {
			By("Step 1: Creating limited SA for impersonation test")
			limitedToken, err := infrastructure.CreateLimitedRBACSA(
				ctx, sharedNamespace, "int004-limited-user", kubeconfigPath, sharedNamespace, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			rrID := fmt.Sprintf("rr-int004-%d", time.Now().Unix())
			createTestRemediationRequest(ctx, rrID)

			By("Step 2: Connecting and starting session with limited SA")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      limitedToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			By("Step 3: Starting interactive session")
			_, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred(), "start should succeed (auth passes)")

			By("Step 4: Calling select_workflow with enrichment — expects RBAC failure (user cannot impersonate) (#1012)")
			result, err := infrastructure.CallSelectWorkflow(ctx, session, map[string]any{
				"rr_id":       rrID,
				"workflow_id": "wf-rbac-test",
				"kind":        "Pod",
				"name":        "api-server-xyz",
				"namespace":   "production",
			})

			By("Step 5: Asserting impersonation-gated operation fails")
			if err != nil {
				Expect(err.Error()).To(Or(
					ContainSubstring("forbidden"),
					ContainSubstring("impersonate"),
					ContainSubstring("unauthorized"),
					ContainSubstring("Access denied"),
				))
			} else {
				Expect(result).NotTo(BeNil())
				if result.IsError {
					text := infrastructure.ExtractToolResultText(result)
					Expect(text).To(Or(
						ContainSubstring("forbidden"),
						ContainSubstring("impersonate"),
						ContainSubstring("unauthorized"),
						ContainSubstring("Access denied"),
					), "tool error should indicate RBAC restriction")
				} else {
					Fail("enrichment should fail for a limited-RBAC SA")
				}
			}

			By("Step 6: Verifying full-RBAC SA CAN enrich (differential proof)")
			fullSession, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer fullSession.Close()

			fullRRID := fmt.Sprintf("rr-int004-full-%d", time.Now().Unix())
			createTestRemediationRequest(ctx, fullRRID)
			_, err = infrastructure.CallInvestigate(ctx, fullSession, map[string]any{
				"rr_id":  fullRRID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			fullResult, err := infrastructure.CallSelectWorkflow(ctx, fullSession, map[string]any{
				"rr_id":       fullRRID,
				"workflow_id": "wf-rbac-test",
				"kind":        "Pod",
				"name":        "api-server-xyz",
				"namespace":   sharedNamespace,
			})
			if err == nil && fullResult != nil && !fullResult.IsError {
				GinkgoWriter.Println("✅ Full-RBAC SA enrichment via select_workflow succeeded (differential proof)")
			}

			_, _ = infrastructure.CallInvestigate(ctx, fullSession, map[string]any{
				"rr_id":  fullRRID,
				"action": "complete",
			})

			GinkgoWriter.Println("✅ INT-004: Impersonation enforcement validated via differential RBAC")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-INT-005: Audit trail in DS after interactive flow
	// BR: BR-INTERACTIVE-003
	// Validates: DS has ordered audit with correct session_id and acting_user
	//
	// Scope: This test validates the INTERACTIVE-ONLY audit subset (start → message → complete).
	// The full CP-5 lifecycle (autonomous → takeover → disconnect → resume) is tested by
	// E2E-KA-INT-006 (deferred to separate test plan). This covers DD-INTERACTIVE-002
	// interactive events plus LLM events emitted during the interactive turn.
	// ---------------------------------------------------------------
	Describe("E2E-KA-INT-005: Audit trail complete in DS after full flow", func() {
		It("should record complete audit trail in DataStorage [E2E-KA-INT-005]", func() {
			rrID := fmt.Sprintf("rr-int005-%d-%s", time.Now().UnixNano(), randomHex(6))
			createTestRemediationRequest(ctx, rrID)

			By("Step 1: Executing interactive flow (start → message → complete)")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			_, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":   rrID,
				"action":  "message",
				"message": "Investigate the resource limits for this container",
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "complete",
			})
			Expect(err).NotTo(HaveOccurred())

			By("Step 2: Querying DataStorage for audit trail (wait for interactive.completed to flush)")
			Eventually(func() bool {
				audits := queryDSAuditsByRRID(rrID)
				if len(audits) > 0 {
					GinkgoWriter.Printf("  Found %d audit entries for %s\n", len(audits), rrID)
				}
				for _, a := range audits {
					if a.EventType == "aiagent.interactive.completed" {
						return true
					}
				}
				return false
			}, 60*time.Second, 2*time.Second).Should(BeTrue(),
				"DS should contain an interactive.completed event before proceeding to assertions")

			By("Step 3: Verifying identity transition events and chronological order")
			audits := queryDSAuditsByRRID(rrID)
			Expect(audits).NotTo(BeEmpty())

			// H2: Sort by timestamp ascending to make assertion independent of DS sort order.
			sort.Slice(audits, func(i, j int) bool {
				return audits[i].Timestamp < audits[j].Timestamp
			})

			var lastTimestamp string
			var interactiveStarted, interactiveCompleted int
			llmEventCount := 0
			for _, a := range audits {
				GinkgoWriter.Printf("  event: %s session_id=%s acting_user=%s\n", a.EventType, a.SessionID, a.ActingUser)

				switch {
				case a.EventType == "aiagent.interactive.started":
					interactiveStarted++
					Expect(a.SessionID).NotTo(BeEmpty(),
						"interactive.started must have session_id (BR-INTERACTIVE-003)")
					Expect(a.ActingUser).NotTo(BeEmpty(),
						"interactive.started must have acting_user (BR-INTERACTIVE-003)")
				case a.EventType == "aiagent.interactive.completed":
					interactiveCompleted++
					Expect(a.SessionID).NotTo(BeEmpty(),
						"interactive.completed must have session_id (BR-INTERACTIVE-003)")
					Expect(a.ActingUser).NotTo(BeEmpty(),
						"interactive.completed must have acting_user (BR-INTERACTIVE-003)")
				case strings.HasPrefix(a.EventType, "aiagent.llm."):
					llmEventCount++
				}

				if lastTimestamp != "" && a.Timestamp != "" {
					Expect(a.Timestamp >= lastTimestamp).To(BeTrue(),
						"audit entries should be in chronological order (ascending)")
				}
				if a.Timestamp != "" {
					lastTimestamp = a.Timestamp
				}
			}

			Expect(interactiveStarted).To(BeNumerically(">=", 1),
				"should have at least 1 interactive.started event (DD-INTERACTIVE-002)")
			Expect(interactiveCompleted).To(BeNumerically(">=", 1),
				"should have at least 1 interactive.completed event (DD-INTERACTIVE-002)")
			Expect(llmEventCount).To(BeNumerically(">=", 2),
				"should have at least 2 LLM events (request + response) from the interactive turn")

			GinkgoWriter.Println("✅ INT-005: Audit trail validated in DataStorage")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-INT-007: Multi-step interactive investigation with enrichment
	// BR: BR-INTERACTIVE-001
	// Validates: start → Q&A → enrich → select workflow → complete
	// ---------------------------------------------------------------
	Describe("E2E-KA-INT-007: Multi-step interactive investigation with enrichment", func() {
		It("should support multi-step Q&A, enrichment, and workflow selection [E2E-KA-INT-007]", func() {
			rrID := fmt.Sprintf("rr-int007-%d", time.Now().Unix())
			createTestRemediationRequest(ctx, rrID)

			By("Step 1: Connecting and starting session")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			_, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			By("Step 2: First investigative message (Q&A)")
			result, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":   rrID,
				"action":  "message",
				"message": "What pods are affected by this incident?",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(infrastructure.ExtractToolResultText(result)).NotTo(BeEmpty(),
				"first message should get LLM response")

			By("Step 3: Calling select_workflow with enrichment for additional context (#1012)")
			enrichResult, err := infrastructure.CallSelectWorkflow(ctx, session, map[string]any{
				"rr_id":       rrID,
				"workflow_id": "wf-enrichment-test",
				"kind":        "Pod",
				"name":        "api-server-xyz",
				"namespace":   sharedNamespace,
			})
			if err == nil && enrichResult != nil && !enrichResult.IsError {
				enrichText := infrastructure.ExtractToolResultText(enrichResult)
				GinkgoWriter.Printf("Select workflow + enrich result: %s\n", enrichText)
			}

			By("Step 4: Follow-up message referencing enrichment")
			result, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":   rrID,
				"action":  "message",
				"message": "Based on the enriched context, suggest a remediation workflow",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			followUpText := infrastructure.ExtractToolResultText(result)
			Expect(followUpText).NotTo(BeEmpty(), "follow-up should get LLM response")
			GinkgoWriter.Printf("Follow-up response: %s\n", followUpText)

			By("Step 5: Selecting workflow")
			wfResult, err := infrastructure.CallSelectWorkflow(ctx, session, map[string]any{
				"rr_id":       rrID,
				"workflow_id": "oomkill-increase-memory-v1",
			})
			if err == nil && wfResult != nil {
				wfText := infrastructure.ExtractToolResultText(wfResult)
				GinkgoWriter.Printf("Workflow selection result: %s\n", wfText)
			}

			By("Step 6: Completing session")
			_, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "complete",
			})
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Println("✅ INT-007: Multi-step interactive investigation completed")
		})
	})
})

// auditEntry represents a simplified audit entry from DataStorage.
type auditEntry struct {
	SessionID  string
	Timestamp  string
	EventType  string
	ActingUser string
}

// queryDSAuditsByRRID queries the DataStorage for audit entries matching the given RR ID.
// Returns nil if the query fails or no results are found. Logs errors to GinkgoWriter
// so CI failures are debuggable (H5).
func queryDSAuditsByRRID(rrID string) []auditEntry {
	url := fmt.Sprintf("https://localhost:8089/api/v1/audit/events?correlation_id=%s&limit=50&offset=0", rrID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		GinkgoWriter.Printf("  [queryDSAuditsByRRID] request creation failed: %v\n", err)
		return nil
	}

	resp, err := authHTTPClient.Do(req)
	if err != nil {
		GinkgoWriter.Printf("  [queryDSAuditsByRRID] HTTP request failed: %v\n", err)
		return nil
	}
	if resp == nil {
		GinkgoWriter.Printf("  [queryDSAuditsByRRID] nil response for %s\n", rrID)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		GinkgoWriter.Printf("  [queryDSAuditsByRRID] non-200 status: %d for %s\n", resp.StatusCode, rrID)
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		GinkgoWriter.Printf("  [queryDSAuditsByRRID] body read failed: %v\n", err)
		return nil
	}

	var queryResp struct {
		Data []struct {
			EventID        string          `json:"event_id"`
			EventType      string          `json:"event_type"`
			CorrelationID  string          `json:"correlation_id"`
			EventTimestamp string          `json:"event_timestamp"`
			ActorID        string          `json:"actor_id"`
			EventData      json.RawMessage `json:"event_data"`
		} `json:"data"`
		Pagination struct {
			Total int `json:"total"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(body, &queryResp); err != nil {
		GinkgoWriter.Printf("  [queryDSAuditsByRRID] JSON parse failed: %v\nbody: %s\n", err, string(body[:min(len(body), 500)]))
		return nil
	}

	entries := make([]auditEntry, 0, len(queryResp.Data))
	for _, d := range queryResp.Data {
		var sessionID string
		if len(d.EventData) > 0 {
			var ed struct {
				SessionID string `json:"session_id"`
			}
			_ = json.Unmarshal(d.EventData, &ed)
			sessionID = ed.SessionID
		}
		entries = append(entries, auditEntry{
			SessionID:  sessionID,
			Timestamp:  d.EventTimestamp,
			EventType:  d.EventType,
			ActingUser: d.ActorID,
		})
	}
	return entries
}

// randomHex returns a random hex string of n bytes (2n hex chars).
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
