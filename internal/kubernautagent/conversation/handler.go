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

package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// ConversationLLM generates conversation responses via streaming events (DD-CONV-001).
type ConversationLLM interface {
	Respond(ctx context.Context, sessionID, message string, emit func(ConversationEvent)) error
}

// defaultLLM is a placeholder LLM that emits a single message event.
type defaultLLM struct{}

func (d *defaultLLM) Respond(_ context.Context, _ string, message string, emit func(ConversationEvent)) error {
	emit(ConversationEvent{
		Type: "message",
		Data: mustMarshal(map[string]string{"content": fmt.Sprintf("Based on the investigation, the issue is related to: %s", message)}),
	})
	return nil
}

// NewDefaultLLM returns a default LLM for tests that need a working conversation LLM.
func NewDefaultLLM() ConversationLLM { return &defaultLLM{} }

// failingLLM is a test LLM that always returns an error.
type failingLLM struct{}

func (f *failingLLM) Respond(_ context.Context, _, _ string, _ func(ConversationEvent)) error {
	return fmt.Errorf("LLM service unavailable: connection timeout")
}

// NewFailingLLM returns a failing LLM for error-path tests.
func NewFailingLLM() ConversationLLM { return &failingLLM{} }

func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

// HandlerDeps bundles the dependencies for NewHandler.
// Optional fields (PromptBuilder) default to nil when omitted.
type HandlerDeps struct {
	Authenticator auth.Authenticator
	Authorizer    auth.Authorizer
	AuditStore    auditStore
	Config        config.ConversationConfig
	Logger        *slog.Logger
	PromptBuilder *prompt.Builder // nil is safe for tests that don't exercise prompt rendering
}

// Handler serves the conversation HTTP API.
type Handler struct {
	auth        *ConversationAuth
	sessions    *SessionManager
	sse         *SSEWriter
	rateLimiter *RateLimiter
	auditor     *TurnAuditor
	llm         ConversationLLM
	config      config.ConversationConfig
	logger      *slog.Logger
}

// NewHandler creates a conversation handler from a deps struct.
func NewHandler(deps HandlerDeps) *Handler {
	return &Handler{
		auth:        NewConversationAuth(deps.Authenticator, deps.Authorizer),
		sessions:    NewSessionManager(deps.Config.Session.TTL, deps.PromptBuilder),
		sse:         NewSSEWriter(deps.Config.Session.TTL),
		rateLimiter: NewRateLimiter(deps.Config.RateLimit.PerUserPerMinute, deps.Config.RateLimit.PerSession),
		auditor:     NewTurnAuditor(deps.AuditStore),
		llm:         &defaultLLM{},
		config:      deps.Config,
		logger:      deps.Logger,
	}
}

// WithLLM replaces the default LLM client (for testing).
func (h *Handler) WithLLM(llm ConversationLLM) *Handler {
	h.llm = llm
	return h
}

// WithLLMClient wires the production LLM adapter with tool-call loop (DD-CONV-001).
func (h *Handler) WithLLMClient(deps LLMAdapterDeps) *Handler {
	deps.Sessions = h.sessions
	h.llm = NewLLMAdapter(deps)
	return h
}

