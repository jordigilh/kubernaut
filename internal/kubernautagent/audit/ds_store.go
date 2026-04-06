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

package audit

import (
	"context"
	"fmt"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// AuditCreator is the subset of the ogen client needed by DSAuditStore.
type AuditCreator interface {
	CreateAuditEvent(ctx context.Context, req *ogenclient.AuditEventRequest) (ogenclient.CreateAuditEventRes, error)
}

// DSAuditStore implements AuditStore by sending events to the DataStorage API.
type DSAuditStore struct {
	client AuditCreator
}

// NewDSAuditStore creates a DSAuditStore backed by the given ogen client.
func NewDSAuditStore(client AuditCreator) *DSAuditStore {
	return &DSAuditStore{client: client}
}

func (s *DSAuditStore) StoreAudit(ctx context.Context, event *AuditEvent) error {
	req := &ogenclient.AuditEventRequest{
		Version:        "1.0",
		EventType:      event.EventType,
		EventTimestamp: time.Now().UTC(),
		EventCategory:  ogenclient.AuditEventRequestEventCategory(event.EventCategory),
		EventAction:    event.EventAction,
		EventOutcome:   ogenclient.AuditEventRequestEventOutcome(event.EventOutcome),
		CorrelationID:  event.CorrelationID,
	}

	if ed, ok := buildEventData(event); ok {
		req.EventData = ed
	}

	if _, err := s.client.CreateAuditEvent(ctx, req); err != nil {
		return fmt.Errorf("audit store: %w", err)
	}
	return nil
}

func buildEventData(event *AuditEvent) (ogenclient.AuditEventRequestEventData, bool) {
	switch event.EventType {
	case EventTypeEnrichmentCompleted:
		payload := ogenclient.AIAgentEnrichmentCompletedPayload{
			EventType:  ogenclient.AIAgentEnrichmentCompletedPayloadEventTypeAiagentEnrichmentCompleted,
			EventID:    dataString(event.Data, "event_id"),
			IncidentID: dataString(event.Data, "incident_id"),
			RootOwnerKind:   dataString(event.Data, "root_owner_kind"),
			RootOwnerName:   dataString(event.Data, "root_owner_name"),
			OwnerChainLength: dataInt(event.Data, "owner_chain_length"),
			RemediationHistoryFetched: dataBool(event.Data, "remediation_history_fetched"),
		}
		if ns := dataString(event.Data, "root_owner_namespace"); ns != "" {
			payload.RootOwnerNamespace.SetTo(ns)
		}
		return ogenclient.NewAIAgentEnrichmentCompletedPayloadAuditEventRequestEventData(payload), true

	case EventTypeEnrichmentFailed:
		payload := ogenclient.AIAgentEnrichmentFailedPayload{
			EventType:  ogenclient.AIAgentEnrichmentFailedPayloadEventTypeAiagentEnrichmentFailed,
			EventID:    dataString(event.Data, "event_id"),
			IncidentID: dataString(event.Data, "incident_id"),
			Reason:     dataString(event.Data, "reason"),
			Detail:     dataString(event.Data, "detail"),
			AffectedResourceKind: dataString(event.Data, "affected_resource_kind"),
			AffectedResourceName: dataString(event.Data, "affected_resource_name"),
		}
		if ns := dataString(event.Data, "affected_resource_namespace"); ns != "" {
			payload.AffectedResourceNamespace.SetTo(ns)
		}
		return ogenclient.NewAIAgentEnrichmentFailedPayloadAuditEventRequestEventData(payload), true

	default:
		return ogenclient.AuditEventRequestEventData{}, false
	}
}

func dataString(d map[string]interface{}, key string) string {
	if v, ok := d[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func dataInt(d map[string]interface{}, key string) int {
	if v, ok := d[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}

func dataBool(d map[string]interface{}, key string) bool {
	if v, ok := d[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

var _ AuditStore = (*DSAuditStore)(nil)
