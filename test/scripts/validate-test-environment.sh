#!/bin/bash
# Shared E2E Test Environment Validation Script
# Validates system resources before running E2E tests
#
# Usage:
#   ./validate-test-environment.sh [OPTIONS]
#
# Options:
#   --service <name>          Service name (for display, e.g., "Notification", "Gateway")
#   --ports <port1,port2,...> Required ports (comma-separated)
#   --min-disk-gb <num>       Minimum free disk space in GB (default: 20)
#   --min-memory-gb <num>     Minimum available memory in GB (default: 4)
#   --cluster-name <name>     Expected Kind cluster name (optional, for stale cluster check)
#   --skip-port-check         Skip port availability check
#   --skip-resource-check     Skip disk/memory checks
#   --help                    Show this help message
#
# Exit codes:
#   0 - Validation passed
#   1 - Validation failed
#   2 - Invalid arguments

set -e

# ========================================
# Default Configuration
# ========================================
SERVICE_NAME="E2E"
REQUIRED_PORTS=""
MIN_DISK_GB=20
MIN_MEMORY_GB=4
CLUSTER_NAME=""
SKIP_PORT_CHECK=false
SKIP_RESOURCE_CHECK=false

# ========================================
# Color Codes
# ========================================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ========================================
# Helper Functions
# ========================================
print_header() {
    echo ""
    echo "════════════════════════════════════════════════════════════════════════"
    echo -e "${BLUE}🔍 $SERVICE_NAME Test Environment Validation${NC}"
    echo "════════════════════════════════════════════════════════════════════════"
    echo ""
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

show_help() {
    cat << EOF
Shared E2E Test Environment Validation Script

Usage:
  ./validate-test-environment.sh [OPTIONS]

Options:
  --service <name>          Service name (e.g., "Notification", "Gateway")
  --ports <port1,port2,...> Required ports (comma-separated)
  --min-disk-gb <num>       Minimum free disk space in GB (default: 20)
  --min-memory-gb <num>     Minimum available memory in GB (default: 4)
  --cluster-name <name>     Expected Kind cluster name (for cleanup check)
  --skip-port-check         Skip port availability check
  --skip-resource-check     Skip disk/memory checks
  --help                    Show this help message

Examples:
  # Notification E2E tests
  ./validate-test-environment.sh \\
    --service "Notification" \\
    --ports "15432,16379,18090" \\
    --cluster-name "notification-e2e"

  # Gateway E2E tests (minimal checks)
  ./validate-test-environment.sh \\
    --service "Gateway" \\
    --skip-port-check \\
    --min-disk-gb 10

Exit Codes:
  0 - Validation passed
  1 - Validation failed
  2 - Invalid arguments
EOF
}

# ========================================
# Argument Parsing
# ========================================
while [[ $# -gt 0 ]]; do
    case $1 in
        --service)
            SERVICE_NAME="$2"
            shift 2
            ;;
        --ports)
            REQUIRED_PORTS="$2"
            shift 2
            ;;
        --min-disk-gb)
            MIN_DISK_GB="$2"
            shift 2
            ;;
        --min-memory-gb)
            MIN_MEMORY_GB="$2"
            shift 2
            ;;
        --cluster-name)
            CLUSTER_NAME="$2"
            shift 2
            ;;
        --skip-port-check)
            SKIP_PORT_CHECK=true
            shift
            ;;
        --skip-resource-check)
            SKIP_RESOURCE_CHECK=true
            shift
            ;;
        --help)
            show_help
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Run with --help for usage information"
            exit 2
            ;;
    esac
done

# ========================================
# Validation Functions
# ========================================

# Check if Podman is running
check_podman() {
    print_info "Checking Podman/Docker availability..."

    if command -v podman &> /dev/null; then
        if podman info &>/dev/null; then
            print_success "Podman is running"
            return 0
        else
            print_error "Podman is installed but not running"
            print_info "Start with: podman machine start"
            return 1
        fi
    elif command -v docker &> /dev/null; then
        if docker info &>/dev/null; then
            print_success "Docker is running"
            return 0
        else
            print_error "Docker is installed but not running"
            print_info "Start Docker Desktop or run: systemctl start docker"
            return 1
        fi
    else
        print_error "Neither Podman nor Docker is installed"
        print_info "Install Podman: brew install podman (macOS) or see https://podman.io"
        return 1
    fi
}

# Check disk space
check_disk_space() {
    if [ "$SKIP_RESOURCE_CHECK" = true ]; then
        print_info "Skipping disk space check (--skip-resource-check)"
        return 0
    fi

    print_info "Checking disk space (minimum: ${MIN_DISK_GB}GB)..."

    # Get free space in GB (works on macOS and Linux)
    if [[ "$OSTYPE" == "darwin"* ]]; then
        FREE_SPACE=$(df -g . | awk 'NR==2 {print $4}')
    else
        FREE_SPACE=$(df -BG . | awk 'NR==2 {print $4}' | sed 's/G//')
    fi

    if [ "$FREE_SPACE" -lt "$MIN_DISK_GB" ]; then
        print_error "Insufficient disk space: ${FREE_SPACE}GB free (need ${MIN_DISK_GB}GB)"
        print_info "Free up space or run: make clean-podman-all"
        return 1
    fi

    print_success "Disk space: ${FREE_SPACE}GB free"
    return 0
}

