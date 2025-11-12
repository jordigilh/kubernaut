package dlq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// DLQ CLIENT (TDD GREEN Phase)
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
// Business Requirements:
// - BR-AUDIT-001: Complete audit trail with no data loss
// - DD-009: Dead Letter Queue pattern for error recovery
//
// ========================================

// Client provides Dead Letter Queue functionality using Redis Streams.
type Client struct {
	redisClient *redis.Client
	logger      *zap.Logger
}

// AuditMessage represents a message in the DLQ.
type AuditMessage struct {
	Type       string          `json:"type"`        // e.g., "notification_audit"
	Payload    json.RawMessage `json:"payload"`     // Serialized audit record
	Timestamp  time.Time       `json:"timestamp"`   // When message was added to DLQ
	RetryCount int             `json:"retry_count"` // Number of retry attempts
	LastError  string          `json:"last_error"`  // Error that caused DLQ write
}

// NewClient creates a new DLQ client.
func NewClient(redisClient *redis.Client, logger *zap.Logger) *Client {
	return &Client{
		redisClient: redisClient,
		logger:      logger,
	}
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
	streamKey := "audit:dlq:notification"
	_, err = c.redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		MaxLen: 10000, // Cap at 10,000 messages (FIFO eviction)
		ID:     "*",   // Auto-generate timestamp-based ID
		Values: map[string]interface{}{
			"message": string(messageJSON),
		},
	}).Result()

	if err != nil {
		return fmt.Errorf("failed to add to DLQ: %w", err)
	}

	c.logger.Info("Audit record added to DLQ",
		zap.String("type", "notification_audit"),
		zap.String("notification_id", audit.NotificationID),
		zap.String("error", originalError.Error()),
	)

	return nil
}

// GetDLQDepth returns the number of messages in the DLQ for a given audit type.
func (c *Client) GetDLQDepth(ctx context.Context, auditType string) (int64, error) {
	streamKey := fmt.Sprintf("audit:dlq:%s", auditType)

	length, err := c.redisClient.XLen(ctx, streamKey).Result()
	if err != nil {
		// If stream doesn't exist, Redis returns 0 (not an error)
		// But if Redis is down, we get an error
		if err == redis.Nil {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get DLQ depth: %w", err)
	}

	return length, nil
}

// HealthCheck verifies Redis connectivity.
func (c *Client) HealthCheck(ctx context.Context) error {
	if err := c.redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("DLQ health check failed: %w", err)
	}
	return nil
}
