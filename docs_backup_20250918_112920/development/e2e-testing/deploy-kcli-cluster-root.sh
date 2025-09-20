#!/bin/bash
# OpenShift 4.18 Bare Metal Deployment Script using KCLI - Root User Optimized for RHEL 9.7
# Usage: ./deploy-kcli-cluster-root.sh [cluster-name] [config-file]
# NOTE: This script must be run as root

set -euo pipefail

# Root user validation
if [[ $EUID -ne 0 ]]; then
    echo -e "\033[0;31m[ERROR]\033[0m This script must be run as root for RHEL 9.7 deployment"
    echo "Usage: sudo $0 [cluster-name] [config-file]"
    exit 1
fi

# Default values
CLUSTER_NAME="${1:-kubernaut-e2e}"
CONFIG_FILE="${2:-kcli-baremetal-params-root.yml}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Root-specific paths
ROOT_HOME="/root"
KCLI_CONFIG_DIR="${ROOT_HOME}/.kcli"
KUBECONFIG_DIR="${KCLI_CONFIG_DIR}/clusters"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_header() { echo -e "\n${PURPLE}=== $1 ===${NC}"; }

# Banner
echo -e "${BLUE}"
cat << "EOF"
  ___  ____ ____    _  _  _  ___     ____                  _     ____             _
 / _ \/ ___|  _ \  / || || | / _ \   |  _ \ ___   ___  _ __| |_  |  _ \  ___ _ __ | | ___  _   _
| | | \___ \ |_) | | || || || (_) |  | |_) / _ \ / _ \| '__| __| | | | |/ _ \ '_ \| |/ _ \| | | |
| |_| |___) |  __/  | ||_|| | > _<   |  _ < (_) | (_) | |  | |_  | |_| |  __/ |_) | | (_) | |_| |
 \___/|____/|_|     |_|    |_/_\ \_\ |_| \_\___/ \___/|_|   \__| |____/ \___| .__/|_|\___/ \__, |
                                                                            |_|            |___/
     ____  _   _ _____ _        ___    _____   ____             _
    |  _ \| | | | ____| |      / _ \  |___  | |  _ \ ___   ___ | |_
    | |_) | |_| |  _| | |     | (_) |    / /  | |_) / _ \ / _ \| __|
    |  _ <|  _  | |___| |___   \__, |   / /   |  _ < (_) | (_) | |_
    |_| \_\_| |_|_____|_____|    /_/   /_/    |_| \_\___/ \___/ \__|
EOF
echo -e "${NC}"

log_info "Starting OpenShift 4.18 Bare Metal deployment with KCLI (Root User)"
log_info "RHEL 9.7 Host: $(hostname)"
log_info "Running as: $(whoami)"
log_info "Cluster: ${CLUSTER_NAME}"
log_info "Config: ${CONFIG_FILE}"
log_info "Root Home: ${ROOT_HOME}"

# Check RHEL 9.7
check_rhel97() {
    log_header "RHEL 9.7 System Validation"

    if [[ -f /etc/os-release ]]; then
        OS_NAME=$(grep '^NAME=' /etc/os-release | cut -d'"' -f2)
        OS_VERSION=$(grep '^VERSION=' /etc/os-release | cut -d'"' -f2)
        log_info "Operating System: ${OS_NAME} ${OS_VERSION}"

        if [[ "${OS_NAME}" == *"Red Hat Enterprise Linux"* && "${OS_VERSION}" == *"9.7"* ]]; then
            log_success "RHEL 9.7 detected - proceeding with root deployment"
        else
            log_warning "Expected RHEL 9.7, found: ${OS_NAME} ${OS_VERSION}"
            log_warning "Continuing anyway, but some optimizations may not apply"
        fi
    else
        log_error "Unable to detect operating system"
        exit 1
    fi
}

