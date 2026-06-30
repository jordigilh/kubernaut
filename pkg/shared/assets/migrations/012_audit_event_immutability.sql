-- +goose Up
-- Migration: 012_audit_event_immutability
-- FedRAMP AU-9: Protection of Audit Information
-- SOC2 CC8.1: Tamper-evident audit trail
-- BR-AUDIT-004: Immutability / integrity of audit records
--
-- Prevents UPDATE on critical audit event fields (event_data, event_type,
-- event_outcome, actor_id, actor_type, correlation_id, event_action,
-- resource_type, resource_id, event_category).
--
-- Operational fields (retention_days, legal_hold, legal_hold_reason,
-- legal_hold_placed_by, legal_hold_placed_at, cluster_name) remain updatable.
-- legal_hold removal is separately guarded by enforce_legal_hold_immutability (008).

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION prevent_audit_event_tampering()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.event_data IS DISTINCT FROM NEW.event_data THEN
        RAISE EXCEPTION 'AU-9: event_data cannot be modified after creation'
            USING ERRCODE = '23514';
    END IF;
    IF OLD.event_type IS DISTINCT FROM NEW.event_type THEN
        RAISE EXCEPTION 'AU-9: event_type cannot be modified after creation'
            USING ERRCODE = '23514';
    END IF;
    IF OLD.event_outcome IS DISTINCT FROM NEW.event_outcome THEN
        RAISE EXCEPTION 'AU-9: event_outcome cannot be modified after creation'
            USING ERRCODE = '23514';
    END IF;
    IF OLD.actor_id IS DISTINCT FROM NEW.actor_id THEN
        RAISE EXCEPTION 'AU-9: actor_id cannot be modified after creation'
            USING ERRCODE = '23514';
    END IF;
    IF OLD.actor_type IS DISTINCT FROM NEW.actor_type THEN
        RAISE EXCEPTION 'AU-9: actor_type cannot be modified after creation'
            USING ERRCODE = '23514';
    END IF;
    IF OLD.correlation_id IS DISTINCT FROM NEW.correlation_id THEN
        RAISE EXCEPTION 'AU-9: correlation_id cannot be modified after creation'
            USING ERRCODE = '23514';
    END IF;
    IF OLD.event_action IS DISTINCT FROM NEW.event_action THEN
        RAISE EXCEPTION 'AU-9: event_action cannot be modified after creation'
            USING ERRCODE = '23514';
    END IF;
    IF OLD.resource_type IS DISTINCT FROM NEW.resource_type THEN
        RAISE EXCEPTION 'AU-9: resource_type cannot be modified after creation'
            USING ERRCODE = '23514';
    END IF;
    IF OLD.resource_id IS DISTINCT FROM NEW.resource_id THEN
        RAISE EXCEPTION 'AU-9: resource_id cannot be modified after creation'
            USING ERRCODE = '23514';
    END IF;
    IF OLD.event_category IS DISTINCT FROM NEW.event_category THEN
        RAISE EXCEPTION 'AU-9: event_category cannot be modified after creation'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER enforce_audit_event_immutability
    BEFORE UPDATE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION prevent_audit_event_tampering();

-- +goose Down
DROP TRIGGER IF EXISTS enforce_audit_event_immutability ON audit_events;
DROP FUNCTION IF EXISTS prevent_audit_event_tampering();
