#!/bin/bash

# Integration Test Bootstrap Script
# Manages containerized PostgreSQL and Redis instances for integration testing

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../../" && pwd)"
COMPOSE_FILE="${SCRIPT_DIR}/../docker-compose.integration.yml"
POSTGRES_CONTAINER="kubernaut-integration-postgres"
REDIS_CONTAINER="kubernaut-integration-redis"

# Connection details
POSTGRES_HOST="localhost"
POSTGRES_PORT="5433"
POSTGRES_DB="action_history"
POSTGRES_USER="slm_user"
POSTGRES_PASSWORD="slm_password_dev"

REDIS_HOST="localhost"
REDIS_PORT="6380"
REDIS_PASSWORD="integration_redis_password"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check if podman is available
check_podman() {
    if ! command -v podman &> /dev/null; then
        log_error "Podman is not installed. Please install podman first."
        echo "  macOS: brew install podman"
        echo "  Linux: sudo apt-get install podman (Ubuntu/Debian) or sudo dnf install podman (RHEL/Fedora)"
        exit 1
    fi

    if ! command -v podman-compose &> /dev/null; then
        log_warning "podman-compose is not installed. Attempting to install it..."

        # Try different installation methods
        if command -v pip3 &> /dev/null; then
            log_info "Installing podman-compose via pip3..."
            pip3 install podman-compose || {
                log_warning "pip3 installation failed, trying with --user flag..."
                pip3 install --user podman-compose || {
                    log_error "Failed to install podman-compose via pip3"
                    show_podman_compose_install_help
                    exit 1
                }
            }
        elif command -v pip &> /dev/null; then
            log_info "Installing podman-compose via pip..."
            pip install podman-compose || {
                log_warning "pip installation failed, trying with --user flag..."
                pip install --user podman-compose || {
                    log_error "Failed to install podman-compose via pip"
                    show_podman_compose_install_help
                    exit 1
                }
            }
        else
            log_error "Neither pip3 nor pip is available"
            show_podman_compose_install_help
            exit 1
        fi

        # Verify installation
        if ! command -v podman-compose &> /dev/null; then
            log_error "podman-compose installation succeeded but command not found in PATH"
            show_podman_compose_install_help
            exit 1
        fi
    fi

    log_info "Podman version: $(podman --version)"
    log_info "podman-compose version: $(podman-compose --version)"
}

# Function to show podman-compose installation help
show_podman_compose_install_help() {
    echo ""
    log_error "podman-compose is required but not available"
    echo ""
    echo "📋 Manual installation options:"
    echo ""
    echo "1. Install via pip (recommended):"
    echo "   pip3 install podman-compose"
    echo "   # or"
    echo "   pip3 install --user podman-compose"
    echo ""
    echo "2. Install via package manager:"
    echo "   # macOS (via homebrew)"
    echo "   brew install podman-compose"
    echo ""
    echo "   # Fedora"
    echo "   sudo dnf install podman-compose"
    echo ""
    echo "3. Install from source:"
    echo "   git clone https://github.com/containers/podman-compose.git"
    echo "   cd podman-compose"
    echo "   pip install ."
    echo ""
    echo "4. Alternative: Use docker-compose with podman backend"
    echo "   # If you have docker-compose installed"
    echo "   export DOCKER_HOST=unix:///run/user/\$UID/podman/podman.sock"
    echo ""
    echo "After installation, ensure podman-compose is in your PATH and run this script again."
}

# Function to start Podman machine on macOS
start_podman_machine() {
    if [[ "$OSTYPE" == "darwin"* ]]; then
        log_info "Checking Podman machine status (macOS)..."
        if ! podman machine list 2>/dev/null | grep -q "Currently running"; then
            log_info "Starting Podman machine..."
            podman machine start || {
                log_warning "Podman machine not initialized. Initializing..."
                podman machine init
                podman machine start
            }
        fi
        log_success "Podman machine is running"
    fi
}

# Function to wait for service to be healthy
wait_for_service() {
    local service_name=$1
    local max_attempts=${2:-60}  # Default to 60 attempts (2 minutes)
    local attempt=1

    log_info "Waiting for $service_name to be healthy..."

    while [ $attempt -le $max_attempts ]; do
        # Use podman ps directly instead of podman-compose ps to avoid template issues
        if podman ps --filter name="$service_name" --format "{{.Names}} {{.Status}}" | grep -q "healthy"; then
            log_success "$service_name is healthy"
            return 0
        fi

        echo -n "."
        sleep 2
        ((attempt++))
    done

    log_error "$service_name did not become healthy within expected time"
    return 1
}

