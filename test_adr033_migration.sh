#!/bin/bash
set -e

echo "🧪 Testing ADR-033 Migration Script (012_adr033_multidimensional_tracking.sql)"
echo "=============================================================================="

CONTAINER_NAME="test-postgres-adr033"
DB_PASSWORD="test_password"
DB_USER="slm_user"
DB_NAME="action_history"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo ""
echo "📦 Step 1: Starting PostgreSQL container with pgvector..."
podman stop $CONTAINER_NAME 2>/dev/null || true
podman rm $CONTAINER_NAME 2>/dev/null || true

podman run -d \
  --name $CONTAINER_NAME \
  -p 5435:5432 \
  -e POSTGRES_DB=$DB_NAME \
  -e POSTGRES_USER=$DB_USER \
  -e POSTGRES_PASSWORD=$DB_PASSWORD \
  pgvector/pgvector:pg16

echo "⏳ Waiting for PostgreSQL to be ready..."
sleep 3

for i in {1..30}; do
  if podman exec $CONTAINER_NAME pg_isready -U $DB_USER >/dev/null 2>&1; then
    echo -e "${GREEN}✅ PostgreSQL is ready${NC}"
    break
  fi
  if [ $i -eq 30 ]; then
    echo -e "${RED}❌ PostgreSQL failed to start${NC}"
    exit 1
  fi
  sleep 1
done

echo ""
echo "📋 Step 2: Creating base schema (simulating existing database)..."
podman exec -i $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME << 'EOSQL'
-- Create goose version tracking table
CREATE TABLE IF NOT EXISTS goose_db_version (
    id SERIAL PRIMARY KEY,
    version_id BIGINT NOT NULL,
    is_applied BOOLEAN NOT NULL,
    tstamp TIMESTAMP DEFAULT NOW()
);

