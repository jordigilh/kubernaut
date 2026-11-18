package e2e_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	investigationsessionv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const fleetAFEarlyRCASchema = "early_rca"

// These are wire IDs, not the JSON-RPC request/message ID or the synthetic
// InvestigationSession spec.a2aTaskID (a2a-<rr_id>) used by direct MCP.
type fleetAFInvocation struct {
	ContextID         string
	TaskID            string
	RRID              string
	PreviouslyStarted map[string]bool
}

type fleetAFStreamUpdate struct {
	Event a2aSSEEvent
	Err   error
}

// No Ginkgo assertions run in the reader goroutine. Cancellation unblocks both
// channel sends and HTTP reads (the owner also closes the body and joins done).
func readFleetAFLive(ctx context.Context, body io.Reader) (<-chan fleetAFStreamUpdate, <-chan struct{}) {
	updates := make(chan fleetAFStreamUpdate)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(updates)
		err := scanFleetAFEvents(body, func(event a2aSSEEvent) error {
			select {
			case updates <- fleetAFStreamUpdate{Event: event}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		if err != nil {
			select {
			case updates <- fleetAFStreamUpdate{Err: err}:
			case <-ctx.Done():
			}
		}
	}()
	return updates, done
}

func scanFleetAFEvents(body io.Reader, emit func(a2aSSEEvent) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimRight(scanner.Text(), "\r"))
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var envelope struct {
			Result a2aSSEEvent `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(data), &envelope); err != nil {
			return fmt.Errorf("parse Fleet AF SSE: %w", err)
		}
		if envelope.Error != nil {
			return fmt.Errorf("fleet af json-rpc error %d: %s", envelope.Error.Code, envelope.Error.Message)
		}
		if envelope.Result.Kind != "" {
			if err := emit(envelope.Result); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read Fleet AF SSE: %w", err)
	}
	return nil
}

func (binding *fleetAFInvocation) observe(event a2aSSEEvent) error {
	if event.Kind != statusUpdate && event.Kind != artifactUpdate {
		return nil
	}
	if event.ContextID != binding.ContextID || event.TaskID == "" {
		return fmt.Errorf("stream event does not belong to the current context/task")
	}
	if binding.TaskID != "" && event.TaskID != binding.TaskID {
		return fmt.Errorf("stream mixed task IDs")
	}
	binding.TaskID = event.TaskID
	if id, _ := event.Metadata["rr_id"].(string); id != "" {
		if binding.RRID != "" && binding.RRID != id {
			return fmt.Errorf("stream mixed RR bindings")
		}
		binding.RRID = id
	}
	if event.Status != nil {
		switch event.Status.State {
		case completed, failed, "canceled", "rejected":
			return fmt.Errorf("first stream terminated (%s) before the action=start barrier", event.Status.State)
		}
	}
	return nil
}

// The current stream supplies the exact RR; target matches and synthetic CRD
// task IDs are never ownership evidence. Pre-invocation correlations cannot
// establish a new action=start, even if resource dedup reuses an old RR.
func fleetAFStartedSession(binding fleetAFInvocation, namespace string, sessions []investigationsessionv1alpha1.InvestigationSession) bool {
	if binding.RRID == "" || binding.TaskID == "" || binding.PreviouslyStarted[namespace+"/"+binding.RRID] {
		return false
	}
	matches, started := 0, false
	for i := range sessions {
		session := &sessions[i]
		if session.Spec.RemediationRequestRef.Namespace == namespace && session.Spec.RemediationRequestRef.Name == binding.RRID {
			matches++
			started = session.Status.KACorrelationID != ""
		}
	}
	return matches == 1 && started
}

func waitFleetAFCurrentStart(ctx context.Context, binding fleetAFInvocation, updates <-chan fleetAFStreamUpdate, reader client.Reader, namespace string) (fleetAFInvocation, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return binding, fmt.Errorf("wait for first caller action=start: %w", ctx.Err())
		case update, open := <-updates:
			if !open {
				return binding, fmt.Errorf("first stream ended before the action=start barrier")
			}
			if update.Err != nil {
				return binding, update.Err
			}
			if err := binding.observe(update.Event); err != nil {
				return binding, err
			}
		case <-ticker.C:
		}
		if binding.RRID == "" {
			continue
		}
		if binding.PreviouslyStarted[namespace+"/"+binding.RRID] {
			return binding, fmt.Errorf("first stream reused a previously correlated RR; no new action=start proof")
		}
		var sessions investigationsessionv1alpha1.InvestigationSessionList
		if err := reader.List(ctx, &sessions, client.InNamespace(namespace)); err != nil {
			return binding, fmt.Errorf("list first caller sessions: %w", err)
		}
		if fleetAFStartedSession(binding, namespace, sessions.Items) {
			return binding, nil
		}
	}
}

func validateFleetAFLeaseGuidance(events []a2aSSEEvent, binding fleetAFInvocation) error {
	found := false
	var secondTaskID string
	for _, event := range events {
		for _, metadata := range []map[string]any{event.Metadata, fleetAFArtifactMetadata(event)} {
			if metadata["schema"] == fleetAFEarlyRCASchema || metadata["schema"] == investigationSummarySchema || metadata["type"] == "decision" {
				return fmt.Errorf("lease rejection emitted synthetic investigation findings")
			}
		}
		if event.Artifact != nil {
			for _, part := range event.Artifact.Parts {
				if err := fleetAFRejectFindingData(part); err != nil {
					return err
				}
			}
		}
		if event.Kind != statusUpdate {
			continue
		}
		if event.ContextID != binding.ContextID || event.TaskID == "" || event.TaskID == binding.TaskID {
			return fmt.Errorf("lease guidance is not bound to a separate caller context/task")
		}
		if secondTaskID != "" && event.TaskID != secondTaskID {
			return fmt.Errorf("lease guidance mixed second-caller task IDs")
		}
		secondTaskID = event.TaskID
		if id, _ := event.Metadata["rr_id"].(string); id != "" && id != binding.RRID {
			return fmt.Errorf("lease guidance references another RR")
		}
		if event.Status != nil && (event.Status.State == failed || event.Status.State == "canceled" || event.Status.State == "rejected") {
			return fmt.Errorf("lease guidance stream failed: %s", event.Status.State)
		}
		if event.Metadata["type"] != statusMetadataType || event.Status == nil || event.Status.Message == nil {
			continue
		}
		for _, part := range event.Status.Message.Parts {
			if err := fleetAFRejectFindingData(part); err != nil {
				return err
			}
			if strings.Contains(part.Text, "already in progress") && strings.Contains(part.Text, "Do not retry kubernaut_investigate") &&
				strings.Contains(part.Text, "rr_id "+binding.RRID+" to check its status") && event.Metadata["rr_id"] == binding.RRID {
				found = true
			}
		}
	}
	if !found {
		return fmt.Errorf("current existing-RR caller did not receive session_active status guidance")
	}
	return nil
}

func fleetAFRejectFindingData(part a2aSSEPart) error {
	if len(part.Data) == 0 {
		return nil
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(part.Data, &data); err != nil {
		return fmt.Errorf("invalid lease guidance data: %w", err)
	}
	for _, field := range []string{"rca", "root_cause_analysis", "summary", "confidence", "causal_chain"} {
		if _, found := data[field]; found {
			return fmt.Errorf("lease rejection emitted synthetic %s", field)
		}
	}
	return nil
}

func fleetAFArtifactMetadata(event a2aSSEEvent) map[string]any {
	if event.Artifact == nil {
		return nil
	}
	return event.Artifact.Metadata
}

// A2A wire shapes shared by live and completed-stream contract checks.
type a2aSSEEvent struct {
	Kind      string          `json:"kind"`
	TaskID    string          `json:"taskId,omitempty"`
	ContextID string          `json:"contextId,omitempty"`
	Metadata  map[string]any  `json:"metadata,omitempty"`
	Status    *a2aSSEStatus   `json:"status,omitempty"`
	Artifact  *a2aSSEArtifact `json:"artifact,omitempty"`
}

type a2aSSEStatus struct {
	State   string         `json:"state,omitempty"`
	Message *a2aSSEMessage `json:"message,omitempty"`
}

type a2aSSEMessage struct {
	Parts []a2aSSEPart `json:"parts,omitempty"`
}

type a2aSSEArtifact struct {
	Metadata map[string]any `json:"metadata,omitempty"`
	Parts    []a2aSSEPart   `json:"parts,omitempty"`
}

type a2aSSEPart struct {
	Data json.RawMessage `json:"data,omitempty"`
	Text string          `json:"text,omitempty"`
}
