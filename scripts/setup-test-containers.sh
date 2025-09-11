#!/bin/bash

# Container Setup Script for Milestone 1 Validation
# Creates PostgreSQL + pgvector and Vector DB containers using podman

set -e

echo "🐳 Setting up Test Containers for Milestone 1 Validation"
echo "========================================================"

# Configuration
POSTGRES_CONTAINER="kubernaut-postgres-test"
POSTGRES_PORT="5433"
POSTGRES_DB="kubernaut_test"
POSTGRES_USER="test_user"
POSTGRES_PASSWORD="test_password_123"
# Use podman volumes instead of bind mounts for better compatibility
POSTGRES_VOLUME="kubernaut-postgres-data"
VECTOR_VOLUME="kubernaut-vector-data"

# Vector DB Configuration (separate instance)
VECTOR_CONTAINER="kubernaut-vector-test"
VECTOR_PORT="5434"
VECTOR_DB="kubernaut_vector_test"
VECTOR_USER="vector_user"
VECTOR_PASSWORD="vector_password_123"

NETWORK_NAME="kubernaut-test-network"
MAX_WAIT_SECONDS=60

echo "📋 Configuration:"
echo "  Primary PostgreSQL: localhost:$POSTGRES_PORT/$POSTGRES_DB"
echo "  Vector PostgreSQL: localhost:$VECTOR_PORT/$VECTOR_DB"
echo "  Network: $NETWORK_NAME"
echo ""

# Function to check if container is running
check_container_running() {
    local container_name=$1
    if podman ps --format "{{.Names}}" | grep -q "^${container_name}$"; then
        return 0  # Container is running
    else
        return 1  # Container is not running
    fi
}

# Function to wait for PostgreSQL to be ready using podman exec
wait_for_postgres() {
    local container_name=$1
    local user=$2
    local db=$3
    local max_wait=$4

    echo "⏳ Waiting for PostgreSQL container $container_name to be ready..."

    local count=0
    while [ $count -lt $max_wait ]; do
        if podman exec -e PGPASSWORD="$(echo "$container_name" | grep postgres-test >/dev/null && echo "$POSTGRES_PASSWORD" || echo "$VECTOR_PASSWORD")" "$container_name" psql -U "$user" -d "$db" -c "SELECT 1;" >/dev/null 2>&1; then
            echo "✅ PostgreSQL container $container_name is ready"
            return 0
        fi
        count=$((count + 1))
        echo "  Attempt $count/$max_wait: still waiting..."
        sleep 2
    done

    echo "❌ PostgreSQL container $container_name failed to become ready within $max_wait attempts"
    return 1
}

# Function to cleanup existing containers
cleanup_containers() {
    echo "🧹 Cleaning up existing containers..."

    # Stop and remove containers if they exist
    for container in "$POSTGRES_CONTAINER" "$VECTOR_CONTAINER"; do
        if check_container_running "$container"; then
            echo "  Stopping $container..."
            podman stop "$container" >/dev/null 2>&1 || true
        fi

        if podman container exists "$container" 2>/dev/null; then
            echo "  Removing $container..."
            podman rm "$container" >/dev/null 2>&1 || true
        fi
    done

    # Remove podman volumes
    for volume in "$POSTGRES_VOLUME" "$VECTOR_VOLUME"; do
        if podman volume exists "$volume" 2>/dev/null; then
            echo "  Removing volume $volume..."
            podman volume rm "$volume" >/dev/null 2>&1 || true
        fi
    done

    # Remove network if it exists
    if podman network exists "$NETWORK_NAME" 2>/dev/null; then
        echo "  Removing network $NETWORK_NAME..."
        podman network rm "$NETWORK_NAME" >/dev/null 2>&1 || true
    fi
}

# Function to create network
create_network() {
    echo "🔗 Creating test network: $NETWORK_NAME"
    podman network create "$NETWORK_NAME" >/dev/null 2>&1 || {
        if podman network exists "$NETWORK_NAME" 2>/dev/null; then
            echo "  Network $NETWORK_NAME already exists, continuing..."
        else
            echo "❌ Failed to create network $NETWORK_NAME"
            exit 1
        fi
    }
}