# Setup root environment
setup_root_environment() {
    log_header "Setting up Root Environment"

    # Ensure root directories exist
    mkdir -p "${ROOT_HOME}/.ssh"
    mkdir -p "${KCLI_CONFIG_DIR}"
    mkdir -p "${KUBECONFIG_DIR}"

    # Set proper permissions
    chmod 700 "${ROOT_HOME}/.ssh"
    chmod 755 "${KCLI_CONFIG_DIR}"
    chmod 755 "${KUBECONFIG_DIR}"

    log_success "Root directories created and configured"

    # Check if SSH key exists, generate if needed
    if [[ ! -f "${ROOT_HOME}/.ssh/id_rsa.pub" ]]; then
        log_info "Generating SSH key for root user..."
        ssh-keygen -t rsa -b 4096 -C "root@$(hostname)" -f "${ROOT_HOME}/.ssh/id_rsa" -N ""
        log_success "SSH key generated for root"
    else
        log_success "SSH key already exists for root"
    fi

    # Display SSH key for reference
    log_info "Root SSH public key:"
    cat "${ROOT_HOME}/.ssh/id_rsa.pub"
}

# Install and configure KCLI for root
setup_kcli_for_root() {
    log_header "Installing and Configuring KCLI for Root"

    # Check if KCLI is already installed
    if command -v kcli &> /dev/null; then
        KCLI_VERSION=$(kcli version 2>/dev/null | grep -oP '\d+\.\d+\.\d+' | head -1 || echo "unknown")
        log_success "KCLI already installed: version ${KCLI_VERSION}"
    else
        log_info "Installing KCLI for root user on RHEL 9.7..."

        # Install prerequisites for RHEL 9.7
        dnf install -y python3-pip python3-devel libvirt-devel gcc pkg-config
        dnf install -y libvirt-daemon-kvm qemu-kvm libvirt-daemon-config-network

        # Install KCLI with all extras
        pip3 install --upgrade pip
        pip3 install kcli[all]

        # Verify installation
        if command -v kcli &> /dev/null; then
            KCLI_VERSION=$(kcli version 2>/dev/null | grep -oP '\d+\.\d+\.\d+' | head -1 || echo "unknown")
            log_success "KCLI installed successfully: version ${KCLI_VERSION}"
        else
            log_error "KCLI installation failed"
            exit 1
        fi
    fi

    # Configure KCLI for root
    export PATH="/usr/local/bin:$PATH"

    # Ensure KCLI can find the right paths
    kcli list host &>/dev/null || log_info "KCLI host configuration initialized"
}

# Setup and configure libvirt for root
setup_libvirt_for_root() {
    log_header "Configuring libvirt for Root User"

    # Enable and start libvirt services
    systemctl enable libvirtd
    systemctl start libvirtd

    # Wait for libvirt to be ready
    sleep 5

    # Check libvirt status
    if systemctl is-active --quiet libvirtd; then
        log_success "libvirtd is running"
    else
        log_error "libvirtd is not running"
        systemctl status libvirtd
        exit 1
    fi

    # Check KVM capabilities
    if [[ -e /dev/kvm ]]; then
        log_success "KVM acceleration available"
        ls -la /dev/kvm
    else
        log_warning "/dev/kvm not found - checking virtualization support"
        if grep -q vmx /proc/cpuinfo || grep -q svm /proc/cpuinfo; then
            log_warning "CPU supports virtualization but KVM module may not be loaded"
            modprobe kvm
            modprobe kvm_intel 2>/dev/null || modprobe kvm_amd 2>/dev/null || true
        else
            log_error "CPU does not support virtualization"
            exit 1
        fi
    fi

    # Configure libvirt default network
    if ! virsh net-list --all | grep -q "default.*active"; then
        log_info "Starting libvirt default network..."
        virsh net-start default 2>/dev/null || log_info "Default network already configured"
        virsh net-autostart default 2>/dev/null || log_info "Default network autostart configured"
    fi

    # Check and create storage pool if needed
    if ! virsh pool-list | grep -q "default.*active"; then
        log_info "Creating libvirt default storage pool..."
        mkdir -p /var/lib/libvirt/images
        virsh pool-define-as default dir --target /var/lib/libvirt/images 2>/dev/null || true
        virsh pool-start default 2>/dev/null || true
        virsh pool-autostart default 2>/dev/null || true
        log_success "libvirt storage pool configured"
    else
        log_success "libvirt storage pool already active"
    fi

    # Set proper ownership and permissions
    chown -R root:root /var/lib/libvirt/images
    chmod 755 /var/lib/libvirt/images

    log_success "libvirt configured for root user"
}

