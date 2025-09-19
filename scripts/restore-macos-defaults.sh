#!/bin/bash

# =============================================================================
# macOS System Restore Script for Mac Studio
# Restores system settings modified by start-ramalama-server.sh
# =============================================================================

set -euo pipefail

# Configuration
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly LOG_DIR="${PROJECT_ROOT}/logs"
readonly RESTORE_LOG="${LOG_DIR}/macos-restore.log"

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# Functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1" >&2
    echo "$(date '+%Y-%m-%d %H:%M:%S') [INFO] $1" >> "${RESTORE_LOG}"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1" >&2
    echo "$(date '+%Y-%m-%d %H:%M:%S') [WARN] $1" >> "${RESTORE_LOG}"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
    echo "$(date '+%Y-%m-%d %H:%M:%S') [ERROR] $1" >> "${RESTORE_LOG}"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1" >&2
    echo "$(date '+%Y-%m-%d %H:%M:%S') [SUCCESS] $1" >> "${RESTORE_LOG}"
}

check_platform() {
    if [[ "$(uname)" != "Darwin" ]]; then
        log_error "This script is only for macOS systems"
        exit 1
    fi

    log_info "Running on macOS $(sw_vers -productVersion)"
}

setup_logging() {
    mkdir -p "${LOG_DIR}"
    log_info "Starting macOS system restore process"
    log_info "Restore log: ${RESTORE_LOG}"
}

restore_system_settings() {
    log_info "Restoring macOS system settings to defaults..."

    if command -v sysctl &> /dev/null; then
        # Restore VM and memory settings to defaults
        log_info "Restoring virtual memory settings..."
        sysctl -w vm.compressor_mode=2 2>/dev/null || log_warn "Could not restore VM compressor to default"

        # Restore file limits to macOS defaults
        log_info "Restoring file limits..."
        sysctl -w kern.maxfiles=65536 2>/dev/null || log_warn "Could not restore system file limit"

        # Restore network settings to defaults
        log_info "Restoring network settings..."
        sysctl -w net.inet.tcp.sendspace=65536 2>/dev/null || log_warn "Could not restore TCP send buffer"
        sysctl -w net.inet.tcp.recvspace=65536 2>/dev/null || log_warn "Could not restore TCP receive buffer"
        sysctl -w net.inet.tcp.delayed_ack=1 2>/dev/null || log_warn "Could not restore delayed ACK"

        # Restore memory pressure settings to defaults
        log_info "Restoring memory pressure settings..."
        sysctl -w vm.pressure_threshold=80 2>/dev/null || log_warn "Could not restore memory pressure threshold"

        log_success "System settings restored"
    else
        log_error "sysctl command not found"
    fi
}

restore_launchd_services() {
    log_info "Restoring macOS system services..."

    # Re-enable Spotlight indexing
    log_info "Re-enabling Spotlight (mds) service..."
    if launchctl load -w /System/Library/LaunchDaemons/com.apple.metadata.mds.plist 2>/dev/null; then
        log_success "Spotlight service restored"
    else
        log_warn "Could not restore Spotlight service (may already be running)"
    fi

    # Re-enable Photo Analysis service
    log_info "Re-enabling Photo Analysis service..."
    if launchctl load -w /System/Library/LaunchAgents/com.apple.photoanalysisd.plist 2>/dev/null; then
        log_success "Photo Analysis service restored"
    else
        log_warn "Could not restore Photo Analysis service (may already be running)"
    fi

    log_success "System services restoration completed"
}

restore_environment_variables() {
    log_info "Clearing optimized environment variables..."

    # List of environment variables that were set for optimization
    local env_vars=(
        "MALLOC_ARENA_MAX"
        "RAMALAMA_NUM_THREAD"
        "RAMALAMA_LOG_LEVEL"
        "GGML_METAL_NDEBUG"
    )

    for var in "${env_vars[@]}"; do
        if [[ -n "${!var:-}" ]]; then
            log_info "Unsetting ${var}"
            unset "${var}"
        fi
    done

    log_success "Environment variables cleared"
}

restore_user_limits() {
    log_info "Restoring user file descriptor limits..."

    # Restore file descriptor limits to macOS defaults
    ulimit -n 256 2>/dev/null || log_warn "Could not restore file descriptor limit"

    log_success "User limits restored"
}

verify_restoration() {
    log_info "Verifying system restoration..."

    # Check system settings
    local vm_compressor=$(sysctl -n vm.compressor_mode 2>/dev/null || echo "unknown")
    local max_files=$(sysctl -n kern.maxfiles 2>/dev/null || echo "unknown")
    local tcp_sendspace=$(sysctl -n net.inet.tcp.sendspace 2>/dev/null || echo "unknown")
    local pressure_threshold=$(sysctl -n vm.pressure_threshold 2>/dev/null || echo "unknown")

    echo
    echo "📊 Current System Settings:"
    echo "   VM Compressor Mode: ${vm_compressor} (default: 2)"
    echo "   Max Files: ${max_files} (default: 65536)"
    echo "   TCP Send Space: ${tcp_sendspace} (default: 65536)"
    echo "   Memory Pressure Threshold: ${pressure_threshold} (default: 80)"

    # Check service status
    echo
    echo "🔍 Service Status:"

    if launchctl list | grep -q "com.apple.metadata.mds"; then
        echo "   ✅ Spotlight (mds): Running"
    else
        echo "   ❌ Spotlight (mds): Not running"
    fi

    if launchctl list | grep -q "com.apple.photoanalysisd"; then
        echo "   ✅ Photo Analysis: Running"
    else
        echo "   ❌ Photo Analysis: Not running"
    fi

    # Check file descriptor limit
    local fd_limit=$(ulimit -n)
    echo "   File Descriptor Limit: ${fd_limit}"

    echo
}

show_restore_summary() {
    log_success "macOS system restore completed!"
    echo
    echo "📋 Restoration Summary:"
    echo "   ✅ System settings restored to defaults"
    echo "   ✅ Network settings restored"
    echo "   ✅ Memory settings restored"
    echo "   ✅ System services re-enabled"
    echo "   ✅ Environment variables cleared"
    echo "   ✅ User limits restored"
    echo
    echo "📁 Restore log: ${RESTORE_LOG}"
    echo
    echo "ℹ️  Notes:"
    echo "   - Some changes may require a system restart to take full effect"
    echo "   - Spotlight may take time to re-index after restoration"
    echo "   - Performance optimizations have been completely removed"
    echo
    echo "🔄 To re-apply optimizations, run:"
    echo "   ${SCRIPT_DIR}/start-ramalama-server.sh"
}

prompt_confirmation() {
    echo
    echo "⚠️  This script will restore macOS system settings to defaults."
    echo "   This will undo all performance optimizations applied by start-ramalama-server.sh"
    echo
    read -p "Do you want to continue? (yes/no): " -r
    echo
    if [[ ! $REPLY =~ ^[Yy]([Ee][Ss])?$ ]]; then
        log_info "Restoration cancelled by user"
        exit 0
    fi
}

main() {
    echo "🔄 macOS System Restore Script"
    echo "================================"

    check_platform
    setup_logging
    prompt_confirmation

    log_info "Starting system restoration..."

    restore_system_settings
    restore_launchd_services
    restore_environment_variables
    restore_user_limits

    verify_restoration
    show_restore_summary

    log_success "System restore process completed successfully!"
}

# Handle signals
trap 'log_error "Script interrupted"; exit 130' INT TERM

# Run main function
main "$@"