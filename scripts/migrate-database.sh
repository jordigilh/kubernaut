#!/bin/bash

# Database Migration Script
# Applies SQL migrations to PostgreSQL database

set -euo pipefail

# Configuration
CONTAINER_NAME="kubernaut-postgres"
DB_NAME="action_history"
DB_USER="slm_user"
MIGRATIONS_DIR="$(pwd)/migrations"

echo "🔄 Running database migrations"
echo "=============================="

# Check if container is running
if ! podman ps --format "{{.Names}}" | grep -q "^${CONTAINER_NAME}$"; then
    echo "❌ PostgreSQL container is not running. Please start it first:"
    echo "   podman start $CONTAINER_NAME"
    exit 1
fi

# Check if migrations directory exists
if [ ! -d "$MIGRATIONS_DIR" ]; then
    echo "❌ Migrations directory not found: $MIGRATIONS_DIR"
    exit 1
fi

# Create migrations tracking table if it doesn't exist
echo "📝 Setting up migration tracking..."
podman exec "$CONTAINER_NAME" psql -U "$DB_USER" -d "$DB_NAME" -c "
CREATE TABLE IF NOT EXISTS schema_migrations (
    version VARCHAR(255) PRIMARY KEY,
    applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);"

# Get list of applied migrations
applied_migrations=$(podman exec "$CONTAINER_NAME" psql -U "$DB_USER" -d "$DB_NAME" -t -c "SELECT version FROM schema_migrations ORDER BY version;" | tr -d ' ')

# Apply each migration file
for migration_file in "$MIGRATIONS_DIR"/*.sql; do
    if [ ! -f "$migration_file" ]; then
        echo "⚠️  No migration files found in $MIGRATIONS_DIR"
        continue
    fi

    # Extract version from filename (e.g., 001_initial_schema.sql -> 001)
    version=$(basename "$migration_file" | cut -d'_' -f1)

    # Check if migration is already applied
    if echo "$applied_migrations" | grep -q "^$version$"; then
        echo "⏭️  Migration $version already applied, skipping..."
        continue
    fi

    echo "🔄 Applying migration: $(basename "$migration_file")"

    # Apply migration
    if podman exec -i "$CONTAINER_NAME" psql -U "$DB_USER" -d "$DB_NAME" < "$migration_file"; then
        # Record successful migration
        podman exec "$CONTAINER_NAME" psql -U "$DB_USER" -d "$DB_NAME" -c "
        INSERT INTO schema_migrations (version) VALUES ('$version');"
        echo "✅ Migration $version applied successfully"
    else
        echo "❌ Migration $version failed"
        exit 1
    fi
done

echo ""
echo "🎉 All migrations completed successfully!"

# Show current schema info
echo ""
echo "📊 Database Schema Summary:"
echo "=========================="
podman exec "$CONTAINER_NAME" psql -U "$DB_USER" -d "$DB_NAME" -c "
SELECT
    schemaname,
    tablename,
    tableowner
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY tablename;"

echo ""
echo "📈 Applied Migrations:"
echo "===================="
podman exec "$CONTAINER_NAME" psql -U "$DB_USER" -d "$DB_NAME" -c "
SELECT version, applied_at FROM schema_migrations ORDER BY version;"