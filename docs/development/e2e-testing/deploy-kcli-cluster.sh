#!/bin/bash
# OpenShift 4.18 Bare Metal Deployment Script using KCLI
# Usage: ./deploy-kcli-cluster.sh [cluster-name] [config-file]

set -euo pipefail

# Default values
CLUSTER_NAME="${1:-ocp418-baremetal}"
CONFIG_FILE="${2:-kcli-baremetal-params.yml}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Banner
echo -e "${BLUE}"
cat << "EOF"
  ___  ____ ____    _  _  _  ___     ____                           _        _
 / _ \/ ___|  _ \  / || || | / _ \   |  _ \  ___ _ __  | | ___ _   _ | |_ _ __(_) |___ _  _ ___
| | | \___ \ |_) | | || || || (_) |  | | | |/ _ \ '_ \ | |/ _ \ | | \| \| '_ \| __/ _ \| '_  / | | / __|
| |_| |___) |  __/  | ||_|| | > _<   | |_| |  __/ |_) || |  __/ |_| |   | | | | || (_) | | | | |_| \__ \
 \___/|____/|_|     |_|    |_/_\ \_\  |____/ \___| .__/ |_|\___|\__, |_|_|_| |_|\__\___/|_| |_|\__,_|___/
                                                 |_|            |___/
EOF
echo -e "${NC}"

log_info "Starting OpenShift 4.18 Bare Metal deployment with KCLI"
log_info "Cluster: ${CLUSTER_NAME}"
log_info "Config: ${CONFIG_FILE}"

# Pre-flight checks
preflight_checks() {
    log_info "Running pre-flight checks..."

    # Check if kcli is installed
    if ! command -v kcli &> /dev/null; then
        log_error "KCLI is not installed. Please install it first."
        exit 1
    fi

    # Check kcli version
    KCLI_VERSION=$(kcli version 2>/dev/null | grep -oP '\d+\.\d+\.\d+' | head -1 || echo "unknown")
    log_info "KCLI version: ${KCLI_VERSION}"

    # Check if config file exists
    if [[ ! -f "${SCRIPT_DIR}/${CONFIG_FILE}" ]]; then
        log_error "Configuration file not found: ${SCRIPT_DIR}/${CONFIG_FILE}"
        exit 1
    fi

    # Check for pull secret
    PULL_SECRET_PATH=$(grep -oP "pull_secret: '\K[^']*" "${SCRIPT_DIR}/${CONFIG_FILE}" | sed "s|~|$HOME|")
    if [[ ! -f "${PULL_SECRET_PATH}" ]]; then
        log_warning "Pull secret not found at: ${PULL_SECRET_PATH}"
        log_warning "Please download your pull secret from https://console.redhat.com/openshift/install/pull-secret"
    fi

    # Check for SSH key
    SSH_KEY_PATH=$(grep -oP "ssh_key: '\K[^']*" "${SCRIPT_DIR}/${CONFIG_FILE}" | sed "s|~|$HOME|")
    if [[ ! -f "${SSH_KEY_PATH}" ]]; then
        log_warning "SSH public key not found at: ${SSH_KEY_PATH}"
        log_warning "Please generate SSH keys with: ssh-keygen -t rsa -b 4096"
    fi

    # Check libvirt status
    if ! systemctl is-active --quiet libvirtd; then
        log_error "libvirtd is not running. Please start it: sudo systemctl start libvirtd"
        exit 1
    fi

    # Check if cluster already exists
    if kcli list cluster | grep -q "^${CLUSTER_NAME}"; then
        log_warning "Cluster '${CLUSTER_NAME}' already exists!"
        read -p "Do you want to delete and recreate it? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            log_info "Deleting existing cluster..."
            kcli delete cluster "${CLUSTER_NAME}" --yes
        else
            log_info "Aborting deployment."
            exit 0
        fi
    fi

    log_success "Pre-flight checks completed"
}

# Validate hardware connectivity
validate_hardware() {
    log_info "Validating hardware connectivity..."

    # Extract BMC addresses from config file
    BMC_ADDRESSES=$(grep -oP 'ipmi_address: "\K[^"]*' "${SCRIPT_DIR}/${CONFIG_FILE}")

    if command -v ipmitool &> /dev/null; then
        while IFS= read -r bmc_addr; do
            log_info "Testing connectivity to BMC: ${bmc_addr}"
            if timeout 10 ping -c 1 "${bmc_addr}" &> /dev/null; then
                log_success "BMC ${bmc_addr} is reachable"
            else
                log_warning "BMC ${bmc_addr} is not reachable via ping"
            fi
        done <<< "${BMC_ADDRESSES}"
    else
        log_warning "ipmitool not installed. Skipping BMC connectivity tests."
    fi
}

# Deploy cluster
deploy_cluster() {
    log_info "Starting cluster deployment..."

    # Create deployment command
    DEPLOY_CMD="kcli create cluster openshift --paramfile ${SCRIPT_DIR}/${CONFIG_FILE} ${CLUSTER_NAME}"

    log_info "Running: ${DEPLOY_CMD}"

    # Run deployment with output logging
    if ${DEPLOY_CMD} 2>&1 | tee "/tmp/kcli-deploy-${CLUSTER_NAME}.log"; then
        log_success "Cluster deployment initiated successfully"
    else
        log_error "Cluster deployment failed. Check logs at /tmp/kcli-deploy-${CLUSTER_NAME}.log"
        exit 1
    fi
}

