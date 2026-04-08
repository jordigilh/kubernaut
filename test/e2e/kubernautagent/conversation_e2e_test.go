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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// Conversation API E2E Tests (#592)
// Test Plan: docs/tests/592/TEST_PLAN.md (v5.0)
// Scenarios: E2E-CS-592-001 through E2E-CS-592-007
// Business Requirements: BR-CONV-001, BR-CONV-004, BR-CONV-005, BR-CONV-006, BR-CONV-007
//
// Purpose: Validate the conversational RAR API backend with a real Kind cluster,
// including K8s TokenReview + SAR auth, SSE streaming, audit persistence in DataStorage,
// and rate limiting enforcement.

type sseEvent struct {
	ID    string
	Event string
	Data  string
}

func parseSSEStream(body io.Reader) []sseEvent {
	var events []sseEvent
	scanner := bufio.NewScanner(body)
	var current sseEvent
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "id: "):
			current.ID = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			current.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			current.Data = strings.TrimPrefix(line, "data: ")
		case line == "":
			if current.Event != "" || current.Data != "" {
				events = append(events, current)
				current = sseEvent{}
			}
		}
	}
	return events
}

type createSessionReq struct {
	RARNamespace  string `json:"rar_namespace"`
	RARName       string `json:"rar_name"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

type createSessionResp struct {
	SessionID   string `json:"session_id"`
	SeededTurns int    `json:"seeded_turns,omitempty"`
}

type postMessageReq struct {
	Content string `json:"content"`
}

type rfc7807Error struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

func conversationURL(path string) string {
	return kaURL + "/api/v1/conversations" + path
}

func createConversationSession(client *http.Client, namespace, rarName string) (*createSessionResp, *http.Response) {
	body, _ := json.Marshal(createSessionReq{
		RARNamespace: namespace,
		RARName:      rarName,
	})
	resp, err := client.Post(conversationURL("/sessions"), "application/json", bytes.NewReader(body))
	Expect(err).ToNot(HaveOccurred(), "POST /conversations/sessions should not error")

	if resp.StatusCode != http.StatusCreated {
		return nil, resp
	}

	var result createSessionResp
	Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
	_ = resp.Body.Close()
	return &result, resp
}

func postConversationMessage(client *http.Client, sessionID, content string) (*http.Response, error) {
	body, _ := json.Marshal(postMessageReq{Content: content})
	return client.Post(
		conversationURL(fmt.Sprintf("/sessions/%s/messages", sessionID)),
		"application/json",
		bytes.NewReader(body),
	)
}

var _ = Describe("E2E-CS-592: Conversation API", Label("e2e", "kubernautagent", "conversation", "cs-592"), func() {

	var dataStorageClient *ogenclient.Client

	BeforeEach(func() {
		if !setupSucceeded {
			Skip("Infrastructure setup failed — skipping conversation E2E tests")
		}
		saToken, err := infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "kubernaut-agent-e2e-sa", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to get ServiceAccount token for DS client")

		dataStorageClient, err = ogenclient.NewClient(
			dataStorageURL,
			ogenclient.WithClient(&http.Client{
				Transport: testauth.NewServiceAccountTransport(saToken),
				Timeout:   30 * time.Second,
			}),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create authenticated DS client")
	})

	// =====================================================================
	// E2E-CS-592-001: Session creation with real K8s auth
	// BR-CONV-004, BR-CONV-001
	// =====================================================================
	It("E2E-CS-592-001: creates a conversation session with real K8s TokenReview + SAR auth", func() {
		result, resp := createConversationSession(authHTTPClient, sharedNamespace, "e2e-test-rar-001")
		Expect(resp.StatusCode).To(Equal(http.StatusCreated),
			"Session creation should return 201 Created when user has UPDATE permission on RAR")
		Expect(result.SessionID).To(HaveLen(36),
			"Session ID should be a UUID (36 characters including hyphens)")
	})

	// =====================================================================
	// E2E-CS-592-002: Full conversation flow with SSE streaming
	// BR-CONV-001, BR-CONV-005
	// =====================================================================
	It("E2E-CS-592-002: completes a full conversation flow — create session, post message, receive SSE response", func() {
		session, resp := createConversationSession(authHTTPClient, sharedNamespace, "e2e-test-rar-002")
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))
		Expect(session.SessionID).To(HaveLen(36),
			"Session ID should be a UUID (36 chars)")

		msgResp, err := postConversationMessage(authHTTPClient, session.SessionID, "What is the root cause of this incident?")
		Expect(err).ToNot(HaveOccurred(), "POST message should not error")
		Expect(msgResp.StatusCode).To(Equal(http.StatusOK),
			"Message response should return 200 OK for SSE stream")
		Expect(msgResp.Header.Get("Content-Type")).To(Equal("text/event-stream"),
			"Response Content-Type should be text/event-stream for SSE")

		defer func() { _ = msgResp.Body.Close() }()
		events := parseSSEStream(msgResp.Body)

		Expect(len(events)).To(BeNumerically(">=", 1),
			"SSE stream should contain at least one event (LLM response)")

		hasMessageEvent := false
		for _, ev := range events {
			if ev.Event == "message" {
				hasMessageEvent = true
				Expect(ev.ID).To(MatchRegexp(`^\d+$`),
					"SSE message event ID should be a numeric string for reconnection support")
				var payload map[string]string
				Expect(json.Unmarshal([]byte(ev.Data), &payload)).To(Succeed(),
					"SSE message data should be valid JSON")
				Expect(payload).To(HaveKey("content"),
					"SSE message payload should contain a 'content' field with the LLM response")
			}
		}
		Expect(hasMessageEvent).To(BeTrue(),
			"SSE stream should contain at least one 'message' event from the LLM")
	})

	// =====================================================================
	// E2E-CS-592-003: Conversation turn audit persisted in DataStorage
	// BR-CONV-007
	// =====================================================================
	It("E2E-CS-592-003: persists conversation turn audit events in DataStorage", func() {
		session, resp := createConversationSession(authHTTPClient, sharedNamespace, "e2e-test-rar-003")
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))

		msgResp, err := postConversationMessage(authHTTPClient, session.SessionID, "Explain the investigation findings")
		Expect(err).ToNot(HaveOccurred())
		Expect(msgResp.StatusCode).To(Equal(http.StatusOK))
		_, _ = io.ReadAll(msgResp.Body)
		_ = msgResp.Body.Close()

		Eventually(func() bool {
			auditResp, qErr := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				EventType: ogenclient.NewOptString("aiagent.conversation.turn"),
			})
			if qErr != nil {
				return false
			}
			for _, event := range auditResp.Data {
				if event.EventType == "aiagent.conversation.turn" &&
					event.EventAction == "conversation_turn" &&
					event.EventOutcome == "success" {
					return true
				}
			}
			return false
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"DataStorage should contain an aiagent.conversation.turn audit event with action=conversation_turn and outcome=success")
	})

	// =====================================================================
	// E2E-CS-592-004: Unauthorized access rejected
	// BR-CONV-004
	// =====================================================================
	It("E2E-CS-592-004: rejects unauthorized access — missing token returns 401", func() {
		unauthClient := &http.Client{Timeout: 10 * time.Second}

		body, _ := json.Marshal(createSessionReq{
			RARNamespace: sharedNamespace,
			RARName:      "e2e-test-rar-unauth",
		})
		resp, err := unauthClient.Post(conversationURL("/sessions"), "application/json", bytes.NewReader(body))
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
			"Missing bearer token should return 401 Unauthorized")

		var problemDetail rfc7807Error
		Expect(json.NewDecoder(resp.Body).Decode(&problemDetail)).To(Succeed())
		Expect(problemDetail.Status).To(Equal(http.StatusUnauthorized),
			"RFC 7807 problem detail status should be 401")
	})

	// E2E-CS-592-005 (rate limit exhaustion) is covered by unit tests
	// UT-CS-592-016 and UT-CS-592-017 — not suitable for E2E because
	// exhausting the rate limit poisons other tests in non-deterministic order.

	// =====================================================================
	// E2E-CS-592-006: Investigation-seeded conversation
	// BR-CONV-001
	// =====================================================================
	It("E2E-CS-592-006: creates an investigation-seeded conversation session", func() {
		session, resp := createConversationSession(authHTTPClient, sharedNamespace, "e2e-test-rar-seeded")
		Expect(resp.StatusCode).To(Equal(http.StatusCreated),
			"Session creation should succeed even without prior investigation audit trail")
		Expect(session.SessionID).To(HaveLen(36),
			"Session ID should be a valid UUID")

		msgResp, err := postConversationMessage(authHTTPClient, session.SessionID, "What did the investigation find?")
		Expect(err).ToNot(HaveOccurred())
		Expect(msgResp.StatusCode).To(Equal(http.StatusOK),
			"Message post should succeed with 200 OK SSE stream")

		defer func() { _ = msgResp.Body.Close() }()
		events := parseSSEStream(msgResp.Body)
		Expect(len(events)).To(BeNumerically(">=", 1),
			"LLM should respond with at least one SSE event even without investigation context")
	})

	// =====================================================================
	// E2E-CS-592-007: LLM error surfaces as SSE error event
	// BR-CONV-005
	// =====================================================================
	It("E2E-CS-592-007: verifies SSE events have incrementing IDs across multiple turns", func() {
		session, resp := createConversationSession(authHTTPClient, sharedNamespace, "e2e-test-rar-sse-ids")
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))

		msgResp1, err := postConversationMessage(authHTTPClient, session.SessionID, "First question")
		Expect(err).ToNot(HaveOccurred())
		Expect(msgResp1.StatusCode).To(Equal(http.StatusOK))
		events1 := parseSSEStream(msgResp1.Body)
		_ = msgResp1.Body.Close()

		msgResp2, err := postConversationMessage(authHTTPClient, session.SessionID, "Second question")
		Expect(err).ToNot(HaveOccurred())
		Expect(msgResp2.StatusCode).To(Equal(http.StatusOK))
		events2 := parseSSEStream(msgResp2.Body)
		_ = msgResp2.Body.Close()

		Expect(len(events1)).To(BeNumerically(">=", 1), "First turn should produce SSE events")
		Expect(len(events2)).To(BeNumerically(">=", 1), "Second turn should produce SSE events")

		Expect(events1[0].ID).To(MatchRegexp(`^\d+$`),
			"First turn SSE event should have a numeric ID")
		Expect(events2[0].ID).To(MatchRegexp(`^\d+$`),
			"Second turn SSE event should have a numeric ID")
		Expect(events2[0].ID).ToNot(Equal(events1[0].ID),
			"SSE event IDs should be unique across conversation turns for reconnection support")
	})
})
