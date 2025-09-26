#!/usr/bin/env bash
# Integration Test Dependencies Health Check
# Comprehensive health validation for all external dependencies

set -e

# Check if we're running with bash
if [ -z "$BASH_VERSION" ]; then
    echo "Error: This script requires bash (associative arrays)"
    echo "Please run with: bash $0"
    exit 1
fi

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HEALTH_CHECK_TIMEOUT=30
VERBOSE=${VERBOSE:-false}

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

# Health check results
declare -A HEALTH_STATUS
declare -A HEALTH_DETAILS
OVERALL_HEALTH=true

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
    
    local health_cmd="PGPASSWORD='$POSTGRES_PASSWORD' psql -h $POSTGRES_HOST -p $POSTGRES_PORT -U $POSTGRES_USER -d $POSTGRES_DB -c 'SELECT 1;' -t"
    
    if timeout $HEALTH_CHECK_TIMEOUT bash -c "$health_cmd" >/dev/null 2>&1; then
        HEALTH_STATUS["postgres"]="healthy"
        HEALTH_DETAILS["postgres"]="Connected to $POSTGRES_HOST:$POSTGRES_PORT/$POSTGRES_DB"
        log_success "$service_name is healthy"
        
        # Check pgvector extension
        local vector_check="PGPASSWORD='$POSTGRES_PASSWORD' psql -h $POSTGRES_HOST -p $POSTGRES_PORT -U $POSTGRES_USER -d $POSTGRES_DB -c 'SELECT * FROM pg_extension WHERE extname = '\''vector'\'';' -t"
        if timeout 10 bash -c "$vector_check" | grep -q vector; then
            HEALTH_DETAILS["postgres"]+=" (pgvector enabled)"
            log_verbose "pgvector extension is available"
        else
            log_warning "pgvector extension not found in action history database"
        fi
    else
        HEALTH_STATUS["postgres"]="unhealthy"
        HEALTH_DETAILS["postgres"]="Failed to connect to $POSTGRES_HOST:$POSTGRES_PORT/$POSTGRES_DB"
        log_error "$service_name is unhealthy"
        OVERALL_HEALTH=false
    fi
}

check_vector_db() {
    local service_name="Vector Database (PostgreSQL)"
    log_info "Checking $service_name..."
    
    local health_cmd="PGPASSWORD='$VECTOR_DB_PASSWORD' psql -h $VECTOR_DB_HOST -p $VECTOR_DB_PORT -U $VECTOR_DB_USER -d $VECTOR_DB_NAME -c 'SELECT 1;' -t"
    
    if timeout $HEALTH_CHECK_TIMEOUT bash -c "$health_cmd" >/dev/null 2>&1; then
        HEALTH_STATUS["vector_db"]="healthy"
        HEALTH_DETAILS["vector_db"]="Connected to $VECTOR_DB_HOST:$VECTOR_DB_PORT/$VECTOR_DB_NAME"
        log_success "$service_name is healthy"
        
        # Check pgvector extension
        local vector_check="PGPASSWORD='$VECTOR_DB_PASSWORD' psql -h $VECTOR_DB_HOST -p $VECTOR_DB_PORT -U $VECTOR_DB_USER -d $VECTOR_DB_NAME -c 'SELECT * FROM pg_extension WHERE extname = '\''vector'\'';' -t"
        if timeout 10 bash -c "$vector_check" | grep -q vector; then
            HEALTH_DETAILS["vector_db"]+=" (pgvector enabled)"
            log_verbose "pgvector extension is available in vector database"
        else
            log_warning "pgvector extension not found in vector database"
        fi
        
        # Check for embeddings table
        local table_check="PGPASSWORD='$VECTOR_DB_PASSWORD' psql -h $VECTOR_DB_HOST -p $VECTOR_DB_PORT -U $VECTOR_DB_USER -d $VECTOR_DB_NAME -c '\dt' | grep embeddings"
        if timeout 10 bash -c "$table_check" >/dev/null 2>&1; then
            log_verbose "Embeddings table found in vector database"
        else
            log_verbose "Embeddings table not found (may be created during tests)"
        fi
    else
        HEALTH_STATUS["vector_db"]="unhealthy"
        HEALTH_DETAILS["vector_db"]="Failed to connect to $VECTOR_DB_HOST:$VECTOR_DB_PORT/$VECTOR_DB_NAME"
        log_error "$service_name is unhealthy"
        OVERALL_HEALTH=false
    fi
}

