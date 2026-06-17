/*
Spike 4: a2acompat/a2av0 Roundtrip Validation

Validates that kubernaut's EventBridge event types survive the v2 -> v0.3
conversion roundtrip with data integrity. This is critical for Kagenti v0.6
wire format compatibility during the dual-protocol migration window.

Test matrix:
  - TaskStatusUpdateEvent with metadata.type tags (reasoning, status, keepalive)
  - TaskStatusUpdateEvent with RR context metadata
  - TaskArtifactUpdateEvent with DataPart + TextPart
  - TaskStatusUpdateEvent keepalive (no message, metadata only)
  - JSON wire format key name validation (v0.3 uses different key names)

This is a throwaway spike -- not production code.
*/
package spike

import (
	"encoding/json"
	"testing"
	"time"

	a2alegacy "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"
)

func TestCompat_StatusEventWithReasoningMetadata(t *testing.T) {
	now := time.Now().UTC()
	v2Event := &a2a.TaskStatusUpdateEvent{
		TaskID:    "task-123",
		ContextID: "ctx-456",
		Status: a2a.TaskStatus{
			State:     a2a.TaskStateWorking,
			Timestamp: &now,
			Message: &a2a.Message{
				Role: a2a.MessageRoleAgent,
				Parts: a2a.ContentParts{
					&a2a.Part{Content: a2a.Text("Investigating pod crash loop...")},
				},
			},
		},
		Metadata: map[string]any{
			"type": "reasoning",
		},
	}

	legacy := a2av0.FromV1TaskStatusUpdateEvent(v2Event)
	if legacy == nil {
		t.Fatal("FromV1TaskStatusUpdateEvent returned nil")
	}

	if string(legacy.TaskID) != "task-123" {
		t.Errorf("TaskID mismatch: got %q", legacy.TaskID)
	}
	if legacy.ContextID != "ctx-456" {
		t.Errorf("ContextID mismatch: got %q", legacy.ContextID)
	}
	if legacy.Status.State != a2alegacy.TaskStateWorking {
		t.Errorf("State mismatch: got %q", legacy.Status.State)
	}

	if legacy.Status.Message == nil {
		t.Fatal("Message is nil after conversion")
	}
	if legacy.Status.Message.Role != a2alegacy.MessageRoleAgent {
		t.Errorf("Role mismatch: got %q", legacy.Status.Message.Role)
	}
	if len(legacy.Status.Message.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(legacy.Status.Message.Parts))
	}
	textPart, ok := legacy.Status.Message.Parts[0].(a2alegacy.TextPart)
	if !ok {
		t.Fatalf("expected TextPart, got %T", legacy.Status.Message.Parts[0])
	}
	if textPart.Text != "Investigating pod crash loop..." {
		t.Errorf("text mismatch: got %q", textPart.Text)
	}

	metaType, ok := legacy.Metadata["type"].(string)
	if !ok || metaType != "reasoning" {
		t.Errorf("metadata.type mismatch: got %v", legacy.Metadata["type"])
	}

	// Roundtrip: v0.3 -> v2
	v2Back, err := a2av0.ToV1TaskStatusUpdateEvent(legacy)
	if err != nil {
		t.Fatalf("ToV1TaskStatusUpdateEvent error: %v", err)
	}
	if string(v2Back.TaskID) != "task-123" {
		t.Errorf("roundtrip TaskID mismatch: got %q", v2Back.TaskID)
	}
	if v2Back.Status.State != a2a.TaskStateWorking {
		t.Errorf("roundtrip State mismatch: got %q", v2Back.Status.State)
	}
	if v2Back.Status.Message == nil || len(v2Back.Status.Message.Parts) != 1 {
		t.Fatal("roundtrip message/parts lost")
	}

	t.Log("reasoning status event roundtrips correctly through compat layer")
}

