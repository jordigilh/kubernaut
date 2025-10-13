-- Execution Audit Schema
-- BR-STORAGE-004: Audit trail for action execution

CREATE TABLE IF NOT EXISTS execution_audit (
    -- Primary key
    id BIGSERIAL PRIMARY KEY,

    -- Relationships
    workflow_id VARCHAR(255) NOT NULL,
    execution_id VARCHAR(255) NOT NULL UNIQUE,

    -- Action details
    action_type VARCHAR(100) NOT NULL,
    target_resource VARCHAR(512) NOT NULL,
    cluster_name VARCHAR(255) NOT NULL,

    -- Execution result
    success BOOLEAN NOT NULL,

    -- Timing information
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE,
    execution_time BIGINT NOT NULL CHECK (execution_time >= 0), -- milliseconds

    -- Error tracking
    error_message TEXT,

    -- Metadata (JSON)
    metadata TEXT NOT NULL DEFAULT '{}',

    -- Audit timestamp
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for common queries
CREATE INDEX IF NOT EXISTS idx_execution_audit_workflow_id ON execution_audit(workflow_id);
CREATE INDEX IF NOT EXISTS idx_execution_audit_success ON execution_audit(success);
CREATE INDEX IF NOT EXISTS idx_execution_audit_action_type ON execution_audit(action_type);
CREATE INDEX IF NOT EXISTS idx_execution_audit_start_time ON execution_audit(start_time DESC);
CREATE INDEX IF NOT EXISTS idx_execution_audit_created_at ON execution_audit(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_execution_audit_cluster ON execution_audit(cluster_name);