type createSessionRequest struct {
	RARNamespace  string `json:"rar_namespace"`
	RARName       string `json:"rar_name"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

type createSessionResponse struct {
	SessionID   string `json:"session_id"`
	SeededTurns int    `json:"seeded_turns,omitempty"`
}

type postMessageRequest struct {
	Content string `json:"content"`
}

func extractBearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return h[7:]
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeRFC7807(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"type":   "https://kubernaut.ai/problems/conversation-error",
		"title":  http.StatusText(status),
		"status": status,
		"detail": detail,
	})
}

// HandleCreateSession handles POST /conversations/sessions.
func (h *Handler) HandleCreateSession(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	userID, err := h.auth.Authenticate(r.Context(), token)
	if err != nil {
		writeRFC7807(w, http.StatusUnauthorized, "invalid or missing bearer token")
		return
	}

	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRFC7807(w, http.StatusBadRequest, "invalid request body")
		return
	}

	allowed, err := h.auth.AuthorizeRAR(r.Context(), userID, req.RARNamespace, req.RARName)
	if err != nil {
		h.logger.Error("SAR check failed", slog.String("user", userID), slog.Any("error", err))
		writeRFC7807(w, http.StatusInternalServerError, "authorization check failed")
		return
	}
	if !allowed {
		writeRFC7807(w, http.StatusForbidden,
			fmt.Sprintf("user %s does not have UPDATE permission on RAR %s/%s", userID, req.RARNamespace, req.RARName))
		return
	}

	session, err := h.sessions.Create(req.RARName, req.RARNamespace, userID, req.CorrelationID)
	if err != nil {
		h.logger.Error("session creation failed", slog.Any("error", err))
		writeRFC7807(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	writeJSON(w, http.StatusCreated, createSessionResponse{
		SessionID: session.ID,
	})
}

// HandlePostMessage handles POST /conversations/sessions/{id}/messages.
func (h *Handler) HandlePostMessage(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	userID, err := h.auth.Authenticate(r.Context(), token)
	if err != nil {
		writeRFC7807(w, http.StatusUnauthorized, "invalid or missing bearer token")
		return
	}

	sessionID := chi.URLParam(r, "sessionID")
	session, err := h.sessions.Get(sessionID)
	if err != nil {
		writeRFC7807(w, http.StatusConflict,
			fmt.Sprintf("session %s not found or expired", sessionID))
		return
	}

	if session.IsClosed() {
		writeRFC7807(w, http.StatusConflict, "session is closed")
		return
	}

	allowed, err := h.auth.AuthorizeRAR(r.Context(), userID, session.RARNamespace, session.RARName)
	if err != nil {
		h.logger.Error("SAR check failed", slog.String("user", userID), slog.Any("error", err))
		writeRFC7807(w, http.StatusInternalServerError, "authorization check failed")
		return
	}
	if !allowed {
		writeRFC7807(w, http.StatusForbidden, "access denied to this session")
		return
	}

	var req postMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRFC7807(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !h.rateLimiter.AllowUser(userID) {
		writeRFC7807(w, http.StatusTooManyRequests, "per-user rate limit exceeded")
		return
	}

	// DD-F5: check-then-increment — rejected turns do not consume a slot.
	if !h.rateLimiter.AllowSession(session.ID, session.TurnCount+1) {
		writeRFC7807(w, http.StatusTooManyRequests, "per-session turn limit exceeded")
		return
	}

	if _, err := h.sessions.IncrementTurnAndTouch(session.ID, time.Now()); err != nil {
		h.logger.Error("failed to increment turn", slog.String("session", session.ID), slog.Any("error", err))
		writeRFC7807(w, http.StatusInternalServerError, "failed to update session")
		return
	}

	w.Header().Set("Content-Type", SSEContentType)
	w.Header().Set("Cache-Control", SSECacheControl)
	w.Header().Set("Connection", SSEConnection)
	w.Header().Set(SSEAccelBufferingKey, SSEAccelBufferingValue)
	w.WriteHeader(http.StatusOK)

	flusher, canFlush := w.(http.Flusher)
	var lastContent string
	emit := func(ev ConversationEvent) {
		sseEvt := h.sse.WriteEvent(ev.Type, string(ev.Data))
		_, _ = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", sseEvt.ID, sseEvt.Event, sseEvt.Data)
		if canFlush {
			flusher.Flush()
		}
		if ev.Type == "message" {
			var payload map[string]string
			if json.Unmarshal(ev.Data, &payload) == nil {
				lastContent = payload["content"]
			}
		}
	}

	if llmErr := h.llm.Respond(r.Context(), session.ID, req.Content, emit); llmErr != nil {
		if errors.Is(llmErr, ErrMaxToolTurnsExceeded) {
			h.logger.Warn("tool-call loop exhausted", slog.String("session", session.ID))
			return
		}
		h.logger.Error("LLM failure during conversation", slog.String("session", session.ID), slog.Any("error", llmErr))
		errData := mustMarshal(map[string]string{"error": llmErr.Error()})
		errEvt := h.sse.WriteEvent("error", string(errData))
		_, _ = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", errEvt.ID, errEvt.Event, errEvt.Data)
		if canFlush {
			flusher.Flush()
		}
		return
	}

	h.auditor.EmitTurn(r.Context(), session.ID, userID, session.CorrelationID, req.Content, lastContent)
}
