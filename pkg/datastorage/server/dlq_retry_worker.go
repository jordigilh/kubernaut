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

package server

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// DLQ RETRY WORKER (DD-009 V1.0)
// 📋 Design Decision: DD-009 | BR-AUDIT-001 | ADR-032
// Authority: DD-009 (Audit Write Error Recovery - V1.0 Goroutine Approach)
// ========================================
//
// V1.0: Runs as a goroutine inside Data Storage server
// V1.1+: May be extracted to standalone binary if DLQ volume grows
//
// Business Requirements:
// - BR-AUDIT-001: Complete audit trail with no data loss
// - ADR-032: "No Audit Loss" mandate
//
// Retry Strategy (DD-009):
// - Exponential backoff: 1m, 5m, 15m, 1h, 4h, 24h
// - Max retries: 6
// - After max retries: Move to dead letter
//
// ========================================

// backoffIntervals defines the exponential backoff schedule per DD-009.
// After 6 retries, messages are moved to dead letter for manual investigation.
var backoffIntervals = []time.Duration{
	1 * time.Minute,  // Retry 0: Quick recovery for transient issues
	5 * time.Minute,  // Retry 1: Network partition recovery
	15 * time.Minute, // Retry 2: Pod restart recovery
	1 * time.Hour,    // Retry 3: Rolling upgrade recovery
	4 * time.Hour,    // Retry 4: Extended outage
	24 * time.Hour,   // Retry 5: Manual intervention required
}

// DLQRetryWorkerConfig holds configuration for the DLQ retry worker.
type DLQRetryWorkerConfig struct {
	PollInterval  time.Duration
	MaxBatchSize  int64
	MaxRetries    int
	ConsumerGroup string
	ConsumerName  string
}

// DefaultDLQRetryWorkerConfig returns sensible defaults per DD-009.
func DefaultDLQRetryWorkerConfig() DLQRetryWorkerConfig {
	return DLQRetryWorkerConfig{
		PollInterval:  30 * time.Second,
		MaxBatchSize:  10,
		MaxRetries:    6,
		ConsumerGroup: "data-storage-retry-workers",
		ConsumerName:  "worker-default",
	}
}

// DLQRetryWorker processes DLQ messages and retries writes to PostgreSQL.
// V1.0: Runs as goroutine inside Data Storage server (DD-007 lifecycle).
type DLQRetryWorker struct {
	dlqClient     *dlq.Client
	auditRepo     *repository.AuditEventsRepository
	logger        logr.Logger
	consumerGroup string
	consumerName  string

	// Configuration
	pollInterval     time.Duration
	maxBatchSize     int64
	maxRetriesPerMsg int

	// Lifecycle
	stopCh chan struct{}
	doneCh chan struct{}
}

// NewDLQRetryWorker creates a new DLQ retry worker.
func NewDLQRetryWorker(
	dlqClient *dlq.Client,
	auditRepo *repository.AuditEventsRepository,
	config DLQRetryWorkerConfig,
	logger logr.Logger,
) *DLQRetryWorker {
	return &DLQRetryWorker{
		dlqClient:        dlqClient,
		auditRepo:        auditRepo,
		logger:           logger.WithName("dlq-retry-worker"),
		consumerGroup:    config.ConsumerGroup,
		consumerName:     config.ConsumerName,
		pollInterval:     config.PollInterval,
		maxBatchSize:     config.MaxBatchSize,
		maxRetriesPerMsg: config.MaxRetries,
		stopCh:           make(chan struct{}),
		doneCh:           make(chan struct{}),
	}
}

// Start begins the retry loop in a background goroutine.
func (w *DLQRetryWorker) Start() {
	go w.retryLoop()
	w.logger.Info("DLQ retry worker started (DD-009 V1.0)",
		"poll_interval", w.pollInterval,
		"max_batch_size", w.maxBatchSize,
		"consumer_group", w.consumerGroup,
	)
}

// Stop gracefully stops the retry worker (DD-007 integration).
func (w *DLQRetryWorker) Stop() {
	close(w.stopCh)
	<-w.doneCh
	w.logger.Info("DLQ retry worker stopped")
}

func (w *DLQRetryWorker) retryLoop() {
	defer close(w.doneCh)

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.processRetryBatch()
		case <-w.stopCh:
			// Process any remaining messages before shutdown
			w.processRetryBatch()
			return
		}
	}
}

func (w *DLQRetryWorker) processRetryBatch() {
	ctx := context.Background()

	// Process both audit types
	auditTypes := []string{"events", "notifications"}

	for _, auditType := range auditTypes {
		messages, err := w.dlqClient.ReadMessages(ctx, auditType, w.consumerGroup, w.consumerName, 0)
		if err != nil {
			w.logger.Error(err, "Failed to read from DLQ", "audit_type", auditType)
			continue
		}

		for _, msg := range messages {
			w.processMessage(ctx, auditType, msg)
		}
	}
}

