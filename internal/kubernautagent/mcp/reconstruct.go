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
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
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

// SignalContextToMetadata preserves signal fields needed by interactive
// disconnect reconstruction without coupling the lease manager to signal DTOs.
func SignalContextToMetadata(signal katypes.SignalContext) map[string]string {
	metadata := make(map[string]string, 30)
	put := func(key, value string) {
		if value != "" {
			metadata[key] = value
		}
	}
	putMap := func(key string, value map[string]string) {
		if value == nil {
			return
		}
		encoded, err := json.Marshal(value)
		if err == nil {
			metadata[key] = string(encoded)
		}
	}
	putBool := func(key string, value *bool) {
		if value != nil {
			metadata[key] = strconv.FormatBool(*value)
		}
	}
	putInt := func(key string, value *int) {
		if value != nil {
			metadata[key] = strconv.Itoa(*value)
		}
	}
	put("signal_name", signal.Name)
	put("namespace", signal.Namespace)
	put("severity", signal.Severity)
	put("message", signal.Message)
	put("incident_id", signal.IncidentID)
	put("remediation_id", signal.RemediationID)
	put("resource_kind", signal.ResourceKind)
	put("resource_name", signal.ResourceName)
	put("resource_api_version", signal.ResourceAPIVersion)
	put("cluster_id", signal.ClusterID)
	put("cluster_name", signal.ClusterName)
	put("environment", signal.Environment)
	put("priority", signal.Priority)
	put("risk_tolerance", signal.RiskTolerance)
	put("signal_source", signal.SignalSource)
	put("business_category", signal.BusinessCategory)
	put("description", signal.Description)
	put("signal_mode", signal.SignalMode)
	put("firing_time", signal.FiringTime)
	put("received_time", signal.ReceivedTime)
	putBool("is_duplicate", signal.IsDuplicate)
	putInt("occurrence_count", signal.OccurrenceCount)
	putMap("signal_annotations", signal.SignalAnnotations)
	putMap("signal_labels", signal.SignalLabels)
	put("detected_labels_json", signal.DetectedLabelsJSON)
	putInt("deduplication_window_minutes", signal.DeduplicationWindowMinutes)
	put("first_seen", signal.FirstSeen)
	put("last_seen", signal.LastSeen)
	if signal.Interactive {
		metadata["interactive"] = strconv.FormatBool(signal.Interactive)
	}
	put("cluster_classification", signal.ClusterClassification)
	return metadata
}

// SignalContextFromMetadata restores signal context captured before an
// interactive lease was released. It returns false when no signal fields exist.
func SignalContextFromMetadata(metadata map[string]string) (katypes.SignalContext, bool) {
	if len(metadata) == 0 {
		return katypes.SignalContext{}, false
	}
	signal := katypes.SignalContext{
		Name: metadata["signal_name"], Namespace: metadata["namespace"], Severity: metadata["severity"],
		Message: metadata["message"], IncidentID: metadata["incident_id"], RemediationID: metadata["remediation_id"],
		ResourceKind: metadata["resource_kind"], ResourceName: metadata["resource_name"], ResourceAPIVersion: metadata["resource_api_version"],
		ClusterID: metadata["cluster_id"], ClusterName: metadata["cluster_name"], Environment: metadata["environment"],
		Priority: metadata["priority"], RiskTolerance: metadata["risk_tolerance"], SignalSource: metadata["signal_source"],
		BusinessCategory: metadata["business_category"], Description: metadata["description"], SignalMode: metadata["signal_mode"],
		FiringTime: metadata["firing_time"], ReceivedTime: metadata["received_time"], FirstSeen: metadata["first_seen"],
		LastSeen: metadata["last_seen"], DetectedLabelsJSON: metadata["detected_labels_json"],
		ClusterClassification: metadata["cluster_classification"],
	}
	restoreOptionalSignalMetadata(&signal, metadata)
	return signal, hasSignalMetadata(metadata)
}

func restoreOptionalSignalMetadata(signal *katypes.SignalContext, metadata map[string]string) {
	if value, ok := metadata["is_duplicate"]; ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			signal.IsDuplicate = &parsed
		}
	}
	if value, ok := metadata["occurrence_count"]; ok {
		if parsed, err := strconv.Atoi(value); err == nil {
			signal.OccurrenceCount = &parsed
		}
	}
	if value, ok := metadata["signal_annotations"]; ok {
		var parsed map[string]string
		if err := json.Unmarshal([]byte(value), &parsed); err == nil {
			signal.SignalAnnotations = parsed
		}
	}
	if value, ok := metadata["signal_labels"]; ok {
		var parsed map[string]string
		if err := json.Unmarshal([]byte(value), &parsed); err == nil {
			signal.SignalLabels = parsed
		}
	}
	if value, ok := metadata["deduplication_window_minutes"]; ok {
		if parsed, err := strconv.Atoi(value); err == nil {
			signal.DeduplicationWindowMinutes = &parsed
		}
	}
	if value, ok := metadata["interactive"]; ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			signal.Interactive = parsed
		}
	}
}

func hasSignalMetadata(metadata map[string]string) bool {
	for _, key := range []string{
		"signal_name", "namespace", "severity", "message", "incident_id", "remediation_id",
		"resource_kind", "resource_name", "resource_api_version", "cluster_id", "cluster_name",
		"environment", "priority", "risk_tolerance", "signal_source", "business_category",
		"description", "signal_mode", "firing_time", "received_time", "is_duplicate",
		"occurrence_count", "signal_annotations", "signal_labels", "detected_labels_json",
		"deduplication_window_minutes", "first_seen", "last_seen", "interactive", "cluster_classification",
	} {
		if _, ok := metadata[key]; ok {
			return true
		}
	}
	return false
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

// SetAuditStore enables aiagent.session.resumed audit emission (BR-INTERACTIVE-003
// #5, audit catalog gap follow-up). A setter rather than a constructor
// parameter to avoid a breaking-change ripple across the many pre-existing
// NewReconstructionSpawner call sites that don't need it (mirrors
// K8sAdapter.SetLogger's pattern for the same reason).
func (s *ReconstructionSpawner) SetAuditStore(store audit.AuditStore) {
	s.auditStore = store
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
	if signal, ok := SignalContextFromMetadata(entry.SignalMeta); ok {
		if signal.RemediationID == "" {
			signal.RemediationID = entry.CorrelationID
		}
		ctx = katypes.WithSignalContext(ctx, signal)
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
	if clusterID := entry.SignalMeta["cluster_id"]; clusterID != "" {
		event.ClusterID = clusterID
	}
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
