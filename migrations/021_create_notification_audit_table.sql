-- +goose Up
-- +goose StatementBegin

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- Migration 021: Create notification_audit Table
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
--
-- Authority:
--   - docs/services/crd-controllers/06-notification/database-integration.md
--   - pkg/datastorage/models/notification_audit.go
--   - pkg/datastorage/repository/notification_audit_repository.go
--
-- Business Requirements:
--   - BR-NOT-062: Notification Audit Persistence
--   - BR-NOT-063: Notification Delivery Tracking
--   - BR-NOT-064: Notification Escalation Tracking
--
-- Purpose:
--   Creates the notification_audit table for tracking notification delivery
--   attempts, failures, and escalations. Used by the Notification Controller
--   via Data Storage HTTP API.
--
-- Design Decisions:
--   - notification_id has UNIQUE constraint (one audit per notification)
--   - Status uses CHECK constraint for valid values only
--   - Channel uses CHECK constraint for supported channels
--   - Indexes on common query patterns (by notification_id, remediation_id)
--   - Timestamps use TIMESTAMP WITH TIME ZONE for timezone awareness
--
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

CREATE TABLE IF NOT EXISTS notification_audit (
    -- Primary Key
    id BIGSERIAL PRIMARY KEY,

    -- Identity and Relationships
    remediation_id VARCHAR(255) NOT NULL,      -- Links to RemediationRequest CRD
    notification_id VARCHAR(255) NOT NULL UNIQUE,  -- NotificationRequest CRD name (must be unique)

    -- Notification Details
    recipient VARCHAR(255) NOT NULL,           -- Email, Slack user ID, PagerDuty service, etc.
    channel VARCHAR(50) NOT NULL CHECK (channel IN ('email', 'slack', 'pagerduty', 'sms')),
    message_summary TEXT NOT NULL,             -- Short summary of notification content

    -- Delivery Status
    status VARCHAR(50) NOT NULL CHECK (status IN ('sent', 'failed', 'acknowledged', 'escalated')),
    sent_at TIMESTAMP WITH TIME ZONE NOT NULL,

    -- Delivery Details (Optional)
    delivery_status TEXT,                      -- Provider response (e.g., "200 OK", "rate_limited")
    error_message TEXT,                        -- Error details if delivery failed

    -- Escalation Tracking
    escalation_level INTEGER NOT NULL DEFAULT 0,  -- 0=initial, 1=first escalation, etc.

    -- Audit Timestamps
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- Indexes for Common Query Patterns
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

-- Query by notification_id (most common: lookup single notification)
CREATE INDEX IF NOT EXISTS idx_notification_audit_notification_id
ON notification_audit(notification_id);

-- Query by remediation_id (common: get all notifications for a remediation)
CREATE INDEX IF NOT EXISTS idx_notification_audit_remediation_id
ON notification_audit(remediation_id);

-- Query by channel (analytics: delivery rates per channel)
CREATE INDEX IF NOT EXISTS idx_notification_audit_channel
ON notification_audit(channel);

-- Query by status (monitoring: failed notifications)
CREATE INDEX IF NOT EXISTS idx_notification_audit_status
ON notification_audit(status);

-- Query by timestamp (timeline reconstruction: DESC order for recent first)
CREATE INDEX IF NOT EXISTS idx_notification_audit_created_at
ON notification_audit(created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- Rollback: Drop notification_audit Table and Indexes
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

DROP INDEX IF EXISTS idx_notification_audit_created_at;
DROP INDEX IF EXISTS idx_notification_audit_status;
DROP INDEX IF EXISTS idx_notification_audit_channel;
DROP INDEX IF EXISTS idx_notification_audit_remediation_id;
DROP INDEX IF EXISTS idx_notification_audit_notification_id;

DROP TABLE IF EXISTS notification_audit;

-- +goose StatementEnd
