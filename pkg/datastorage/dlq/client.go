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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/helpers"
)

// ========================================
// DLQ CLIENT (TDD GREEN Phase + V1.0 REFACTOR)
// 📋 Tests Define Contract: client_test.go
// Authority: DD-009 - Audit Write Error Recovery
// ========================================
//
// This file implements Dead Letter Queue (DLQ) for audit write failures.
//
// TDD DRIVEN DESIGN:
// - Tests written FIRST (client_test.go - 8 tests)
// - Production code implements MINIMAL functionality to pass tests
// - Contract defined by test expectations
//
// V1.0 REFACTOR:
// - Metrics extracted to metrics.go
// - Monitoring extracted to monitoring.go
// - Core client operations remain here
//
// Business Requirements:
// - BR-AUDIT-001: Complete audit trail with no data loss
// - DD-009: Dead Letter Queue pattern for error recovery
//
// ========================================

// EventCreator abstracts audit event persistence for dependency injection.
// Defined in the dlq package to avoid duplicate declarations (ARCH-M2).
type EventCreator interface {
	Create(context.Context, *repository.AuditEvent) (*repository.AuditEvent, error)
}

// NotificationCreator abstracts notification persistence for dependency injection.
// Defined in the dlq package to avoid duplicate declarations (ARCH-M2).
type NotificationCreator interface {
	Create(context.Context, *models.NotificationAudit) (*models.NotificationAudit, error)
}

// maxPayloadSize caps the DLQ message payload that will be unmarshalled,
// preventing unbounded memory allocation from oversized messages (SI-10).
const maxPayloadSize = 1 << 20 // 1 MiB

// MaxPayloadSize returns the maximum allowed DLQ payload size in bytes.
func MaxPayloadSize() int { return maxPayloadSize }

// Client provides Dead Letter Queue functionality using Redis Streams.
type Client struct {
	redisClient *redis.Client
	logger      logr.Logger
	maxLen      int64 // Maximum DLQ stream length (for capacity monitoring - Gap 3.3)

	// xaddCounter is optional (#1048 Phase 5 / AU-11); nil if not wired by server wiring.
	xaddCounter *prometheus.CounterVec
}

// AuditMessage represents a message in the DLQ.
type AuditMessage struct {
	Type       string          `json:"type"`        // e.g., "notification_audit", "audit_event"
	Payload    json.RawMessage `json:"payload"`     // Serialized audit record
	Timestamp  time.Time       `json:"timestamp"`   // When message was added to DLQ
	RetryCount int             `json:"retry_count"` // Number of retry attempts
	LastError  string          `json:"last_error"`  // Error that caused DLQ write
}

// CorrelationID extracts correlation_id from the payload for logging/debugging.
func (m *AuditMessage) CorrelationID() string {
	var payload struct {
		CorrelationID string `json:"correlation_id"`
	}
	if err := json.Unmarshal(m.Payload, &payload); err != nil {
		return ""
	}
	return payload.CorrelationID
}

// DLQMessage represents a message read from the DLQ stream.
// GAP-8: Used by consumer methods (ReadMessages, AckMessage, MoveToDeadLetter)
type DLQMessage struct {
	ID           string       // Redis Stream message ID (e.g., "1234567890123-0")
	AuditMessage AuditMessage // Parsed audit message
	RawValues    map[string]interface{}
}

// NewClient creates a new DLQ client.
// maxLen parameter enables capacity monitoring (Gap 3.3: DLQ Near-Capacity Warning)
func NewClient(redisClient *redis.Client, logger logr.Logger, maxLen int64) (*Client, error) {
	if redisClient == nil {
		return nil, fmt.Errorf("redis client cannot be nil")
	}
	if maxLen <= 0 {
		maxLen = 10000 // Default max length if not specified
	}
	return &Client{
		redisClient: redisClient,
		logger:      logger,
		maxLen:      maxLen,
	}, nil
}

// SetXAddCounter wires the Prometheus counter for successful XADD calls (optional).
func (c *Client) SetXAddCounter(counter *prometheus.CounterVec) {
	c.xaddCounter = counter
}

// xaddStreamMetricLabel maps Redis stream keys to bounded Prometheus label values (#1048 / AU-11).
func xaddStreamMetricLabel(streamKey string) string {
	switch streamKey {
	case "audit:dlq:notifications":
		return "notifications"
	case "audit:dlq:events":
		return "audit_events"
	case "audit:dead-letter:notifications":
		return "dead_letter_notifications"
	case "audit:dead-letter:events":
		return "dead_letter_audit_events"
	default:
		return "unknown"
	}
}

