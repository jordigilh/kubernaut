#!/usr/bin/env bash
# Integration Test Dependencies Health Check
# Comprehensive health validation for all external dependencies

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
HEALTH_CHECK_TIMEOUT=30
VERBOSE=${VERBOSE:-false}
OVERALL_HEALTH=true

# Dependency endpoints
POSTGRES_HOST=${POSTGRES_HOST:-localhost}
POSTGRES_PORT=${POSTGRES_PORT:-5433}
POSTGRES_DB=${POSTGRES_DB:-action_history}
POSTGRES_USER=${POSTGRES_USER:-slm_user}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-slm_password_dev}

VECTOR_DB_HOST=${VECTOR_DB_HOST:-localhost}
VECTOR_DB_PORT=${VECTOR_DB_PORT:-5434}
VECTOR_DB_NAME=${VECTOR_DB_NAME:-vector_store}
VECTOR_DB_USER=${VECTOR_DB_USER:-vector_user}
VECTOR_DB_PASSWORD=${VECTOR_DB_PASSWORD:-vector_password_dev}

REDIS_HOST=${REDIS_HOST:-localhost}
REDIS_PORT=${REDIS_PORT:-6380}
REDIS_PASSWORD=${REDIS_PASSWORD:-integration_redis_password}

CONTEXT_API_URL=${CONTEXT_API_URL:-http://localhost:8091}
HOLMESGPT_API_URL=${HOLMESGPT_API_URL:-http://localhost:3000}
LLM_ENDPOINT=${LLM_ENDPOINT:-http://localhost:8010}

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

log_verbose() {
    if [[ "$VERBOSE" == "true" ]]; then
        echo -e "${BLUE}[VERBOSE]${NC} $1"
    fi
}

# Health check functions
check_postgres() {
    local service_name="PostgreSQL (Action History)"
    log_info "Checking $service_name..."

    if command -v psql >/dev/null 2>&1; then
        if PGPASSWORD="$POSTGRES_PASSWORD" timeout $HEALTH_CHECK_TIMEOUT psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c 'SELECT 1;' >/dev/null 2>&1; then
            log_success "$service_name is healthy"
            log_verbose "Connected to $POSTGRES_HOST:$POSTGRES_PORT/$POSTGRES_DB"

            # Check pgvector extension
            if PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT * FROM pg_extension WHERE extname = 'vector';" -t 2>/dev/null | grep -q vector; then
                log_verbose "pgvector extension is available"
            else
                log_warning "pgvector extension not found in action history database"
            fi
            return 0
        else
            log_error "$service_name is unhealthy - Failed to connect to $POSTGRES_HOST:$POSTGRES_PORT/$POSTGRES_DB"
            OVERALL_HEALTH=false
            return 1
        fi
    else
        log_warning "$service_name check skipped (psql not available)"
        return 0
    fi
}

check_vector_db() {
    local service_name="Vector Database (PostgreSQL)"
    log_info "Checking $service_name..."

    if command -v psql >/dev/null 2>&1; then
        if PGPASSWORD="$VECTOR_DB_PASSWORD" timeout $HEALTH_CHECK_TIMEOUT psql -h "$VECTOR_DB_HOST" -p "$VECTOR_DB_PORT" -U "$VECTOR_DB_USER" -d "$VECTOR_DB_NAME" -c 'SELECT 1;' >/dev/null 2>&1; then
            log_success "$service_name is healthy"
            log_verbose "Connected to $VECTOR_DB_HOST:$VECTOR_DB_PORT/$VECTOR_DB_NAME"

            # Check pgvector extension
            if PGPASSWORD="$VECTOR_DB_PASSWORD" psql -h "$VECTOR_DB_HOST" -p "$VECTOR_DB_PORT" -U "$VECTOR_DB_USER" -d "$VECTOR_DB_NAME" -c "SELECT * FROM pg_extension WHERE extname = 'vector';" -t 2>/dev/null | grep -q vector; then
                log_verbose "pgvector extension is available in vector database"
            else
                log_warning "pgvector extension not found in vector database"
            fi
            return 0
        else
            log_error "$service_name is unhealthy - Failed to connect to $VECTOR_DB_HOST:$VECTOR_DB_PORT/$VECTOR_DB_NAME"
            OVERALL_HEALTH=false
            return 1
        fi
    else
        log_warning "$service_name check skipped (psql not available)"
        return 0
    fi
}

check_redis() {
    local service_name="Redis Cache"
    log_info "Checking $service_name..."

    if command -v redis-cli >/dev/null 2>&1; then
        if timeout $HEALTH_CHECK_TIMEOUT redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -a "$REDIS_PASSWORD" --no-auth-warning ping 2>/dev/null | grep -q PONG; then
            log_success "$service_name is healthy"
            log_verbose "Connected to $REDIS_HOST:$REDIS_PORT"

            # Get Redis info
            local redis_info=$(timeout 5 redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -a "$REDIS_PASSWORD" --no-auth-warning info server 2>/dev/null | grep redis_version || echo "")
            if [[ -n "$redis_info" ]]; then
                log_verbose "Redis version: $redis_info"
            fi
            return 0
        else
            log_error "$service_name is unhealthy - Failed to ping $REDIS_HOST:$REDIS_PORT"
            OVERALL_HEALTH=false
            return 1
        fi
    else
        log_warning "$service_name check skipped (redis-cli not available)"
        # Test if Redis port is open
        if timeout 5 bash -c "echo > /dev/tcp/$REDIS_HOST/$REDIS_PORT" 2>/dev/null; then
            log_warning "Port $REDIS_HOST:$REDIS_PORT is accessible but cannot perform full health check"
            return 0
        else
            log_error "$service_name is unhealthy - Port $REDIS_HOST:$REDIS_PORT is not accessible"
            OVERALL_HEALTH=false
            return 1
        fi
    fi
}

check_http_endpoint() {
    local service_name="$1"
    local endpoint="$2"

    log_info "Checking $service_name..."

    if command -v curl >/dev/null 2>&1; then
        if timeout $HEALTH_CHECK_TIMEOUT curl -s -f "$endpoint" >/dev/null 2>&1; then
            log_success "$service_name is healthy"
            log_verbose "Health endpoint accessible at $endpoint"
            return 0
        else
            # Try basic connectivity
            local host=$(echo "$endpoint" | sed 's|http[s]*://||' | cut -d':' -f1 | cut -d'/' -f1)
            local port=$(echo "$endpoint" | sed 's|http[s]*://||' | cut -d':' -f2 | cut -d'/' -f1)

            if [[ "$port" == "$host" ]]; then
                port="80"
                if echo "$endpoint" | grep -q "https://"; then
                    port="443"
                fi
            fi

            if timeout 5 bash -c "echo > /dev/tcp/$host/$port" 2>/dev/null; then
                log_warning "$service_name port is accessible but health endpoint failed"
                log_verbose "Port $host:$port accessible but $endpoint failed"
                return 0
            else
                log_error "$service_name is unhealthy - Not accessible at $endpoint"
                OVERALL_HEALTH=false
                return 1
            fi
        fi
    else
        log_warning "$service_name check skipped (curl not available)"
        return 0
    fi
}

check_context_api() {
    check_http_endpoint "Context API (Kubernaut)" "$CONTEXT_API_URL/api/v1/context/health"
}

check_holmesgpt_api() {
    check_http_endpoint "HolmesGPT API" "$HOLMESGPT_API_URL/health"
}

check_llm_endpoint() {
    check_http_endpoint "LLM Service" "$LLM_ENDPOINT/health"
}

check_kubernetes_access() {
    local service_name="Kubernetes Cluster"
    log_info "Checking $service_name..."

    if command -v kubectl >/dev/null 2>&1; then
        if timeout $HEALTH_CHECK_TIMEOUT kubectl cluster-info >/dev/null 2>&1; then
            log_success "$service_name is healthy"
            local cluster_info=$(kubectl config current-context 2>/dev/null || echo "unknown")
            log_verbose "Cluster accessible (context: $cluster_info)"

            # Check if it's a Kind cluster
            if echo "$cluster_info" | grep -q "kind"; then
                log_verbose "Kind cluster detected: $cluster_info"
            fi
            return 0
        else
            log_error "$service_name is unhealthy - Cluster not accessible or kubectl not configured"
            OVERALL_HEALTH=false
            return 1
        fi
    else
        log_warning "$service_name check skipped (kubectl not available)"
        return 0
    fi
}

check_container_services() {
    local service_name="Container Services"
    log_info "Checking $service_name..."

    if command -v podman >/dev/null 2>&1; then
        local containers=$(podman ps --format "{{.Names}}" 2>/dev/null | grep kubernaut || true)
        if [[ -n "$containers" ]]; then
            local container_count=$(echo "$containers" | wc -l)
            log_success "$service_name: $container_count containers running"

            if [[ "$VERBOSE" == "true" ]]; then
                echo "$containers" | while read -r container; do
                    local status=$(podman ps --filter "name=$container" --format "{{.Status}}" 2>/dev/null || echo "unknown")
                    log_verbose "Container $container: $status"
                done
            fi
            return 0
        else
            log_warning "$service_name: No kubernaut containers found"
            return 0
        fi
    elif command -v docker >/dev/null 2>&1; then
        local containers=$(docker ps --format "{{.Names}}" 2>/dev/null | grep kubernaut || true)
        if [[ -n "$containers" ]]; then
            local container_count=$(echo "$containers" | wc -l)
            log_success "$service_name: $container_count containers running"
            return 0
        else
            log_warning "$service_name: No kubernaut containers found"
            return 0
        fi
    else
        log_warning "$service_name check skipped (no container runtime available)"
        return 0
    fi
}

# Generate health report
generate_health_report() {
    echo
    echo "========================================"
    echo "Integration Test Dependencies Health Report"
    echo "========================================"
    echo

    # Overall status
    if [[ "$OVERALL_HEALTH" == "true" ]]; then
        log_success "Overall Health: HEALTHY - All critical dependencies are accessible"
        echo
        echo "✅ Integration tests can proceed"
        echo
        echo "To run integration tests:"
        echo "  LLM_ENDPOINT=$LLM_ENDPOINT make test-integration-dev"
        echo "  # or"
        echo "  LLM_ENDPOINT=$LLM_ENDPOINT go test -v -tags=integration ./test/integration/..."
        return 0
    else
        log_error "Overall Health: UNHEALTHY - Some critical dependencies are not accessible"
        echo
        echo "❌ Integration tests may fail due to dependency issues"
        echo
        echo "Recommended actions:"
        echo "1. Run 'make bootstrap-dev' to start all services"
        echo "2. Check individual service logs for specific issues"
        echo "3. Verify network connectivity and firewall settings"
        echo "4. Ensure LLM service is running at $LLM_ENDPOINT"
        return 1
    fi
}

# Help function
show_help() {
    cat << EOF
Kubernaut Integration Test Dependencies Health Check

USAGE:
    $0 [OPTIONS]

OPTIONS:
    -v, --verbose           Enable verbose output
    -h, --help             Show this help message
    --timeout SECONDS      Set health check timeout (default: 30)
    --llm-endpoint URL     LLM service endpoint (default: http://localhost:8010)

ENVIRONMENT VARIABLES:
    POSTGRES_HOST, POSTGRES_PORT, POSTGRES_DB, POSTGRES_USER, POSTGRES_PASSWORD
    VECTOR_DB_HOST, VECTOR_DB_PORT, VECTOR_DB_NAME, VECTOR_DB_USER, VECTOR_DB_PASSWORD
    REDIS_HOST, REDIS_PORT, REDIS_PASSWORD
    CONTEXT_API_URL, HOLMESGPT_API_URL, LLM_ENDPOINT
    VERBOSE

EXAMPLES:
    # Basic health check
    $0

    # Verbose output
    $0 --verbose

    # Custom LLM endpoint
    $0 --llm-endpoint http://localhost:8010

    # With environment variables
    LLM_ENDPOINT=http://localhost:8010 VERBOSE=true $0

EOF
}

# Main execution
main() {
    echo "🔍 Kubernaut Integration Test Dependencies Health Check"
    echo "======================================================"
    echo

    # Run all health checks
    check_postgres
    check_vector_db
    check_redis
    check_context_api
    check_holmesgpt_api
    check_llm_endpoint
    check_kubernetes_access
    check_container_services

    # Generate report
    generate_health_report
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        --timeout)
            HEALTH_CHECK_TIMEOUT="$2"
            shift 2
            ;;
        --llm-endpoint)
            LLM_ENDPOINT="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Run main function
main

