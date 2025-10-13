-- Workflow Audit Schema
-- BR-STORAGE-003: Audit trail for workflow execution

CREATE TABLE IF NOT EXISTS workflow_audit (
    -- Primary key
    id BIGSERIAL PRIMARY KEY,

    -- Relationships
    remediation_request_id VARCHAR(255) NOT NULL,
    workflow_id VARCHAR(255) NOT NULL UNIQUE,

    -- Workflow state
    phase VARCHAR(50) NOT NULL, -- planning, executing, completed, failed
    total_steps INTEGER NOT NULL CHECK (total_steps >= 0),
    completed_steps INTEGER NOT NULL CHECK (completed_steps >= 0),

    -- Timing information
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE,

    -- Metadata (JSON)
    metadata TEXT NOT NULL DEFAULT '{}',

    -- Audit timestamp
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for common queries
CREATE INDEX IF NOT EXISTS idx_workflow_audit_request_id ON workflow_audit(remediation_request_id);
CREATE INDEX IF NOT EXISTS idx_workflow_audit_phase ON workflow_audit(phase);
CREATE INDEX IF NOT EXISTS idx_workflow_audit_start_time ON workflow_audit(start_time DESC);
CREATE INDEX IF NOT EXISTS idx_workflow_audit_created_at ON workflow_audit(created_at DESC);
