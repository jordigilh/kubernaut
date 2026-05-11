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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/helpers"
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
	dlqClient        *dlq.Client
	auditRepo        dlq.EventCreator
	notificationRepo dlq.NotificationCreator
	logger           logr.Logger
	consumerGroup    string
	consumerName     string

	// Configuration
	pollInterval     time.Duration
	maxBatchSize     int64
	maxRetriesPerMsg int

	// #1048 Phase 4: Prometheus counter for DLQ validation failures (nil-safe)
	validationMetrics *prometheus.CounterVec

	// Lifecycle: cancel func interrupts blocking Redis reads on Stop()
	cancel context.CancelFunc
	doneCh chan struct{}
}

// NewDLQRetryWorker creates a new DLQ retry worker.
// #1048 DF-1: Added notificationRepo parameter so notification DLQ messages
// are persisted instead of silently dropped.
func NewDLQRetryWorker(
	dlqClient *dlq.Client,
	auditRepo dlq.EventCreator,
	notificationRepo dlq.NotificationCreator,
	config DLQRetryWorkerConfig,
	logger logr.Logger,
	validationMetrics *prometheus.CounterVec,
) *DLQRetryWorker {
	return &DLQRetryWorker{
		dlqClient:         dlqClient,
		auditRepo:         auditRepo,
		notificationRepo:  notificationRepo,
		logger:            logger.WithName("dlq-retry-worker"),
		consumerGroup:     config.ConsumerGroup,
		consumerName:      config.ConsumerName,
		pollInterval:      config.PollInterval,
		maxBatchSize:      config.MaxBatchSize,
		maxRetriesPerMsg:  config.MaxRetries,
		validationMetrics: validationMetrics,
		doneCh:            make(chan struct{}),
	}
}

// Start begins the retry loop in a background goroutine.
// Issue #667/M4: Accepts a parent context so the worker lifecycle is
// bound to the caller (typically the server) rather than using an
// unbounded context.Background().
func (w *DLQRetryWorker) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	go w.retryLoop(ctx)
	w.logger.Info("DLQ retry worker started (DD-009 V1.0)",
		"poll_interval", w.pollInterval,
		"max_batch_size", w.maxBatchSize,
		"consumer_group", w.consumerGroup,
	)
}

// Stop gracefully stops the retry worker (DD-007 integration).
// Safe to call even if Start() was never called (e.g. tests that use Handler() only).
func (w *DLQRetryWorker) Stop() {
	if w.cancel == nil {
		return // Start() was never called; nothing to stop
	}
	w.cancel()
	<-w.doneCh
	w.logger.Info("DLQ retry worker stopped")
}