check_redis() {
    local service_name="Redis Cache"
    log_info "Checking $service_name..."
    
    if command -v redis-cli >/dev/null 2>&1; then
        if timeout $HEALTH_CHECK_TIMEOUT redis-cli -h $REDIS_HOST -p $REDIS_PORT -a "$REDIS_PASSWORD" --no-auth-warning ping | grep -q PONG; then
            HEALTH_STATUS["redis"]="healthy"
            HEALTH_DETAILS["redis"]="Connected to $REDIS_HOST:$REDIS_PORT"
            log_success "$service_name is healthy"
            
            # Get Redis info
            local redis_info=$(timeout 5 redis-cli -h $REDIS_HOST -p $REDIS_PORT -a "$REDIS_PASSWORD" --no-auth-warning info server | grep redis_version)
            if [[ -n "$redis_info" ]]; then
                HEALTH_DETAILS["redis"]+=" ($redis_info)"
                log_verbose "Redis version: $redis_info"
            fi
        else
            HEALTH_STATUS["redis"]="unhealthy"
            HEALTH_DETAILS["redis"]="Failed to ping $REDIS_HOST:$REDIS_PORT"
            log_error "$service_name is unhealthy"
            OVERALL_HEALTH=false
        fi
    else
        log_warning "redis-cli not available, testing with curl..."
        # Fallback: test if Redis port is open
        if timeout 5 bash -c "echo > /dev/tcp/$REDIS_HOST/$REDIS_PORT" 2>/dev/null; then
            HEALTH_STATUS["redis"]="partial"
            HEALTH_DETAILS["redis"]="Port $REDIS_HOST:$REDIS_PORT is accessible (redis-cli not available for full test)"
            log_warning "$service_name port is accessible but cannot perform full health check"
        else
            HEALTH_STATUS["redis"]="unhealthy"
            HEALTH_DETAILS["redis"]="Port $REDIS_HOST:$REDIS_PORT is not accessible"
            log_error "$service_name is unhealthy"
            OVERALL_HEALTH=false
        fi
    fi
}

check_context_api() {
    local service_name="Context API (Kubernaut)"
    log_info "Checking $service_name..."
    
    local health_endpoint="$CONTEXT_API_URL/health"
    
    if response=$(timeout $HEALTH_CHECK_TIMEOUT curl -s -f "$health_endpoint" 2>/dev/null); then
        HEALTH_STATUS["context_api"]="healthy"
        HEALTH_DETAILS["context_api"]="Health endpoint accessible at $health_endpoint"
        log_success "$service_name is healthy"
        
        # Parse health response if it's JSON
        if echo "$response" | jq . >/dev/null 2>&1; then
            local status=$(echo "$response" | jq -r '.status // "unknown"')
            HEALTH_DETAILS["context_api"]+=" (status: $status)"
            log_verbose "Context API status: $status"
        fi
    else
        # Try basic connectivity
        if timeout 5 bash -c "echo > /dev/tcp/localhost/8091" 2>/dev/null; then
            HEALTH_STATUS["context_api"]="partial"
            HEALTH_DETAILS["context_api"]="Port accessible but health endpoint failed at $health_endpoint"
            log_warning "$service_name port is accessible but health check failed"
        else
            HEALTH_STATUS["context_api"]="unhealthy"
            HEALTH_DETAILS["context_api"]="Not accessible at $CONTEXT_API_URL"
            log_error "$service_name is unhealthy"
            OVERALL_HEALTH=false
        fi
    fi
}