func (c *Client) observeXAddSuccess(streamKey string) {
	if c.xaddCounter == nil {
		return
	}
	c.xaddCounter.WithLabelValues(xaddStreamMetricLabel(streamKey)).Inc()
}

// EnqueueNotificationAudit adds a NotificationAudit record to the DLQ.
// This is called when the primary write to PostgreSQL fails.
func (c *Client) EnqueueNotificationAudit(ctx context.Context, audit *models.NotificationAudit, originalError error) error {
	// Serialize audit payload
	payloadJSON, err := json.Marshal(audit)
	if err != nil {
		return fmt.Errorf("failed to marshal audit payload: %w", err)
	}

	// Create DLQ message
	auditMsg := AuditMessage{
		Type:       "notification_audit",
		Payload:    payloadJSON,
		Timestamp:  time.Now(),
		RetryCount: 0,
		LastError:  originalError.Error(),
	}

	// Serialize message
	messageJSON, err := json.Marshal(auditMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal DLQ message: %w", err)
	}

	// Add to Redis Stream
	streamKey := "audit:dlq:notifications"
	_, err = c.redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		MaxLen: c.maxLen, // Use configured max length (Gap 3.3)
		Approx: true,     // ~ trimming: Redis best practice for stream performance
		ID:     "*",      // Auto-generate timestamp-based ID
		Values: map[string]interface{}{
			"message": string(messageJSON),
		},
	}).Result()

	if err != nil {
		return fmt.Errorf("failed to enqueue to DLQ: %w", err)
	}
	c.observeXAddSuccess(streamKey)

	// Gap 3.3 REFACTOR: Monitor DLQ capacity using extracted monitoring function
	c.monitorDLQCapacity(ctx, "notifications", streamKey, "notification_audit")

	// Log success
	c.logNotificationEnqueueSuccess(c.logger, audit.NotificationID, originalError.Error())

	return nil
}

// EnqueueAuditEvent adds a generic AuditEvent record to the DLQ.
// This is called when the primary write to PostgreSQL fails for unified audit events.
func (c *Client) EnqueueAuditEvent(ctx context.Context, audit *audit.AuditEvent, originalError error) error {
	// Serialize audit payload
	payloadJSON, err := json.Marshal(audit)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event payload: %w", err)
	}

	// Create DLQ message
	auditMsg := AuditMessage{
		Type:       "audit_event",
		Payload:    payloadJSON,
		Timestamp:  time.Now(),
		RetryCount: 0,
		LastError:  originalError.Error(),
	}

	// Serialize message
	messageJSON, err := json.Marshal(auditMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal DLQ message: %w", err)
	}

	// Add to Redis Stream
	streamKey := "audit:dlq:events" // Unique stream key for generic audit events
	_, err = c.redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		MaxLen: c.maxLen, // Use configured max length (Gap 3.3)
		Approx: true,     // ~ trimming: Redis best practice for stream performance
		ID:     "*",      // Auto-generate timestamp-based ID
		Values: map[string]interface{}{
			"message": string(messageJSON),
		},
	}).Result()

	if err != nil {
		return fmt.Errorf("failed to add audit event to DLQ: %w", err)
	}
	c.observeXAddSuccess(streamKey)

	// Gap 3.3 REFACTOR: Monitor DLQ capacity using extracted monitoring function
	c.monitorDLQCapacity(ctx, "events", streamKey, "audit_event")

	// Log success
	c.logEnqueueSuccess(c.logger, "audit_event", audit.EventID.String(), originalError.Error())

	return nil
}

// GetDLQDepth returns the number of messages in the DLQ for a given audit type.
func (c *Client) GetDLQDepth(ctx context.Context, auditType string) (int64, error) {
	streamKey := fmt.Sprintf("audit:dlq:%s", auditType)

	length, err := c.redisClient.XLen(ctx, streamKey).Result()
	if err != nil {
		// If stream doesn't exist, Redis returns 0 (not an error)
		// But if Redis is down, we get an error
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get DLQ depth: %w", err)
	}

	return length, nil
}

// HealthCheck verifies Redis connectivity.
func (c *Client) HealthCheck(ctx context.Context) error {
	if err := c.redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	return nil
}

