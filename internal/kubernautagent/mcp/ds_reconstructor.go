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
	"cmp"
	"context"
	"log/slog"
	"slices"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// AuditQuerier abstracts the DS client method used by DSContextReconstructor.
type AuditQuerier interface {
	QueryAuditEvents(ctx context.Context, params ogenclient.QueryAuditEventsParams) (*ogenclient.AuditEventsQueryResponse, error)
}

// DSContextReconstructor implements ContextReconstructor by querying DS audit
// events and mapping LLM request/response pairs to ConversationTurns.
// BR-INTERACTIVE-008: returns empty slice on DS failure (best-effort).
type DSContextReconstructor struct {
	querier AuditQuerier
	timeout time.Duration
	logger  *slog.Logger
}

// NewDSContextReconstructor creates a new reconstructor backed by the given DS querier.
func NewDSContextReconstructor(querier AuditQuerier, timeout time.Duration, logger *slog.Logger) *DSContextReconstructor {
	return &DSContextReconstructor{
		querier: querier,
		timeout: timeout,
		logger:  logger,
	}
}

// Reconstruct queries DS for LLM request/response events matching correlationID,
// optionally excludes events from a specific actor, and returns turns ordered by
// timestamp. Returns empty slice (not error) if DS is unavailable.
func (r *DSContextReconstructor) Reconstruct(ctx context.Context, correlationID string, excludeActorID string) ([]ConversationTurn, error) {
	queryCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	params := ogenclient.QueryAuditEventsParams{
		CorrelationID: ogenclient.OptString{Value: correlationID, Set: true},
		Limit:         ogenclient.OptInt{Value: 1000, Set: true},
	}

	resp, err := r.querier.QueryAuditEvents(queryCtx, params)
	if err != nil {
		r.logger.Warn("DS query failed during reconstruction; continuing with empty context",
			slog.String("correlation_id", correlationID),
			slog.String("error", err.Error()))
		return nil, nil
	}

	if resp == nil || len(resp.Data) == 0 {
		return nil, nil
	}

	turns := make([]ConversationTurn, 0, len(resp.Data))
	for _, evt := range resp.Data {
		if excludeActorID != "" {
			if actorID, ok := evt.ActorID.Get(); ok && actorID == excludeActorID {
				continue
			}
		}

		turn, ok := eventToTurn(evt)
		if !ok {
			continue
		}
		turns = append(turns, turn)
	}

	slices.SortStableFunc(turns, func(a, b ConversationTurn) int {
		return cmp.Compare(a.Timestamp.UnixNano(), b.Timestamp.UnixNano())
	})

	return turns, nil
}

func eventToTurn(evt ogenclient.AuditEvent) (ConversationTurn, bool) {
	actorID, _ := evt.ActorID.Get()

	switch {
	case evt.EventData.IsLLMRequestPayload():
		payload, _ := evt.EventData.GetLLMRequestPayload()
		return ConversationTurn{
			ActingUser: actorID,
			Role:       "user",
			Content:    payload.PromptPreview,
			Timestamp:  evt.EventTimestamp,
		}, true

	case evt.EventData.IsLLMResponsePayload():
		payload, _ := evt.EventData.GetLLMResponsePayload()
		return ConversationTurn{
			ActingUser: actorID,
			Role:       "assistant",
			Content:    payload.AnalysisPreview,
			Timestamp:  evt.EventTimestamp,
		}, true

	default:
		return ConversationTurn{}, false
	}
}
