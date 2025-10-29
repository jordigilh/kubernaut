#!/bin/bash
# Start Redis for Gateway Integration Tests
#
# This script ensures Redis is running with proper configuration for integration tests:
# - 2GB memory limit (prevents OOM errors during tests)
# - allkeys-lru eviction policy (graceful memory management)
# - Port 6379 (standard Redis port)
# - Works with Kind cluster (kubeconfig: ~/.kube/kind-config)
#
# Usage: ./scripts/start-redis-for-tests.sh
# Or:    make redis-start

set -euo pipefail

# Configuration
REDIS_CONTAINER_NAME="redis-gateway"
REDIS_PORT="6379"
REDIS_MEMORY="2gb"
REDIS_IMAGE="redis:7-alpine"
KIND_KUBECONFIG="${HOME}/.kube/kind-config"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if Kind cluster is available
check_kind_cluster() {
    if [ ! -f "${KIND_KUBECONFIG}" ]; then
        log_warning "Kind kubeconfig not found at ${KIND_KUBECONFIG}"
        return 1
    fi
    
    if ! KUBECONFIG="${KIND_KUBECONFIG}" kubectl cluster-info 2>/dev/null | grep -q "running"; then
        log_warning "Kind cluster not running"
        return 1
    fi
    
    return 0
}

# Check if Redis is already running
check_redis_running() {
    if podman ps --filter "name=${REDIS_CONTAINER_NAME}" --format "{{.Names}}" | grep -q "${REDIS_CONTAINER_NAME}"; then
        return 0
    else
        return 1
    fi
}

# Check Redis memory configuration
check_redis_memory() {
    local current_memory
    current_memory=$(podman exec "${REDIS_CONTAINER_NAME}" redis-cli CONFIG GET maxmemory 2>/dev/null | tail -1)
    
    # Convert to GB for comparison (2GB = 2147483648 bytes)
    if [ "$current_memory" -ge 2000000000 ]; then
        return 0
    else
        return 1
    fi
}

# Stop existing Redis container
stop_redis() {
    log_info "Stopping existing Redis container..."
    podman stop "${REDIS_CONTAINER_NAME}" 2>/dev/null || true
    podman rm "${REDIS_CONTAINER_NAME}" 2>/dev/null || true
    log_success "Stopped existing Redis container"
}

# Start Redis with proper configuration
start_redis() {
    log_info "Starting Redis container: ${REDIS_CONTAINER_NAME}"
    log_info "Configuration:"
    echo "  • Memory: ${REDIS_MEMORY}"
    echo "  • Port: ${REDIS_PORT}"
    echo "  • Eviction Policy: allkeys-lru"
    echo ""
    
    podman run -d \
        --name "${REDIS_CONTAINER_NAME}" \
        -p "${REDIS_PORT}:6379" \
        "${REDIS_IMAGE}" \
        redis-server \
        --maxmemory "${REDIS_MEMORY}" \
        --maxmemory-policy allkeys-lru
    
    # Wait for Redis to be ready
    log_info "Waiting for Redis to be ready..."
    local max_attempts=10
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        if podman exec "${REDIS_CONTAINER_NAME}" redis-cli ping >/dev/null 2>&1; then
            log_success "Redis is ready!"
            break
        fi
        sleep 1
        ((attempt++))
    done
    
    if [ $attempt -gt $max_attempts ]; then
        log_error "Redis failed to start within expected time"
        return 1
    fi
}

# Verify Redis configuration
verify_redis_config() {
    log_info "Verifying Redis configuration..."
    
    local max_memory
    max_memory=$(podman exec "${REDIS_CONTAINER_NAME}" redis-cli CONFIG GET maxmemory | tail -1)
    
    local eviction_policy
    eviction_policy=$(podman exec "${REDIS_CONTAINER_NAME}" redis-cli CONFIG GET maxmemory-policy | tail -1)
    
    echo ""
    echo "Redis Configuration:"
    echo "  • Max Memory: $((max_memory / 1024 / 1024 / 1024)) GB ($max_memory bytes)"
    echo "  • Eviction Policy: ${eviction_policy}"
    echo "  • Port: ${REDIS_PORT}"
    echo "  • Container: ${REDIS_CONTAINER_NAME}"
    echo ""
    
    if [ "$max_memory" -ge 2000000000 ]; then
        log_success "Redis memory configuration is correct (≥2GB)"
    else
        log_warning "Redis memory is less than 2GB (current: $((max_memory / 1024 / 1024)) MB)"
    fi
}

# Main function
main() {
    log_info "Redis Setup for Gateway Integration Tests"
    echo ""
    
    # Check Kind cluster
    if check_kind_cluster; then
        log_success "Kind cluster is running"
        log_info "Kubeconfig: ${KIND_KUBECONFIG}"
    else
        log_warning "Kind cluster not detected (tests may fail without K8s)"
    fi
    
    echo ""
    
    # Check if Redis is already running with correct configuration
    if check_redis_running; then
        log_info "Redis container is already running"
        
        if check_redis_memory; then
            log_success "Redis memory configuration is correct (≥2GB)"
            verify_redis_config
            
            echo ""
            log_success "✅ Redis is ready for integration tests!"
            echo ""
            echo "To run Gateway integration tests:"
            echo "  export KUBECONFIG=${KIND_KUBECONFIG}"
            echo "  go test ./test/integration/gateway -v -timeout 10m"
            echo ""
            echo "Or use the Makefile:"
            echo "  make test-gateway"
            echo ""
            exit 0
        else
            log_warning "Redis is running but with insufficient memory (<2GB)"
            log_info "Restarting Redis with correct configuration..."
            stop_redis
        fi
    fi
    
    # Start Redis with correct configuration
    start_redis
    verify_redis_config
    
    echo ""
    log_success "✅ Redis is ready for integration tests!"
    echo ""
    echo "Environment Setup:"
    echo "  • Redis: localhost:6379 (Podman container)"
    echo "  • Kind Cluster: ${KIND_KUBECONFIG}"
    echo "  • Memory: 2GB (prevents OOM errors)"
    echo ""
    echo "To run Gateway integration tests:"
    echo "  export KUBECONFIG=${KIND_KUBECONFIG}"
    echo "  go test ./test/integration/gateway -v -timeout 10m"
    echo ""
    echo "Or use the Makefile:"
    echo "  make test-gateway"
    echo ""
}

# Execute main function
main "$@"