func (w *DLQRetryWorker) retryLoop(ctx context.Context) {
	defer close(w.doneCh)

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.processRetryBatch(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (w *DLQRetryWorker) processRetryBatch(ctx context.Context) {

	// Process both audit types
	auditTypes := []string{"events", "notifications"}

	for _, auditType := range auditTypes {
		messages, err := w.dlqClient.ReadMessages(ctx, auditType, w.consumerGroup, w.consumerName, w.maxBatchSize, -1)
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

// writeToPostgres persists a DLQ message to PostgreSQL.
// #1048 DF-1: Added "notifications" case (was silently returning nil).
// #1048 DF-2: Replaced manual getString parsing with json.Unmarshal +
// ConvertToRepositoryAuditEvent, aligning with the drain path in dlq/client.go.
func (w *DLQRetryWorker) writeToPostgres(ctx context.Context, auditType string, msg *dlq.DLQMessage) error {
	if len(msg.AuditMessage.Payload) > dlq.MaxPayloadSize() {
		return fmt.Errorf("payload exceeds maximum size (%d > %d bytes)", len(msg.AuditMessage.Payload), dlq.MaxPayloadSize())
	}

	switch auditType {
	case "events":
		if w.auditRepo == nil {
			return fmt.Errorf("audit event repository not configured; cannot persist event DLQ message")
		}
		var auditEvent audit.AuditEvent
		if err := json.Unmarshal(msg.AuditMessage.Payload, &auditEvent); err != nil {
			w.incValidationMetric(auditType, "unmarshal_error")
			return fmt.Errorf("failed to unmarshal audit event payload: %w", err)
		}
		if len(auditEvent.EventData) == 0 {
			auditEvent.EventData = []byte("{}")
		}
		if err := auditEvent.Validate(); err != nil {
			w.incValidationMetric(auditType, "field_validation")
			return fmt.Errorf("audit event validation failed: %w", err)
		}
		if err := validateEventData(auditEvent.EventData); err != nil {
			w.incValidationMetric(auditType, "size_or_depth")
			return fmt.Errorf("audit event EventData validation failed: %w", err)
		}
		repoEvent, err := helpers.ConvertToRepositoryAuditEvent(&auditEvent)
		if err != nil {
			return fmt.Errorf("failed to convert audit event: %w", err)
		}
		_, err = w.auditRepo.Create(ctx, repoEvent)
		return err

	case "notifications":
		if w.notificationRepo == nil {
			return fmt.Errorf("notification repository not configured; cannot persist notification DLQ message")
		}
		var notifAudit models.NotificationAudit
		if err := json.Unmarshal(msg.AuditMessage.Payload, &notifAudit); err != nil {
			w.incValidationMetric(auditType, "unmarshal_error")
			return fmt.Errorf("failed to unmarshal notification audit payload: %w", err)
		}
		if err := notifAudit.Validate(); err != nil {
			w.incValidationMetric(auditType, "field_validation")
			return fmt.Errorf("notification audit validation failed: %w", err)
		}
		_, err := w.notificationRepo.Create(ctx, &notifAudit)
		return err

	default:
		return fmt.Errorf("unknown audit type for DLQ retry: %s", auditType)
	}
}

func (w *DLQRetryWorker) handleRetryFailure(ctx context.Context, auditType string, msg *dlq.DLQMessage, writeErr error) {
	w.logger.Error(writeErr, "DLQ retry failed",
		"message_id", msg.ID,
		"audit_type", auditType,
		"retry_count", msg.AuditMessage.RetryCount,
		"correlation_id", msg.AuditMessage.CorrelationID(),
		"event_type", extractPayloadField(msg.AuditMessage.Payload, "event_type"),
		"payload_size", len(msg.AuditMessage.Payload),
	)

	if err := w.dlqClient.IncrementRetryCount(ctx, auditType, msg, writeErr); err != nil {
		w.logger.Error(err, "Failed to increment retry count", "message_id", msg.ID)
	}
}

// extractPayloadField best-effort extracts a top-level string field from raw JSON payload.
func extractPayloadField(payload json.RawMessage, field string) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(payload, &m); err != nil {
		return ""
	}
	raw, ok := m[field]
	if !ok {
		return ""
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	return v
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

func (w *DLQRetryWorker) incValidationMetric(auditType, reason string) {
	if w.validationMetrics != nil {
		w.validationMetrics.WithLabelValues(auditType, reason).Inc()
	}
}

// ========================================
// EventData Validation (Fix 7 / SC-5, SI-10)
// ========================================

const (
	maxEventDataSize  = 256 * 1024 // 256 KB — consistent with gateway MaxRequestBodySize
	maxEventDataDepth = 10         // prevent billion-laughs / recursive JSON attacks
)

// validateEventData checks EventData size and JSON nesting depth.
// Uses a streaming json.Decoder (token-by-token) to count depth without
// loading the full structure into memory.
func validateEventData(data []byte) error {
	if len(data) > maxEventDataSize {
		return fmt.Errorf("EventData exceeds maximum size (%d > %d bytes)", len(data), maxEventDataSize)
	}
	if len(data) == 0 {
		return nil
	}
	return validateJSONDepth(data, maxEventDataDepth)
}

// validateJSONDepth walks JSON tokens and returns an error if nesting exceeds maxDepth.
func validateJSONDepth(data []byte, maxDepth int) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var depth int
	for {
		t, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("invalid JSON in EventData: %w", err)
		}
		switch t {
		case json.Delim('{'), json.Delim('['):
			depth++
			if depth > maxDepth {
				return fmt.Errorf("EventData JSON nesting depth exceeds maximum (%d)", maxDepth)
			}
		case json.Delim('}'), json.Delim(']'):
			depth--
		}
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
