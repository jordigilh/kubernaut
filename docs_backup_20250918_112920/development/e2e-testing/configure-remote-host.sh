#!/bin/bash
# Remote Host Configuration Script for E2E Testing
# Configures and validates remote host connection for Kubernaut E2E deployment

set -euo pipefail

# Default values
REMOTE_HOST="${1:-helios08}"
REMOTE_USER="${2:-root}"
TEST_CONNECTION="${TEST_CONNECTION:-true}"
SETUP_SSH_KEY="${SETUP_SSH_KEY:-false}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_header() { echo -e "\n${CYAN}=== $1 ===${NC}"; }

# Banner
echo -e "${CYAN}"
cat << "EOF"
 ____                     _         _   _           _      ____             __ _
|  _ \ ___ _ __ ___   ___ | |_ ___  | | | | ___  ___| |_   / ___|___  _ __  / _(_) __ _
| |_) / _ \ '_ ` _ \ / _ \| __/ _ \ | |_| |/ _ \/ __| __| | |   / _ \| '_ \| |_| |/ _` |
|  _ <  __/ | | | | | (_) | ||  __/ |  _  | (_) \__ \ |_  | |__| (_) | | | |  _| | (_| |
|_| \_\___|_| |_| |_|\___/ \__\___| |_| |_|\___/|___/\__|  \____\___/|_| |_|_| |_|\__, |
                                                                                 |___/
EOF
echo -e "${NC}"

log_info "Remote Host Configuration for Kubernaut E2E Testing"
log_info "Target Host: ${REMOTE_USER}@${REMOTE_HOST}"

# Display usage information
usage() {
    cat << EOF
Usage: $0 [remote-host] [remote-user] [options]

Arguments:
  remote-host    Remote hostname or IP (default: helios08)
  remote-user    Remote username (default: root)

Environment Variables:
  TEST_CONNECTION=true|false    Test SSH connection (default: true)
  SETUP_SSH_KEY=true|false      Setup SSH key if needed (default: false)

Examples:
  $0 helios08 root                    # Configure helios08 as root
  $0 192.168.122.100 admin              # Configure IP with admin user
  TEST_CONNECTION=false $0 myhost     # Skip connection test

EOF
}

# Test SSH connection to remote host
test_ssh_connection() {
    log_header "Testing SSH Connection"

    log_info "Testing SSH connection to ${REMOTE_USER}@${REMOTE_HOST}..."

    if ssh -o ConnectTimeout=10 -o BatchMode=yes "${REMOTE_USER}@${REMOTE_HOST}" "echo 'SSH connection successful'" 2>/dev/null; then
        log_success "SSH connection to ${REMOTE_USER}@${REMOTE_HOST} is working ✅"

        # Test sudo capabilities if not root
        if [[ "$REMOTE_USER" != "root" ]]; then
            if ssh "${REMOTE_USER}@${REMOTE_HOST}" "sudo echo 'Sudo access confirmed'" 2>/dev/null; then
                log_success "Sudo access confirmed for ${REMOTE_USER}"
            else
                log_warning "Sudo access may not be available for ${REMOTE_USER}"
            fi
        fi

        return 0
    else
        log_error "Cannot connect to ${REMOTE_USER}@${REMOTE_HOST}"
        log_info "Please ensure:"
        log_info "  1. Host ${REMOTE_HOST} is reachable"
        log_info "  2. SSH key authentication is configured"
        log_info "  3. User ${REMOTE_USER} exists on the remote host"
        return 1
    fi
}

# Get remote host information
get_remote_host_info() {
    log_header "Remote Host Information"

    if ! ssh "${REMOTE_USER}@${REMOTE_HOST}" "echo 'Connected'" &>/dev/null; then
        log_error "Cannot connect to remote host for info gathering"
        return 1
    fi

    # Get basic system information
    log_info "Gathering system information from ${REMOTE_HOST}..."

    REMOTE_HOSTNAME=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "hostname" 2>/dev/null || echo "unknown")
    REMOTE_OS=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "cat /etc/os-release | grep '^PRETTY_NAME=' | cut -d'\"' -f2" 2>/dev/null || echo "unknown")
    REMOTE_KERNEL=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "uname -r" 2>/dev/null || echo "unknown")
    REMOTE_ARCH=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "uname -m" 2>/dev/null || echo "unknown")

    # Get hardware information
    REMOTE_CPU=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "lscpu | grep 'Model name:' | sed 's/Model name: *//' | head -1" 2>/dev/null || echo "unknown")
    REMOTE_CPU_CORES=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "nproc" 2>/dev/null || echo "unknown")
    REMOTE_MEMORY_GB=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "free -g | awk '/^Mem:/ {print \$2}'" 2>/dev/null || echo "unknown")
    REMOTE_DISK_GB=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "df -BG / | awk 'NR==2 {print \$4}' | sed 's/G//'" 2>/dev/null || echo "unknown")

    # Display information
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  REMOTE HOST INFORMATION${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo -e "Hostname:       ${REMOTE_HOSTNAME}"
    echo -e "Operating System: ${REMOTE_OS}"
    echo -e "Kernel:         ${REMOTE_KERNEL}"
    echo -e "Architecture:   ${REMOTE_ARCH}"
    echo -e "CPU:            ${REMOTE_CPU}"
    echo -e "CPU Cores:      ${REMOTE_CPU_CORES}"
    echo -e "Memory:         ${REMOTE_MEMORY_GB} GB"
    echo -e "Available Disk: ${REMOTE_DISK_GB} GB"
    echo -e "User:           ${REMOTE_USER}"
    echo -e "${GREEN}========================================${NC}"

    # Check RHEL version specifically
    if ssh "${REMOTE_USER}@${REMOTE_HOST}" "grep -q 'Red Hat Enterprise Linux.*9.7' /etc/os-release" 2>/dev/null; then
        log_success "RHEL 9.7 detected - optimal for E2E deployment ✅"
    elif ssh "${REMOTE_USER}@${REMOTE_HOST}" "grep -q 'Red Hat Enterprise Linux.*9' /etc/os-release" 2>/dev/null; then
        log_warning "RHEL 9.x detected (not 9.7) - should work but 9.7 is recommended"
    else
        log_warning "Non-RHEL or different version detected - scripts are optimized for RHEL 9.7"
    fi

    # Check minimum requirements
    if [[ "$REMOTE_CPU_CORES" != "unknown" && "$REMOTE_CPU_CORES" -ge 16 ]]; then
        log_success "CPU cores (${REMOTE_CPU_CORES}) meet minimum requirements"
    elif [[ "$REMOTE_CPU_CORES" != "unknown" ]]; then
        log_warning "CPU cores (${REMOTE_CPU_CORES}) may be insufficient for full cluster"
    fi

    if [[ "$REMOTE_MEMORY_GB" != "unknown" && "$REMOTE_MEMORY_GB" -ge 64 ]]; then
        log_success "Memory (${REMOTE_MEMORY_GB} GB) meets minimum requirements"
    elif [[ "$REMOTE_MEMORY_GB" != "unknown" ]]; then
        log_warning "Memory (${REMOTE_MEMORY_GB} GB) may be insufficient for full cluster"
    fi

    if [[ "$REMOTE_DISK_GB" != "unknown" && "$REMOTE_DISK_GB" -ge 500 ]]; then
        log_success "Disk space (${REMOTE_DISK_GB} GB) meets minimum requirements"
    elif [[ "$REMOTE_DISK_GB" != "unknown" ]]; then
        log_warning "Disk space (${REMOTE_DISK_GB} GB) may be insufficient for cluster + storage"
    fi
}

# Check prerequisites on remote host
check_remote_prerequisites() {
    log_header "Checking Remote Prerequisites"

    # Check if remote host has basic tools
    MISSING_TOOLS=()

    if ! ssh "${REMOTE_USER}@${REMOTE_HOST}" "command -v python3 >/dev/null 2>&1"; then
        MISSING_TOOLS+=("python3")
    fi

    if ! ssh "${REMOTE_USER}@${REMOTE_HOST}" "command -v curl >/dev/null 2>&1"; then
        MISSING_TOOLS+=("curl")
    fi

    if ! ssh "${REMOTE_USER}@${REMOTE_HOST}" "systemctl is-active libvirtd >/dev/null 2>&1"; then
        MISSING_TOOLS+=("libvirtd")
    fi

    if [[ ${#MISSING_TOOLS[@]} -eq 0 ]]; then
        log_success "All basic prerequisites are available"
    else
        log_warning "Missing prerequisites: ${MISSING_TOOLS[*]}"
        log_info "These will be installed during E2E setup"
    fi

    # Check if virtualization is supported
    if ssh "${REMOTE_USER}@${REMOTE_HOST}" "grep -q 'vmx\|svm' /proc/cpuinfo" 2>/dev/null; then
        log_success "CPU virtualization support detected"
    else
        log_error "CPU virtualization support not detected"
        log_info "Ensure virtualization is enabled in BIOS/UEFI"
        return 1
    fi
}

# Update Makefile with new remote host configuration
update_makefile_config() {
    log_header "Updating Makefile Configuration"

    MAKEFILE_PATH="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/Makefile"

    if [[ ! -f "$MAKEFILE_PATH" ]]; then
        log_error "Makefile not found at: $MAKEFILE_PATH"
        return 1
    fi

    # Check if remote configuration exists
    if grep -q "REMOTE_HOST=" "$MAKEFILE_PATH"; then
        log_info "Updating existing remote host configuration in Makefile..."

        # Update remote host configuration
        sed -i.bak "s/^REMOTE_HOST=.*/REMOTE_HOST=${REMOTE_HOST}/" "$MAKEFILE_PATH"
        sed -i.bak "s/^REMOTE_USER=.*/REMOTE_USER=${REMOTE_USER}/" "$MAKEFILE_PATH"

        log_success "Makefile updated with:"
        log_info "  REMOTE_HOST=${REMOTE_HOST}"
        log_info "  REMOTE_USER=${REMOTE_USER}"

        # Remove backup file
        rm -f "${MAKEFILE_PATH}.bak"
    else
        log_warning "Remote host configuration not found in Makefile"
        log_info "Please ensure the remote targets are present in the Makefile"
    fi
}

# Display available Makefile targets
show_makefile_targets() {
    log_header "Available Makefile Targets for Remote Deployment"

    cat << EOF
${GREEN}Remote E2E Deployment Targets:${NC}

${BLUE}Setup and Deployment:${NC}
  make validate-e2e-remote     # Validate remote host readiness
  make setup-e2e-remote        # Complete E2E environment setup
  make deploy-cluster-remote   # Deploy only OpenShift cluster

${BLUE}Testing and Management:${NC}
  make test-e2e-remote         # Run E2E tests
  make status-e2e-remote       # Check environment status
  make logs-e2e-remote         # View deployment logs

${BLUE}Maintenance:${NC}
  make cleanup-e2e-remote      # Complete environment cleanup
  make ssh-e2e-remote          # SSH to remote host

${BLUE}Example Workflow:${NC}
  1. make validate-e2e-remote  # Ensure host is ready
  2. make setup-e2e-remote     # Deploy complete environment
  3. make test-e2e-remote      # Run tests
  4. make cleanup-e2e-remote   # Clean up when done

${YELLOW}Note:${NC} All targets will connect to ${REMOTE_USER}@${REMOTE_HOST}
EOF
}

# Main execution function
main() {
    # Handle help option
    if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
        usage
        exit 0
    fi

    log_info "Configuring remote host for Kubernaut E2E testing"

    # Test SSH connection if enabled
    if [[ "$TEST_CONNECTION" == "true" ]]; then
        if ! test_ssh_connection; then
            log_error "SSH connection test failed"
            exit 1
        fi

        # Get remote host information
        get_remote_host_info

        # Check prerequisites
        check_remote_prerequisites
    else
        log_info "Skipping SSH connection test"
    fi

    # Update Makefile configuration
    update_makefile_config

    # Show available targets
    show_makefile_targets

    log_success "Remote host configuration completed!"
    log_info "You can now use 'make validate-e2e-remote' to validate the remote host"
    log_info "or 'make setup-e2e-remote' to deploy the complete E2E environment"
}

# Check if script is being sourced or executed
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
