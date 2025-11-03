-- Migration 010: Audit Write API Phase 1 - Notification Audit Table
-- Created: 2025-11-03
-- Updated: 2025-11-03 (V4.8 - Phased Audit Table Development)
-- Purpose: Implement audit trail persistence for Notification Controller (ONLY fully implemented controller)
-- Authority: docs/architecture/decisions/ADR-032-data-access-layer-isolation.md v1.3
-- Related: IMPLEMENTATION_PLAN_V4.8.md Phase 1 (1 Audit Table)
--
-- PHASED APPROACH RATIONALE:
-- - Phase 1: Implement ONLY notification_audit (Notification Controller is fully operational)
-- - Phase 2: Implement remaining 5 audit tables during controller TDD (RemediationProcessor, 
--   RemediationOrchestrator, AIAnalysis, WorkflowExecution, EffectivenessMonitor)
-- - Benefit: 100% schema accuracy - audit tables match actual controller implementation
-- - Risk Mitigation: Zero rework risk - deferred tables created with real CRD status fields

-- Enable pgvector extension (idempotent) - Required for future AI analysis embeddings
CREATE EXTENSION IF NOT EXISTS vector;

-- ============================================================================
-- Table 1: notification_audit (PHASE 1 - ONLY FULLY IMPLEMENTED CONTROLLER)
-- Service: Notification Controller
-- Endpoint: POST /api/v1/audit/notifications
-- Schema Authority: docs/services/crd-controllers/06-notification/database-integration.md
-- Controller Status: ✅ FULLY IMPLEMENTED & OPERATIONAL
-- ============================================================================

CREATE TABLE IF NOT EXISTS notification_audit (
    id BIGSERIAL PRIMARY KEY,
    
    -- Identity
    remediation_id VARCHAR(255) NOT NULL,  -- Links to RemediationRequest CRD
    notification_id VARCHAR(255) NOT NULL UNIQUE,  -- Links to NotificationRequest CRD
    recipient VARCHAR(255) NOT NULL,  -- Target recipient (e.g., email, Slack user ID)
    channel VARCHAR(50) NOT NULL,  -- Communication channel (e.g., "email", "slack", "pagerduty")
    message_summary TEXT NOT NULL,  -- Short summary of the notification content
    
    -- Status & Outcome
    status VARCHAR(50) NOT NULL CHECK (status IN ('sent', 'failed', 'acknowledged', 'escalated')),
    sent_at TIMESTAMP WITH TIME ZONE NOT NULL,  -- Timestamp when notification was sent
    delivery_status TEXT,  -- Detailed status from provider (e.g., "200 OK", "rate_limited")
    error_message TEXT,  -- Error details if notification failed
    escalation_level INTEGER DEFAULT 0,  -- 0 = initial, 1 = first escalation, etc.
    
    -- Metadata
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for notification_audit
CREATE UNIQUE INDEX IF NOT EXISTS notification_audit_notification_id_key ON notification_audit (notification_id);
CREATE INDEX IF NOT EXISTS idx_notification_audit_remediation_id ON notification_audit(remediation_id);
CREATE INDEX IF NOT EXISTS idx_notification_audit_recipient ON notification_audit(recipient);
CREATE INDEX IF NOT EXISTS idx_notification_audit_channel ON notification_audit(channel);
CREATE INDEX IF NOT EXISTS idx_notification_audit_status ON notification_audit(status);
CREATE INDEX IF NOT EXISTS idx_notification_audit_sent_at ON notification_audit(sent_at DESC);

-- ============================================================================
-- PHASE 2 AUDIT TABLES (DEFERRED - TDD-ALIGNED)
-- ============================================================================
-- The following 5 audit tables will be created during controller TDD implementation:
--
-- 1. signal_processing_audit (Migration 011) - RemediationProcessor Controller
--    - Created during: RemediationProcessor TDD (controller implementation)
--    - Endpoint: POST /api/v1/audit/signal-processing
--    - Status: CRD placeholder exists, controller not implemented
--
-- 2. orchestration_audit (Migration 012) - RemediationOrchestrator Controller
--    - Created during: RemediationOrchestrator TDD (controller implementation)
--    - Endpoint: POST /api/v1/audit/orchestration
--    - Status: CRD placeholder exists, controller not implemented
--
-- 3. ai_analysis_audit (Migration 013) - AIAnalysis Controller
--    - Created during: AIAnalysis TDD (controller implementation)
--    - Endpoint: POST /api/v1/audit/ai-decisions
--    - Status: CRD placeholder exists, controller not implemented
--    - Special: Includes embedding vector(1536) column for semantic search
--
-- 4. workflow_execution_audit (Migration 014) - WorkflowExecution Controller
--    - Created during: WorkflowExecution TDD (controller implementation)
--    - Endpoint: POST /api/v1/audit/executions
--    - Status: CRD placeholder exists, controller not implemented
--
-- 5. effectiveness_audit (Migration 015) - Effectiveness Monitor Service
--    - Created during: Effectiveness Monitor service TDD (service implementation)
--    - Endpoint: POST /api/v1/audit/effectiveness
--    - Status: Business logic exists (pkg/ai/insights/), no HTTP service wrapper
--
-- RATIONALE FOR PHASED APPROACH:
-- - Eliminates schema rework risk (40% probability of changes if created before controller implementation)
-- - Ensures 100% schema accuracy (audit tables match actual CRD status fields)
-- - Follows TDD methodology (build audit schema during controller GREEN phase)
-- - Reduces Data Storage Write API scope (1 endpoint vs 6 endpoints for Phase 1)
-- - Timeline savings: 4.5 days (35h) vs 12.5 days (100h) for Phase 1-3
-- ============================================================================

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to automatically update updated_at for notification_audit
CREATE OR REPLACE TRIGGER trigger_notification_audit_updated_at
    BEFORE UPDATE ON notification_audit
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_timestamp();

-- ============================================================================
-- MIGRATION VALIDATION
-- ============================================================================
-- To validate this migration:
-- 1. Apply migration: psql -d kubernaut -f migrations/010_audit_write_api_phase1.sql
-- 2. Verify table exists: \dt notification_audit
-- 3. Verify indexes: \di notification_audit*
-- 4. Verify trigger: \dy trigger_notification_audit_updated_at
-- 5. Test insert: INSERT INTO notification_audit (remediation_id, notification_id, recipient, channel, message_summary, status, sent_at) VALUES ('test-remediation-1', 'test-notification-1', 'test@example.com', 'email', 'Test notification', 'sent', NOW());
-- 6. Verify updated_at trigger: UPDATE notification_audit SET status = 'acknowledged' WHERE notification_id = 'test-notification-1'; SELECT updated_at FROM notification_audit WHERE notification_id = 'test-notification-1';
-- 7. Cleanup test data: DELETE FROM notification_audit WHERE notification_id = 'test-notification-1';
-- ============================================================================