# Function to create PostgreSQL containers
create_postgres_containers() {
    echo "🗄️  Creating PostgreSQL containers..."

    # Create podman volumes (more reliable than bind mounts)
    echo "  Creating podman volumes..."
    podman volume create "$POSTGRES_VOLUME" >/dev/null 2>&1 || {
        if podman volume exists "$POSTGRES_VOLUME" 2>/dev/null; then
            echo "  Volume $POSTGRES_VOLUME already exists"
        else
            echo "❌ Failed to create PostgreSQL volume: $POSTGRES_VOLUME"
            exit 1
        fi
    }

    podman volume create "$VECTOR_VOLUME" >/dev/null 2>&1 || {
        if podman volume exists "$VECTOR_VOLUME" 2>/dev/null; then
            echo "  Volume $VECTOR_VOLUME already exists"
        else
            echo "❌ Failed to create Vector volume: $VECTOR_VOLUME"
            exit 1
        fi
    }

    echo "  ✅ Podman volumes created successfully"

    # Primary PostgreSQL container (for main application data)
    echo "  Creating primary PostgreSQL container ($POSTGRES_CONTAINER)..."
    podman run -d \
        --name "$POSTGRES_CONTAINER" \
        --network "$NETWORK_NAME" \
        -p "$POSTGRES_PORT:5432" \
        -e POSTGRES_DB="$POSTGRES_DB" \
        -e POSTGRES_USER="$POSTGRES_USER" \
        -e POSTGRES_PASSWORD="$POSTGRES_PASSWORD" \
        -e POSTGRES_INITDB_ARGS="--auth-host=scram-sha-256 --auth-local=scram-sha-256" \
        -v "$POSTGRES_VOLUME:/var/lib/postgresql/data" \
        --health-cmd='pg_isready -U $POSTGRES_USER -d $POSTGRES_DB' \
        --health-interval=10s \
        --health-timeout=5s \
        --health-retries=5 \
        docker.io/postgres:15-alpine

    # Vector PostgreSQL container (for vector database operations)
    echo "  Creating vector PostgreSQL container ($VECTOR_CONTAINER)..."
    podman run -d \
        --name "$VECTOR_CONTAINER" \
        --network "$NETWORK_NAME" \
        -p "$VECTOR_PORT:5432" \
        -e POSTGRES_DB="$VECTOR_DB" \
        -e POSTGRES_USER="$VECTOR_USER" \
        -e POSTGRES_PASSWORD="$VECTOR_PASSWORD" \
        -e POSTGRES_INITDB_ARGS="--auth-host=scram-sha-256 --auth-local=scram-sha-256" \
        -v "$VECTOR_VOLUME:/var/lib/postgresql/data" \
        --health-cmd='pg_isready -U $VECTOR_USER -d $VECTOR_DB' \
        --health-interval=10s \
        --health-timeout=5s \
        --health-retries=5 \
        pgvector/pgvector:pg15  # This image includes pgvector extension

    echo "✅ PostgreSQL containers created"
}

# Function to wait for containers to be healthy
wait_for_containers() {
    echo "⏳ Waiting for containers to be healthy..."

    # Wait for primary PostgreSQL
    wait_for_postgres "$POSTGRES_CONTAINER" "$POSTGRES_USER" "$POSTGRES_DB" 30

    # Wait for vector PostgreSQL
    wait_for_postgres "$VECTOR_CONTAINER" "$VECTOR_USER" "$VECTOR_DB" 30

    echo "✅ All containers are healthy and ready"
}

