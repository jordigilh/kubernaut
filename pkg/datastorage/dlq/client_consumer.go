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

	"github.com/redis/go-redis/v9"
)

// ============================================================================
// GAP-8: DLQ Consumer Methods (DD-009)
// Authority: DD-009 (Audit Write Error Recovery - Dead Letter Queue Pattern)
// Business Requirement: BR-AUDIT-001 (Complete audit trail with no data loss)
// ============================================================================
// Split from client.go (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3, pure code
// motion, no behavior change). See client.go for the Client struct and
// producer-side Enqueue* methods, and client_drain.go for the DD-008
// graceful-shutdown drain path.

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
	msg.AuditMessage.LastError = SanitizeError(retryError)
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