func TestCompat_StatusEventWithRRContextMetadata(t *testing.T) {
	now := time.Now().UTC()
	v2Event := &a2a.TaskStatusUpdateEvent{
		TaskID:    "task-rr",
		ContextID: "ctx-rr",
		Status: a2a.TaskStatus{
			State:     a2a.TaskStateWorking,
			Timestamp: &now,
			Message: &a2a.Message{
				Role: a2a.MessageRoleAgent,
				Parts: a2a.ContentParts{
					&a2a.Part{Content: a2a.Text("Analyzing alert...")},
				},
			},
		},
		Metadata: map[string]any{
			"type":       "investigation",
			"rr_id":      "rr-abc-123",
			"namespace":  "production",
			"kind":       "Deployment",
			"target":     "api-server",
			"alert_name": "HighErrorRate",
			"phase":      "investigating",
		},
	}

	legacy := a2av0.FromV1TaskStatusUpdateEvent(v2Event)
	if legacy == nil {
		t.Fatal("conversion returned nil")
	}

	expectedKeys := []string{"type", "rr_id", "namespace", "kind", "target", "alert_name", "phase"}
	for _, key := range expectedKeys {
		if _, exists := legacy.Metadata[key]; !exists {
			t.Errorf("metadata key %q missing after conversion", key)
		}
	}

	if legacy.Metadata["rr_id"] != "rr-abc-123" {
		t.Errorf("rr_id mismatch: got %v", legacy.Metadata["rr_id"])
	}
	if legacy.Metadata["phase"] != "investigating" {
		t.Errorf("phase mismatch: got %v", legacy.Metadata["phase"])
	}

	// Roundtrip
	v2Back, err := a2av0.ToV1TaskStatusUpdateEvent(legacy)
	if err != nil {
		t.Fatalf("roundtrip error: %v", err)
	}
	for _, key := range expectedKeys {
		if _, exists := v2Back.Metadata[key]; !exists {
			t.Errorf("metadata key %q lost in roundtrip", key)
		}
	}

	t.Log("RR context metadata survives compat roundtrip")
}

func TestCompat_KeepaliveEvent(t *testing.T) {
	now := time.Now().UTC()
	v2Event := &a2a.TaskStatusUpdateEvent{
		TaskID:    "task-ka",
		ContextID: "ctx-ka",
		Status: a2a.TaskStatus{
			State:     a2a.TaskStateWorking,
			Timestamp: &now,
		},
		Metadata: map[string]any{
			"type": "keepalive",
			"dot":  ".",
		},
	}

	legacy := a2av0.FromV1TaskStatusUpdateEvent(v2Event)
	if legacy == nil {
		t.Fatal("conversion returned nil")
	}

	// Keepalive events have NO message (nil Status.Message)
	if legacy.Status.Message != nil {
		t.Log("note: keepalive has Status.Message (not nil) -- Kagenti may render it as text")
	}

	metaType := legacy.Metadata["type"]
	if metaType != "keepalive" {
		t.Errorf("keepalive metadata.type lost: got %v", metaType)
	}

	// Roundtrip
	v2Back, err := a2av0.ToV1TaskStatusUpdateEvent(legacy)
	if err != nil {
		t.Fatalf("roundtrip error: %v", err)
	}
	if v2Back.Metadata["type"] != "keepalive" {
		t.Errorf("keepalive type lost in roundtrip")
	}

	t.Log("keepalive event roundtrips correctly (no message body)")
}