# Monitor installation
monitor_installation() {
    log_info "Monitoring installation progress..."

    local max_wait=3600  # 60 minutes
    local wait_time=0
    local sleep_interval=30

    while [[ ${wait_time} -lt ${max_wait} ]]; do
        # Get cluster status
        if CLUSTER_STATUS=$(kcli info cluster "${CLUSTER_NAME}" 2>/dev/null); then
            if echo "${CLUSTER_STATUS}" | grep -q "Status.*ready"; then
                log_success "Cluster is ready!"
                break
            elif echo "${CLUSTER_STATUS}" | grep -q "Status.*failed"; then
                log_error "Cluster deployment failed!"
                echo "${CLUSTER_STATUS}"
                exit 1
            else
                log_info "Cluster status: Installing... (${wait_time}/${max_wait} seconds)"
            fi
        else
            log_info "Waiting for cluster status... (${wait_time}/${max_wait} seconds)"
        fi

        sleep ${sleep_interval}
        wait_time=$((wait_time + sleep_interval))
    done

    if [[ ${wait_time} -ge ${max_wait} ]]; then
        log_error "Installation timeout reached (${max_wait} seconds)"
        exit 1
    fi
}

# Post-installation setup
post_installation() {
    log_info "Running post-installation setup..."

    # Download kubeconfig
    if kcli download kubeconfig -c "${CLUSTER_NAME}"; then
        log_success "Kubeconfig downloaded"
    else
        log_error "Failed to download kubeconfig"
        exit 1
    fi

    # Set kubeconfig environment
    KUBECONFIG_PATH="$HOME/.kcli/clusters/${CLUSTER_NAME}/auth/kubeconfig"
    if [[ -f "${KUBECONFIG_PATH}" ]]; then
        export KUBECONFIG="${KUBECONFIG_PATH}"
        log_success "KUBECONFIG set to: ${KUBECONFIG_PATH}"
    fi

    # Verify cluster
    log_info "Verifying cluster..."
    if command -v oc &> /dev/null; then
        oc get nodes
        oc get co

        # Wait for all cluster operators to be available
        log_info "Waiting for cluster operators to stabilize..."
        local max_wait=600  # 10 minutes
        local count=0
        while [[ $count -lt $max_wait ]]; do
            if oc get co --no-headers | grep -qv "True.*False.*False"; then
                log_info "Cluster operators still stabilizing... (${count}/${max_wait}s)"
                sleep 30
                count=$((count + 30))
            else
                log_success "All cluster operators are stable"
                break
            fi
        done

        # Get cluster info
        CONSOLE_URL=$(oc get routes console -n openshift-console -o jsonpath='{.spec.host}' 2>/dev/null || echo "Not available")
        ADMIN_PASSWORD_FILE="$HOME/.kcli/clusters/${CLUSTER_NAME}/auth/kubeadmin-password"

        log_success "Cluster verification completed"

        # Setup storage if configured
        setup_storage_if_enabled

        # Display access information
        echo -e "\n${GREEN}========================================${NC}"
        echo -e "${GREEN}  CLUSTER ACCESS INFORMATION${NC}"
        echo -e "${GREEN}========================================${NC}"
        echo -e "Cluster Name:     ${CLUSTER_NAME}"
        echo -e "Console URL:      https://${CONSOLE_URL}"
        echo -e "Kubeconfig:       ${KUBECONFIG_PATH}"
        echo -e "Admin User:       kubeadmin"
        if [[ -f "${ADMIN_PASSWORD_FILE}" ]]; then
            echo -e "Admin Password:   $(cat "${ADMIN_PASSWORD_FILE}")"
        fi
        echo -e "\nTo access the cluster:"
        echo -e "  export KUBECONFIG=${KUBECONFIG_PATH}"
        echo -e "  oc get nodes"
        echo -e "${GREEN}========================================${NC}\n"
    else
        log_warning "OpenShift CLI (oc) not found. Please install it to manage the cluster."
    fi
}

# Setup storage if enabled in configuration
setup_storage_if_enabled() {
    log_info "Checking storage configuration..."

    # Check if storage operators are enabled in config
    if grep -q "storage_operators: true" "${SCRIPT_DIR}/${CONFIG_FILE}" 2>/dev/null; then
        log_info "Storage operators enabled, setting up storage..."

        # Make storage setup script executable
        chmod +x "${SCRIPT_DIR}/setup-storage.sh"

        # Run storage setup
        if "${SCRIPT_DIR}/setup-storage.sh"; then
            log_success "Storage setup completed successfully"
        else
            log_warning "Storage setup encountered issues, but cluster is functional"
            log_info "You can run storage setup manually: ${SCRIPT_DIR}/setup-storage.sh"
        fi
    else
        log_info "Storage operators not enabled in configuration, skipping storage setup"
        log_info "To enable storage: set 'storage_operators: true' in ${CONFIG_FILE}"
    fi
}

# Cleanup on exit
cleanup() {
    log_info "Cleaning up temporary files..."
    # Add any cleanup logic here if needed
}

# Trap cleanup on exit
trap cleanup EXIT

# Main execution
main() {
    preflight_checks
    validate_hardware
    deploy_cluster
    monitor_installation
    post_installation

    log_success "OpenShift 4.18 deployment completed successfully!"
}

# Check if script is being sourced or executed
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