// ============================================================================
// GAP-8: DLQ Consumer Methods (DD-009)
// Authority: DD-009 (Audit Write Error Recovery - Dead Letter Queue Pattern)
// Business Requirement: BR-AUDIT-001 (Complete audit trail with no data loss)
// ============================================================================

// ReadMessages reads messages from the DLQ using Redis consumer groups.
//
// This method uses XREADGROUP for at-least-once delivery semantics.
// Messages are claimed by this consumer and must be acknowledged with AckMessage.
//
// Parameters:
//   - auditType: Type of audit messages to read ("events" or "notifications")
//   - consumerGroup: Consumer group name (e.g., "audit-retry-workers")
//   - consumerName: Consumer instance name (e.g., "worker-1")
//   - count: Maximum number of messages to return per call
//   - timeout: How long to block waiting for messages
func (c *Client) ReadMessages(ctx context.Context, auditType, consumerGroup, consumerName string, count int64, timeout time.Duration) ([]*DLQMessage, error) {
	streamKey := fmt.Sprintf("audit:dlq:%s", auditType)

	// Create consumer group if it doesn't exist
	// MKSTREAM creates the stream if it doesn't exist
	err := c.redisClient.XGroupCreateMkStream(ctx, streamKey, consumerGroup, "0").Err()
	if err != nil && !isConsumerGroupExistsError(err) {
		return nil, fmt.Errorf("failed to create consumer group: %w", err)
	}

	if count <= 0 {
		count = 10
	}

	// Read messages using XREADGROUP
	// ">" means read only new messages not yet delivered to any consumer
	streams, err := c.redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    consumerGroup,
		Consumer: consumerName,
		Streams:  []string{streamKey, ">"},
		Count:    count,
		Block:    timeout,
	}).Result()

	if err != nil {
		if errors.Is(err, redis.Nil) {
			// No messages available (timeout)
			return []*DLQMessage{}, nil
		}
		// Handle NOGROUP error (stream doesn't exist or consumer group creation race condition)
		// This can happen when stream was recently deleted or in parallel test execution
		if isNoGroupError(err) {
			// Stream doesn't exist yet - return empty slice (no messages)
			return []*DLQMessage{}, nil
		}
		return nil, fmt.Errorf("failed to read from DLQ: %w", err)
	}

	// Parse messages
	var messages []*DLQMessage
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			dlqMsg, err := c.parseStreamMessage(msg)
			if err != nil {
				c.logger.Error(err, "Failed to parse DLQ message", "id", msg.ID)
				continue
			}
			messages = append(messages, dlqMsg)
		}
	}

	if len(messages) > 0 {
		c.logger.Info("Read messages from DLQ",
			"count", len(messages),
			"audit_type", auditType,
			"consumer_group", consumerGroup,
		)
	}

	return messages, nil
}

// AutoClaimMessages reclaims messages that have been idle for longer than minIdleTime.
// #1048 Phase 5 / AU-2: PEL recovery for stuck unprocessable messages.
// Returns claimed messages and the next cursor ID for incremental XAUTOCLAIM sweeps (Redis next-start).
func (c *Client) AutoClaimMessages(ctx context.Context, auditType, consumerGroup, consumer string,
	minIdleTime time.Duration, startID string, count int64) ([]DLQMessage, string, error) {
	streamKey := fmt.Sprintf("audit:dlq:%s", auditType)

	err := c.redisClient.XGroupCreateMkStream(ctx, streamKey, consumerGroup, "0").Err()
	if err != nil && !isConsumerGroupExistsError(err) {
		return nil, "", fmt.Errorf("failed to ensure consumer group for XAUTOCLAIM: %w", err)
	}

	if count <= 0 {
		count = 10
	}

	redisMsgs, nextStart, err := c.redisClient.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   streamKey,
		Group:    consumerGroup,
		Consumer: consumer,
		MinIdle:  minIdleTime,
		Start:    startID,
		Count:    count,
	}).Result()
	if err != nil {
		return nil, "", fmt.Errorf("XAUTOCLAIM failed for stream %s: %w", streamKey, err)
	}

	var messages []DLQMessage
	for _, msg := range redisMsgs {
		dlqMsg, parseErr := c.parseStreamMessage(msg)
		if parseErr != nil {
			c.logger.Error(parseErr, "Invalid message format in PEL claim",
				"stream", streamKey,
				"message_id", msg.ID)
			continue
		}
		messages = append(messages, *dlqMsg)
	}

	return messages, nextStart, nil
}

