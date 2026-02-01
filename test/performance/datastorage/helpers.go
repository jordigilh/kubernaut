/*
Copyright 2025 Jordi Gil.

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

package datastorage

import (
	"context"
	"fmt"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// PERFORMANCE TEST HELPERS
// ========================================
// These helpers provide a clean interface for performance tests
// to use the typed OpenAPI client
// ========================================

// createOpenAPIClient creates a configured OpenAPI client for performance tests
func createOpenAPIClient(baseURL string) (*ogenclient.Client, error) {
	return ogenclient.NewClient(baseURL)
}

// createAuditEventRequest is a helper to build a typed AuditEventRequest for performance tests
func createAuditEventRequest(
	eventType string,
	eventCategory string,
	eventAction string,
	eventOutcome string,
	correlationID string,
	eventData map[string]interface{},
) ogenclient.AuditEventRequest {
	// Use timestamp 5 seconds in the past to avoid clock skew validation failures
	timestamp := time.Now().Add(-5 * time.Second).UTC()
	version := "1.0"

	// Convert string to typed enum (ADR-034 v1.2)
	category := ogenclient.AuditEventRequestEventCategory(eventCategory)
	outcome := ogenclient.AuditEventRequestEventOutcome(eventOutcome)

	// Create valid discriminated union based on event category
	var eventDataUnion ogenclient.AuditEventRequestEventData
	switch category {
	case ogenclient.AuditEventRequestEventCategoryGateway:
		eventDataUnion = ogenclient.AuditEventRequestEventData{
			Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
			GatewayAuditPayload: ogenclient.GatewayAuditPayload{
				EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
				SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
				AlertName:   "test-alert",
				Namespace:   "default",
				Fingerprint: "test-fingerprint",
			},
		}
	case ogenclient.AuditEventRequestEventCategoryAnalysis:
		eventDataUnion = ogenclient.AuditEventRequestEventData{
			Type: ogenclient.AuditEventRequestEventDataAianalysisAnalysisCompletedAuditEventRequestEventData,
			AIAnalysisAuditPayload: ogenclient.AIAnalysisAuditPayload{
				EventType:    ogenclient.AIAnalysisAuditPayloadEventTypeAianalysisAnalysisCompleted,
				AnalysisName: "test-analysis",
				Phase:        ogenclient.AIAnalysisAuditPayloadPhaseCompleted,
			},
		}
	case ogenclient.AuditEventRequestEventCategoryWorkflow:
		eventDataUnion = ogenclient.AuditEventRequestEventData{
			Type: ogenclient.AuditEventRequestEventDataWorkflowexecutionWorkflowStartedAuditEventRequestEventData,
			WorkflowExecutionAuditPayload: ogenclient.WorkflowExecutionAuditPayload{
				EventType:  ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionWorkflowStarted,
				WorkflowID: "test-workflow",
			},
		}
	default:
		// Fallback to Gateway event
		eventDataUnion = ogenclient.AuditEventRequestEventData{
			Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
			GatewayAuditPayload: ogenclient.GatewayAuditPayload{
				EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
				SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
				AlertName:   "test-alert",
				Namespace:   "default",
				Fingerprint: "test-fingerprint",
			},
		}
	}

	return ogenclient.AuditEventRequest{
		Version:        version,
		EventType:      eventType,
		EventTimestamp: timestamp,
		EventCategory:  category,
		EventAction:    eventAction,
		EventOutcome:   outcome,
		CorrelationID:  correlationID,
		EventData:      eventDataUnion,
	}
}

// postAuditEvent is a helper that sends an audit event using the OpenAPI client
// Returns the event ID and any error
func postAuditEvent(
	ctx context.Context,
	client *ogenclient.Client,
	event ogenclient.AuditEventRequest,
) (string, error) {
	resp, err := client.CreateAuditEvent(ctx, &event)
	if err != nil {
		return "", fmt.Errorf("failed to create audit event: %w", err)
	}

	// Extract event ID from response (ogen unions require type checking)
	switch r := resp.(type) {
	case *ogenclient.AuditEventResponse:
		// 201 Created - synchronous response with event ID
		return r.EventID.String(), nil
	case *ogenclient.AsyncAcceptanceResponse:
		// 202 Accepted - async response (no event ID yet)
		// Return status message as placeholder since ID not available yet
		return "", fmt.Errorf("async response (no event ID): %s", r.Message)
	default:
		return "", fmt.Errorf("unexpected response type: %T", resp)
	}
}
