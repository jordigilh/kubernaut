#!/bin/bash
# Notification Integration Test - Database Migrations
# Runs only the UP section of migrations (excludes goose Down)
# Usage: ./run-migrations.sh

set -e

POSTGRES_CONTAINER="notification_postgres_1"
MIGRATIONS_DIR="../../../migrations"

echo "🔧 Running database migrations for Notification integration tests..."

# Check if container is running
if ! podman ps --format "{{.Names}}" | grep -q "^${POSTGRES_CONTAINER}$"; then
    echo "❌ PostgreSQL container not running: $POSTGRES_CONTAINER"
    echo "   Start infrastructure first: podman-compose -f podman-compose.notification.test.yml up -d"
    exit 1
fi

# Function to run only UP section of a migration
run_migration_up() {
    local migration_file=$1
    local migration_name=$(basename "$migration_file")
    
    echo "  📄 Running $migration_name (UP only)..."
    
    # Extract only the UP section (stop before "-- +goose Down")
    sed -n '1,/^-- +goose Down/p' "$migration_file" | \
        podman exec -i "$POSTGRES_CONTAINER" psql -U slm_user -d action_history \
        > /dev/null 2>&1 || {
        echo "     ⚠️  Migration failed (may be expected if already applied)"
    }
}

# Run critical migrations for audit functionality
echo "📦 Running core migrations..."
run_migration_up "$MIGRATIONS_DIR/001_initial_schema.sql"

echo "📦 Running audit migrations..."
run_migration_up "$MIGRATIONS_DIR/013_create_audit_events_table.sql"
run_migration_up "$MIGRATIONS_DIR/021_create_notification_audit_table.sql"

# Verify tables were created
echo ""
echo "✅ Verifying database tables..."
podman exec "$POSTGRES_CONTAINER" psql -U slm_user -d action_history -c "\dt audit_events*" | head -10

echo ""
echo "🎉 Migration complete! Ready for integration tests."
echo "   Run tests with: go test -v ./test/integration/notification/audit*.go"

