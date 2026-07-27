-- +goose Up
-- Issue #1661 Change 7 (DD-WORKFLOW-018): etcd is the single source of truth
-- for RemediationWorkflow content; Postgres audit_events is the sole source
-- for execution-outcome history. total_executions/successful_executions/
-- actual_success_rate stop being stored, UPDATE-maintained catalog columns
-- (Repository.UpdateSuccessMetrics, deleted alongside this migration) and
-- become an on-demand aggregation over audit_events
-- (workflowexecution.workflow.completed/.failed events), computed at query
-- time by pkg/datastorage/repository.AuditEventsRepository.GetSuccessMetrics.

DROP INDEX IF EXISTS idx_workflow_catalog_success_rate;

ALTER TABLE remediation_workflow_catalog
    DROP COLUMN IF EXISTS actual_success_rate,
    DROP COLUMN IF EXISTS total_executions,
    DROP COLUMN IF EXISTS successful_executions;

-- +goose Down
ALTER TABLE remediation_workflow_catalog
    ADD COLUMN actual_success_rate DECIMAL(4,3),
    ADD COLUMN total_executions INTEGER DEFAULT 0,
    ADD COLUMN successful_executions INTEGER DEFAULT 0;

ALTER TABLE remediation_workflow_catalog
    ADD CONSTRAINT remediation_workflow_catalog_actual_success_rate_check
        CHECK (actual_success_rate IS NULL OR (actual_success_rate >= 0 AND actual_success_rate <= 1)),
    ADD CONSTRAINT remediation_workflow_catalog_total_executions_check
        CHECK (total_executions >= 0),
    ADD CONSTRAINT remediation_workflow_catalog_successful_executions_check
        CHECK (successful_executions >= 0 AND successful_executions <= total_executions);

CREATE INDEX idx_workflow_catalog_success_rate ON remediation_workflow_catalog(actual_success_rate DESC) WHERE status = 'active';