// ReadPendingMessages reads messages from this consumer's pending entries list (PEL).
// Used for two-phase startup: drain pending before reading new messages.
// Pass startID "0" (or "0-0") to read from the beginning of the consumer's pending backlog.
//
// Blocking: Uses non-blocking XREADGROUP (same pattern as ReadMessages with negative Block).
func (c *Client) ReadPendingMessages(ctx context.Context, auditType, consumerGroup, consumer string,
	count int64, startID string) ([]DLQMessage, error) {
	streamKey := fmt.Sprintf("audit:dlq:%s", auditType)

	err := c.redisClient.XGroupCreateMkStream(ctx, streamKey, consumerGroup, "0").Err()
	if err != nil && !isConsumerGroupExistsError(err) {
		return nil, fmt.Errorf("failed to create consumer group: %w", err)
	}

	if count <= 0 {
		count = 10
	}

	if startID == "" {
		startID = "0"
	}

	// Block must be negative so go-redis omits BLOCK and the call returns immediately
	// (see redis.XReadGroup: negative Block skips the block argument).
	const noBlock time.Duration = -1

	streams, err := c.redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    consumerGroup,
		Consumer: consumer,
		Streams:  []string{streamKey, startID},
		Count:    count,
		Block:    noBlock,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		if isNoGroupError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read pending messages: %w", err)
	}

	var messages []DLQMessage
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			dlqMsg, parseErr := c.parseStreamMessage(msg)
			if parseErr != nil {
				c.logger.Error(parseErr, "Failed to unmarshal pending message",
					"stream", streamKey,
					"message_id", msg.ID)
				continue
			}
			messages = append(messages, *dlqMsg)
		}
	}

	return messages, nil
}

// AckMessage acknowledges a successfully processed message.
//
// After acknowledgment, the message is removed from the pending entries list
// and won't be re-delivered to this consumer group.
func (c *Client) AckMessage(ctx context.Context, auditType, consumerGroup, messageID string) error {
	streamKey := fmt.Sprintf("audit:dlq:%s", auditType)

	acknowledged, err := c.redisClient.XAck(ctx, streamKey, consumerGroup, messageID).Result()
	if err != nil {
		return fmt.Errorf("failed to acknowledge message: %w", err)
	}

	if acknowledged == 0 {
		c.logger.Info("Message already acknowledged or not found",
			"message_id", messageID,
			"audit_type", auditType,
		)
	}

	return nil
}

// MoveToDeadLetter moves a permanently failed message to the dead letter stream.
//
// This is called after a message has exceeded max retries (e.g., 6 retries per DD-009).
// The message is moved to "audit:dead-letter:{auditType}" for manual investigation
// and removed from the main DLQ stream.
func (c *Client) MoveToDeadLetter(ctx context.Context, auditType string, msg *DLQMessage) error {
	sourceStreamKey := fmt.Sprintf("audit:dlq:%s", auditType)
	deadLetterKey := fmt.Sprintf("audit:dead-letter:%s", auditType)

	// Re-serialize the message with updated metadata
	messageJSON, err := json.Marshal(msg.AuditMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal message for dead letter: %w", err)
	}

	// Add to dead letter stream
	_, err = c.redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: deadLetterKey,
		MaxLen: c.maxLen,
		Approx: true,
		ID:     "*",
		Values: map[string]interface{}{
			"message":           string(messageJSON),
			"original_id":       msg.ID,
			"moved_at":          time.Now().Format(time.RFC3339),
			"final_retry_count": msg.AuditMessage.RetryCount,
		},
	}).Result()
	if err != nil {
		return fmt.Errorf("failed to write to dead letter: %w", err)
	}
	c.observeXAddSuccess(deadLetterKey)

	// Remove from original DLQ stream
	_, err = c.redisClient.XDel(ctx, sourceStreamKey, msg.ID).Result()
	if err != nil {
		c.logger.Error(err, "Failed to remove message from DLQ after dead letter move",
			"message_id", msg.ID,
		)
		// Don't return error - message is safely in dead letter
	}

	c.logger.Info("Message moved to dead letter",
		"message_id", msg.ID,
		"audit_type", auditType,
		"retry_count", msg.AuditMessage.RetryCount,
	)

	return nil
}

