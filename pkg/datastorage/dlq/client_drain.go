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
	"time"

	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/redis/go-redis/v9"
)

// ========================================
// DD-008: DLQ DRAIN DURING GRACEFUL SHUTDOWN
// ========================================
// Split from client.go (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3, pure code
// motion, no behavior change). See client.go for the Client struct and
// producer-side Enqueue* methods, and client_consumer.go for the GAP-8
// consumer-group read/ack/claim methods.

// DrainStats tracks statistics for DLQ drain operation
type DrainStats struct {
	NotificationsProcessed int
	EventsProcessed        int
	TotalProcessed         int
	Errors                 []error
	Duration               time.Duration
	TimedOut               bool
}

// JoinErrors returns a single error combining all drain errors, or nil if none.
// SRE-M2: Used by shutdown to propagate drain failures into shutdownErrors.
func (s *DrainStats) JoinErrors() error {
	if len(s.Errors) == 0 {
		return nil
	}
	return errors.Join(s.Errors...)
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
	if notificationRepo == nil || eventsRepo == nil {
		return &DrainStats{Errors: []error{fmt.Errorf("DrainWithTimeout: repos must not be nil (DLQ is a hard dependency)")}},
			fmt.Errorf("DrainWithTimeout: repos must not be nil (DLQ is a hard dependency)")
	}
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

	// Process notification DLQ stream; returns early (timed-out result) if the
	// context expired before events could be attempted.
	if timedOut := c.drainNotificationsWithDeadlineCheck(ctx, notificationRepo, stats, startTime); timedOut {
		return stats, stats.JoinErrors()
	}

	// Process events DLQ stream
	eventsStats, err := c.drainStream(ctx, "events", c.eventWriter(eventsRepo))
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

	// DF-H1: Return joined errors so shutdown can surface drain failures
	return stats, stats.JoinErrors()
}

// drainNotificationsWithDeadlineCheck drains the notification DLQ stream into
// stats, then checks whether ctx has already expired. Returns true if the
// caller should return immediately (deadline exceeded before events could be
// attempted) — mirroring the original inline `select`/return-early shape.
// Extracted from DrainWithTimeout (Wave 6 6f GREEN: funlen remediation) —
// pure code motion, no behavior change.
func (c *Client) drainNotificationsWithDeadlineCheck(ctx context.Context, notificationRepo NotificationCreator, stats *DrainStats, startTime time.Time) bool {
	notificationStats, err := c.drainStream(ctx, "notifications", c.notificationWriter(notificationRepo))
	if err != nil {
		stats.Errors = append(stats.Errors, fmt.Errorf("notification drain error: %w", err))
	}
	stats.NotificationsProcessed = notificationStats

	select {
	case <-ctx.Done():
		stats.TimedOut = true
		stats.Duration = time.Since(startTime)
		c.logger.Info("DLQ drain timed out after processing notifications",
			"notifications_processed", stats.NotificationsProcessed,
			"duration", stats.Duration,
			"errors", len(stats.Errors),
			"dd", "DD-008-drain-timeout")
		return true
	default:
		return false
	}
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
// messageWriteFunc writes a single DLQ message to persistent storage.
// ARCH-M1: Replaces the former interface{} + type-assertion pattern.
type messageWriteFunc func(ctx context.Context, msg *DLQMessage) error

// notificationWriter returns a messageWriteFunc that persists notifications.
func (c *Client) notificationWriter(repo NotificationCreator) messageWriteFunc {
	return func(ctx context.Context, msg *DLQMessage) error {
		if len(msg.AuditMessage.Payload) > maxPayloadSize {
			return fmt.Errorf("payload exceeds maximum size (%d > %d bytes)", len(msg.AuditMessage.Payload), maxPayloadSize)
		}
		var notifAudit models.NotificationAudit
		if err := json.Unmarshal(msg.AuditMessage.Payload, &notifAudit); err != nil {
			return fmt.Errorf("failed to unmarshal notification audit: %w", err)
		}
		_, err := repo.Create(ctx, &notifAudit)
		return err
	}
}

// eventWriter returns a messageWriteFunc that persists audit events.
// Mirrors the original writeMessageToDB "events" path: unmarshal → validate → convert → create.
func (c *Client) eventWriter(repo EventCreator) messageWriteFunc {
	return func(ctx context.Context, msg *DLQMessage) error {
		if len(msg.AuditMessage.Payload) > maxPayloadSize {
			return fmt.Errorf("payload exceeds maximum size (%d > %d bytes)", len(msg.AuditMessage.Payload), maxPayloadSize)
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
		repoEvent, err := repository.ConvertFromAuditEvent(&auditEvent)
		if err != nil {
			return fmt.Errorf("failed to convert audit event: %w", err)
		}
		_, err = repo.Create(ctx, repoEvent)
		return err
	}
}

// #1048 PERF-3: Uses XRangeN with cursor to avoid loading the entire stream
// into memory at once.
func (c *Client) drainStream(ctx context.Context, auditType string, writeFn messageWriteFunc) (int, error) {
	streamKey := c.getStreamKey(auditType)
	total := 0

	n, hadFailures, err := c.drainStreamPass(ctx, streamKey, auditType, writeFn)
	total += n
	if err != nil {
		return total, err
	}

	// Retry pass: if any writes failed, sweep from "-" once more.
	// Successfully XDel'd messages won't reappear. Messages that fail
	// a second time remain in Redis for the next startup's retry worker.
	if hadFailures {
		n, stillFailing, err := c.drainStreamPass(ctx, streamKey, auditType, writeFn)
		total += n
		if err != nil {
			return total, err
		}
		// DF-H1: If messages still fail after second pass, report the failure
		if stillFailing {
			return total, fmt.Errorf("drain %s: messages remain after 2 passes (repo may be unavailable)", auditType)
		}
	}

	return total, nil
}

// drainStreamPass performs a single forward sweep of the stream.
// Returns the number of messages successfully written+deleted, whether any
// write failures occurred, and any stream-level read error.
func (c *Client) drainStreamPass(ctx context.Context, streamKey, auditType string, writeFn messageWriteFunc) (int, bool, error) {
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

			if c.drainOneMessage(ctx, streamKey, auditType, msg, writeFn) {
				processed++
			} else {
				hadFailures = true
			}
		}
	}

	return processed, hadFailures, nil
}

// drainOneMessage parses, writes, and deletes a single DLQ stream message.
// Returns true when the message was successfully written and processed
// should count toward the drain total; false when parsing or writing failed
// (the message is left in Redis for a subsequent drain pass).
func (c *Client) drainOneMessage(ctx context.Context, streamKey, auditType string, msg redis.XMessage, writeFn messageWriteFunc) bool {
	dlqMsg, err := c.parseStreamMessage(msg)
	if err != nil {
		c.logger.Error(err, "Failed to parse DLQ message during drain",
			"message_id", msg.ID,
			"audit_type", auditType)
		return false
	}

	if err := writeFn(ctx, dlqMsg); err != nil {
		c.logger.Error(err, "Failed to write DLQ message to database during drain",
			"message_id", msg.ID,
			"audit_type", auditType,
			"correlation_id", dlqMsg.AuditMessage.CorrelationID())
		return false
	}

	if err := c.redisClient.XDel(ctx, streamKey, msg.ID).Err(); err != nil {
		c.logger.Error(err, "Failed to delete DLQ message during drain",
			"message_id", msg.ID,
			"audit_type", auditType)
	}

	return true
}

// getStreamKey returns the Redis stream key for a given audit type
func (c *Client) getStreamKey(auditType string) string {
	return fmt.Sprintf("audit:dlq:%s", auditType)
}