# Pre-flight checks adapted for root
preflight_checks() {
    log_header "Root User Pre-flight Checks"

    # Check if config file exists
    if [[ ! -f "${SCRIPT_DIR}/${CONFIG_FILE}" ]]; then
        log_error "Configuration file not found: ${SCRIPT_DIR}/${CONFIG_FILE}"
        log_info "Please ensure you're using the root configuration file: kcli-baremetal-params-root.yml"
        exit 1
    fi

    # Check for pull secret
    PULL_SECRET_PATH=$(grep -oP "pull_secret: '\K[^']*" "${SCRIPT_DIR}/${CONFIG_FILE}")
    if [[ ! -f "${PULL_SECRET_PATH}" ]]; then
        log_warning "Pull secret not found at: ${PULL_SECRET_PATH}"
        log_info "Please download your pull secret from https://console.redhat.com/openshift/install/pull-secret"
        log_info "Save it as: ${PULL_SECRET_PATH}"

        # Prompt for pull secret if missing
        read -p "Do you want to continue without pull secret validation? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Please download the pull secret and try again"
            exit 1
        fi
    else
        log_success "Pull secret found: ${PULL_SECRET_PATH}"
    fi

    # Check for SSH key
    SSH_KEY_PATH=$(grep -oP "ssh_key: '\K[^']*" "${SCRIPT_DIR}/${CONFIG_FILE}")
    if [[ ! -f "${SSH_KEY_PATH}" ]]; then
        log_error "SSH public key not found at: ${SSH_KEY_PATH}"
        log_info "SSH key should have been generated during environment setup"
        exit 1
    else
        log_success "SSH key found: ${SSH_KEY_PATH}"
    fi

    # Check for existing clusters and VMs that might conflict
    if kcli list cluster | grep -q "^${CLUSTER_NAME}"; then
        log_warning "Cluster '${CLUSTER_NAME}' already exists!"
        log_info "Existing cluster will cause KCLI to skip VM creation, leading to deployment failure"
        log_info "Automatically cleaning up existing cluster for fresh deployment..."
        
        # Always cleanup existing cluster to ensure fresh deployment
        log_info "Deleting existing cluster and VMs..."
        kcli delete cluster "${CLUSTER_NAME}" --yes || log_warning "Cluster deletion had issues, continuing..."
        
        # Wait for cleanup to complete
        sleep 10
        
        # Verify cleanup
        if kcli list cluster | grep -q "^${CLUSTER_NAME}"; then
            log_error "Failed to cleanup existing cluster. Manual cleanup required."
            log_info "Run: kcli delete cluster ${CLUSTER_NAME} --yes"
            exit 1
        fi
        
        log_success "Existing cluster cleaned up successfully"
    fi
    
    # Additional check for orphaned VMs that might cause skipping
    EXISTING_VMS=$(virsh list --all | grep "${CLUSTER_NAME}" | awk '{print $2}' || true)
    if [[ -n "$EXISTING_VMS" ]]; then
        log_warning "Found existing VMs that might conflict with deployment:"
        echo "$EXISTING_VMS" | while read vm; do
            [[ -n "$vm" ]] && log_info "  - $vm"
        done
        
        log_info "Cleaning up conflicting VMs to ensure fresh deployment..."
        echo "$EXISTING_VMS" | while read vm; do
            if [[ -n "$vm" ]]; then
                log_info "Removing VM: $vm"
                virsh destroy "$vm" 2>/dev/null || true
                virsh undefine "$vm" --remove-all-storage 2>/dev/null || true
            fi
        done
        
        log_success "Conflicting VMs cleaned up"
    fi

    log_success "Pre-flight checks completed"
}

# Deploy cluster with proper state management
deploy_cluster() {
    log_header "Deploying OpenShift Cluster"

    # Create deployment command with root paths
    DEPLOY_CMD="kcli create cluster openshift --paramfile ${SCRIPT_DIR}/${CONFIG_FILE} ${CLUSTER_NAME}"

    log_info "Running: ${DEPLOY_CMD}"
    log_info "This will take approximately 60-80 minutes..."

    # Run deployment with output logging
    LOG_FILE="/root/kcli-deploy-${CLUSTER_NAME}.log"
    
    # Run the deployment with timeout
    if timeout 7200 ${DEPLOY_CMD} 2>&1 | tee "${LOG_FILE}"; then
        log_success "Cluster deployment completed successfully"
        log_info "Deployment log saved to: ${LOG_FILE}"
    else
        local exit_code=$?
        log_error "Cluster deployment failed with exit code: ${exit_code}"
        log_error "Check logs at ${LOG_FILE}"
        
        # Check if this was a VM skipping issue (common cause)
        if grep -q "skipped on local" "${LOG_FILE}"; then
            log_error "VMs were skipped during deployment - this indicates conflicting existing resources"
            log_error "This script should have cleaned up conflicts in pre-flight checks"
            log_info "Try running the deployment again - cleanup may resolve the issue"
        fi
        
        exit $exit_code
    fi
}

