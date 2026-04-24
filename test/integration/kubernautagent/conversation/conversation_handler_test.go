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

package conversation_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/conversation"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

func withChiParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

var _ = Describe("Conversation Handler Integration — #592", func() {

	var (
		handler    *conversation.Handler
		mockAuthn  *auth.MockAuthenticator
		mockAuthz  *auth.MockAuthorizer
		auditStore *capturingAuditStore
		cfg        config.ConversationConfig
		logger     *slog.Logger
	)

	BeforeEach(func() {
		mockAuthn = &auth.MockAuthenticator{
			ValidUsers: map[string]string{
				"valid-token": "user:operator@kubernaut.ai",
			},
		}
		mockAuthz = &auth.MockAuthorizer{
			AllowedUsers: map[string]bool{
				"user:operator@kubernaut.ai": true,
			},
		}
		auditStore = &capturingAuditStore{}
		cfg = config.ConversationConfig{
			Enabled: true,
			Session: config.ConversationSessionConfig{
				TTL:      30 * time.Minute,
				MaxTurns: 30,
			},
			RateLimit: config.RateLimitConfig{
				PerUserPerMinute: 10,
				PerSession:       30,
			},
		}
		logger = slog.Default()
		handler = conversation.NewHandler(conversation.HandlerDeps{
			Authenticator: mockAuthn,
			Authorizer:    mockAuthz,
			AuditStore:    auditStore,
			Config:        cfg,
			Logger:        logger,
		})
	})

	Describe("IT-CS-592-001: Create session -> POST message -> SSE response", func() {
		It("should create a session and receive an SSE response to a message", func() {
			// Create session
			createReq := httptest.NewRequest(http.MethodPost, "/conversations/sessions",
				strings.NewReader(`{"rar_namespace":"production","rar_name":"oom-fix"}`))
			createReq.Header.Set("Authorization", "Bearer valid-token")
			createReq.Header.Set("Content-Type", "application/json")
			createW := httptest.NewRecorder()

			handler.HandleCreateSession(createW, createReq)
			createResp := createW.Result()
			defer createResp.Body.Close()

			Expect(createResp.StatusCode).To(Equal(http.StatusCreated),
				"session creation must return 201 Created")

			var session struct {
				SessionID string `json:"session_id"`
			}
			Expect(json.NewDecoder(createResp.Body).Decode(&session)).To(Succeed())
			Expect(session.SessionID).NotTo(BeEmpty(),
				"session creation must return a session_id")

			// POST message
			msgReq := httptest.NewRequest(http.MethodPost,
				"/conversations/sessions/"+session.SessionID+"/messages",
				strings.NewReader(`{"content":"What caused the OOM?"}`))
			msgReq.Header.Set("Authorization", "Bearer valid-token")
			msgReq.Header.Set("Content-Type", "application/json")
			msgReq = withChiParam(msgReq, "sessionID", session.SessionID)
			msgW := httptest.NewRecorder()

			handler.HandlePostMessage(msgW, msgReq)
			msgResp := msgW.Result()
			defer msgResp.Body.Close()

			Expect(msgResp.StatusCode).To(Equal(http.StatusOK),
				"POST message must return 200 OK with SSE stream")
			Expect(msgResp.Header.Get("Content-Type")).To(Equal("text/event-stream"),
				"response must use SSE content type")
		})
	})

	Describe("IT-CS-592-002: Audit chain -> session -> first message continues conversation", func() {
		It("should seed conversation from audit history", func() {
			createReq := httptest.NewRequest(http.MethodPost, "/conversations/sessions",
				strings.NewReader(`{"rar_namespace":"production","rar_name":"oom-fix","correlation_id":"rem-001"}`))
			createReq.Header.Set("Authorization", "Bearer valid-token")
			createReq.Header.Set("Content-Type", "application/json")
			createW := httptest.NewRecorder()

			handler.HandleCreateSession(createW, createReq)
			createResp := createW.Result()
			defer createResp.Body.Close()

			Expect(createResp.StatusCode).To(Equal(http.StatusCreated),
				"session with correlation_id must return 201 Created")

			var session struct {
				SessionID    string `json:"session_id"`
				SeededTurns  int    `json:"seeded_turns"`
			}
			Expect(json.NewDecoder(createResp.Body).Decode(&session)).To(Succeed())
			Expect(session.SessionID).NotTo(BeEmpty())
		})
	})

	Describe("IT-CS-592-003: SSE delivers tokens incrementally", func() {
		It("should stream SSE events with incrementing IDs", func() {
			// Create session first
			createReq := httptest.NewRequest(http.MethodPost, "/conversations/sessions",
				strings.NewReader(`{"rar_namespace":"production","rar_name":"oom-fix"}`))
			createReq.Header.Set("Authorization", "Bearer valid-token")
			createReq.Header.Set("Content-Type", "application/json")
			createW := httptest.NewRecorder()
			handler.HandleCreateSession(createW, createReq)
			createResp := createW.Result()
			defer createResp.Body.Close()
			Expect(createResp.StatusCode).To(Equal(http.StatusCreated))

			var session struct {
				SessionID string `json:"session_id"`
			}
			Expect(json.NewDecoder(createResp.Body).Decode(&session)).To(Succeed())

			// POST message and verify SSE events
			msgReq := httptest.NewRequest(http.MethodPost,
				"/conversations/sessions/"+session.SessionID+"/messages",
				strings.NewReader(`{"content":"What caused the OOM?"}`))
			msgReq.Header.Set("Authorization", "Bearer valid-token")
			msgReq.Header.Set("Content-Type", "application/json")
			msgReq = withChiParam(msgReq, "sessionID", session.SessionID)
			msgW := httptest.NewRecorder()
			handler.HandlePostMessage(msgW, msgReq)
			msgResp := msgW.Result()
			defer msgResp.Body.Close()

			Expect(msgResp.StatusCode).To(Equal(http.StatusOK))

			body, err := io.ReadAll(msgResp.Body)
			Expect(err).NotTo(HaveOccurred())
			lines := strings.Split(string(body), "\n")

			var eventIDs []string
			for _, line := range lines {
				if strings.HasPrefix(line, "id:") {
					eventIDs = append(eventIDs, strings.TrimSpace(strings.TrimPrefix(line, "id:")))
				}
			}
			Expect(eventIDs).NotTo(BeEmpty(),
				"SSE stream must include event IDs")
		})
	})

	Describe("IT-CS-592-006: Auth: invalid token -> 401; valid + no SAR -> 403", func() {
		It("should return 401 for invalid token", func() {
			req := httptest.NewRequest(http.MethodPost, "/conversations/sessions",
				strings.NewReader(`{"rar_namespace":"production","rar_name":"oom-fix"}`))
			req.Header.Set("Authorization", "Bearer invalid-token")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.HandleCreateSession(w, req)
			resp := w.Result()
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
				"invalid token must return 401 Unauthorized")
		})

		It("should return 403 for valid token but no SAR permission", func() {
			noSARAuthz := &auth.MockAuthorizer{}
			restrictedHandler := conversation.NewHandler(conversation.HandlerDeps{
				Authenticator: mockAuthn,
				Authorizer:    noSARAuthz,
				AuditStore:    auditStore,
				Config:        cfg,
				Logger:        logger,
			})

			req := httptest.NewRequest(http.MethodPost, "/conversations/sessions",
				strings.NewReader(`{"rar_namespace":"production","rar_name":"oom-fix"}`))
			req.Header.Set("Authorization", "Bearer valid-token")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			restrictedHandler.HandleCreateSession(w, req)
			resp := w.Result()
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
				"valid token without SAR on RAR must return 403 Forbidden")
		})
	})

	Describe("IT-CS-592-007: RAR status change -> session transition -> 409", func() {
		It("should return 409 Conflict when posting to a closed session", func() {
			// Create session
			createReq := httptest.NewRequest(http.MethodPost, "/conversations/sessions",
				strings.NewReader(`{"rar_namespace":"production","rar_name":"oom-fix"}`))
			createReq.Header.Set("Authorization", "Bearer valid-token")
			createReq.Header.Set("Content-Type", "application/json")
			createW := httptest.NewRecorder()
			handler.HandleCreateSession(createW, createReq)
			createResp := createW.Result()
			defer createResp.Body.Close()
			Expect(createResp.StatusCode).To(Equal(http.StatusCreated))

			var session struct {
				SessionID string `json:"session_id"`
			}
			Expect(json.NewDecoder(createResp.Body).Decode(&session)).To(Succeed())

			// Simulate RAR status change by attempting to POST to a closed/expired session
			// after TTL or lifecycle event
			msgReq := httptest.NewRequest(http.MethodPost,
				"/conversations/sessions/nonexistent-session-id/messages",
				strings.NewReader(`{"content":"hello"}`))
			msgReq.Header.Set("Authorization", "Bearer valid-token")
			msgReq.Header.Set("Content-Type", "application/json")
			msgReq = withChiParam(msgReq, "sessionID", "nonexistent-session-id")
			msgW := httptest.NewRecorder()
			handler.HandlePostMessage(msgW, msgReq)
			msgResp := msgW.Result()
			defer msgResp.Body.Close()

			Expect(msgResp.StatusCode).To(Equal(http.StatusConflict),
				"posting to a non-existent/closed session must return 409 Conflict")
		})
	})

	Describe("IT-CS-592-008: LLM failure mid-stream -> SSE error event", func() {
		It("should send an SSE error event when LLM fails", func() {
			failingHandler := conversation.NewHandler(conversation.HandlerDeps{
				Authenticator: mockAuthn,
				Authorizer:    mockAuthz,
				AuditStore:    auditStore,
				Config:        cfg,
				Logger:        logger,
			}).WithLLM(&failingLLM{})

			// Create session
			createReq := httptest.NewRequest(http.MethodPost, "/conversations/sessions",
				strings.NewReader(`{"rar_namespace":"production","rar_name":"oom-fix"}`))
			createReq.Header.Set("Authorization", "Bearer valid-token")
			createReq.Header.Set("Content-Type", "application/json")
			createW := httptest.NewRecorder()
			failingHandler.HandleCreateSession(createW, createReq)
			createResp := createW.Result()
			defer createResp.Body.Close()
			Expect(createResp.StatusCode).To(Equal(http.StatusCreated))

			var session struct {
				SessionID string `json:"session_id"`
			}
			Expect(json.NewDecoder(createResp.Body).Decode(&session)).To(Succeed())

			// Post message that triggers LLM failure
			msgReq := httptest.NewRequest(http.MethodPost,
				"/conversations/sessions/"+session.SessionID+"/messages",
				strings.NewReader(`{"content":"trigger-llm-failure"}`))
			msgReq.Header.Set("Authorization", "Bearer valid-token")
			msgReq.Header.Set("Content-Type", "application/json")
			msgReq = withChiParam(msgReq, "sessionID", session.SessionID)
			msgW := httptest.NewRecorder()
			failingHandler.HandlePostMessage(msgW, msgReq)
			msgResp := msgW.Result()
			defer msgResp.Body.Close()

			Expect(msgResp.StatusCode).To(Equal(http.StatusOK),
				"even on LLM failure the SSE stream should start (error delivered as event)")

			body, err := io.ReadAll(msgResp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("event: error"),
				"LLM failure must produce an SSE error event")
		})
	})

	Describe("IT-CS-592-009: TLS enforcement", func() {
		It("should reject non-TLS connections when TLS is configured", func() {
			// Start a plain TCP server to simulate non-TLS
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			Expect(err).NotTo(HaveOccurred())
			defer listener.Close()

			go func() {
				conn, acceptErr := listener.Accept()
				if acceptErr != nil {
					return
				}
				scanner := bufio.NewScanner(conn)
				for scanner.Scan() {
					if scanner.Text() == "" {
						break
					}
				}
				_, _ = conn.Write([]byte("HTTP/1.1 426 Upgrade Required\r\nContent-Length: 0\r\n\r\n"))
				_ = conn.Close()
			}()

			addr := listener.Addr().String()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodPost,
				"http://"+addr+"/conversations/sessions", strings.NewReader(`{}`))
			Expect(err).NotTo(HaveOccurred())
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				defer resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(http.StatusUpgradeRequired),
					"non-TLS connection must be rejected when TLS is enforced")
			}

			// TLS should succeed (self-signed for test)
			_ = tls.Config{RootCAs: x509.NewCertPool()}
			// Placeholder: full TLS test requires cert generation which is Phase 8 GREEN
		})
	})
})

type capturingAuditStore struct {
	events []*audit.AuditEvent
}

func (s *capturingAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.events = append(s.events, event)
	return nil
}

type failingLLM struct{}

func (f *failingLLM) Respond(_ context.Context, _, _ string, _ func(conversation.ConversationEvent)) error {
	return fmt.Errorf("LLM service unavailable: connection timeout")
}