// IncrementRetryCount updates the retry count for a message that failed to process.
//
// This re-adds the message to the DLQ with an incremented retry count,
// so it can be picked up by the next retry cycle.
func (c *Client) IncrementRetryCount(ctx context.Context, auditType string, msg *DLQMessage, retryError error) error {
	streamKey := fmt.Sprintf("audit:dlq:%s", auditType)

	// Update the message
	msg.AuditMessage.RetryCount++
	msg.AuditMessage.LastError = retryError.Error()
	msg.AuditMessage.Timestamp = time.Now() // Update timestamp for backoff calculation

	// Re-serialize
	messageJSON, err := json.Marshal(msg.AuditMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal updated message: %w", err)
	}

	// Add updated message to stream (new ID)
	_, err = c.redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		MaxLen: c.maxLen,
		Approx: true,
		ID:     "*",
		Values: map[string]interface{}{
			"message": string(messageJSON),
		},
	}).Result()
	if err != nil {
		return fmt.Errorf("failed to re-add message with incremented retry count: %w", err)
	}
	c.observeXAddSuccess(streamKey)

	// Remove old message
	_, err = c.redisClient.XDel(ctx, streamKey, msg.ID).Result()
	if err != nil {
		c.logger.Error(err, "Failed to remove old message after retry increment",
			"message_id", msg.ID,
		)
	}

	c.logger.Info("Incremented retry count for message",
		"message_id", msg.ID,
		"new_retry_count", msg.AuditMessage.RetryCount,
		"audit_type", auditType,
	)

	return nil
}

// GetPendingMessages returns the count of unacknowledged messages for a consumer group.
func (c *Client) GetPendingMessages(ctx context.Context, auditType, consumerGroup string) (int64, error) {
	streamKey := fmt.Sprintf("audit:dlq:%s", auditType)

	pending, err := c.redisClient.XPending(ctx, streamKey, consumerGroup).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) || isNoSuchKeyError(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get pending count: %w", err)
	}

	return pending.Count, nil
}

// parseStreamMessage parses a Redis stream message into a DLQMessage.
func (c *Client) parseStreamMessage(msg redis.XMessage) (*DLQMessage, error) {
	messageStr, ok := msg.Values["message"].(string)
	if !ok {
		return nil, fmt.Errorf("message field not found or not a string")
	}

	var auditMsg AuditMessage
	if err := json.Unmarshal([]byte(messageStr), &auditMsg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal audit message: %w", err)
	}

	return &DLQMessage{
		ID:           msg.ID,
		AuditMessage: auditMsg,
		RawValues:    msg.Values,
	}, nil
}

// isConsumerGroupExistsError checks if the error indicates the consumer group already exists.
func isConsumerGroupExistsError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "BUSYGROUP")
}

// isNoSuchKeyError checks if the error indicates the stream or consumer group doesn't exist.
func isNoSuchKeyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "NOGROUP") || strings.Contains(errStr, "no such key")
}

// isNoGroupError checks if the error indicates the stream or consumer group doesn't exist during XREADGROUP.
// This can happen when:
// - Stream was recently deleted (e.g., in test cleanup)
// - Consumer group creation race condition
// - Parallel test execution with shared Redis instance
func isNoGroupError(err error) bool {
	if err == nil {
		return false
	}
	// Redis returns NOGROUP errors in various forms
	errStr := err.Error()
	return strings.Contains(errStr, "NOGROUP") &&
		(strings.Contains(errStr, "No such key") || strings.Contains(errStr, "consumer group"))
}

// ========================================
// DD-008: DLQ DRAIN DURING GRACEFUL SHUTDOWN
// ========================================

// DrainStats tracks statistics for DLQ drain operation
type DrainStats struct {
	NotificationsProcessed int
	EventsProcessed        int
	TotalProcessed         int
	Errors                 []error
	Duration               time.Duration
	TimedOut               bool
}

