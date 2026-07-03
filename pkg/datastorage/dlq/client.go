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
// File layout (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3 split, pure code
// motion, no behavior change):
//   - client.go (this file): Client struct, message types, NewClient,
//     producer-side Enqueue*/GetDLQDepth/HealthCheck
//   - client_consumer.go: GAP-8 consumer-group read/ack/claim methods
//   - client_drain.go: DD-008 graceful-shutdown drain path
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

// maxLastErrorLen caps the LastError field to prevent internal error
// details (SQL, driver info) from leaking into the DLQ stream (SEC-L1).
const maxLastErrorLen = 256

// SanitizeError truncates and redacts error strings stored in DLQ LastError.
// SEC-L1 / SEC-M1: Prevents raw SQL/driver/credential details from persisting in Redis.
func SanitizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	// SEC-H1 / SEC-M1: Redact any error containing SQL/driver/credential patterns.
	for _, pattern := range []string{
		"pq:", "pgx:", "sql:", "driver:", "sqlstate",
		"error:", "fatal:",
		"connection refused", "connection reset",
		"password", "secret", "token",
		"postgres://", "postgresql://", "redis://",
	} {
		if strings.Contains(lower, pattern) {
			return "database write failed"
		}
	}
	if len(msg) > maxLastErrorLen {
		msg = string([]rune(msg)[:maxLastErrorLen]) + "..."
	}
	return msg
}

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

	auditMsg := AuditMessage{
		Type:       "notification_audit",
		Payload:    payloadJSON,
		Timestamp:  time.Now(),
		RetryCount: 0,
		LastError:  SanitizeError(originalError),
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

	c.logNotificationEnqueueSuccess(c.logger, audit.NotificationID, SanitizeError(originalError))

	return nil
}

// EnqueueAuditEvent adds a generic AuditEvent record to the DLQ.
// This is called when the primary write to PostgreSQL fails for unified audit events.
// D3/SI-10: Validates the event before enqueue to prevent replay rejection.
func (c *Client) EnqueueAuditEvent(ctx context.Context, audit *audit.AuditEvent, originalError error) error {
	// D3: Validate before enqueue so replay never rejects payloads we accepted
	if err := audit.Validate(); err != nil {
		return fmt.Errorf("DLQ enqueue rejected: audit event invalid: %w", err)
	}
	if err := ValidateEventData(audit.EventData); err != nil {
		return fmt.Errorf("DLQ enqueue rejected: EventData invalid: %w", err)
	}

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
		LastError:  SanitizeError(originalError),
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

	c.logEnqueueSuccess(c.logger, "audit_event", audit.EventID.String(), SanitizeError(originalError))

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