func (w *DLQRetryWorker) processMessage(ctx context.Context, auditType string, msg *dlq.DLQMessage) {
	// Check if message is ready for retry (backoff period elapsed)
	if !IsReadyForRetry(msg.AuditMessage.RetryCount, msg.AuditMessage.Timestamp) {
		w.logger.V(1).Info("Message not ready for retry (backoff not elapsed)",
			"message_id", msg.ID,
			"retry_count", msg.AuditMessage.RetryCount,
			"created_at", msg.AuditMessage.Timestamp,
		)
		return
	}

	// Check if max retries exceeded
	if msg.AuditMessage.RetryCount >= w.maxRetriesPerMsg {
		w.moveToDeadLetter(ctx, auditType, msg)
		return
	}

	// Attempt write to PostgreSQL
	err := w.writeToPostgres(ctx, auditType, msg)
	if err != nil {
		w.handleRetryFailure(ctx, auditType, msg, err)
		return
	}

	// Success - acknowledge message
	if err := w.dlqClient.AckMessage(ctx, auditType, w.consumerGroup, msg.ID); err != nil {
		w.logger.Error(err, "Failed to ack DLQ message after successful write",
			"message_id", msg.ID,
			"audit_type", auditType,
		)
	} else {
		w.logger.Info("DLQ message processed successfully",
			"message_id", msg.ID,
			"audit_type", auditType,
			"retry_count", msg.AuditMessage.RetryCount,
			"correlation_id", msg.AuditMessage.CorrelationID(),
		)
	}
}

func (w *DLQRetryWorker) writeToPostgres(ctx context.Context, auditType string, msg *dlq.DLQMessage) error {
	// Direct PostgreSQL write (bypass HTTP layer)
	switch auditType {
	case "events":
		// Parse JSON payload and create audit event
		event, err := w.parseAuditEventPayload(msg.AuditMessage.Payload)
		if err != nil {
			return err
		}
		_, err = w.auditRepo.Create(ctx, event)
		return err
	default:
		// For other types, log and skip
		w.logger.Info("Unsupported audit type for DLQ retry",
			"audit_type", auditType,
			"message_id", msg.ID,
		)
		return nil
	}
}

// parseAuditEventPayload parses JSON payload into AuditEvent.
func (w *DLQRetryWorker) parseAuditEventPayload(payload []byte) (*repository.AuditEvent, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, err
	}

	// Extract required fields
	event := &repository.AuditEvent{
		EventType:    getString(data, "event_type"),
		EventAction:  getString(data, "event_action"),
		EventOutcome: getString(data, "event_outcome"),
		ActorType:    getString(data, "actor_type"),
		ActorID:      getString(data, "actor_id"),
	}

	// Set defaults for missing fields
	if event.EventAction == "" {
		event.EventAction = getString(data, "operation")
	}
	if event.EventOutcome == "" {
		event.EventOutcome = getString(data, "outcome")
	}

	return event, nil
}

// getString safely extracts a string from map.
func getString(data map[string]interface{}, key string) string {
	if v, ok := data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (w *DLQRetryWorker) handleRetryFailure(ctx context.Context, auditType string, msg *dlq.DLQMessage, writeErr error) {
	w.logger.Error(writeErr, "DLQ retry failed",
		"message_id", msg.ID,
		"audit_type", auditType,
		"retry_count", msg.AuditMessage.RetryCount,
		"correlation_id", msg.AuditMessage.CorrelationID(),
	)

	// Increment retry count for next attempt (includes the error for tracking)
	if err := w.dlqClient.IncrementRetryCount(ctx, auditType, msg, writeErr); err != nil {
		w.logger.Error(err, "Failed to increment retry count", "message_id", msg.ID)
	}
}

func (w *DLQRetryWorker) moveToDeadLetter(ctx context.Context, auditType string, msg *dlq.DLQMessage) {
	if err := w.dlqClient.MoveToDeadLetter(ctx, auditType, msg); err != nil {
		w.logger.Error(err, "CRITICAL: Failed to move to dead letter (audit data loss)",
			"message_id", msg.ID,
			"audit_type", auditType,
			"correlation_id", msg.AuditMessage.CorrelationID(),
		)
	} else {
		// Ack original message after moving to dead letter
		// Intentionally ignore error - message already in dead letter, best effort cleanup
		_ = w.dlqClient.AckMessage(ctx, auditType, w.consumerGroup, msg.ID)
		w.logger.Info("Message moved to dead letter after max retries (ADR-032 violation)",
			"message_id", msg.ID,
			"audit_type", auditType,
			"retry_count", msg.AuditMessage.RetryCount,
			"correlation_id", msg.AuditMessage.CorrelationID(),
		)
	}
}

// ========================================
// Helper Functions (Exported for Testing)
// ========================================

// GetBackoffInterval returns the backoff interval for the given retry count.
// Per DD-009: 1m, 5m, 15m, 1h, 4h, 24h
func GetBackoffInterval(retryCount int) time.Duration {
	if retryCount < 0 {
		return backoffIntervals[0]
	}
	if retryCount >= len(backoffIntervals) {
		return backoffIntervals[len(backoffIntervals)-1]
	}
	return backoffIntervals[retryCount]
}

// IsReadyForRetry checks if a message is ready for retry based on backoff.
func IsReadyForRetry(retryCount int, createdAt time.Time) bool {
	backoff := GetBackoffInterval(retryCount)
	nextRetryTime := createdAt.Add(backoff)
	return time.Now().After(nextRetryTime)
}

// ParseTimestamp parses a Unix timestamp string to time.Time.
func ParseTimestamp(timestampStr string) time.Time {
	unix, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(unix, 0)
}

// ParseRetryCount parses a retry count string to int.
func ParseRetryCount(retryCountStr string) int {
	count, err := strconv.Atoi(retryCountStr)
	if err != nil {
		return 0
	}
	return count
}