# Function to test database connection
test_database_connection() {
    local host=$1
    local port=$2
    local db=$3
    local user=$4
    local password=$5
    local service_name=$6

    log_info "Testing $service_name database connection..."

    max_attempts=30
    attempt=1

    while [ $attempt -le $max_attempts ]; do
        # Test connection using container's psql client to avoid host dependency
        local container_name=""
        if [[ "$service_name" == "PostgreSQL" ]]; then
            container_name="kubernaut-integration-postgres"
        elif [[ "$service_name" == "Vector DB" ]]; then
            container_name="kubernaut-integration-vectordb"
        fi

        if [ -n "$container_name" ]; then
            if podman exec "$container_name" psql -U "$user" -d "$db" -c "SELECT 1;" >/dev/null 2>&1; then
                log_success "$service_name database connection successful"
                return 0
            fi
        else
            # Fallback to host connection if psql is available
            if command -v psql >/dev/null 2>&1; then
                if PGPASSWORD="$password" psql -h "$host" -p "$port" -U "$user" -d "$db" -c "SELECT 1;" >/dev/null 2>&1; then
                    log_success "$service_name database connection successful"
                    return 0
                fi
            else
                # Skip detailed connection test if no psql available
                log_warning "PostgreSQL client not available, skipping detailed connection test"
                log_success "$service_name container is healthy (basic connectivity assumed)"
                return 0
            fi
        fi

        if [ $attempt -eq $max_attempts ]; then
            log_error "$service_name database connection failed after $max_attempts attempts"
            return 1
        fi

        echo -n "."
        sleep 2
        ((attempt++))
    done
}

