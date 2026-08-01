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

package mcp

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
)

const kaServiceAccount = "system:serviceaccount:kubernaut:kubernaut-agent"

// ReconMessage represents a single conversation message for reconstruction.
// Mirrors tools.LLMMessage to avoid import cycles.
type ReconMessage struct {
	Role    string
	Content string
}

// ReconRunner is the interface for executing LLM turns during reconstruction.
// Implemented by the same adapter as tools.InvestigatorRunner.
type ReconRunner interface {
	RunReconTurn(ctx context.Context, messages []ReconMessage, correlationID string) (string, error)
}

// ReconstructionContext holds the information needed to reconstruct an
// autonomous investigation after an interactive session ends.
type ReconstructionContext struct {
	CorrelationID string
	SessionID     string
	SignalMeta    map[string]string
}

// ReconstructionSpawner rebuilds the conversation context from DS audit events
// and spawns a new autonomous investigation via RunReconTurn.
// SEC-04: uses explicit KA SA identity for reconstructed sessions.
type ReconstructionSpawner struct {
	runner     ReconRunner
	recon      ContextReconstructor
	logger     logr.Logger
	auditStore audit.AuditStore
}

// NewReconstructionSpawner creates a spawner with the given dependencies.
func NewReconstructionSpawner(runner ReconRunner, recon ContextReconstructor, logger logr.Logger) *ReconstructionSpawner {
	return &ReconstructionSpawner{
		runner: runner,
		recon:  recon,
		logger: logger,
	}
}

// ServiceAccountIdentity returns the KA service account used for reconstructed sessions.
func (s *ReconstructionSpawner) ServiceAccountIdentity() string {
	return kaServiceAccount
}

// SetAuditStore enables aiagent.session.resumed audit emission (BR-INTERACTIVE-003
// #5, audit catalog gap follow-up). A setter rather than a constructor
// parameter to avoid a breaking-change ripple across the many pre-existing
// NewReconstructionSpawner call sites that don't need it (mirrors
// K8sAdapter.SetLogger's pattern for the same reason).
func (s *ReconstructionSpawner) SetAuditStore(store audit.AuditStore) {
	s.auditStore = store
}

// SpawnReconstruct rebuilds conversation context and invokes RunReconTurn
// with the reconstructed messages. Best-effort: empty context is acceptable
// (BR-INTERACTIVE-008). Safe to call as a goroutine: panics are recovered.
func (s *ReconstructionSpawner) SpawnReconstruct(ctx context.Context, entry *ReconstructionContext) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic in SpawnReconstruct: %v", r)
			s.logger.Error(retErr, "panic recovered during reconstruction",
				"correlation_id", entry.CorrelationID,
				"panic", r)
		}
	}()

	if entry == nil {
		return fmt.Errorf("reconstruction context must not be nil")
	}

	turns, reconErr := s.recon.Reconstruct(ctx, entry.CorrelationID, entry.SessionID)
	if reconErr != nil {
		s.logger.Info("context reconstruction returned error; proceeding with empty context",
			"correlation_id", entry.CorrelationID,
			"error", reconErr.Error())
	}

	messages := turnsToReconMessages(turns)

	// Identity transition back to the KA SA happens here, regardless of
	// whether the reconstructed context was complete (best-effort above) or
	// of RunReconTurn's own outcome below -- KA has definitively reclaimed
	// control of the investigation from the interactive session identified
	// by entry.SessionID (BR-INTERACTIVE-003 #5, mirrors
	// EventTypeSessionSuspended's counterpart at takeover time).
	s.emitSessionResumed(entry, len(messages)) //nolint:contextcheck // emitSessionResumed uses audit.StoreBestEffort by design (ADR-038); see its doc comment

	_, err := s.runner.RunReconTurn(ctx, messages, entry.CorrelationID)
	if err != nil {
		return fmt.Errorf("reconstruction RunReconTurn: %w", err)
	}

	return nil
}

// emitSessionResumed records aiagent.session.resumed for the interactive
// session that just ended. Fire-and-forget (ADR-038): uses context.Background()
// so a caller-cancelled ctx never drops the event, and StoreBestEffort
// swallows store errors after logging.
func (s *ReconstructionSpawner) emitSessionResumed(entry *ReconstructionContext, reconstructedTurnCount int) {
	if s.auditStore == nil {
		return
	}
	event := audit.NewEvent(audit.EventTypeSessionResumed, entry.CorrelationID,
		audit.WithSessionID(entry.SessionID),
	)
	event.EventAction = audit.ActionSessionResumed
	event.EventOutcome = audit.OutcomeSuccess
	event.Data["reconstructed_turn_count"] = reconstructedTurnCount
	audit.StoreBestEffort(context.Background(), s.auditStore, event, s.logger)
}

func turnsToReconMessages(turns []ConversationTurn) []ReconMessage {
	if len(turns) == 0 {
		return nil
	}
	messages := make([]ReconMessage, 0, len(turns))
	for _, t := range turns {
		if t.Content == "" {
			continue
		}
		messages = append(messages, ReconMessage{Role: t.Role, Content: t.Content})
	}
	if len(messages) == 0 {
		return nil
	}
	return messages
}