// DrainWithTimeout attempts to process all pending DLQ messages within the given timeout.
// This is called during graceful shutdown (DD-007 + DD-008) to ensure DLQ messages are
// not lost when the service shuts down.
//
// DD-008: Graceful Shutdown Sequence
// 1. Complete in-flight audit traces (DD-007 Step 3)
// 2. Drain DLQ with timeout (DD-008 - THIS METHOD)
// 3. Close resources (DD-007 Step 4)
//
// The method processes messages from both notification and event DLQ streams until:
// - All messages are processed, OR
// - The context timeout is reached, OR
// - An unrecoverable error occurs
//
// Parameters:
// - ctx: Context with timeout (typically 10s during shutdown)
// - repository: Repository to write DLQ messages to database
//
// Returns:
// - DrainStats: Statistics about the drain operation
// - error: Only returns error for critical failures (Redis unavailable, etc.)
//
// Business Requirement: BR-AUDIT-001 - Complete audit trail with no data loss
func (c *Client) DrainWithTimeout(ctx context.Context, notificationRepo NotificationCreator, eventsRepo EventCreator) (*DrainStats, error) {
	startTime := time.Now()
	stats := &DrainStats{}

	deadline, hasDeadline := ctx.Deadline()
	timeoutInfo := "no timeout"
	if hasDeadline {
		timeoutInfo = time.Until(deadline).String()
	}

	c.logger.Info("Starting DLQ drain for graceful shutdown",
		"timeout", timeoutInfo,
		"dd", "DD-008-drain-start")

	// Process notification DLQ stream
	notificationStats, err := c.drainStream(ctx, "notifications", notificationRepo)
	if err != nil {
		stats.Errors = append(stats.Errors, fmt.Errorf("notification drain error: %w", err))
	}
	stats.NotificationsProcessed = notificationStats

	// Check if we still have time for events
	select {
	case <-ctx.Done():
		stats.TimedOut = true
		stats.Duration = time.Since(startTime)
		c.logger.Info("DLQ drain timed out after processing notifications",
			"notifications_processed", stats.NotificationsProcessed,
			"duration", stats.Duration,
			"dd", "DD-008-drain-timeout")
		return stats, nil
	default:
		// Continue to events
	}

	// Process events DLQ stream
	eventsStats, err := c.drainStream(ctx, "events", eventsRepo)
	if err != nil {
		stats.Errors = append(stats.Errors, fmt.Errorf("events drain error: %w", err))
	}
	stats.EventsProcessed = eventsStats

	stats.TotalProcessed = stats.NotificationsProcessed + stats.EventsProcessed
	stats.Duration = time.Since(startTime)

	// Check if we timed out
	select {
	case <-ctx.Done():
		stats.TimedOut = true
	default:
		// Completed within timeout
	}

	c.logger.Info("DLQ drain complete",
		"notifications_processed", stats.NotificationsProcessed,
		"events_processed", stats.EventsProcessed,
		"total_processed", stats.TotalProcessed,
		"duration", stats.Duration,
		"timed_out", stats.TimedOut,
		"errors", len(stats.Errors),
		"dd", "DD-008-drain-complete")

	return stats, nil
}

// drainBatchSize limits the number of messages read per XRangeN call during
// drain to bound memory usage (PERF-3).
const drainBatchSize int64 = 100

// drainStream processes all messages from a specific DLQ stream using
// cursor-based iteration with a bounded retry pass.
//
// #1048 DF-3: Only XDel after confirmed DB write success. Failed messages
// remain in the stream for the next startup's retry worker to pick up.
//
// #1048 ARCH-F1: Two-pass drain ensures that messages skipped by the forward
// cursor after a write failure get a second chance within the same shutdown
// window. Pass 1 sweeps forward; if any writes fail, pass 2 restarts from "-"
// to retry the survivors (XDel'd messages are gone, so only failures remain).
// Bounded at 2 passes to prevent infinite loops on permanent failures.
//
// #1048 PERF-3: Uses XRangeN with cursor to avoid loading the entire stream
// into memory at once.
func (c *Client) drainStream(ctx context.Context, auditType string, repo interface{}) (int, error) {
	streamKey := c.getStreamKey(auditType)
	total := 0

	n, hadFailures, err := c.drainStreamPass(ctx, streamKey, auditType, repo)
	total += n
	if err != nil {
		return total, err
	}

	// Retry pass: if any writes failed, sweep from "-" once more.
	// Successfully XDel'd messages won't reappear. Messages that fail
	// a second time remain in Redis for the next startup's retry worker.
	if hadFailures {
		n, _, err = c.drainStreamPass(ctx, streamKey, auditType, repo)
		total += n
		if err != nil {
			return total, err
		}
	}

	return total, nil
}