check_holmesgpt_api() {
    local service_name="HolmesGPT API"
    log_info "Checking $service_name..."
    
    local health_endpoint="$HOLMESGPT_API_URL/health"
    
    if response=$(timeout $HEALTH_CHECK_TIMEOUT curl -s -f "$health_endpoint" 2>/dev/null); then
        HEALTH_STATUS["holmesgpt_api"]="healthy"
        HEALTH_DETAILS["holmesgpt_api"]="Health endpoint accessible at $health_endpoint"
        log_success "$service_name is healthy"
        
        # Parse health response if it's JSON
        if echo "$response" | jq . >/dev/null 2>&1; then
            local status=$(echo "$response" | jq -r '.status // "unknown"')
            HEALTH_DETAILS["holmesgpt_api"]+=" (status: $status)"
            log_verbose "HolmesGPT API status: $status"
        fi
    else
        # Try basic connectivity
        if timeout 5 bash -c "echo > /dev/tcp/localhost/3000" 2>/dev/null; then
            HEALTH_STATUS["holmesgpt_api"]="partial"
            HEALTH_DETAILS["holmesgpt_api"]="Port accessible but health endpoint failed at $health_endpoint"
            log_warning "$service_name port is accessible but health check failed"
        else
            HEALTH_STATUS["holmesgpt_api"]="unhealthy"
            HEALTH_DETAILS["holmesgpt_api"]="Not accessible at $HOLMESGPT_API_URL"
            log_error "$service_name is unhealthy"
            OVERALL_HEALTH=false
        fi
    fi
}

check_llm_endpoint() {
    local service_name="LLM Service"
    log_info "Checking $service_name..."
    
    # Try health endpoint first
    local health_endpoint="$LLM_ENDPOINT/health"
    if response=$(timeout $HEALTH_CHECK_TIMEOUT curl -s -f "$health_endpoint" 2>/dev/null); then
        HEALTH_STATUS["llm_service"]="healthy"
        HEALTH_DETAILS["llm_service"]="Health endpoint accessible at $health_endpoint"
        log_success "$service_name is healthy"
        log_verbose "LLM health response: $response"
    else
        # Try basic connectivity to LLM endpoint
        local llm_host=$(echo "$LLM_ENDPOINT" | sed 's|http[s]*://||' | cut -d':' -f1)
        local llm_port=$(echo "$LLM_ENDPOINT" | sed 's|http[s]*://||' | cut -d':' -f2 | cut -d'/' -f1)
        
        if [[ "$llm_port" == "$llm_host" ]]; then
            llm_port="80"
        fi
        
        if timeout 5 bash -c "echo > /dev/tcp/$llm_host/$llm_port" 2>/dev/null; then
            HEALTH_STATUS["llm_service"]="partial"
            HEALTH_DETAILS["llm_service"]="Port accessible but health endpoint failed at $LLM_ENDPOINT"
            log_warning "$service_name port is accessible but health check failed"
        else
            HEALTH_STATUS["llm_service"]="unhealthy"
            HEALTH_DETAILS["llm_service"]="Not accessible at $LLM_ENDPOINT"
            log_error "$service_name is unhealthy"
            OVERALL_HEALTH=false
        fi
    fi
}

check_kubernetes_access() {
    local service_name="Kubernetes Cluster"
    log_info "Checking $service_name..."
    
    if command -v kubectl >/dev/null 2>&1; then
        if timeout $HEALTH_CHECK_TIMEOUT kubectl cluster-info >/dev/null 2>&1; then
            HEALTH_STATUS["kubernetes"]="healthy"
            local cluster_info=$(kubectl config current-context 2>/dev/null || echo "unknown")
            HEALTH_DETAILS["kubernetes"]="Cluster accessible (context: $cluster_info)"
            log_success "$service_name is healthy"
            
            # Check if it's a Kind cluster
            if echo "$cluster_info" | grep -q "kind"; then
                log_verbose "Kind cluster detected: $cluster_info"
            fi
        else
            HEALTH_STATUS["kubernetes"]="unhealthy"
            HEALTH_DETAILS["kubernetes"]="Cluster not accessible or kubectl not configured"
            log_error "$service_name is unhealthy"
            OVERALL_HEALTH=false
        fi
    else
        HEALTH_STATUS["kubernetes"]="unavailable"
        HEALTH_DETAILS["kubernetes"]="kubectl not available"
        log_warning "$service_name check skipped (kubectl not available)"
    fi
}

