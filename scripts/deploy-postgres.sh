#!/bin/bash

# PostgreSQL Deployment Script for Action History Storage
# Uses Podman for container management

set -euo pipefail

# Configuration
POSTGRES_VERSION="16"
CONTAINER_NAME="prometheus-alerts-slm-postgres"
DB_NAME="action_history"
DB_USER="slm_user"
DB_PASSWORD="slm_password_dev"
DB_PORT="5432"
POSTGRES_DATA_DIR="$(pwd)/postgres-data"

echo "🚀 Deploying PostgreSQL for Prometheus Alerts SLM"
echo "=================================================="

# Function to check if container exists
container_exists() {
    podman ps -a --format "{{.Names}}" | grep -q "^${CONTAINER_NAME}$"
}

# Function to check if container is running
container_running() {
    podman ps --format "{{.Names}}" | grep -q "^${CONTAINER_NAME}$"
}

# Stop and remove existing container if it exists
if container_exists; then
    echo "📦 Existing container found. Stopping and removing..."
    if container_running; then
        podman stop "$CONTAINER_NAME"
    fi
    podman rm "$CONTAINER_NAME"
    echo "✅ Existing container removed"
fi

# Create data directory with proper permissions
echo "📁 Creating PostgreSQL data directory..."
mkdir -p "$POSTGRES_DATA_DIR"
chmod 755 "$POSTGRES_DATA_DIR"
echo "✅ Data directory created: $POSTGRES_DATA_DIR"

# Pull PostgreSQL image
echo "🐳 Pulling PostgreSQL $POSTGRES_VERSION image..."
podman pull docker.io/library/postgres:$POSTGRES_VERSION
echo "✅ PostgreSQL image pulled"

# Run PostgreSQL container (without persistent volume for now)
echo "🚀 Starting PostgreSQL container..."
podman run -d \
    --name "$CONTAINER_NAME" \
    --env POSTGRES_DB="$DB_NAME" \
    --env POSTGRES_USER="$DB_USER" \
    --env POSTGRES_PASSWORD="$DB_PASSWORD" \
    --publish "$DB_PORT:5432" \
    postgres:$POSTGRES_VERSION

echo "✅ PostgreSQL container started"

# Wait for PostgreSQL to be ready
echo "⏳ Waiting for PostgreSQL to be ready..."
max_attempts=30
attempt=1

while [ $attempt -le $max_attempts ]; do
    if podman exec "$CONTAINER_NAME" pg_isready -U "$DB_USER" -d "$DB_NAME" >/dev/null 2>&1; then
        echo "✅ PostgreSQL is ready!"
        break
    fi
    
    if [ $attempt -eq $max_attempts ]; then
        echo "❌ PostgreSQL failed to start within expected time"
        exit 1
    fi
    
    echo "⏳ Attempt $attempt/$max_attempts - waiting 2 seconds..."
    sleep 2
    ((attempt++))
done

# Test connection
echo "🔍 Testing database connection..."
podman exec "$CONTAINER_NAME" psql -U "$DB_USER" -d "$DB_NAME" -c "SELECT version();"

if [ $? -eq 0 ]; then
    echo "✅ Database connection successful!"
else
    echo "❌ Database connection failed"
    exit 1
fi

# Display connection information
echo ""
echo "🎉 PostgreSQL deployment successful!"
echo "=================================================="
echo "Database Details:"
echo "  Container Name: $CONTAINER_NAME"
echo "  Database Name:  $DB_NAME"
echo "  Username:       $DB_USER"
echo "  Password:       $DB_PASSWORD"
echo "  Port:           $DB_PORT"
echo "  Data Directory: $POSTGRES_DATA_DIR"
echo ""
echo "Connection String:"
echo "  postgres://$DB_USER:$DB_PASSWORD@localhost:$DB_PORT/$DB_NAME"
echo ""
echo "Container Management:"
echo "  Stop:    podman stop $CONTAINER_NAME"
echo "  Start:   podman start $CONTAINER_NAME"
echo "  Logs:    podman logs $CONTAINER_NAME"
echo "  Connect: podman exec -it $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME"
echo ""
echo "Ready for schema migration!"