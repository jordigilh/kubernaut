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
	"log/slog"
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
	runner ReconRunner
	recon  ContextReconstructor
	logger *slog.Logger
}

// NewReconstructionSpawner creates a spawner with the given dependencies.
func NewReconstructionSpawner(runner ReconRunner, recon ContextReconstructor, logger *slog.Logger) *ReconstructionSpawner {
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

// SpawnReconstruct rebuilds conversation context and invokes RunReconTurn
// with the reconstructed messages. Best-effort: empty context is acceptable
// (BR-INTERACTIVE-008). Safe to call as a goroutine: panics are recovered.
func (s *ReconstructionSpawner) SpawnReconstruct(ctx context.Context, entry *ReconstructionContext) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic in SpawnReconstruct: %v", r)
			s.logger.Error("panic recovered during reconstruction",
				slog.String("correlation_id", entry.CorrelationID),
				slog.Any("panic", r))
		}
	}()

	if entry == nil {
		return fmt.Errorf("reconstruction context must not be nil")
	}

	turns, reconErr := s.recon.Reconstruct(ctx, entry.CorrelationID, entry.SessionID)
	if reconErr != nil {
		s.logger.Warn("context reconstruction returned error; proceeding with empty context",
			slog.String("correlation_id", entry.CorrelationID),
			slog.String("error", reconErr.Error()))
	}

	messages := turnsToReconMessages(turns)

	_, err := s.runner.RunReconTurn(ctx, messages, entry.CorrelationID)
	if err != nil {
		s.logger.Error("reconstruction RunReconTurn failed",
			slog.String("correlation_id", entry.CorrelationID),
			slog.String("error", err.Error()))
		return fmt.Errorf("reconstruction RunReconTurn: %w", err)
	}

	return nil
}

func turnsToReconMessages(turns []ConversationTurn) []ReconMessage {
	if len(turns) == 0 {
		return nil
	}
	messages := make([]ReconMessage, len(turns))
	for i, t := range turns {
		messages[i] = ReconMessage{Role: t.Role, Content: t.Content}
	}
	return messages
}