// drainStreamPass performs a single forward sweep of the stream.
// Returns the number of messages successfully written+deleted, whether any
// write failures occurred, and any stream-level read error.
func (c *Client) drainStreamPass(ctx context.Context, streamKey, auditType string, repo interface{}) (int, bool, error) {
	processed := 0
	hadFailures := false
	cursor := "-"

	for {
		select {
		case <-ctx.Done():
			return processed, hadFailures, nil
		default:
		}

		messages, err := c.redisClient.XRangeN(ctx, streamKey, cursor, "+", drainBatchSize).Result()
		if err != nil {
			return processed, hadFailures, fmt.Errorf("failed to read stream: %w", err)
		}

		if len(messages) == 0 {
			break
		}

		for _, msg := range messages {
			select {
			case <-ctx.Done():
				return processed, hadFailures, nil
			default:
			}

			// Advance cursor past this message to prevent re-reading on the
			// next outer-loop iteration. Failed messages are retried in pass 2.
			cursor = "(" + msg.ID

			dlqMsg, err := c.parseStreamMessage(msg)
			if err != nil {
				c.logger.Error(err, "Failed to parse DLQ message during drain",
					"message_id", msg.ID,
					"audit_type", auditType)
				hadFailures = true
				continue
			}

			if err := c.writeMessageToDB(ctx, auditType, dlqMsg, repo); err != nil {
				c.logger.Error(err, "Failed to write DLQ message to database during drain",
					"message_id", msg.ID,
					"audit_type", auditType,
					"correlation_id", dlqMsg.AuditMessage.CorrelationID())
				hadFailures = true
				continue
			}

			if err := c.redisClient.XDel(ctx, streamKey, msg.ID).Err(); err != nil {
				c.logger.Error(err, "Failed to delete DLQ message during drain",
					"message_id", msg.ID,
					"audit_type", auditType)
			}

			processed++
		}
	}

	return processed, hadFailures, nil
}

// writeMessageToDB writes a DLQ message to the database.
// Uses the package-level EventCreator/NotificationCreator interfaces (ARCH-M2).
// SI-10: Rejects payloads exceeding maxPayloadSize before unmarshalling.
func (c *Client) writeMessageToDB(ctx context.Context, auditType string, msg *DLQMessage, repo interface{}) error {
	if repo == nil {
		return fmt.Errorf("repository is nil for audit type: %s", auditType)
	}

	if len(msg.AuditMessage.Payload) > maxPayloadSize {
		return fmt.Errorf("payload exceeds maximum size (%d > %d bytes)", len(msg.AuditMessage.Payload), maxPayloadSize)
	}

	switch auditType {
	case "notifications":
		notifRepo, ok := repo.(NotificationCreator)
		if !ok {
			return fmt.Errorf("repository does not implement NotificationCreator interface (type: %T)", repo)
		}
		var notifAudit models.NotificationAudit
		if err := json.Unmarshal(msg.AuditMessage.Payload, &notifAudit); err != nil {
			return fmt.Errorf("failed to unmarshal notification audit: %w", err)
		}
		_, err := notifRepo.Create(ctx, &notifAudit)
		return err

	case "events":
		eventsRepo, ok := repo.(EventCreator)
		if !ok {
			return fmt.Errorf("repository does not implement EventCreator interface (type: %T)", repo)
		}
		var auditEvent audit.AuditEvent
		if err := json.Unmarshal(msg.AuditMessage.Payload, &auditEvent); err != nil {
			return fmt.Errorf("failed to unmarshal audit event: %w", err)
		}
		if len(auditEvent.EventData) == 0 {
			auditEvent.EventData = []byte("{}")
		}
		if err := auditEvent.Validate(); err != nil {
			return fmt.Errorf("drain: audit event validation failed: %w", err)
		}
		if err := ValidateEventData(auditEvent.EventData); err != nil {
			return fmt.Errorf("drain: audit event EventData validation failed: %w", err)
		}
		repoEvent, err := helpers.ConvertToRepositoryAuditEvent(&auditEvent)
		if err != nil {
			return fmt.Errorf("failed to convert audit event: %w", err)
		}
		_, err = eventsRepo.Create(ctx, repoEvent)
		return err

	default:
		return fmt.Errorf("unknown audit type: %s", auditType)
	}
}

// getStreamKey returns the Redis stream key for a given audit type
func (c *Client) getStreamKey(auditType string) string {
	return fmt.Sprintf("audit:dlq:%s", auditType)
}
