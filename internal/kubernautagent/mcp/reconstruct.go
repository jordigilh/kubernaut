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
	"log/slog"
	"sort"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// AuditQuerier is the subset of the ogenclient used for context reconstruction.
// Mirrors the pattern from pkg/effectivenessmonitor/client/ds_querier.go.
type AuditQuerier interface {
	QueryAuditEvents(ctx context.Context, params ogenclient.QueryAuditEventsParams) (*ogenclient.AuditEventsQueryResponse, error)
}

// DSContextReconstructor implements ContextReconstructor by querying DS audit
// events. BR-INTERACTIVE-007: rebuild conversation from prior sessions.
// BR-INTERACTIVE-008: DS unavailable = empty slice (best-effort).
type DSContextReconstructor struct {
	querier AuditQuerier
	timeout time.Duration
	logger  *slog.Logger
}

// NewDSContextReconstructor creates a reconstructor backed by the DS audit API.
func NewDSContextReconstructor(querier AuditQuerier, timeout time.Duration, logger *slog.Logger) *DSContextReconstructor {
	return &DSContextReconstructor{
		querier: querier,
		timeout: timeout,
		logger:  logger,
	}
}

const reconstructPageSize = 200

func (r *DSContextReconstructor) Reconstruct(ctx context.Context, correlationID string, excludeActorID string) ([]ConversationTurn, error) {
	queryCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	params := ogenclient.QueryAuditEventsParams{
		CorrelationID: ogenclient.OptString{Value: correlationID, Set: true},
		EventCategory: ogenclient.OptString{Value: "aiagent", Set: true},
		Limit:         ogenclient.OptInt{Value: reconstructPageSize, Set: true},
	}

	resp, err := r.querier.QueryAuditEvents(queryCtx, params)
	if err != nil {
		r.logger.Warn("DS audit query failed, returning empty context (best-effort)",
			slog.String("correlation_id", correlationID),
			slog.String("error", err.Error()),
		)
		return []ConversationTurn{}, nil
	}

	if resp == nil || len(resp.Data) == 0 {
		return []ConversationTurn{}, nil
	}

	var turns []ConversationTurn
	for _, evt := range resp.Data {
		role := eventTypeToRole(evt.EventType)
		if role == "" {
			continue
		}

		if excludeActorID != "" && evt.ActorID.IsSet() && evt.ActorID.Value == excludeActorID {
			continue
		}

		content := extractContent(evt)
		actingUser := ""
		if evt.ActorID.IsSet() {
			actingUser = evt.ActorID.Value
		}

		turns = append(turns, ConversationTurn{
			ActingUser: actingUser,
			Role:       role,
			Content:    content,
			Timestamp:  evt.EventTimestamp,
		})
	}

	sort.Slice(turns, func(i, j int) bool {
		return turns[i].Timestamp.Before(turns[j].Timestamp)
	})

	return turns, nil
}

func eventTypeToRole(eventType string) string {
	switch eventType {
	case "aiagent.llm.request":
		return "user"
	case "aiagent.llm.response":
		return "assistant"
	default:
		return ""
	}
}

func extractContent(evt ogenclient.AuditEvent) string {
	if evt.EventData.IsLLMRequestPayload() {
		payload, ok := evt.EventData.GetLLMRequestPayload()
		if ok {
			return payload.PromptPreview
		}
	}
	if evt.EventData.IsLLMResponsePayload() {
		payload, ok := evt.EventData.GetLLMResponsePayload()
		if ok {
			if payload.AnalysisFull.IsSet() {
				return payload.AnalysisFull.Value
			}
			return payload.AnalysisPreview
		}
	}
	return ""
}

var _ ContextReconstructor = (*DSContextReconstructor)(nil)
