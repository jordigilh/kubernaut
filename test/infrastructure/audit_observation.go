package infrastructure

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// AuditObservation is test-only attribution, not a guessed request/session correlation.
type AuditObservation struct {
	Actor    string
	Tool     string
	Since    time.Time
	Baseline map[uuid.UUID]struct{}
}

func QueryAuditSnapshot(ctx context.Context, client *ogenclient.Client, params ogenclient.QueryAuditEventsParams) ([]ogenclient.AuditEvent, error) {
	limit := params.Limit.Value
	if !params.Limit.IsSet() {
		limit = 1000
	}
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("invalid audit snapshot page size")
	}
	params.Limit = ogenclient.NewOptInt(limit)
	if !params.Until.IsSet() {
		// Freeze each traversal. The API serializer drops fractional seconds;
		// ceil the upper bound rather than exclude a newly emitted same-second row.
		params.Until = ogenclient.NewOptString(time.Now().UTC().Truncate(time.Second).Add(time.Second).Format(time.RFC3339))
	}
	offset := params.Offset.Value
	events := make([]ogenclient.AuditEvent, 0, limit)
	seen := make(map[uuid.UUID]struct{}, limit)
	for pageIndex := 0; pageIndex < 10; pageIndex++ {
		params.Offset = ogenclient.NewOptInt(offset)
		page, err := client.QueryAuditEvents(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("audit snapshot offset %d: %w", offset, err)
		}
		if page == nil {
			return nil, fmt.Errorf("audit snapshot offset %d returned no structured page", offset)
		}
		for _, event := range page.Data {
			if !event.EventID.IsSet() || event.EventID.Value == uuid.Nil {
				return nil, fmt.Errorf("audit snapshot row missing persisted event uuid")
			}
			if _, exists := seen[event.EventID.Value]; exists {
				continue
			}
			seen[event.EventID.Value] = struct{}{}
			events = append(events, event)
		}
		offset += len(page.Data)
		if len(page.Data) < limit {
			return events, nil
		}
	}
	return nil, fmt.Errorf("audit snapshot exceeded ten-page page budget; filters=%+v", params)
}

func FindNewToolAudit(events []ogenclient.AuditEvent, observation AuditObservation) (ogenclient.AuditEvent, bool) {
	for _, event := range events {
		if !event.EventID.IsSet() || event.EventID.Value == uuid.Nil {
			continue
		}
		if _, existed := observation.Baseline[event.EventID.Value]; existed {
			continue
		}
		if event.EventType != "apifrontend.tool.executed" || event.EventAction != "executed" || event.EventOutcome != "success" {
			continue
		}
		if event.ActorID.Value != observation.Actor || event.EventTimestamp.Before(observation.Since.UTC().Truncate(time.Second)) {
			continue
		}
		payload, ok := event.EventData.GetApifrontendToolExecutedPayload()
		if ok && payload.ToolName == observation.Tool && payload.ToolOutcome == "success" && event.CorrelationID != "" {
			return event, true
		}
	}
	return ogenclient.AuditEvent{}, false
}
