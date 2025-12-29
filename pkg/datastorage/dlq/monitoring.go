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

package dlq

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
)

// ========================================
// DLQ CAPACITY MONITORING (Gap 3.3 REFACTOR)
// 📋 Authority: DD-009 (DLQ Fallback), ADR-038 (Async Buffered Audit)
// ========================================
//
// Capacity monitoring logic with threshold-based alerting.
//
// REFACTOR extraction:
// - Centralized monitoring logic (was duplicated in client.go)
// - Prometheus metric export for capacity monitoring
// - Three-tier warning system: 80% (warning), 90% (critical), 95% (imminent overflow)
// - Per-stream granularity (notifications, events)
//
// Business Value:
// - Early detection of DLQ capacity issues
// - Graduated alert levels for operational response
// - Prevents silent audit data loss
// ========================================

// monitorDLQCapacity checks DLQ depth and exports Prometheus metrics + log warnings.
// This is called after each enqueue operation to provide real-time capacity monitoring.
//
// Parameters:
//   - ctx: Context for Redis operations
//   - client: DLQ client for accessing Redis and logger
//   - stream: Stream name (e.g., "notifications", "events") for metric labeling
//   - streamKey: Full Redis stream key (e.g., "audit:dlq:notifications") for logging
//   - messageType: Message type (e.g., "notification_audit", "audit_event") for metrics
//
// Capacity Thresholds:
//   - 80%: INFO log + warning metric (monitoring recommended)
//   - 90%: ERROR log + critical metric (urgent action needed)
//   - 95%: ERROR log + imminent overflow metric (immediate action required)
func (c *Client) monitorDLQCapacity(ctx context.Context, stream, streamKey, messageType string) {
	// Get current DLQ depth
	depth, err := c.GetDLQDepth(ctx, stream)
	if err != nil || c.maxLen <= 0 {
		// Skip monitoring if depth check fails or maxLen not configured
		return
	}

	// Calculate capacity ratio (0.0 to 1.0)
	capacityRatio := float64(depth) / float64(c.maxLen)

	// Export Prometheus metrics (Gap 3.3 REFACTOR enhancement)
	dlqCapacityRatio.WithLabelValues(stream).Set(capacityRatio)
	dlqDepth.WithLabelValues(stream).Set(float64(depth))
	dlqEnqueueTotal.WithLabelValues(stream, messageType).Inc()

	// Reset all alert gauges first, then set active ones
	// This ensures only the highest active threshold is set
	dlqWarning.WithLabelValues(stream).Set(0)
	dlqCritical.WithLabelValues(stream).Set(0)
	dlqOverflowImminent.WithLabelValues(stream).Set(0)

	// Log warnings at increasing severity levels + set alert metrics
	// Three-tier warning system based on capacity ratio
	if capacityRatio >= 0.95 {
		// IMMINENT OVERFLOW (95%+): Immediate action required
		dlqOverflowImminent.WithLabelValues(stream).Set(1)
		c.logger.Error(nil, "DLQ OVERFLOW IMMINENT - immediate action required",
			"depth", depth,
			"max", c.maxLen,
			"ratio", fmt.Sprintf("%.2f%%", capacityRatio*100),
			"stream", streamKey)
	} else if capacityRatio >= 0.90 {
		// CRITICAL (90%+): Urgent action needed
		dlqCritical.WithLabelValues(stream).Set(1)
		c.logger.Error(nil, "DLQ CRITICAL capacity - urgent action needed",
			"depth", depth,
			"max", c.maxLen,
			"ratio", fmt.Sprintf("%.2f%%", capacityRatio*100),
			"stream", streamKey)
	} else if capacityRatio >= 0.80 {
		// WARNING (80%+): Monitoring recommended
		dlqWarning.WithLabelValues(stream).Set(1)
		c.logger.Info("DLQ approaching capacity - monitoring recommended",
			"depth", depth,
			"max", c.maxLen,
			"ratio", fmt.Sprintf("%.2f%%", capacityRatio*100),
			"stream", streamKey)
	}
}

// logEnqueueSuccess logs successful DLQ enqueue operations.
// This provides audit trail for DLQ usage and helps with debugging.
func (c *Client) logEnqueueSuccess(logger logr.Logger, messageType, identifier, errorMsg string) {
	logger.Info("Audit event added to DLQ",
		"type", messageType,
		"event_id", identifier,
		"correlation_id", identifier, // For backwards compatibility
		"error", errorMsg,
	)
}

// logNotificationEnqueueSuccess logs successful notification audit DLQ enqueue.
func (c *Client) logNotificationEnqueueSuccess(logger logr.Logger, notificationID, errorMsg string) {
	logger.Info("Audit record added to DLQ",
		"type", "notification_audit",
		"notification_id", notificationID,
		"error", errorMsg,
	)
}
