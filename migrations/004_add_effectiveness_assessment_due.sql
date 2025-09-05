-- Add missing effectiveness_assessment_due column
-- Migration: 004_add_effectiveness_assessment_due.sql

-- Add effectiveness_assessment_due column to resource_action_traces table
ALTER TABLE resource_action_traces
ADD COLUMN effectiveness_assessment_due TIMESTAMP WITH TIME ZONE;

-- Create index for the new column for efficient queries
CREATE INDEX idx_rat_effectiveness_due ON resource_action_traces (effectiveness_assessment_due);

-- Update the partition tables as well (they inherit from the parent)
-- The column will be automatically added to existing partitions since they inherit the schema