# Check for dangling images (warning only)
check_dangling_images() {
    print_info "Checking for dangling images..."

    CONTAINER_TOOL="podman"
    if ! command -v podman &> /dev/null; then
        CONTAINER_TOOL="docker"
    fi

    DANGLING_COUNT=$($CONTAINER_TOOL images -f "dangling=true" -q | wc -l | xargs)

    if [ "$DANGLING_COUNT" -gt 0 ]; then
        print_warning "${DANGLING_COUNT} dangling images found"
        print_info "Consider running: make clean-podman"
    else
        print_success "No dangling images"
    fi

    return 0
}

# Check port availability
check_ports() {
    if [ "$SKIP_PORT_CHECK" = true ] || [ -z "$REQUIRED_PORTS" ]; then
        if [ "$SKIP_PORT_CHECK" = true ]; then
            print_info "Skipping port check (--skip-port-check)"
        fi
        return 0
    fi

    print_info "Checking port availability..."

    # Convert comma-separated ports to array
    IFS=',' read -ra PORTS <<< "$REQUIRED_PORTS"

    local port_conflicts=0
    for port in "${PORTS[@]}"; do
        port=$(echo "$port" | xargs) # Trim whitespace
        if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
            print_error "Port $port is already in use"
            port_conflicts=1
        fi
    done

    if [ $port_conflicts -eq 1 ]; then
        print_info "Run appropriate cleanup target: make clean-<service>-test-ports"
        return 1
    fi

    print_success "All required ports are available"
    return 0
}

# Check available memory
check_memory() {
    if [ "$SKIP_RESOURCE_CHECK" = true ]; then
        print_info "Skipping memory check (--skip-resource-check)"
        return 0
    fi

    print_info "Checking available memory (minimum: ${MIN_MEMORY_GB}GB)..."

    # Get available memory in GB (works on macOS and Linux)
    if [[ "$OSTYPE" == "darwin"* ]]; then
        AVAILABLE_MEMORY=$(sysctl -n hw.memsize 2>/dev/null)
        AVAILABLE_GB=$((AVAILABLE_MEMORY / 1024 / 1024 / 1024))
    else
        AVAILABLE_MEMORY=$(free -b | awk '/^Mem:/{print $7}')
        AVAILABLE_GB=$((AVAILABLE_MEMORY / 1024 / 1024 / 1024))
    fi

    if [ "$AVAILABLE_GB" -lt "$MIN_MEMORY_GB" ]; then
        print_error "Insufficient memory: ${AVAILABLE_GB}GB available (need ${MIN_MEMORY_GB}GB)"
        print_info "Close some applications to free up memory"
        return 1
    fi

    print_success "Memory: ${AVAILABLE_GB}GB available"
    return 0
}

# Check for stale Kind clusters (warning only)
check_stale_clusters() {
    if ! command -v kind &> /dev/null; then
        return 0
    fi

    print_info "Checking for stale Kind clusters..."

    STALE_CLUSTERS=$(kind get clusters 2>/dev/null | grep -E "e2e|test" || true)

    if [ -n "$STALE_CLUSTERS" ]; then
        print_warning "Found stale Kind clusters:"
        echo "$STALE_CLUSTERS" | sed 's/^/   - /'

        if [ -n "$CLUSTER_NAME" ]; then
            if echo "$STALE_CLUSTERS" | grep -q "^$CLUSTER_NAME$"; then
                print_info "Expected cluster '$CLUSTER_NAME' exists (will be reused or recreated)"
            fi
        fi

        print_info "Consider cleanup: kind delete cluster --name <cluster-name>"
    else
        print_success "No stale Kind clusters found"
    fi

    return 0
}

# ========================================
# Main Validation Sequence
# ========================================
main() {
    print_header

    local failed=0

    # Run all checks
    check_podman || failed=1
    check_disk_space || failed=1
    check_memory || failed=1
    check_ports || failed=1

    # Warning-only checks (don't fail validation)
    check_dangling_images
    check_stale_clusters

    echo ""
    echo "════════════════════════════════════════════════════════════════════════"

    if [ $failed -eq 0 ]; then
        print_success "Environment validation passed - ready to run $SERVICE_NAME tests"
        echo "════════════════════════════════════════════════════════════════════════"
        echo ""
        return 0
    else
        print_error "Environment validation failed - fix issues above before running tests"
        echo "════════════════════════════════════════════════════════════════════════"
        echo ""
        return 1
    fi
}

# Run validation
main
exit $?