func TestCompat_ArtifactEventWithDataAndText(t *testing.T) {
	v2Event := &a2a.TaskArtifactUpdateEvent{
		TaskID:    "task-art",
		ContextID: "ctx-art",
		Artifact: &a2a.Artifact{
			ID: "art-001",
			Parts: a2a.ContentParts{
				&a2a.Part{
					Content:  a2a.Data{Value: map[string]any{"severity": "critical", "count": float64(42)}},
					Metadata: map[string]any{"mediaType": "application/json"},
				},
				&a2a.Part{Content: a2a.Text("42 critical alerts found")},
			},
			Metadata: map[string]any{"artifact_type": "investigation_summary"},
		},
		LastChunk: true,
	}

	legacy := a2av0.FromV1TaskArtifactUpdateEvent(v2Event)
	if legacy == nil {
		t.Fatal("conversion returned nil")
	}

	if string(legacy.TaskID) != "task-art" {
		t.Errorf("TaskID mismatch: got %q", legacy.TaskID)
	}
	if !legacy.LastChunk {
		t.Error("LastChunk lost")
	}
	if legacy.Artifact == nil {
		t.Fatal("Artifact nil after conversion")
	}
	if len(legacy.Artifact.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(legacy.Artifact.Parts))
	}

	// Part[0] should be DataPart
	dataPart, ok := legacy.Artifact.Parts[0].(a2alegacy.DataPart)
	if !ok {
		t.Fatalf("expected DataPart, got %T", legacy.Artifact.Parts[0])
	}
	severity, ok := dataPart.Data["severity"].(string)
	if !ok || severity != "critical" {
		t.Errorf("DataPart.severity mismatch: got %v", dataPart.Data["severity"])
	}

	// Part[1] should be TextPart
	textPart, ok := legacy.Artifact.Parts[1].(a2alegacy.TextPart)
	if !ok {
		t.Fatalf("expected TextPart, got %T", legacy.Artifact.Parts[1])
	}
	if textPart.Text != "42 critical alerts found" {
		t.Errorf("text mismatch: got %q", textPart.Text)
	}

	// Roundtrip
	v2Back, err := a2av0.ToV1TaskArtifactUpdateEvent(legacy)
	if err != nil {
		t.Fatalf("roundtrip error: %v", err)
	}
	if string(v2Back.TaskID) != "task-art" {
		t.Errorf("roundtrip TaskID mismatch")
	}
	if !v2Back.LastChunk {
		t.Error("roundtrip LastChunk lost")
	}

	t.Log("artifact event with DataPart+TextPart roundtrips correctly")
}

func TestCompat_TerminalStateSetsFinalFlag(t *testing.T) {
	now := time.Now().UTC()
	states := []struct {
		v2State     a2a.TaskState
		expectFinal bool
	}{
		{a2a.TaskStateWorking, false},
		{a2a.TaskStateCompleted, true},
		{a2a.TaskStateFailed, true},
		{a2a.TaskStateCanceled, true},
		{a2a.TaskStateInputRequired, true},
		{a2a.TaskStateSubmitted, false},
	}

	for _, tc := range states {
		t.Run(string(tc.v2State), func(t *testing.T) {
			v2Event := &a2a.TaskStatusUpdateEvent{
				TaskID: "task-final",
				Status: a2a.TaskStatus{
					State:     tc.v2State,
					Timestamp: &now,
				},
			}
			legacy := a2av0.FromV1TaskStatusUpdateEvent(v2Event)
			if legacy.Final != tc.expectFinal {
				t.Errorf("state=%s: expected Final=%v, got %v", tc.v2State, tc.expectFinal, legacy.Final)
			}
		})
	}

	t.Log("terminal state -> Final flag mapping is correct for Kagenti SSE consumer")
}

