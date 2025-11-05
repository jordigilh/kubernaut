#!/bin/bash
set -e

echo "🚀 Day 12.1: Running ADR-033 Migration on Data Storage Database"
echo "================================================================"

CONTAINER_NAME="datastorage-postgres-dev"
DB_PASSWORD="dev_password"
DB_USER="slm_user"
DB_NAME="action_history"
DB_PORT="5433"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo ""
echo "📦 Step 1: Checking for existing PostgreSQL container..."
if podman ps -a --format "{{.Names}}" | grep -q "^${CONTAINER_NAME}$"; then
    echo -e "${YELLOW}⚠️  Container $CONTAINER_NAME already exists${NC}"
    echo "   Checking if it's running..."
    
    if podman ps --format "{{.Names}}" | grep -q "^${CONTAINER_NAME}$"; then
        echo -e "${GREEN}✅ Container is running${NC}"
    else
        echo "   Starting existing container..."
        podman start $CONTAINER_NAME
        sleep 3
        echo -e "${GREEN}✅ Container started${NC}"
    fi
else
    echo "   Creating new PostgreSQL container..."
    podman run -d \
      --name $CONTAINER_NAME \
      -p $DB_PORT:5432 \
      -e POSTGRES_DB=$DB_NAME \
      -e POSTGRES_USER=$DB_USER \
      -e POSTGRES_PASSWORD=$DB_PASSWORD \
      pgvector/pgvector:pg16
    
    echo "⏳ Waiting for PostgreSQL to be ready..."
    sleep 5
    
    for i in {1..30}; do
      if podman exec $CONTAINER_NAME pg_isready -U $DB_USER >/dev/null 2>&1; then
        echo -e "${GREEN}✅ PostgreSQL is ready${NC}"
        break
      fi
      if [ $i -eq 30 ]; then
        echo "❌ PostgreSQL failed to start"
        exit 1
      fi
      sleep 1
    done
fi

echo ""
echo "🔍 Step 2: Checking current schema version..."
CURRENT_COLUMNS=$(podman exec -i $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME -t << 'EOSQL'
SELECT COUNT(*) FROM information_schema.columns 
WHERE table_name = 'resource_action_traces' 
  AND column_name IN (
    'incident_type', 'alert_name', 'incident_severity',
    'playbook_id', 'playbook_version', 'playbook_step_number', 'playbook_execution_id',
    'ai_selected_playbook', 'ai_chained_playbooks', 'ai_manual_escalation', 'ai_playbook_customization'
  );
EOSQL
)

CURRENT_COLUMNS=$(echo $CURRENT_COLUMNS | tr -d ' ')
echo "   ADR-033 columns currently present: $CURRENT_COLUMNS/11"

if [ "$CURRENT_COLUMNS" = "11" ]; then
    echo -e "${GREEN}✅ ADR-033 migration already applied${NC}"
    echo ""
    echo "📊 Verifying indexes..."
    INDEX_COUNT=$(podman exec -i $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME -t << 'EOSQL'
SELECT COUNT(*) FROM pg_indexes 
WHERE tablename = 'resource_action_traces' 
  AND indexname IN (
    'idx_incident_type_success',
    'idx_playbook_success',
    'idx_multidimensional_success',
    'idx_playbook_execution',
    'idx_ai_execution_mode',
    'idx_alert_name_lookup'
  );
EOSQL
)
    INDEX_COUNT=$(echo $INDEX_COUNT | tr -d ' ')
    echo "   ADR-033 indexes present: $INDEX_COUNT/6"
    
    if [ "$INDEX_COUNT" = "6" ]; then
        echo -e "${GREEN}✅ All ADR-033 indexes present${NC}"
    else
        echo -e "${YELLOW}⚠️  Expected 6 indexes, found $INDEX_COUNT${NC}"
    fi
    
    echo ""
    echo "✅ Day 12.1 Complete: ADR-033 migration already applied"
    exit 0
fi

echo ""
echo "🚀 Step 3: Running ADR-033 migration (Migration 012)..."
export GOOSE_DRIVER=postgres
export GOOSE_DBSTRING="host=localhost port=$DB_PORT user=$DB_USER password=$DB_PASSWORD dbname=$DB_NAME sslmode=disable"

# Run migration up to version 012
goose -dir migrations up-to 12

echo ""
echo "🔍 Step 4: Verifying migration results..."

# Verify columns
FINAL_COLUMNS=$(podman exec -i $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME -t << 'EOSQL'
SELECT COUNT(*) FROM information_schema.columns 
WHERE table_name = 'resource_action_traces' 
  AND column_name IN (
    'incident_type', 'alert_name', 'incident_severity',
    'playbook_id', 'playbook_version', 'playbook_step_number', 'playbook_execution_id',
    'ai_selected_playbook', 'ai_chained_playbooks', 'ai_manual_escalation', 'ai_playbook_customization'
  );
EOSQL
)

FINAL_COLUMNS=$(echo $FINAL_COLUMNS | tr -d ' ')
echo "   ADR-033 columns created: $FINAL_COLUMNS/11"

if [ "$FINAL_COLUMNS" != "11" ]; then
    echo "❌ FAIL: Expected 11 columns, found $FINAL_COLUMNS"
    exit 1
fi
echo -e "${GREEN}✅ All 11 ADR-033 columns created${NC}"

# Verify indexes
FINAL_INDEXES=$(podman exec -i $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME -t << 'EOSQL'
SELECT COUNT(*) FROM pg_indexes 
WHERE tablename = 'resource_action_traces' 
  AND indexname IN (
    'idx_incident_type_success',
    'idx_playbook_success',
    'idx_multidimensional_success',
    'idx_playbook_execution',
    'idx_ai_execution_mode',
    'idx_alert_name_lookup'
  );
EOSQL
)

FINAL_INDEXES=$(echo $FINAL_INDEXES | tr -d ' ')
echo "   ADR-033 indexes created: $FINAL_INDEXES/6"

if [ "$FINAL_INDEXES" != "6" ]; then
    echo "❌ FAIL: Expected 6 indexes, found $FINAL_INDEXES"
    exit 1
fi
echo -e "${GREEN}✅ All 6 ADR-033 indexes created${NC}"

echo ""
echo "🔍 Step 5: Checking goose version table..."
goose -dir migrations status

echo ""
echo "================================================================"
echo -e "${GREEN}✅ Day 12.1 COMPLETE: ADR-033 Migration Successfully Applied${NC}"
echo "================================================================"
echo ""
echo "Summary:"
echo "  ✅ Migration 012 applied"
echo "  ✅ 11 ADR-033 columns created"
echo "  ✅ 6 ADR-033 indexes created"
echo "  ✅ Database ready for Day 12.2 (Model Updates)"
echo ""
echo "Next Step: Day 12.2 - Update Go models with ADR-033 fields"
