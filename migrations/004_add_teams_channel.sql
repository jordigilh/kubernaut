-- Migration 004: Add 'teams' and 'pagerduty' to notification_audit channel CHECK constraint
-- Issues: #60 (PagerDuty), #593 (Microsoft Teams delivery channel)
-- BR-NOT-104: Multi-channel notification delivery support
--
-- PostgreSQL requires dropping and recreating CHECK constraints to add enum values.
-- The constraint name is derived from the table and column: notification_audit_channel_check

-- +goose Up
-- +goose StatementBegin
ALTER TABLE notification_audit
    DROP CONSTRAINT IF EXISTS notification_audit_channel_check;

ALTER TABLE notification_audit
    ADD CONSTRAINT notification_audit_channel_check
    CHECK (channel IN ('email', 'slack', 'pagerduty', 'teams', 'sms'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE notification_audit
    DROP CONSTRAINT IF EXISTS notification_audit_channel_check;

ALTER TABLE notification_audit
    ADD CONSTRAINT notification_audit_channel_check
    CHECK (channel IN ('email', 'slack', 'sms'));
-- +goose StatementEnd
