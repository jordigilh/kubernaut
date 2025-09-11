#!/bin/bash
# Bare Metal Setup Validation Script for OpenShift KCLI Deployment
# This script validates hardware, network, and prerequisites before deployment

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
CONFIG_FILE="${1:-kcli-baremetal-params.yml}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Status counters
CHECKS_PASSED=0
CHECKS_FAILED=0
CHECKS_WARNING=0

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[PASS]${NC} $1"; ((CHECKS_PASSED++)); }
log_warning() { echo -e "${YELLOW}[WARN]${NC} $1"; ((CHECKS_WARNING++)); }
log_error() { echo -e "${RED}[FAIL]${NC} $1"; ((CHECKS_FAILED++)); }
log_header() { echo -e "\n${CYAN}=== $1 ===${NC}"; }

# Banner
echo -e "${BLUE}"
cat << "EOF"
 ____                   __  __      _        _  __     __    _ _     _       _   _
| __ )  __ _ _ __ ___   |  \/  | ___| |_ __ _| | \ \   / /_ _| (_) __| | __ _| |_(_) ___  _ __
|  _ \ / _` | '__/ _ \  | |\/| |/ _ \ __/ _` | |  \ \ / / _` | | |/ _` |/ _` | __| |/ _ \| '_ \
| |_) | (_| | | |  __/  | |  | |  __/ || (_| | |   \ V / (_| | | | (_| | (_| | |_| | (_) | | | |
|____/ \__,_|_|  \___|  |_|  |_|\___|\__\__,_|_|    \_/ \__,_|_|_|\__,_|\__,_|\__|_|\___/|_| |_|
EOF
echo -e "${NC}"

log_info "Validating bare metal setup for OpenShift 4.18 KCLI deployment"
log_info "Configuration file: ${CONFIG_FILE}"

# Check if configuration file exists
if [[ ! -f "${SCRIPT_DIR}/${CONFIG_FILE}" ]]; then
    log_error "Configuration file not found: ${SCRIPT_DIR}/${CONFIG_FILE}"
    exit 1
fi

# 1. System Requirements Check for RHEL 9.7
check_system_requirements() {
    log_header "RHEL 9.7 Host Requirements"

    # Check OS - specifically for RHEL 9.7
    if [[ -f /etc/os-release ]]; then
        OS_NAME=$(grep '^NAME=' /etc/os-release | cut -d'"' -f2)
        OS_VERSION=$(grep '^VERSION=' /etc/os-release | cut -d'"' -f2)
        log_info "Operating System: ${OS_NAME} ${OS_VERSION}"

        if [[ "${OS_NAME}" == *"Red Hat Enterprise Linux"* && "${OS_VERSION}" == *"9.7"* ]]; then
            log_success "RHEL 9.7 detected ✅"
        else
            log_warning "Expected RHEL 9.7, found: ${OS_NAME} ${OS_VERSION}"
        fi
    else
        log_error "Unable to detect operating system"
    fi

    # Check CPU - Intel Xeon Gold 5218R expected
    CPU_MODEL=$(lscpu | grep "Model name" | sed 's/Model name: *//')
    CPU_CORES=$(nproc)
    log_info "CPU Model: ${CPU_MODEL}"
    log_info "CPU Threads: ${CPU_CORES}"

    if [[ "${CPU_MODEL}" == *"Xeon"* && "${CPU_MODEL}" == *"5218R"* ]]; then
        log_success "Intel Xeon Gold 5218R detected ✅"
    else
        log_info "Expected Intel Xeon Gold 5218R, found: ${CPU_MODEL}"
    fi

    if [[ ${CPU_CORES} -ge 28 ]]; then
        log_success "CPU threads: ${CPU_CORES} (sufficient for cluster + headroom)"
    elif [[ ${CPU_CORES} -ge 24 ]]; then
        log_success "CPU threads: ${CPU_CORES} (adequate for testing cluster)"
    else
        log_warning "CPU threads: ${CPU_CORES} (may be insufficient for full cluster)"
    fi

    # Check memory - 256GB expected
    MEMORY_GB=$(free -g | awk '/^Mem:/ {print $2}')
    log_info "Total Memory: ${MEMORY_GB} GB"

    if [[ ${MEMORY_GB} -ge 250 ]]; then
        log_success "Memory: ${MEMORY_GB} GB (matches expected ~256 GB) ✅"
    elif [[ ${MEMORY_GB} -ge 100 ]]; then
        log_success "Memory: ${MEMORY_GB} GB (sufficient for testing cluster)"
    else
        log_error "Memory: ${MEMORY_GB} GB (insufficient for OpenShift cluster)"
    fi

    # Calculate cluster resource usage
    CLUSTER_RAM_GB=84  # 48 (masters) + 36 (workers)
    BOOTSTRAP_RAM_GB=16 # temporary during installation
    TOTAL_PEAK_GB=$((CLUSTER_RAM_GB + BOOTSTRAP_RAM_GB))
    AVAILABLE_RAM_GB=$((MEMORY_GB - TOTAL_PEAK_GB))

    log_info "Cluster RAM usage: ${CLUSTER_RAM_GB} GB + ${BOOTSTRAP_RAM_GB} GB bootstrap = ${TOTAL_PEAK_GB} GB peak"
    if [[ ${AVAILABLE_RAM_GB} -gt 50 ]]; then
        log_success "Available headroom: ${AVAILABLE_RAM_GB} GB (excellent)"
    elif [[ ${AVAILABLE_RAM_GB} -gt 20 ]]; then
        log_success "Available headroom: ${AVAILABLE_RAM_GB} GB (adequate)"
    else
        log_warning "Available headroom: ${AVAILABLE_RAM_GB} GB (may be tight)"
    fi

    # Check disk space - 3TB expected
    DISK_SPACE_GB=$(df -BG /var/lib/libvirt/images 2>/dev/null | awk 'NR==2 {print $4}' | sed 's/G//' || df -BG "${HOME}" | awk 'NR==2 {print $4}' | sed 's/G//')
    DISK_SPACE_TB=$((DISK_SPACE_GB / 1024))

    log_info "Available disk space: ${DISK_SPACE_GB} GB (~${DISK_SPACE_TB} TB)"

    if [[ ${DISK_SPACE_TB} -ge 2 ]]; then
        log_success "Disk space: ${DISK_SPACE_TB} TB (excellent for testing environment) ✅"
    elif [[ ${DISK_SPACE_GB} -ge 600 ]]; then
        log_success "Disk space: ${DISK_SPACE_GB} GB (sufficient for cluster)"
    else
        log_error "Disk space: ${DISK_SPACE_GB} GB (insufficient for OpenShift cluster + storage)"
    fi

    # Calculate storage usage
    CLUSTER_STORAGE_GB=500  # ~480 GB for VMs + overhead
    ODF_STORAGE_GB=600      # 200 GB × 3 workers
    TOTAL_USAGE_GB=$((CLUSTER_STORAGE_GB + ODF_STORAGE_GB))
    AVAILABLE_STORAGE_GB=$((DISK_SPACE_GB - TOTAL_USAGE_GB))

    log_info "Estimated cluster storage usage: ${TOTAL_USAGE_GB} GB (VMs + ODF)"
    if [[ ${AVAILABLE_STORAGE_GB} -gt 1000 ]]; then
        log_success "Remaining storage: ${AVAILABLE_STORAGE_GB} GB (plenty of headroom)"
    elif [[ ${AVAILABLE_STORAGE_GB} -gt 200 ]]; then
        log_success "Remaining storage: ${AVAILABLE_STORAGE_GB} GB (adequate headroom)"
    else
        log_warning "Remaining storage: ${AVAILABLE_STORAGE_GB} GB (monitor usage carefully)"
    fi
}

# 2. Software Dependencies Check for RHEL 9.7
check_software_dependencies() {
    log_header "RHEL 9.7 Software Dependencies"

    # Check KCLI
    if command -v kcli &> /dev/null; then
        KCLI_VERSION=$(kcli version 2>/dev/null | grep -oP '\d+\.\d+\.\d+' | head -1 || echo "unknown")
        log_success "KCLI installed: version ${KCLI_VERSION}"
    else
        log_error "KCLI is not installed"
        log_info "Install with: sudo dnf install -y python3-pip python3-devel libvirt-devel gcc pkg-config && sudo pip3 install kcli[all]"
    fi

    # Check libvirt - should already be installed and running
    if command -v virsh &> /dev/null; then
        if systemctl is-active --quiet libvirtd; then
            log_success "libvirtd is running ✅"
        else
            log_error "libvirtd is not running"
            log_info "Start with: sudo systemctl start libvirtd"
        fi

        # Check user in libvirt group
        if groups "${USER}" | grep -q libvirt; then
            log_success "User ${USER} is in libvirt group ✅"
        else
            log_warning "User ${USER} is not in libvirt group"
            log_info "Add with: sudo usermod -aG libvirt ${USER} && newgrp libvirt"
        fi

        # Check KVM capabilities
        if [[ -e /dev/kvm ]]; then
            log_success "KVM acceleration available ✅"
        else
            log_warning "/dev/kvm not found - virtualization may be slow"
        fi

        # Check libvirt storage pool
        if virsh pool-list | grep -q "default.*active"; then
            POOL_PATH=$(virsh pool-dumpxml default | grep -oP '<path>\K[^<]*' || echo "/var/lib/libvirt/images")
            log_success "libvirt default storage pool is active: ${POOL_PATH}"
        else
            log_warning "libvirt default storage pool is not active"
            log_info "This may be created automatically during KCLI usage"
        fi
    else
        log_error "libvirt is not installed"
        log_info "Install with: sudo dnf install libvirt-daemon-kvm qemu-kvm"
    fi

    # Check RHEL 9.7 specific packages
    REQUIRED_PACKAGES=("python3-devel" "libvirt-devel" "gcc" "pkg-config")
    for pkg in "${REQUIRED_PACKAGES[@]}"; do
        if rpm -q "$pkg" &>/dev/null; then
            log_success "Required package installed: $pkg"
        else
            log_warning "Missing package: $pkg"
            log_info "Install with: sudo dnf install -y $pkg"
        fi
    done

    # Check ipmitool
    if command -v ipmitool &> /dev/null; then
        log_success "ipmitool is available for BMC testing"
    else
        log_warning "ipmitool is not installed (recommended for BMC validation)"
        log_info "Install with: sudo dnf install ipmitool"
    fi

    # Check OpenShift CLI
    if command -v oc &> /dev/null; then
        OC_VERSION=$(oc version --client -o json 2>/dev/null | jq -r '.releaseClientVersion' || echo "unknown")
        log_success "OpenShift CLI installed: version ${OC_VERSION}"
    else
        log_warning "OpenShift CLI (oc) is not installed"
        log_info "KCLI can download it automatically, or install manually"
    fi

    # Check jq
    if command -v jq &> /dev/null; then
        log_success "jq is available for JSON processing"
    else
        log_warning "jq is not installed (helpful for debugging)"
        log_info "Install with: sudo dnf install jq"
    fi
}

# 3. Authentication Files Check
check_authentication_files() {
    log_header "Authentication Files"

    # Check pull secret
    PULL_SECRET_PATH=$(grep -oP "pull_secret: '\K[^']*" "${SCRIPT_DIR}/${CONFIG_FILE}" | sed "s|~|$HOME|")
    if [[ -f "${PULL_SECRET_PATH}" ]]; then
        # Validate pull secret format
        if jq empty "${PULL_SECRET_PATH}" 2>/dev/null; then
            log_success "Pull secret found and valid JSON: ${PULL_SECRET_PATH}"

            # Check for required registries
            REQUIRED_REGISTRIES=("quay.io" "registry.redhat.io" "cloud.openshift.com")
            for registry in "${REQUIRED_REGISTRIES[@]}"; do
                if jq -e ".auths.\"${registry}\"" "${PULL_SECRET_PATH}" &>/dev/null; then
                    log_success "Pull secret contains ${registry} registry"
                else
                    log_warning "Pull secret missing ${registry} registry"
                fi
            done
        else
            log_error "Pull secret is not valid JSON: ${PULL_SECRET_PATH}"
        fi
    else
        log_error "Pull secret not found: ${PULL_SECRET_PATH}"
        log_info "Download from: https://console.redhat.com/openshift/install/pull-secret"
    fi

    # Check SSH key
    SSH_KEY_PATH=$(grep -oP "ssh_key: '\K[^']*" "${SCRIPT_DIR}/${CONFIG_FILE}" | sed "s|~|$HOME|")
    if [[ -f "${SSH_KEY_PATH}" ]]; then
        # Validate SSH key format
        if ssh-keygen -l -f "${SSH_KEY_PATH}" &>/dev/null; then
            SSH_KEY_TYPE=$(ssh-keygen -l -f "${SSH_KEY_PATH}" | awk '{print $4}')
            log_success "SSH public key found and valid: ${SSH_KEY_PATH} (${SSH_KEY_TYPE})"
        else
            log_error "SSH public key is not valid: ${SSH_KEY_PATH}"
        fi
    else
        log_error "SSH public key not found: ${SSH_KEY_PATH}"
        log_info "Generate with: ssh-keygen -t rsa -b 4096 -C 'your-email@kubernaut.io'"
    fi
}

# 4. Network Configuration Check
check_network_configuration() {
    log_header "Network Configuration"

    # Extract network settings from config
    API_IP=$(grep -oP 'api_ip: \K[0-9.]+' "${SCRIPT_DIR}/${CONFIG_FILE}")
    INGRESS_IP=$(grep -oP 'ingress_ip: \K[0-9.]+' "${SCRIPT_DIR}/${CONFIG_FILE}")
    CIDR=$(grep -oP 'cidr: \K[0-9./]+' "${SCRIPT_DIR}/${CONFIG_FILE}")
    DOMAIN=$(grep -oP 'domain: \K\S+' "${SCRIPT_DIR}/${CONFIG_FILE}")
    CLUSTER=$(grep -oP 'cluster: \K\S+' "${SCRIPT_DIR}/${CONFIG_FILE}")

    log_info "Network configuration:"
    log_info "  API VIP: ${API_IP}"
    log_info "  Ingress VIP: ${INGRESS_IP}"
    log_info "  Network CIDR: ${CIDR}"
    log_info "  Domain: ${DOMAIN}"
    log_info "  Cluster: ${CLUSTER}"

    # Check if VIPs are reachable (they shouldn't be before installation)
    if ping -c 1 -W 2 "${API_IP}" &>/dev/null; then
        log_warning "API VIP ${API_IP} is already responding (should be unused)"
    else
        log_success "API VIP ${API_IP} is unused (good)"
    fi

    if ping -c 1 -W 2 "${INGRESS_IP}" &>/dev/null; then
        log_warning "Ingress VIP ${INGRESS_IP} is already responding (should be unused)"
    else
        log_success "Ingress VIP ${INGRESS_IP} is unused (good)"
    fi

    # Check DNS resolution (if configured)
    DNS_NAMES=(
        "api.${CLUSTER}.${DOMAIN}"
        "api-int.${CLUSTER}.${DOMAIN}"
        "*.apps.${CLUSTER}.${DOMAIN}"
    )

    for dns_name in "${DNS_NAMES[@]}"; do
        # Skip wildcard resolution test
        if [[ "${dns_name}" == "*"* ]]; then
            log_info "DNS name: ${dns_name} (wildcard - manual verification needed)"
            continue
        fi

        if nslookup "${dns_name}" &>/dev/null; then
            RESOLVED_IP=$(nslookup "${dns_name}" | grep -A1 "Name:" | tail -1 | awk '{print $2}')
            log_success "DNS resolution: ${dns_name} → ${RESOLVED_IP}"
        else
            log_warning "DNS resolution failed: ${dns_name}"
            log_info "Configure DNS or use /etc/hosts entries"
        fi
    done
}

# 5. Virtual Environment Check (No BMC needed)
check_virtual_environment() {
    log_header "Virtual Environment Configuration"

    # This is a virtual environment running on RHEL 9.7, so no physical BMC checks needed
    log_info "Running KCLI virtual deployment on RHEL 9.7 host"
    log_success "BMC connectivity not required for virtual environment ✅"

    # Check if configuration contains virtual node definitions
    if grep -q "ipmi_address" "${SCRIPT_DIR}/${CONFIG_FILE}"; then
        NODE_COUNT=$(grep -c "ipmi_address" "${SCRIPT_DIR}/${CONFIG_FILE}")
        log_info "Configuration contains ${NODE_COUNT} virtual node definitions"

        # Verify the configuration is properly formatted for KCLI virtual deployment
        if grep -q "hypervisor: kvm" "${SCRIPT_DIR}/${CONFIG_FILE}"; then
            log_success "KVM hypervisor configured in parameters ✅"
        else
            log_warning "KVM hypervisor not explicitly configured - KCLI will use defaults"
        fi

        # Check for reasonable resource allocation
        MASTER_MEM=$(grep -oP 'ctlplane_memory: \K\d+' "${SCRIPT_DIR}/${CONFIG_FILE}" || echo "0")
        WORKER_MEM=$(grep -oP 'worker_memory: \K\d+' "${SCRIPT_DIR}/${CONFIG_FILE}" || echo "0")

        if [[ ${MASTER_MEM} -le 16384 && ${WORKER_MEM} -le 12288 ]]; then
            log_success "Memory allocation optimized for testing: Masters=${MASTER_MEM}MB, Workers=${WORKER_MEM}MB"
        else
            log_warning "Memory allocation may be high for testing environment"
        fi

        # Check storage configuration
        if grep -q "storage_operators: true" "${SCRIPT_DIR}/${CONFIG_FILE}"; then
            log_success "Storage operators enabled for automatic setup ✅"

            ODF_SIZE=$(grep -oP 'odf_size: "\K[^"]*' "${SCRIPT_DIR}/${CONFIG_FILE}" || echo "unknown")
            log_info "ODF size configured: ${ODF_SIZE} per worker"

            if [[ "${ODF_SIZE}" == "200Gi" ]]; then
                log_success "ODF size optimized for testing environment ✅"
            elif [[ "${ODF_SIZE}" =~ ^[0-9]+Gi$ ]]; then
                log_info "ODF size: ${ODF_SIZE} (custom configuration)"
            else
                log_warning "ODF size may be too large for testing: ${ODF_SIZE}"
            fi
        else
            log_info "Storage operators not enabled - manual storage setup required"
        fi
    else
        log_warning "No virtual node definitions found in configuration"
    fi
}

# 6. Hardware Inventory Check
check_hardware_inventory() {
    log_header "Hardware Inventory"

    # Count nodes by role
    MASTER_COUNT=$(grep -c 'role: master' "${SCRIPT_DIR}/${CONFIG_FILE}" || echo 0)
    WORKER_COUNT=$(grep -c 'role: worker' "${SCRIPT_DIR}/${CONFIG_FILE}" || echo 0)
    TOTAL_NODES=$((MASTER_COUNT + WORKER_COUNT))

    log_info "Hardware inventory:"
    log_info "  Master nodes: ${MASTER_COUNT}"
    log_info "  Worker nodes: ${WORKER_COUNT}"
    log_info "  Total nodes: ${TOTAL_NODES}"

    # Validate minimum requirements
    if [[ ${MASTER_COUNT} -ge 3 ]]; then
        log_success "Master node count: ${MASTER_COUNT} (≥3 required)"
    else
        log_error "Master node count: ${MASTER_COUNT} (minimum 3 required)"
    fi

    if [[ ${WORKER_COUNT} -ge 2 ]]; then
        log_success "Worker node count: ${WORKER_COUNT} (≥2 required)"
    else
        log_warning "Worker node count: ${WORKER_COUNT} (minimum 2 recommended)"
    fi

    # Check for duplicate MAC addresses
    MAC_ADDRESSES=$(grep -oP 'mac: "\K[^"]*' "${SCRIPT_DIR}/${CONFIG_FILE}")
    DUPLICATE_MACS=$(echo "${MAC_ADDRESSES}" | sort | uniq -d)

    if [[ -z "${DUPLICATE_MACS}" ]]; then
        log_success "No duplicate MAC addresses found"
    else
        log_error "Duplicate MAC addresses found:"
        echo "${DUPLICATE_MACS}" | while read -r mac; do
            log_error "  Duplicate MAC: ${mac}"
        done
    fi

    # Check for duplicate BMC addresses
    DUPLICATE_BMCS=$(echo "${BMC_ADDRESSES}" | sort | uniq -d)

    if [[ -z "${DUPLICATE_BMCS}" ]]; then
        log_success "No duplicate BMC addresses found"
    else
        log_error "Duplicate BMC addresses found:"
        echo "${DUPLICATE_BMCS}" | while read -r bmc; do
            log_error "  Duplicate BMC: ${bmc}"
        done
    fi
}

# 7. Storage Check
check_storage() {
    log_header "Storage Configuration"

    # Check if libvirt storage pool exists
    if command -v virsh &>/dev/null; then
        if virsh pool-list | grep -q "default.*active"; then
            POOL_PATH=$(virsh pool-dumpxml default | grep -oP '<path>\K[^<]*')
            POOL_AVAILABLE=$(df -BG "${POOL_PATH}" | awk 'NR==2 {print $4}' | sed 's/G//')

            log_success "libvirt default storage pool is active: ${POOL_PATH}"

            if [[ ${POOL_AVAILABLE} -ge 500 ]]; then
                log_success "Storage pool available space: ${POOL_AVAILABLE} GB (≥500 GB recommended)"
            else
                log_warning "Storage pool available space: ${POOL_AVAILABLE} GB (500+ GB recommended for cluster)"
            fi
        else
            log_warning "libvirt default storage pool is not active"
            log_info "Create with: virsh pool-define-as default dir --target /var/lib/libvirt/images"
        fi
    fi
}

# Summary
print_summary() {
    log_header "Validation Summary"

    TOTAL_CHECKS=$((CHECKS_PASSED + CHECKS_FAILED + CHECKS_WARNING))

    echo -e "${GREEN}Passed: ${CHECKS_PASSED}${NC}"
    echo -e "${YELLOW}Warnings: ${CHECKS_WARNING}${NC}"
    echo -e "${RED}Failed: ${CHECKS_FAILED}${NC}"
    echo -e "Total checks: ${TOTAL_CHECKS}"

    if [[ ${CHECKS_FAILED} -eq 0 ]]; then
        echo -e "\n${GREEN}✓ System is ready for OpenShift deployment!${NC}"
        if [[ ${CHECKS_WARNING} -gt 0 ]]; then
            echo -e "${YELLOW}⚠ Please review warnings above${NC}"
        fi
        echo -e "\nNext steps:"
        echo -e "  1. Review and customize ${CONFIG_FILE}"
        echo -e "  2. Run: ./deploy-kcli-cluster.sh"
        exit 0
    else
        echo -e "\n${RED}✗ System is not ready for deployment${NC}"
        echo -e "${RED}Please fix the failed checks above${NC}"
        exit 1
    fi
}

# Main execution
main() {
    check_system_requirements
    check_software_dependencies
    check_authentication_files
    check_network_configuration
    check_virtual_environment
    check_hardware_inventory
    check_storage
    print_summary
}

# Run main function
main "$@"