check_container_services() {
    local service_name="Container Services"
    log_info "Checking $service_name..."
    
    if command -v podman >/dev/null 2>&1; then
        local containers=$(podman ps --format "{{.Names}}" | grep kubernaut || true)
        if [[ -n "$containers" ]]; then
            local container_count=$(echo "$containers" | wc -l)
            HEALTH_STATUS["containers"]="healthy"
            HEALTH_DETAILS["containers"]="$container_count kubernaut containers running"
            log_success "$service_name: $container_count containers running"
            
            if [[ "$VERBOSE" == "true" ]]; then
                echo "$containers" | while read -r container; do
                    local status=$(podman ps --filter "name=$container" --format "{{.Status}}")
                    log_verbose "Container $container: $status"
                done
            fi
        else
            HEALTH_STATUS["containers"]="partial"
            HEALTH_DETAILS["containers"]="No kubernaut containers found running"
            log_warning "$service_name: No kubernaut containers found"
        fi
    elif command -v docker >/dev/null 2>&1; then
        local containers=$(docker ps --format "{{.Names}}" | grep kubernaut || true)
        if [[ -n "$containers" ]]; then
            local container_count=$(echo "$containers" | wc -l)
            HEALTH_STATUS["containers"]="healthy"
            HEALTH_DETAILS["containers"]="$container_count kubernaut containers running"
            log_success "$service_name: $container_count containers running"
        else
            HEALTH_STATUS["containers"]="partial"
            HEALTH_DETAILS["containers"]="No kubernaut containers found running"
            log_warning "$service_name: No kubernaut containers found"
        fi
    else
        HEALTH_STATUS["containers"]="unavailable"
        HEALTH_DETAILS["containers"]="Neither podman nor docker available"
        log_warning "$service_name check skipped (no container runtime available)"
    fi
}

# Generate health report
generate_health_report() {
    echo
    echo "========================================"
    echo "Integration Test Dependencies Health Report"
    echo "========================================"
    echo
    
    # Summary table
    printf "%-25s %-12s %s\n" "Service" "Status" "Details"
    printf "%-25s %-12s %s\n" "-------" "------" "-------"
    
    for service in postgres vector_db redis context_api holmesgpt_api llm_service kubernetes containers; do
        if [[ -n "${HEALTH_STATUS[$service]}" ]]; then
            local status="${HEALTH_STATUS[$service]}"
            local details="${HEALTH_DETAILS[$service]}"
            
            case "$status" in
                "healthy")
                    printf "%-25s ${GREEN}%-12s${NC} %s\n" "$service" "$status" "$details"
                    ;;
                "partial")
                    printf "%-25s ${YELLOW}%-12s${NC} %s\n" "$service" "$status" "$details"
                    ;;
                "unhealthy")
                    printf "%-25s ${RED}%-12s${NC} %s\n" "$service" "$status" "$details"
                    ;;
                "unavailable")
                    printf "%-25s ${BLUE}%-12s${NC} %s\n" "$service" "$status" "$details"
                    ;;
            esac
        fi
    done
    
    echo
    
    # Overall status
    if [[ "$OVERALL_HEALTH" == "true" ]]; then
        log_success "Overall Health: HEALTHY - All critical dependencies are accessible"
        echo
        echo "✅ Integration tests can proceed"
        exit 0
    else
        log_error "Overall Health: UNHEALTHY - Some critical dependencies are not accessible"
        echo
        echo "❌ Integration tests may fail due to dependency issues"
        echo
        echo "Recommended actions:"
        echo "1. Run 'make bootstrap-dev' to start all services"
        echo "2. Check individual service logs for specific issues"
        echo "3. Verify network connectivity and firewall settings"
        exit 1
    fi
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
    --postgres-host HOST   PostgreSQL host (default: localhost)
    --postgres-port PORT   PostgreSQL port (default: 5433)
    --redis-host HOST      Redis host (default: localhost)
    --redis-port PORT      Redis port (default: 6380)
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
        --postgres-host)
            POSTGRES_HOST="$2"
            shift 2
            ;;
        --postgres-port)
            POSTGRES_PORT="$2"
            shift 2
            ;;
        --redis-host)
            REDIS_HOST="$2"
            shift 2
            ;;
        --redis-port)
            REDIS_PORT="$2"
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
