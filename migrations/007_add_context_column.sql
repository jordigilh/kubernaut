-- Add missing context column to action_patterns table
-- Migration: 007_add_context_column.sql
-- Required for disaster recovery backup operations (BR-2B: Database schema consistency)

-- Add context column for storing action context information
ALTER TABLE action_patterns
ADD COLUMN IF NOT EXISTS context TEXT;

-- Create index for context searches
CREATE INDEX IF NOT EXISTS action_patterns_context_idx ON action_patterns(context);

-- Update view to include context information
CREATE OR REPLACE VIEW pattern_analytics_summary AS
SELECT
    COUNT(*) as total_patterns,
    COUNT(DISTINCT action_type) as unique_action_types,
    COUNT(DISTINCT alert_name) as unique_alert_names,
    COUNT(DISTINCT alert_severity) as unique_severities,
    COUNT(DISTINCT namespace) as unique_namespaces,
    COUNT(DISTINCT resource_type) as unique_resource_types,
    COUNT(DISTINCT context) as unique_contexts,
    AVG((effectiveness_data->>'score')::float) as avg_effectiveness_score,
    COUNT(*) FILTER (WHERE effectiveness_data->>'score' IS NOT NULL) as patterns_with_effectiveness,
    COUNT(*) FILTER (WHERE context IS NOT NULL) as patterns_with_context,
    MIN(created_at) as oldest_pattern,
    MAX(created_at) as newest_pattern
FROM action_patterns;

-- Add comment for new column
COMMENT ON COLUMN action_patterns.context IS 'Textual context information for action pattern execution';