# Function to bootstrap databases
bootstrap_databases() {
    echo "🏗️  Bootstrapping databases with schema and test data..."

    # Bootstrap primary database
    echo "  Bootstrapping primary database..."
    MIGRATIONS_DIR="/Users/jgil/go/src/github.com/jordigilh/kubernaut/migrations"

    if [ ! -d "$MIGRATIONS_DIR" ]; then
        echo "❌ Migrations directory not found: $MIGRATIONS_DIR"
        exit 1
    fi

    # Apply migrations to primary database
    for migration_file in $(ls "$MIGRATIONS_DIR"/*.sql | sort); do
        migration_name=$(basename "$migration_file")
        echo "    Applying $migration_name to primary DB..."

        if podman exec -i -e PGPASSWORD="$POSTGRES_PASSWORD" "$POSTGRES_CONTAINER" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" < "$migration_file" >/dev/null 2>&1; then
            echo "    ✅ $migration_name applied successfully"
        else
            echo "    ⚠️  $migration_name failed (may already exist, continuing...)"
        fi
    done

    # Bootstrap vector database (only vector-related migrations)
    echo "  Bootstrapping vector database..."

    # Enable pgvector extension
    echo "    Enabling pgvector extension..."
    podman exec -e PGPASSWORD="$VECTOR_PASSWORD" "$VECTOR_CONTAINER" psql -U "$VECTOR_USER" -d "$VECTOR_DB" -c "CREATE EXTENSION IF NOT EXISTS vector;" >/dev/null 2>&1

    # Apply vector-specific migrations
    if [ -f "$MIGRATIONS_DIR/005_vector_schema.sql" ]; then
        echo "    Applying vector schema..."
        if podman exec -i -e PGPASSWORD="$VECTOR_PASSWORD" "$VECTOR_CONTAINER" psql -U "$VECTOR_USER" -d "$VECTOR_DB" < "$MIGRATIONS_DIR/005_vector_schema.sql" >/dev/null 2>&1; then
            echo "    ✅ Vector schema applied successfully"
        else
            echo "    ⚠️  Vector schema failed (may already exist, continuing...)"
        fi
    fi

    # Insert test data for validation
    echo "  Inserting test data..."

    # Test data for primary database
    podman exec -i -e PGPASSWORD="$POSTGRES_PASSWORD" "$POSTGRES_CONTAINER" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" << 'EOF'
-- Insert test resource reference
INSERT INTO resource_references (resource_uid, api_version, kind, name, namespace)
VALUES ('test-resource-uid-001', 'apps/v1', 'Deployment', 'test-webapp', 'default')
ON CONFLICT (resource_uid) DO NOTHING;

-- Insert test action history
INSERT INTO action_histories (resource_id, max_actions, total_actions)
SELECT id, 1000, 5 FROM resource_references WHERE resource_uid = 'test-resource-uid-001'
ON CONFLICT (resource_id) DO NOTHING;

-- Insert test oscillation pattern
INSERT INTO oscillation_patterns (pattern_type, pattern_name, description, min_occurrences, time_window_minutes)
VALUES ('test-pattern', 'Test Pattern', 'Test pattern for validation', 2, 60)
ON CONFLICT DO NOTHING;
EOF

    # Test data for vector database
    podman exec -i -e PGPASSWORD="$VECTOR_PASSWORD" "$VECTOR_CONTAINER" psql -U "$VECTOR_USER" -d "$VECTOR_DB" << 'EOF'
-- Insert test action pattern (with random vector for testing)
INSERT INTO action_patterns (
    id, action_type, alert_name, alert_severity, namespace, resource_type, resource_name,
    action_parameters, context_labels, embedding
) VALUES (
    'test-pattern-001',
    'scale_deployment',
    'HighMemoryUsage',
    'warning',
    'default',
    'Deployment',
    'test-webapp',
    '{"replicas": 3}',
    '{"app": "test"}',
    ARRAY(SELECT random() FROM generate_series(1, 384))::vector
) ON CONFLICT (id) DO NOTHING;
EOF

    echo "✅ Database bootstrapping completed"
}

# Function to verify setup
verify_setup() {
    echo "🔍 Verifying container setup..."

    # Check container status
    echo "  Container status:"
    for container in "$POSTGRES_CONTAINER" "$VECTOR_CONTAINER"; do
        if check_container_running "$container"; then
            health_status=$(podman inspect "$container" --format='{{.State.Health.Status}}' 2>/dev/null || echo "unknown")
            echo "    ✅ $container: running (health: $health_status)"
        else
            echo "    ❌ $container: not running"
            exit 1
        fi
    done

    # Test database connections
    echo "  Database connections:"

    # Test primary database
    if podman exec -e PGPASSWORD="$POSTGRES_PASSWORD" "$POSTGRES_CONTAINER" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT COUNT(*) FROM resource_references;" >/dev/null 2>&1; then
        resource_count=$(podman exec -e PGPASSWORD="$POSTGRES_PASSWORD" "$POSTGRES_CONTAINER" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c "SELECT COUNT(*) FROM resource_references;" | xargs)
        echo "    ✅ Primary DB: connected (resources: $resource_count)"
    else
        echo "    ❌ Primary DB: connection failed"
        exit 1
    fi

    # Test vector database
    if podman exec -e PGPASSWORD="$VECTOR_PASSWORD" "$VECTOR_CONTAINER" psql -U "$VECTOR_USER" -d "$VECTOR_DB" -c "SELECT COUNT(*) FROM action_patterns;" >/dev/null 2>&1; then
        pattern_count=$(podman exec -e PGPASSWORD="$VECTOR_PASSWORD" "$VECTOR_CONTAINER" psql -U "$VECTOR_USER" -d "$VECTOR_DB" -t -c "SELECT COUNT(*) FROM action_patterns;" | xargs)
        echo "    ✅ Vector DB: connected (patterns: $pattern_count)"
    else
        echo "    ❌ Vector DB: connection failed"
        exit 1
    fi

    # Test pgvector extension
    if podman exec -e PGPASSWORD="$VECTOR_PASSWORD" "$VECTOR_CONTAINER" psql -U "$VECTOR_USER" -d "$VECTOR_DB" -c "SELECT extname FROM pg_extension WHERE extname='vector';" | grep -q vector; then
        echo "    ✅ pgvector extension: available"
    else
        echo "    ❌ pgvector extension: not available"
        exit 1
    fi
}

# Function to generate connection info
generate_connection_info() {
    cat > /tmp/kubernaut-container-connections.env << EOF
# Kubernaut Test Container Connection Information
# Generated: $(date -Iseconds)

# Primary PostgreSQL (for main application data)
export TEST_POSTGRES_HOST="localhost"
export TEST_POSTGRES_PORT="$POSTGRES_PORT"
export TEST_POSTGRES_DB="$POSTGRES_DB"
export TEST_POSTGRES_USER="$POSTGRES_USER"
export TEST_POSTGRES_PASSWORD="$POSTGRES_PASSWORD"

# Vector PostgreSQL (for vector database operations)
export TEST_VECTOR_HOST="localhost"
export TEST_VECTOR_PORT="$VECTOR_PORT"
export TEST_VECTOR_DB="$VECTOR_DB"
export TEST_VECTOR_USER="$VECTOR_USER"
export TEST_VECTOR_PASSWORD="$VECTOR_PASSWORD"

# Container Management
export POSTGRES_CONTAINER_NAME="$POSTGRES_CONTAINER"
export VECTOR_CONTAINER_NAME="$VECTOR_CONTAINER"
export TEST_NETWORK_NAME="$NETWORK_NAME"

# Usage Examples:
# Source this file: source /tmp/kubernaut-container-connections.env
# Connect to primary: PGPASSWORD=\$TEST_POSTGRES_PASSWORD psql -h \$TEST_POSTGRES_HOST -p \$TEST_POSTGRES_PORT -U \$TEST_POSTGRES_USER -d \$TEST_POSTGRES_DB
# Connect to vector: PGPASSWORD=\$TEST_VECTOR_PASSWORD psql -h \$TEST_VECTOR_HOST -p \$TEST_VECTOR_PORT -U \$TEST_VECTOR_USER -d \$TEST_VECTOR_DB
EOF

    echo "📄 Connection information saved to: /tmp/kubernaut-container-connections.env"
}

# Function to show usage information
show_usage_info() {
    echo ""
    echo "🎉 Container Setup Complete!"
    echo "============================"
    echo ""
    echo "📋 Available Services:"
    echo "  • Primary PostgreSQL: localhost:$POSTGRES_PORT/$POSTGRES_DB"
    echo "  • Vector PostgreSQL:  localhost:$VECTOR_PORT/$VECTOR_DB"
    echo ""
    echo "🔗 Connection Information:"
    echo "  source /tmp/kubernaut-container-connections.env"
    echo ""
    echo "🧪 Run Validation Tests:"
    echo "  cd /Users/jgil/go/src/github.com/jordigilh/kubernaut"
    echo "  source /tmp/kubernaut-container-connections.env"
    echo "  ./scripts/validate-milestone1-with-containers.sh"
    echo ""
    echo "🧹 Cleanup Commands:"
    echo "  podman stop $POSTGRES_CONTAINER $VECTOR_CONTAINER"
    echo "  podman rm $POSTGRES_CONTAINER $VECTOR_CONTAINER"
    echo "  podman network rm $NETWORK_NAME"
    echo "  rm -rf $POSTGRES_DATA_DIR $VECTOR_DATA_DIR"
    echo ""
    echo "📊 Container Status:"
    podman ps --filter "name=$POSTGRES_CONTAINER" --filter "name=$VECTOR_CONTAINER" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
    echo ""
}

# Main execution
main() {
    echo "Starting container setup process..."
    echo ""

    # Parse command line arguments
    FORCE_CLEANUP=false
    if [ "$1" = "--force" ] || [ "$1" = "-f" ]; then
        FORCE_CLEANUP=true
        echo "⚠️  Force cleanup mode enabled"
    fi

    # Check if podman is available
    if ! command -v podman >/dev/null 2>&1; then
        echo "❌ podman is not installed or not available in PATH"
        echo "   Please install podman first: https://podman.io/getting-started/installation"
        exit 1
    fi

    # Cleanup existing containers if they exist or if force mode
    if $FORCE_CLEANUP || check_container_running "$POSTGRES_CONTAINER" || check_container_running "$VECTOR_CONTAINER"; then
        cleanup_containers
    fi

    # Create network
    create_network

    # Create containers
    create_postgres_containers

    # Wait for containers to be ready
    wait_for_containers

    # Bootstrap databases
    bootstrap_databases

    # Verify setup
    verify_setup

    # Generate connection info
    generate_connection_info

    # Show usage information
    show_usage_info
}

# Run main function with all arguments
main "$@"
