-- Add November 2025 partition for resource_action_traces
-- Needed for integration tests running in November 2025

CREATE TABLE IF NOT EXISTS resource_action_traces_y2025m10
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_y2025m11
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');