func TestCompat_JSONWireFormatKeyNames(t *testing.T) {
	now := time.Now().UTC()
	v2Event := &a2a.TaskStatusUpdateEvent{
		TaskID:    "task-wire",
		ContextID: "ctx-wire",
		Status: a2a.TaskStatus{
			State:     a2a.TaskStateWorking,
			Timestamp: &now,
			Message: &a2a.Message{
				Role: a2a.MessageRoleAgent,
				Parts: a2a.ContentParts{
					&a2a.Part{Content: a2a.Text("hello from kubernaut")},
				},
			},
		},
		Metadata: map[string]any{"type": "status"},
	}

	legacy := a2av0.FromV1TaskStatusUpdateEvent(v2Event)
	bytes, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var wireMap map[string]any
	if err := json.Unmarshal(bytes, &wireMap); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	// v0.3 wire format uses camelCase keys and a "kind" discriminator
	// (not snake_case like v2). Kagenti v0.6 expects these exact keys.
	requiredKeys := []string{"kind", "contextId", "status", "metadata", "final"}
	for _, key := range requiredKeys {
		if _, exists := wireMap[key]; !exists {
			t.Errorf("v0.3 wire format missing key: %q", key)
		}
	}

	if wireMap["kind"] != "status-update" {
		t.Errorf("expected kind=status-update, got %v", wireMap["kind"])
	}

	// Verify status sub-object has correct keys
	statusMap, ok := wireMap["status"].(map[string]any)
	if !ok {
		t.Fatal("status is not a map")
	}
	statusKeys := []string{"state", "message"}
	for _, key := range statusKeys {
		if _, exists := statusMap[key]; !exists {
			t.Errorf("status missing key: %q", key)
		}
	}

	// Verify message has v0.3-style keys: kind, role, parts, messageId
	msgMap, ok := statusMap["message"].(map[string]any)
	if !ok {
		t.Fatal("message is not a map")
	}
	if msgMap["role"] != "agent" {
		t.Errorf("role mismatch: got %v", msgMap["role"])
	}
	if msgMap["kind"] != "message" {
		t.Errorf("message kind mismatch: got %v", msgMap["kind"])
	}
	parts, ok := msgMap["parts"].([]any)
	if !ok || len(parts) == 0 {
		t.Fatal("parts missing or empty")
	}

	// Verify part has v0.3 "kind":"text" discriminator
	partMap, ok := parts[0].(map[string]any)
	if !ok {
		t.Fatal("part is not a map")
	}
	if partMap["kind"] != "text" {
		t.Errorf("part kind mismatch: got %v", partMap["kind"])
	}

	t.Logf("v0.3 wire format: %s", string(bytes[:min(len(bytes), 300)]))
	t.Log("JSON wire format key names are v0.3-compatible for Kagenti")
}

func TestCompat_AllTaskStatesMapBidirectionally(t *testing.T) {
	v2States := []a2a.TaskState{
		a2a.TaskStateAuthRequired,
		a2a.TaskStateCanceled,
		a2a.TaskStateCompleted,
		a2a.TaskStateFailed,
		a2a.TaskStateInputRequired,
		a2a.TaskStateRejected,
		a2a.TaskStateSubmitted,
		a2a.TaskStateWorking,
	}

	for _, s := range v2States {
		legacy := a2av0.FromV1TaskState(s)
		if legacy == a2alegacy.TaskStateUnspecified {
			t.Errorf("v2 state %q maps to Unspecified", s)
			continue
		}
		back := a2av0.ToV1TaskState(legacy)
		if back != s {
			t.Errorf("roundtrip failed for %q: got %q", s, back)
		}
	}

	t.Log("all v2 task states map bidirectionally through compat layer")
}

func TestCompat_MessageRolesMapBidirectionally(t *testing.T) {
	now := time.Now().UTC()
	roles := []a2a.MessageRole{
		a2a.MessageRoleAgent,
		a2a.MessageRoleUser,
	}

	for _, role := range roles {
		v2Event := &a2a.TaskStatusUpdateEvent{
			TaskID: "task-role",
			Status: a2a.TaskStatus{
				State:     a2a.TaskStateWorking,
				Timestamp: &now,
				Message: &a2a.Message{
					Role:  role,
					Parts: a2a.ContentParts{&a2a.Part{Content: a2a.Text("test")}},
				},
			},
		}

		legacy := a2av0.FromV1TaskStatusUpdateEvent(v2Event)
		v2Back, err := a2av0.ToV1TaskStatusUpdateEvent(legacy)
		if err != nil {
			t.Fatalf("roundtrip error for role %q: %v", role, err)
		}
		if v2Back.Status.Message.Role != role {
			t.Errorf("role %q lost in roundtrip: got %q", role, v2Back.Status.Message.Role)
		}
	}

	t.Log("message roles map bidirectionally through compat layer")
}