# Monitor installation with root-specific paths
monitor_installation() {
    log_header "Monitoring Installation Progress"

    local max_wait=5400  # 90 minutes
    local wait_time=0
    local sleep_interval=60

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

# Post-installation setup for root
post_installation() {
    log_header "Post-installation Setup for Root"

    # Download kubeconfig to root directory
    KUBECONFIG_PATH="${ROOT_HOME}/.kcli/clusters/${CLUSTER_NAME}/auth/kubeconfig"

    if kcli download kubeconfig -c "${CLUSTER_NAME}"; then
        log_success "Kubeconfig downloaded to root directory"
    else
        log_error "Failed to download kubeconfig"
        exit 1
    fi

    # Set kubeconfig environment for root
    if [[ -f "${KUBECONFIG_PATH}" ]]; then
        export KUBECONFIG="${KUBECONFIG_PATH}"
        log_success "KUBECONFIG set for root: ${KUBECONFIG_PATH}"

        # Add to root's bashrc for persistence
        if ! grep -q "KUBECONFIG.*${CLUSTER_NAME}" "${ROOT_HOME}/.bashrc" 2>/dev/null; then
            echo "# Kubernaut E2E Cluster" >> "${ROOT_HOME}/.bashrc"
            echo "export KUBECONFIG=${KUBECONFIG_PATH}" >> "${ROOT_HOME}/.bashrc"
            log_success "KUBECONFIG added to root's .bashrc"
        fi
    else
        log_error "Kubeconfig not found: ${KUBECONFIG_PATH}"
        exit 1
    fi

    # Install OpenShift CLI if not present
    if ! command -v oc &> /dev/null; then
        log_info "Installing OpenShift CLI for root..."

        # Download and install oc CLI for RHEL 9.7
        cd /tmp
        curl -s https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable/openshift-client-linux.tar.gz \
            -o openshift-client-linux.tar.gz
        tar xzf openshift-client-linux.tar.gz
        mv oc kubectl /usr/local/bin/
        chmod +x /usr/local/bin/oc /usr/local/bin/kubectl

        # Verify installation
        if command -v oc &> /dev/null; then
            OC_VERSION=$(oc version --client -o json 2>/dev/null | grep -o '"gitVersion":"[^"]*"' | cut -d'"' -f4 || echo "unknown")
            log_success "OpenShift CLI installed: ${OC_VERSION}"
        else
            log_warning "OpenShift CLI installation may have failed"
        fi

        # Cleanup
        rm -f /tmp/openshift-client-linux.tar.gz
    fi

    # Test cluster connectivity
    if oc whoami &>/dev/null; then
        log_success "Cluster authentication successful as root"
        USERNAME=$(oc whoami 2>/dev/null || echo "unknown")
        log_info "Authenticated as: ${USERNAME}"
    else
        log_error "Cannot authenticate with cluster"
        exit 1
    fi

    # Check cluster nodes
    log_info "Checking cluster nodes..."
    if oc get nodes &>/dev/null; then
        TOTAL_NODES=$(oc get nodes --no-headers 2>/dev/null | wc -l || echo "0")
        READY_NODES=$(oc get nodes --no-headers 2>/dev/null | grep -c "Ready" || echo "0")
        log_success "Cluster nodes: ${READY_NODES}/${TOTAL_NODES} Ready"

        # Show node details
        echo ""
        oc get nodes -o wide
        echo ""
    else
        log_error "Cannot access cluster nodes"
        exit 1
    fi

    # Check cluster operators
    log_info "Checking cluster operators status..."
    if oc get co &>/dev/null; then
        TOTAL_OPERATORS=$(oc get co --no-headers 2>/dev/null | wc -l || echo "0")
        AVAILABLE_OPERATORS=$(oc get co --no-headers 2>/dev/null | grep -c "True.*False.*False" || echo "0")

        log_info "Cluster operators: ${AVAILABLE_OPERATORS}/${TOTAL_OPERATORS} Available"
        if [[ $AVAILABLE_OPERATORS -eq $TOTAL_OPERATORS ]]; then
            log_success "All cluster operators are Available"
        else
            log_warning "Some cluster operators are still initializing"
            log_info "This is normal for a new cluster - operators will stabilize over time"
        fi
    else
        log_warning "Cannot check cluster operators - may still be initializing"
    fi

    # Setup storage if configured
    setup_storage_if_enabled

    # Display access information
    print_cluster_info
}

# Setup storage if enabled in configuration
setup_storage_if_enabled() {
    log_info "Checking storage configuration..."

    # Check if storage operators are enabled in config
    if grep -q "storage_operators: true" "${SCRIPT_DIR}/${CONFIG_FILE}" 2>/dev/null; then
        log_info "Storage operators enabled, setting up storage..."

        # Make storage setup script executable
        chmod +x "${SCRIPT_DIR}/setup-storage.sh"

        # Run storage setup with root environment
        if KUBECONFIG="${KUBECONFIG}" "${SCRIPT_DIR}/setup-storage.sh"; then
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

# Print cluster information for root user
print_cluster_info() {
    log_header "Root User Cluster Access Information"

    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  CLUSTER ACCESS INFORMATION (ROOT)${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo -e "Cluster Name:     ${CLUSTER_NAME}"
    echo -e "Deployed by:      root on $(hostname)"
    echo -e "Kubeconfig:       ${KUBECONFIG_PATH}"
    echo -e "Admin User:       kubeadmin"

    # Get admin password
    ADMIN_PASSWORD_FILE="${ROOT_HOME}/.kcli/clusters/${CLUSTER_NAME}/auth/kubeadmin-password"
    if [[ -f "${ADMIN_PASSWORD_FILE}" ]]; then
        echo -e "Admin Password:   $(cat "${ADMIN_PASSWORD_FILE}")"
    fi

    # Get console URL
    CONSOLE_URL=$(oc get routes console -n openshift-console -o jsonpath='{.spec.host}' 2>/dev/null || echo "Not available")
    echo -e "Console URL:      https://${CONSOLE_URL}"

    echo -e "\n${BLUE}To access the cluster as root:${NC}"
    echo -e "  export KUBECONFIG=${KUBECONFIG_PATH}"
    echo -e "  oc get nodes"
    echo -e "  oc get co"

    echo -e "\n${BLUE}Storage Classes:${NC}"
    if oc get storageclass &>/dev/null; then
        oc get storageclass
    else
        echo -e "  Storage classes not yet available"
    fi

    echo -e "\n${BLUE}Cluster Resource Usage:${NC}"
    echo -e "  Nodes: $(oc get nodes --no-headers 2>/dev/null | wc -l || echo '0')"
    if command -v free &>/dev/null; then
        USED_RAM_GB=$(free -g | awk '/^Mem:/ {print $3}')
        TOTAL_RAM_GB=$(free -g | awk '/^Mem:/ {print $2}')
        echo -e "  Host RAM Usage: ${USED_RAM_GB}GB / ${TOTAL_RAM_GB}GB"
    fi

    echo -e "${GREEN}========================================${NC}\n"

    log_success "OpenShift 4.18 deployment completed successfully on RHEL 9.7 as root!"
    log_info "Cluster is ready for Kubernaut E2E testing"
}

# Cleanup on exit
cleanup() {
    log_info "Cleaning up temporary files..."
    # Add any cleanup logic here if needed
}

# Trap cleanup on exit
trap cleanup EXIT

# Main execution function
main() {
    log_info "Starting OpenShift 4.18 deployment for root on RHEL 9.7"
    log_info "This will take approximately 90-120 minutes to complete"
    echo ""

    # Execute all setup steps
    check_rhel97
    setup_root_environment
    setup_kcli_for_root
    setup_libvirt_for_root
    preflight_checks
    deploy_cluster
    monitor_installation
    post_installation

    log_success "Complete OpenShift 4.18 deployment finished successfully!"
}

# Check if script is being sourced or executed
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