# Function to run database migrations
run_migrations() {
    log_info "Running database migrations..."

    # Wait a bit for database to be fully ready
    sleep 5

    # Run migrations using the main database
    cd "$PROJECT_ROOT"

    # Set environment variables for migration
    export DB_HOST="$POSTGRES_HOST"
    export DB_PORT="$POSTGRES_PORT"
    export DB_NAME="$POSTGRES_DB"
    export DB_USER="$POSTGRES_USER"
    export DB_PASSWORD="$POSTGRES_PASSWORD"
    export DB_SSL_MODE="disable"

    # Run migration script if it exists
    if [ -f "scripts/migrate-database.sh" ]; then
        log_info "Running migration script..."
        bash scripts/migrate-database.sh || {
            log_warning "Migration script failed, attempting direct migration..."
            # Fallback: run migrations directly using container's psql
            for migration_file in migrations/*.sql; do
                if [ -f "$migration_file" ]; then
                    log_info "Applying migration: $(basename "$migration_file")"
                    # Copy migration file to container and run it
                    podman cp "$migration_file" kubernaut-integration-postgres:/tmp/$(basename "$migration_file") && \
                    podman exec kubernaut-integration-postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f "/tmp/$(basename "$migration_file")" || {
                        log_warning "Migration $(basename "$migration_file") failed or was already applied"
                    }
                    # Clean up the copied file
                    podman exec kubernaut-integration-postgres rm -f "/tmp/$(basename "$migration_file")" 2>/dev/null || true
                fi
            done
        }
    else
        # Direct migration using container's psql
        log_info "Applying database migrations directly using container client..."
        for migration_file in migrations/*.sql; do
            if [ -f "$migration_file" ]; then
                log_info "Applying migration: $(basename "$migration_file")"
                # Copy migration file to container and run it
                podman cp "$migration_file" kubernaut-integration-postgres:/tmp/$(basename "$migration_file") && \
                podman exec kubernaut-integration-postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f "/tmp/$(basename "$migration_file")" || {
                    log_warning "Migration $(basename "$migration_file") failed or was already applied"
                }
                # Clean up the copied file
                podman exec kubernaut-integration-postgres rm -f "/tmp/$(basename "$migration_file")" 2>/dev/null || true
            fi
        done
    fi

    log_success "Database migrations completed"
}

# Function to verify extensions and test data
verify_setup() {
    log_info "Verifying database setup..."

    # Test main database
    PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
        SELECT
            'Main DB: action_patterns table' as component,
            CASE WHEN EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'action_patterns')
                 THEN 'EXISTS' ELSE 'MISSING' END as status
        UNION ALL
        SELECT
            'Main DB: vector extension' as component,
            CASE WHEN EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector')
                 THEN 'INSTALLED' ELSE 'MISSING' END as status;
    " || log_warning "Main database verification failed"

    log_success "Database setup verification completed"
}

# Function to start integration test services
start_services() {
    log_info "Starting integration test services..."

    check_podman
    start_podman_machine

    # Stop any existing containers
    log_info "Stopping any existing integration test containers..."
    podman-compose -f "$COMPOSE_FILE" down 2>/dev/null || true

    # Start services
    log_info "Starting services with compose file: $COMPOSE_FILE"
    podman-compose -f "$COMPOSE_FILE" up -d

    # Wait for services to be healthy
    wait_for_service "kubernaut-integration-postgres" 90
    wait_for_service "kubernaut-integration-redis" 30

    # Test database connections
    test_database_connection "$POSTGRES_HOST" "$POSTGRES_PORT" "$POSTGRES_DB" "$POSTGRES_USER" "$POSTGRES_PASSWORD" "PostgreSQL"

    # Run migrations
    run_migrations

    # Verify setup
    verify_setup

    log_success "🎉 Integration test services are ready!"
}

# Function to stop integration test services
stop_services() {
    log_info "Stopping integration test services..."
    podman-compose -f "$COMPOSE_FILE" down
    log_success "Integration test services stopped"
}

# Function to show service status
status_services() {
    log_info "Integration test services status:"
    podman-compose -f "$COMPOSE_FILE" ps
}

# Function to show logs
show_logs() {
    local service=${1:-}
    if [ -n "$service" ]; then
        podman-compose -f "$COMPOSE_FILE" logs -f "$service"
    else
        podman-compose -f "$COMPOSE_FILE" logs -f
    fi
}

# Function to display connection information
show_connection_info() {
    echo ""
    log_success "🎉 Integration Test Services Ready!"
    echo ""
    echo "📋 Database Connection Details:"
    echo ""
    echo "🗃️  Main PostgreSQL Database:"
    echo "   Host: $POSTGRES_HOST"
    echo "   Port: $POSTGRES_PORT"
    echo "   Database: $POSTGRES_DB"
    echo "   Username: $POSTGRES_USER"
    echo "   Password: $POSTGRES_PASSWORD"
    echo "   Connection: postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@$POSTGRES_HOST:$POSTGRES_PORT/$POSTGRES_DB?sslmode=disable"
    echo ""
    echo "🔄 Redis Cache:"
    echo "   Host: $REDIS_HOST"
    echo "   Port: $REDIS_PORT"
    echo "   Password: $REDIS_PASSWORD"
    echo ""
    echo "🔧 Environment Variables for Integration Tests:"
    echo "   export DB_HOST=$POSTGRES_HOST"
    echo "   export DB_PORT=$POSTGRES_PORT"
    echo "   export DB_NAME=$POSTGRES_DB"
    echo "   export DB_USER=$POSTGRES_USER"
    echo "   export DB_PASSWORD=$POSTGRES_PASSWORD"
    echo "   export DB_SSL_MODE=disable"
    echo ""
    echo "🚀 Run Integration Tests:"
    echo "   go test ./test/integration/... -tags=integration -v"
    echo ""
    echo "🔍 Service Management:"
    echo "   Status: $0 status"
    echo "   Logs:   $0 logs [service]"
    echo "   Stop:   $0 stop"
    echo ""
}

# Main script logic
case "${1:-start}" in
    start)
        start_services
        show_connection_info
        ;;
    stop)
        stop_services
        ;;
    status)
        status_services
        ;;
    logs)
        show_logs "${2:-}"
        ;;
    restart)
        stop_services
        start_services
        show_connection_info
        ;;
    *)
        echo "Usage: $0 {start|stop|status|logs [service]|restart}"
        echo ""
        echo "Commands:"
        echo "  start    - Start integration test services (default)"
        echo "  stop     - Stop integration test services"
        echo "  status   - Show service status"
        echo "  logs     - Show logs for all services or specific service"
        echo "  restart  - Restart all integration test services"
        exit 1
        ;;
esac