-- Create base table (simulating existing schema before ADR-033)
CREATE TABLE IF NOT EXISTS resource_action_traces (
    id SERIAL PRIMARY KEY,
    action_id VARCHAR(255) NOT NULL,
    action_type VARCHAR(100) NOT NULL,
    action_timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(50) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Add test data
INSERT INTO resource_action_traces (action_id, action_type, action_timestamp, status)
VALUES 
  ('test-action-1', 'restart_pod', NOW(), 'completed'),
  ('test-action-2', 'scale_deployment', NOW(), 'failed');

-- Mark all previous migrations as applied (simulate production state)
INSERT INTO goose_db_version (version_id, is_applied) VALUES
  (1, true),   -- 001_initial_schema.sql
  (2, true),   -- 002_fix_partitioning.sql
  (3, true),   -- 003_stored_procedures.sql
  (4, true),   -- 004_add_effectiveness_assessment_due.sql
  (5, true),   -- 005_vector_schema.sql
  (6, true),   -- 006_effectiveness_assessment.sql
  (7, true),   -- 007_add_context_column.sql
  (8, true),   -- 008_context_api_compatibility.sql
  (9, true),   -- 009_update_vector_dimensions.sql
  (10, true),  -- 010_audit_write_api_phase1.sql
  (11, true);  -- 011_rename_alert_to_signal.sql
EOSQL

echo -e "${GREEN}✅ Base schema created with 2 test records${NC}"

echo ""
echo "📋 Step 3: Verifying base schema (BEFORE migration)..."
BEFORE_COLUMNS=$(podman exec -i $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME -t -c "\d resource_action_traces" | grep -c "incident_type\|playbook_id\|ai_selected_playbook" || echo "0")
echo "   ADR-033 columns found: $BEFORE_COLUMNS (expected: 0)"

if [ "$BEFORE_COLUMNS" -ne "0" ]; then
  echo -e "${RED}❌ FAIL: ADR-033 columns already exist before migration${NC}"
  exit 1
fi
echo -e "${GREEN}✅ Verified: No ADR-033 columns before migration${NC}"

echo ""
echo "🚀 Step 4: Running ADR-033 migration (FIRST RUN)..."
export GOOSE_DRIVER=postgres
export GOOSE_DBSTRING="host=localhost port=5435 user=$DB_USER password=$DB_PASSWORD dbname=$DB_NAME sslmode=disable"

# Only run migration 012 (ADR-033)
goose -dir migrations up-to 12

echo ""
echo "🔍 Step 5: Verifying schema changes (AFTER first migration)..."

# Verify columns
echo "   Checking columns..."
COLUMNS=$(podman exec -i $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME -t << 'EOSQL'
SELECT column_name FROM information_schema.columns 
WHERE table_name = 'resource_action_traces' 
  AND column_name IN (
    'incident_type', 'alert_name', 'incident_severity',
    'playbook_id', 'playbook_version', 'playbook_step_number', 'playbook_execution_id',
    'ai_selected_playbook', 'ai_chained_playbooks', 'ai_manual_escalation', 'ai_playbook_customization'
  )
ORDER BY column_name;
EOSQL
)

COLUMN_COUNT=$(echo "$COLUMNS" | grep -v '^$' | wc -l | tr -d ' ')
echo "   ADR-033 columns found: $COLUMN_COUNT (expected: 11)"

if [ "$COLUMN_COUNT" -ne "11" ]; then
  echo -e "${RED}❌ FAIL: Expected 11 columns, found $COLUMN_COUNT${NC}"
  echo "Columns found:"
  echo "$COLUMNS"
  exit 1
fi
echo -e "${GREEN}✅ All 11 ADR-033 columns created${NC}"

# Verify indexes
echo "   Checking indexes..."
INDEXES=$(podman exec -i $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME -t << 'EOSQL'
SELECT indexname FROM pg_indexes 
WHERE tablename = 'resource_action_traces' 
  AND indexname IN (
    'idx_incident_type_success',
    'idx_playbook_success',
    'idx_multidimensional_success',
    'idx_playbook_execution',
    'idx_ai_execution_mode',
    'idx_alert_name_lookup'
  )
ORDER BY indexname;
EOSQL
)

INDEX_COUNT=$(echo "$INDEXES" | grep -v '^$' | wc -l | tr -d ' ')
echo "   ADR-033 indexes found: $INDEX_COUNT (expected: 6)"

if [ "$INDEX_COUNT" -ne "6" ]; then
  echo -e "${RED}❌ FAIL: Expected 6 indexes, found $INDEX_COUNT${NC}"
  echo "Indexes found:"
  echo "$INDEXES"
  exit 1
fi
echo -e "${GREEN}✅ All 6 ADR-033 indexes created${NC}"

# Verify existing data is intact
echo "   Checking data integrity..."
RECORD_COUNT=$(podman exec -i $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME -t -c "SELECT COUNT(*) FROM resource_action_traces;" | tr -d ' ')
echo "   Records found: $RECORD_COUNT (expected: 2)"

if [ "$RECORD_COUNT" -ne "2" ]; then
  echo -e "${RED}❌ FAIL: Expected 2 records, found $RECORD_COUNT${NC}"
  exit 1
fi
echo -e "${GREEN}✅ Existing data intact (backward compatible)${NC}"

echo ""
echo "🔄 Step 6: Running migration again (IDEMPOTENCY TEST)..."
goose -dir migrations up-to 12

echo ""
echo "🔍 Step 7: Verifying idempotency (AFTER second migration)..."

# Verify column count again
COLUMNS_AFTER=$(podman exec -i $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME -t << 'EOSQL'
SELECT column_name FROM information_schema.columns 
WHERE table_name = 'resource_action_traces' 
  AND column_name IN (
    'incident_type', 'alert_name', 'incident_severity',
    'playbook_id', 'playbook_version', 'playbook_step_number', 'playbook_execution_id',
    'ai_selected_playbook', 'ai_chained_playbooks', 'ai_manual_escalation', 'ai_playbook_customization'
  )
ORDER BY column_name;
EOSQL
)

COLUMN_COUNT_AFTER=$(echo "$COLUMNS_AFTER" | grep -v '^$' | wc -l | tr -d ' ')
echo "   ADR-033 columns found: $COLUMN_COUNT_AFTER (expected: 11)"

if [ "$COLUMN_COUNT_AFTER" -ne "11" ]; then
  echo -e "${RED}❌ FAIL: Column count changed after rerun${NC}"
  exit 1
fi
echo -e "${GREEN}✅ Idempotency verified: Column count unchanged${NC}"

# Verify data integrity after rerun
RECORD_COUNT_AFTER=$(podman exec -i $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME -t -c "SELECT COUNT(*) FROM resource_action_traces;" | tr -d ' ')
echo "   Records found: $RECORD_COUNT_AFTER (expected: 2)"

if [ "$RECORD_COUNT_AFTER" -ne "2" ]; then
  echo -e "${RED}❌ FAIL: Record count changed after rerun${NC}"
  exit 1
fi
echo -e "${GREEN}✅ Data integrity preserved after rerun${NC}"

echo ""
echo "🔍 Step 8: Testing rollback (goose down)..."
goose -dir migrations down

# Verify columns are removed
COLUMNS_ROLLBACK=$(podman exec -i $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME -t << 'EOSQL'
SELECT column_name FROM information_schema.columns 
WHERE table_name = 'resource_action_traces' 
  AND column_name IN (
    'incident_type', 'alert_name', 'incident_severity',
    'playbook_id', 'playbook_version', 'playbook_step_number', 'playbook_execution_id',
    'ai_selected_playbook', 'ai_chained_playbooks', 'ai_manual_escalation', 'ai_playbook_customization'
  )
ORDER BY column_name;
EOSQL
)

COLUMN_COUNT_ROLLBACK=$(echo "$COLUMNS_ROLLBACK" | grep -v '^$' | wc -l | tr -d ' ')
echo "   ADR-033 columns found after rollback: $COLUMN_COUNT_ROLLBACK (expected: 0)"

if [ "$COLUMN_COUNT_ROLLBACK" -ne "0" ]; then
  echo -e "${RED}❌ FAIL: Rollback did not remove all columns${NC}"
  exit 1
fi
echo -e "${GREEN}✅ Rollback successful: All ADR-033 columns removed${NC}"

# Verify data still intact after rollback
RECORD_COUNT_ROLLBACK=$(podman exec -i $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME -t -c "SELECT COUNT(*) FROM resource_action_traces;" | tr -d ' ')
echo "   Records found: $RECORD_COUNT_ROLLBACK (expected: 2)"

if [ "$RECORD_COUNT_ROLLBACK" -ne "2" ]; then
  echo -e "${RED}❌ FAIL: Data lost during rollback${NC}"
  exit 1
fi
echo -e "${GREEN}✅ Data integrity preserved after rollback${NC}"

echo ""
echo "🧹 Step 9: Cleanup..."
podman stop $CONTAINER_NAME >/dev/null 2>&1
podman rm $CONTAINER_NAME >/dev/null 2>&1
echo -e "${GREEN}✅ Containers cleaned up${NC}"

echo ""
echo "=============================================================================="
echo -e "${GREEN}✅ ALL TESTS PASSED${NC}"
echo "=============================================================================="
echo ""
echo "Summary:"
echo "  ✅ Migration applies successfully"
echo "  ✅ All 11 ADR-033 columns created"
echo "  ✅ All 6 ADR-033 indexes created"
echo "  ✅ Backward compatible (existing data intact)"
echo "  ✅ Idempotent (safe to run multiple times)"
echo "  ✅ Rollback works correctly"
echo ""
echo "Migration is READY for Day 12 execution!"

