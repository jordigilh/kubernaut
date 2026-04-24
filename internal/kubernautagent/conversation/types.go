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
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// ConversationEvent is an SSE event emitted during a conversation turn (DD-CONV-001).
type ConversationEvent struct {
	Type string          `json:"type"` // "tool_call", "tool_result", "tool_error", "message", "error"
	Data json.RawMessage `json:"data"`
}

// SessionState represents the lifecycle state of a conversation session.
type SessionState string

const (
	SessionInteractive SessionState = "interactive"
	SessionReadOnly    SessionState = "read_only"
	SessionClosed      SessionState = "closed"
)

// Session holds the state for a conversation about a specific RAR.
type Session struct {
	mu                   sync.RWMutex // protects Messages
	ID                   string
	RARName              string
	RARNamespace         string
	State                SessionState
	Participants         []string
	TTL                  time.Duration
	CreatedAt            time.Time
	LastActivity         time.Time
	TurnCount            int
	Guardrails           *Guardrails
	promptBuilder        *prompt.Builder
	todoWrite            tools.Tool
	CorrelationID        string
	InvestigationSummary string
	Messages             []llm.Message
}

// TodoWrite returns the per-session todo_write tool (DD-CONV-001).
func (s *Session) TodoWrite() tools.Tool {
	return s.todoWrite
}

// SystemPrompt renders the conversation system prompt from the template.
// Returns error when promptBuilder is nil (DD-F6 safety gap).
func (s *Session) SystemPrompt() (string, error) {
	if s.promptBuilder == nil {
		return "", fmt.Errorf("prompt builder not initialized for session %s", s.ID)
	}
	return s.promptBuilder.RenderConversation(prompt.ConversationTemplateData{
		RARName:              s.RARName,
		Namespace:            s.RARNamespace,
		AvailableTools:       s.Guardrails.ReadOnlyToolNames(),
		InvestigationSummary: s.InvestigationSummary,
	})
}

// GetMessages returns a copy of the conversation history.
// System prompt is NOT included (rebuilt from template each turn).
func (s *Session) GetMessages() []llm.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]llm.Message, len(s.Messages))
	copy(cp, s.Messages)
	return cp
}

// AppendMessages stores conversation exchanges for cross-message continuity.
func (s *Session) AppendMessages(msgs ...llm.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, msgs...)
}

// IsReadOnly returns true when the session is in read-only state.
func (s *Session) IsReadOnly() bool {
	return s.State == SessionReadOnly
}

// IsClosed returns true when the session is in closed state.
func (s *Session) IsClosed() bool {
	return s.State == SessionClosed
}

// IsInteractive returns true when the session is in interactive state.
func (s *Session) IsInteractive() bool {
	return s.State == SessionInteractive
}

// SetInvestigationContext populates the investigation context fetched from the RAR CRD.
// Called by the handler after session creation when a RARReader is available.
func (s *Session) SetInvestigationContext(summary string) {
	s.InvestigationSummary = summary
}
