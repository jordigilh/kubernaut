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

package delivery

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// ========================================
// LOG DELIVERY SERVICE (MVP)
// 📋 TDD GREEN Phase: Minimal Implementation
// ========================================
//
// LogDeliveryService outputs notifications as structured JSON to stdout for observability.
//
// PURPOSE: Structured Logging for Observability
// - Outputs notifications as JSON Lines (one JSON object per line)
// - Enables log aggregation systems (Loki, Elasticsearch, etc.) to parse and index
// - Provides searchable notification history without file storage
//
// Business Requirements:
// - BR-NOT-053: At-Least-Once Delivery (log always succeeds to stdout)
// - BR-NOT-034: Audit Trail (structured logs provide audit history)
//
// ========================================

// LogDeliveryService delivers notifications as structured JSON logs to stdout
type LogDeliveryService struct{}

// Ensure LogDeliveryService implements Service interface at compile-time
var _ Service = (*LogDeliveryService)(nil)

// NewLogDeliveryService creates a new log-based delivery service
func NewLogDeliveryService() *LogDeliveryService {
	return &LogDeliveryService{}
}

// Deliver outputs the notification as structured JSON to stdout
//
// TDD GREEN: Minimal implementation
// - Outputs notification as JSON to stdout via logger
// - Always succeeds (stdout write failures are rare)
// - Structured format enables log aggregation and searching
//
// Output Format (JSON Lines):
//
//	{
//	  "timestamp": "2025-12-22T12:34:56Z",
//	  "notification_name": "critical-alert",
//	  "notification_namespace": "default",
//	  "type": "escalation",
//	  "priority": "critical",
//	  "subject": "Critical System Failure",
//	  "body": "System XYZ has failed",
//	  "metadata": {"cluster": "prod", "severity": "critical"}
//	}
//
// Implements: DeliveryService interface
// BR-NOT-053: At-least-once delivery (stdout always succeeds)
func (s *LogDeliveryService) Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	log := ctrl.LoggerFrom(ctx)

	// TDD GREEN: Validate input (nil-check)
	if notification == nil {
		return fmt.Errorf("notification cannot be nil")
	}

	// TDD REFACTOR: Build enriched structured log entry
	logEntry := map[string]interface{}{
		// Core notification identity
		"timestamp":              time.Now().Format(time.RFC3339),
		"notification_name":      notification.Name,
		"notification_namespace": notification.Namespace,
		"notification_uid":       string(notification.UID),

		// Notification content
		"type":     string(notification.Spec.Type),
		"priority": string(notification.Spec.Priority),
		"subject":  notification.Spec.Subject,
		"body":     notification.Spec.Body,

		// Delivery configuration
		"channels": notification.Spec.Channels,
	}

	// Add metadata if present
	if len(notification.Spec.Metadata) > 0 {
		logEntry["metadata"] = notification.Spec.Metadata
	}

	// TDD REFACTOR: Add Kubernetes labels and annotations for searchability
	if len(notification.Labels) > 0 {
		logEntry["labels"] = notification.Labels
	}
	if len(notification.Annotations) > 0 {
		logEntry["annotations"] = notification.Annotations
	}

	// TDD REFACTOR: Add status information for observability
	if notification.Status.Phase != "" {
		logEntry["phase"] = string(notification.Status.Phase)
	}
	if notification.Status.SuccessfulDeliveries > 0 {
		logEntry["successful_deliveries"] = notification.Status.SuccessfulDeliveries
	}
	if notification.Status.FailedDeliveries > 0 {
		logEntry["failed_deliveries"] = notification.Status.FailedDeliveries
	}

	// Add action links if present (for correlation with external systems)
	if len(notification.Spec.ActionLinks) > 0 {
		links := make([]map[string]string, len(notification.Spec.ActionLinks))
		for i, link := range notification.Spec.ActionLinks {
			links[i] = map[string]string{
				"service": link.Service,
				"url":     link.URL,
				"label":   link.Label,
			}
		}
		logEntry["action_links"] = links
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(logEntry)
	if err != nil {
		log.Error(err, "Failed to marshal notification to JSON for log delivery",
			"notification", notification.Name,
			"namespace", notification.Namespace)
		return fmt.Errorf("failed to marshal notification to JSON: %w", err)
	}

	// Output as structured log (JSON Lines format)
	// This goes to stdout and can be captured by log aggregation systems
	log.Info(string(jsonBytes))

	return nil
}

