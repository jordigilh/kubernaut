#!/bin/bash
# Bare Metal Setup Validation Script for OpenShift KCLI Deployment - Root User on RHEL 9.7
# This script validates hardware, network, and prerequisites before deployment as root

set -euo pipefail

# Root user validation
if [[ $EUID -ne 0 ]]; then
    echo -e "\033[0;31m[ERROR]\033[0m This script must be run as root for RHEL 9.7 deployment"
    echo "Usage: sudo $0 [config-file]"
    exit 1
fi

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
CONFIG_FILE="${1:-kcli-baremetal-params-root.yml}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_HOME="/root"

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
 ____             _     _   _       _ _     _       _   _                ____             _
|  _ \ ___   ___ | |_  | | | | __ _| (_) __| | __ _| |_(_) ___  _ __   |  _ \ ___   ___ | |_
| |_) / _ \ / _ \| __| | | | |/ _` | | |/ _` |/ _` | __| |/ _ \| '_ \  | |_) / _ \ / _ \| __|
|  _ < (_) | (_) | |_  | |_| | (_| | | | (_| | (_| | |_| | (_) | | | | |  _ < (_) | (_) | |_
|_| \_\___/ \___/ \__|  \___/ \__,_|_|_|\__,_|\__,_|\__|_|\___/|_| |_| |_| \_\___/ \___/ \__|
     ____  _   _ _____ _        ___    _____   ____             _
    |  _ \| | | | ____| |      / _ \  |___  | |  _ \ ___   ___ | |_
    | |_) | |_| |  _| | |     | (_) |    / /  | |_) / _ \ / _ \| __|
    |  _ <|  _  | |___| |___   \__, |   / /   |  _ < (_) | (_) | |_
    |_| \_\_| |_|_____|_____|    /_/   /_/    |_| \_\___/ \___/ \__|
EOF
echo -e "${NC}"

log_info "Validating bare metal setup for OpenShift 4.18 KCLI deployment (Root User)"
log_info "RHEL 9.7 Host: $(hostname)"
log_info "Running as: $(whoami)"
log_info "Configuration file: ${CONFIG_FILE}"

# Check if configuration file exists
if [[ ! -f "${SCRIPT_DIR}/${CONFIG_FILE}" ]]; then
    log_error "Configuration file not found: ${SCRIPT_DIR}/${CONFIG_FILE}"
    log_info "Please ensure you're using the root configuration file: kcli-baremetal-params-root.yml"
    exit 1
fi

# 1. Root User and System Requirements Check for RHEL 9.7
check_root_system_requirements() {
    log_header "Root User & RHEL 9.7 Host Requirements"

    # Validate root user
    if [[ $EUID -eq 0 ]]; then
        log_success "Running as root user ✅"
        log_info "Root home directory: ${ROOT_HOME}"
    else
        log_error "Not running as root user"
    fi

    # Check OS - specifically for RHEL 9.7
    if [[ -f /etc/os-release ]]; then
        OS_NAME=$(grep '^NAME=' /etc/os-release | cut -d'"' -f2)
        OS_VERSION=$(grep '^VERSION=' /etc/os-release | cut -d'"' -f2)
        OS_ID=$(grep '^ID=' /etc/os-release | cut -d'"' -f2)
        log_info "Operating System: ${OS_NAME} ${OS_VERSION} (${OS_ID})"

        if [[ "${OS_NAME}" == *"Red Hat Enterprise Linux"* && "${OS_VERSION}" == *"9.7"* ]]; then
            log_success "RHEL 9.7 detected - optimal for root deployment ✅"
        elif [[ "${OS_ID}" == "rhel" && "${OS_VERSION}" == *"9"* ]]; then
            log_warning "RHEL 9.x detected (not 9.7) - should work but 9.7 is recommended"
        else
            log_warning "Expected RHEL 9.7, found: ${OS_NAME} ${OS_VERSION}"
        fi
    else
        log_error "Unable to detect operating system"
    fi

    # Check CPU - Intel Xeon Gold 5218R expected
    CPU_MODEL=$(lscpu | grep "Model name" | sed 's/Model name: *//')
    CPU_CORES=$(nproc)
    CPU_SOCKETS=$(lscpu | grep "Socket(s)" | awk '{print $2}')
    CPU_THREADS_PER_CORE=$(lscpu | grep "Thread(s) per core" | awk '{print $4}')

    log_info "CPU Model: ${CPU_MODEL}"
    log_info "CPU Cores: ${CPU_CORES} (${CPU_SOCKETS} socket(s), ${CPU_THREADS_PER_CORE} thread(s)/core)"

    if [[ "${CPU_MODEL}" == *"Xeon"* && "${CPU_MODEL}" == *"5218R"* ]]; then
        log_success "Intel Xeon Gold 5218R detected - optimal configuration ✅"
    elif [[ "${CPU_MODEL}" == *"Xeon"* ]]; then
        log_success "Intel Xeon processor detected - good for virtualization ✅"
    else
        log_info "CPU: ${CPU_MODEL} - ensure it supports virtualization"
    fi

    if [[ ${CPU_CORES} -ge 28 ]]; then
        log_success "CPU threads: ${CPU_CORES} (excellent for cluster + headroom)"
    elif [[ ${CPU_CORES} -ge 24 ]]; then
        log_success "CPU threads: ${CPU_CORES} (good for testing cluster)"
    elif [[ ${CPU_CORES} -ge 16 ]]; then
        log_warning "CPU threads: ${CPU_CORES} (adequate but may be tight for full cluster)"
    else
        log_warning "CPU threads: ${CPU_CORES} (may be insufficient for full cluster with headroom)"
    fi

    # Check memory - 256GB expected
    MEMORY_GB=$(free -g | awk '/^Mem:/ {print $2}')
    MEMORY_MB=$(free -m | awk '/^Mem:/ {print $2}')
    log_info "Total Memory: ${MEMORY_GB} GB (${MEMORY_MB} MB)"

    if [[ ${MEMORY_GB} -ge 250 ]]; then
        log_success "Memory: ${MEMORY_GB} GB (matches expected ~256 GB configuration) ✅"
    elif [[ ${MEMORY_GB} -ge 128 ]]; then
        log_success "Memory: ${MEMORY_GB} GB (sufficient for testing cluster)"
    elif [[ ${MEMORY_GB} -ge 64 ]]; then
        log_warning "Memory: ${MEMORY_GB} GB (may be tight for full cluster)"
    else
        log_error "Memory: ${MEMORY_GB} GB (insufficient for OpenShift cluster with headroom)"
    fi

    # Calculate cluster resource usage for root deployment
    CLUSTER_RAM_GB=84  # 48 (masters) + 36 (workers)
    BOOTSTRAP_RAM_GB=16 # temporary during installation
    TOTAL_PEAK_GB=$((CLUSTER_RAM_GB + BOOTSTRAP_RAM_GB))
    AVAILABLE_RAM_GB=$((MEMORY_GB - TOTAL_PEAK_GB))

    log_info "Estimated cluster RAM usage: ${CLUSTER_RAM_GB} GB + ${BOOTSTRAP_RAM_GB} GB bootstrap = ${TOTAL_PEAK_GB} GB peak"
    if [[ ${AVAILABLE_RAM_GB} -gt 50 ]]; then
        log_success "Available headroom: ${AVAILABLE_RAM_GB} GB (excellent for root deployment)"
    elif [[ ${AVAILABLE_RAM_GB} -gt 20 ]]; then
        log_success "Available headroom: ${AVAILABLE_RAM_GB} GB (adequate for testing)"
    else
        log_warning "Available headroom: ${AVAILABLE_RAM_GB} GB (may be tight - monitor usage)"
    fi

    # Check disk space - 3TB expected, focus on root filesystem and libvirt storage
    ROOT_DISK_GB=$(df -BG / 2>/dev/null | awk 'NR==2 {print $4}' | sed 's/G//' || echo "0")
    LIBVIRT_DISK_GB=$(df -BG /var/lib/libvirt/images 2>/dev/null | awk 'NR==2 {print $4}' | sed 's/G//' || echo "$ROOT_DISK_GB")

    log_info "Root filesystem space: ${ROOT_DISK_GB} GB available"
    log_info "Libvirt storage space: ${LIBVIRT_DISK_GB} GB available"

    # Use the larger of the two for calculations
    EFFECTIVE_DISK_GB=$LIBVIRT_DISK_GB
    DISK_SPACE_TB=$((EFFECTIVE_DISK_GB / 1024))

    if [[ ${DISK_SPACE_TB} -ge 2 ]]; then
        log_success "Disk space: ${DISK_SPACE_TB} TB (~${EFFECTIVE_DISK_GB} GB) - excellent for testing ✅"
    elif [[ ${EFFECTIVE_DISK_GB} -ge 600 ]]; then
        log_success "Disk space: ${EFFECTIVE_DISK_GB} GB - sufficient for cluster + storage"
    elif [[ ${EFFECTIVE_DISK_GB} -ge 400 ]]; then
        log_warning "Disk space: ${EFFECTIVE_DISK_GB} GB - adequate but monitor usage closely"
    else
        log_error "Disk space: ${EFFECTIVE_DISK_GB} GB - insufficient for OpenShift cluster + ODF storage"
    fi

    # Calculate storage usage for root deployment
    CLUSTER_STORAGE_GB=500  # ~480 GB for VMs + overhead
    ODF_STORAGE_GB=600      # 200 GB × 3 workers for ODF
    TOTAL_USAGE_GB=$((CLUSTER_STORAGE_GB + ODF_STORAGE_GB))
    AVAILABLE_STORAGE_GB=$((EFFECTIVE_DISK_GB - TOTAL_USAGE_GB))

    log_info "Estimated cluster storage usage: ${TOTAL_USAGE_GB} GB (VMs + ODF)"
    if [[ ${AVAILABLE_STORAGE_GB} -gt 1000 ]]; then
        log_success "Remaining storage: ${AVAILABLE_STORAGE_GB} GB (plenty of headroom)"
    elif [[ ${AVAILABLE_STORAGE_GB} -gt 200 ]]; then
        log_success "Remaining storage: ${AVAILABLE_STORAGE_GB} GB (adequate headroom)"
    else
        log_warning "Remaining storage: ${AVAILABLE_STORAGE_GB} GB (monitor usage carefully)"
    fi
}

# 2. Root User Software Dependencies Check for RHEL 9.7
check_root_software_dependencies() {
    log_header "Root User Software Dependencies (RHEL 9.7)"

    # Check KCLI installation status
    if command -v kcli &> /dev/null; then
        KCLI_VERSION=$(kcli version 2>/dev/null | grep -oP '\d+\.\d+\.\d+' | head -1 || echo "unknown")
        log_success "KCLI installed for root: version ${KCLI_VERSION}"

        # Check KCLI configuration
        if [[ -d "${ROOT_HOME}/.kcli" ]]; then
            log_success "KCLI configuration directory exists for root"
        else
            log_warning "KCLI configuration directory not found - will be created during setup"
        fi
    else
        log_warning "KCLI is not installed for root user"
        log_info "Root installation: dnf install -y python3-pip && pip3 install kcli[all]"
    fi

    # Check Python 3 and pip
    if command -v python3 &> /dev/null; then
        PYTHON_VERSION=$(python3 --version 2>&1 | awk '{print $2}')
        log_success "Python 3 available: ${PYTHON_VERSION}"
    else
        log_error "Python 3 is not installed"
        log_info "Install with: dnf install -y python3"
    fi

    if command -v pip3 &> /dev/null; then
        PIP_VERSION=$(pip3 --version 2>&1 | awk '{print $2}')
        log_success "pip3 available: ${PIP_VERSION}"
    else
        log_warning "pip3 is not installed"
        log_info "Install with: dnf install -y python3-pip"
    fi

    # Check libvirt and KVM for root
    if systemctl is-active --quiet libvirtd; then
        log_success "libvirtd is running"
        LIBVIRT_VERSION=$(libvirtd --version 2>&1 | grep -o '[0-9]\+\.[0-9]\+\.[0-9]\+' | head -1 || echo "unknown")
        log_info "libvirt version: ${LIBVIRT_VERSION}"
    else
        log_warning "libvirtd is not running"
        log_info "Start with: systemctl start libvirtd"
    fi

    # Check KVM capabilities for root
    if [[ -e /dev/kvm ]]; then
        KVM_PERMS=$(ls -la /dev/kvm)
        log_success "KVM acceleration available: ${KVM_PERMS}"

        # Check if root can access KVM
        if [[ -r /dev/kvm && -w /dev/kvm ]]; then
            log_success "Root has KVM access permissions ✅"
        else
            log_warning "Root may not have proper KVM access"
        fi
    else
        log_warning "/dev/kvm not found - checking virtualization support"
        if grep -q vmx /proc/cpuinfo || grep -q svm /proc/cpuinfo; then
            log_warning "CPU supports virtualization but KVM module may not be loaded"
            log_info "Load with: modprobe kvm && modprobe kvm_intel"
        else
            log_error "CPU does not support virtualization"
        fi
    fi

    # Check RHEL 9.7 specific packages for root deployment
    REQUIRED_PACKAGES=("python3-devel" "libvirt-devel" "gcc" "pkg-config" "libvirt-daemon-kvm" "qemu-kvm")
    for pkg in "${REQUIRED_PACKAGES[@]}"; do
        if rpm -q "$pkg" &>/dev/null; then
            log_success "Required package installed: $pkg"
        else
            log_warning "Missing package: $pkg"
            log_info "Install with: dnf install -y $pkg"
        fi
    done

    # Check libvirt storage pool for root
    if command -v virsh &> /dev/null; then
        if virsh pool-list | grep -q "default.*active"; then
            POOL_PATH=$(virsh pool-dumpxml default | grep -oP '<path>\K[^<]*' || echo "/var/lib/libvirt/images")
            log_success "libvirt default storage pool is active: ${POOL_PATH}"

            # Check pool permissions for root
            if [[ -d "$POOL_PATH" && -w "$POOL_PATH" ]]; then
                log_success "Root has write access to libvirt storage pool"
            else
                log_warning "Root may not have proper access to libvirt storage pool"
            fi
        else
            log_warning "libvirt default storage pool is not active"
            log_info "Will be configured during deployment"
        fi
    else
        log_warning "virsh command not available"
    fi

    # Check additional useful tools
    OPTIONAL_TOOLS=("ipmitool" "jq" "curl" "wget")
    for tool in "${OPTIONAL_TOOLS[@]}"; do
        if command -v "$tool" &> /dev/null; then
            log_success "Optional tool available: $tool"
        else
            log_info "Optional tool not installed: $tool (recommended)"
        fi
    done
}

# 3. Root User Authentication Files Check
check_root_authentication_files() {
    log_header "Root User Authentication Files"

    # Check pull secret for root
    PULL_SECRET_PATH=$(grep -oP "pull_secret: '\K[^']*" "${SCRIPT_DIR}/${CONFIG_FILE}")
    if [[ -f "${PULL_SECRET_PATH}" ]]; then
        # Validate pull secret format
        if command -v jq >/dev/null 2>&1 && jq empty "${PULL_SECRET_PATH}" 2>/dev/null; then
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
            log_warning "Pull secret found but JSON validation failed: ${PULL_SECRET_PATH}"
        fi

        # Check file permissions for root
        PULL_SECRET_PERMS=$(ls -la "${PULL_SECRET_PATH}" | awk '{print $1}')
        log_info "Pull secret permissions: ${PULL_SECRET_PERMS}"
        if [[ -r "${PULL_SECRET_PATH}" ]]; then
            log_success "Root can read pull secret file"
        else
            log_error "Root cannot read pull secret file"
        fi
    else
        log_error "Pull secret not found: ${PULL_SECRET_PATH}"
        log_info "Download from: https://console.redhat.com/openshift/install/pull-secret"
        log_info "Save as: ${PULL_SECRET_PATH}"
    fi

    # Check SSH key for root
    SSH_KEY_PATH=$(grep -oP "ssh_key: '\K[^']*" "${SCRIPT_DIR}/${CONFIG_FILE}")
    if [[ -f "${SSH_KEY_PATH}" ]]; then
        # Validate SSH key format
        if ssh-keygen -l -f "${SSH_KEY_PATH}" &>/dev/null; then
            SSH_KEY_TYPE=$(ssh-keygen -l -f "${SSH_KEY_PATH}" | awk '{print $4}')
            SSH_KEY_BITS=$(ssh-keygen -l -f "${SSH_KEY_PATH}" | awk '{print $1}')
            log_success "SSH public key found and valid: ${SSH_KEY_PATH} (${SSH_KEY_BITS} bits, ${SSH_KEY_TYPE})"
        else
            log_error "SSH public key is not valid: ${SSH_KEY_PATH}"
        fi

        # Check if corresponding private key exists
        SSH_PRIVATE_KEY="${SSH_KEY_PATH%%.pub}"
        if [[ -f "${SSH_PRIVATE_KEY}" ]]; then
            PRIVATE_KEY_PERMS=$(ls -la "${SSH_PRIVATE_KEY}" | awk '{print $1}')
            log_success "SSH private key found: ${SSH_PRIVATE_KEY} (${PRIVATE_KEY_PERMS})"

            # Check private key permissions (should be 600)
            if [[ "${PRIVATE_KEY_PERMS}" == "-rw-------"* ]]; then
                log_success "SSH private key has correct permissions (600)"
            else
                log_warning "SSH private key permissions may be too open: ${PRIVATE_KEY_PERMS}"
                log_info "Fix with: chmod 600 ${SSH_PRIVATE_KEY}"
            fi
        else
            log_warning "SSH private key not found: ${SSH_PRIVATE_KEY}"
        fi
    else
        log_warning "SSH public key not found: ${SSH_KEY_PATH}"
        log_info "Generate with: ssh-keygen -t rsa -b 4096 -C 'root@$(hostname)' -f /root/.ssh/id_rsa"
    fi

    # Check SSH directory permissions for root
    if [[ -d "${ROOT_HOME}/.ssh" ]]; then
        SSH_DIR_PERMS=$(ls -lad "${ROOT_HOME}/.ssh" | awk '{print $1}')
        if [[ "${SSH_DIR_PERMS}" == "drwx------"* ]]; then
            log_success "SSH directory has correct permissions (700)"
        else
            log_warning "SSH directory permissions may be incorrect: ${SSH_DIR_PERMS}"
            log_info "Fix with: chmod 700 ${ROOT_HOME}/.ssh"
        fi
    else
        log_warning "SSH directory not found: ${ROOT_HOME}/.ssh"
        log_info "Will be created during deployment"
    fi
}

# 4. Network Configuration Check (same as before but with root context)
check_network_configuration() {
    log_header "Network Configuration for Root Deployment"

    # Extract network settings from config
    API_IP=$(grep -oP 'api_ip: \K[0-9.]+' "${SCRIPT_DIR}/${CONFIG_FILE}")
    INGRESS_IP=$(grep -oP 'ingress_ip: \K[0-9.]+' "${SCRIPT_DIR}/${CONFIG_FILE}")
    CIDR=$(grep -oP 'cidr: \K[0-9./]+' "${SCRIPT_DIR}/${CONFIG_FILE}")
    DOMAIN=$(grep -oP 'domain: \K\S+' "${SCRIPT_DIR}/${CONFIG_FILE}")
    CLUSTER=$(grep -oP 'cluster: \K\S+' "${SCRIPT_DIR}/${CONFIG_FILE}")

    log_info "Network configuration for root deployment:"
    log_info "  API VIP: ${API_IP}"
    log_info "  Ingress VIP: ${INGRESS_IP}"
    log_info "  Network CIDR: ${CIDR}"
    log_info "  Domain: ${DOMAIN}"
    log_info "  Cluster: ${CLUSTER}"

    # Check if VIPs are reachable (they shouldn't be before installation)
    if timeout 2 ping -c 1 "${API_IP}" &>/dev/null; then
        log_warning "API VIP ${API_IP} is already responding (should be unused)"
    else
        log_success "API VIP ${API_IP} is unused (good)"
    fi

    if timeout 2 ping -c 1 "${INGRESS_IP}" &>/dev/null; then
        log_warning "Ingress VIP ${INGRESS_IP} is already responding (should be unused)"
    else
        log_success "Ingress VIP ${INGRESS_IP} is unused (good)"
    fi

    # Check network interface configuration for root
    log_info "Network interfaces available to root:"
    if command -v ip &>/dev/null; then
        ip addr show | grep -E "^[0-9]+: " | awk '{print "  " $2}' | sed 's/:$//'
    else
        ifconfig 2>/dev/null | grep -E "^[a-zA-Z]" | awk '{print "  " $1}' || log_warning "Cannot enumerate network interfaces"
    fi

    # Check if root can manage network configuration
    if [[ -w /etc/sysconfig/network-scripts/ ]] 2>/dev/null || [[ -w /etc/NetworkManager/ ]] 2>/dev/null; then
        log_success "Root has network configuration access"
    else
        log_info "Network configuration access for root (expected)"
    fi

    # Test DNS resolution as root
    DNS_NAMES=(
        "api.${CLUSTER}.${DOMAIN}"
        "api-int.${CLUSTER}.${DOMAIN}"
    )

    for dns_name in "${DNS_NAMES[@]}"; do
        if timeout 5 nslookup "${dns_name}" &>/dev/null; then
            RESOLVED_IP=$(nslookup "${dns_name}" 2>/dev/null | grep -A1 "Name:" | tail -1 | awk '{print $2}' || echo "unknown")
            log_success "DNS resolution: ${dns_name} → ${RESOLVED_IP}"
        else
            log_info "DNS resolution not configured: ${dns_name} (will use /etc/hosts or configure DNS)"
        fi
    done

    # Check wildcard DNS for apps
    TEST_APP_DNS="test.apps.${CLUSTER}.${DOMAIN}"
    if timeout 5 nslookup "${TEST_APP_DNS}" &>/dev/null; then
        log_success "Wildcard DNS appears to be configured for apps"
    else
        log_info "Wildcard DNS not configured (normal - will be handled by cluster)"
    fi
}

# 5. Root-specific libvirt and KVM Environment Check
check_root_virtualization_environment() {
    log_header "Root Virtualization Environment"

    # This is a root deployment on RHEL 9.7, check virtualization thoroughly
    log_info "Validating virtualization environment for root on RHEL 9.7"

    # Check CPU virtualization features
    if grep -q vmx /proc/cpuinfo; then
        log_success "Intel VT-x virtualization support detected"
    elif grep -q svm /proc/cpuinfo; then
        log_success "AMD-V virtualization support detected"
    else
        log_error "No virtualization support detected in CPU"
        return 1
    fi

    # Check if virtualization is enabled in BIOS
    if [[ -r /sys/module/kvm_intel/parameters/nested ]] || [[ -r /sys/module/kvm_amd/parameters/nested ]]; then
        log_success "KVM modules appear to be loaded"
    else
        log_warning "KVM modules may not be loaded"
    fi

    # Check KVM modules specifically
    if lsmod | grep -q kvm; then
        KVM_MODULES=$(lsmod | grep kvm | awk '{print $1}' | tr '\n' ' ')
        log_success "KVM modules loaded: ${KVM_MODULES}"
    else
        log_warning "KVM modules not loaded - will be loaded during setup"
    fi

    # Check libvirt configuration for root
    if systemctl is-enabled libvirtd &>/dev/null; then
        log_success "libvirtd is enabled for system startup"
    else
        log_warning "libvirtd is not enabled - will be enabled during setup"
    fi

    # Check libvirt network configuration
    if command -v virsh &>/dev/null; then
        if virsh net-list --all | grep -q "default"; then
            DEFAULT_NET_STATUS=$(virsh net-list --all | grep default | awk '{print $2}')
            if [[ "$DEFAULT_NET_STATUS" == "active" ]]; then
                log_success "libvirt default network is active"
            else
                log_info "libvirt default network exists but not active"
            fi
        else
            log_info "libvirt default network not found - will be created during setup"
        fi

        # Check for other networks that might interfere
        NETWORK_COUNT=$(virsh net-list --all | grep -v "^---" | grep -v "Name.*State.*Autostart" | wc -l)
        log_info "libvirt networks configured: ${NETWORK_COUNT}"
    fi

    # Check available storage for VMs
    LIBVIRT_IMAGES_DIR="/var/lib/libvirt/images"
    if [[ -d "$LIBVIRT_IMAGES_DIR" ]]; then
        IMAGES_DIR_SIZE=$(df -BG "$LIBVIRT_IMAGES_DIR" | awk 'NR==2 {print $4}' | sed 's/G//')
        IMAGES_DIR_PERMS=$(ls -lad "$LIBVIRT_IMAGES_DIR" | awk '{print $1 " " $3 ":" $4}')
        log_success "libvirt images directory: ${LIBVIRT_IMAGES_DIR} (${IMAGES_DIR_SIZE}GB available)"
        log_info "Directory permissions: ${IMAGES_DIR_PERMS}"

        if [[ -w "$LIBVIRT_IMAGES_DIR" ]]; then
            log_success "Root has write access to libvirt images directory"
        else
            log_warning "Root may not have write access to libvirt images directory"
        fi
    else
        log_info "libvirt images directory not found - will be created during setup"
    fi

    # Check SELinux context for virtualization
    if command -v getenforce &>/dev/null; then
        SELINUX_STATUS=$(getenforce)
        log_info "SELinux status: ${SELINUX_STATUS}"

        if [[ "$SELINUX_STATUS" == "Enforcing" ]]; then
            # Check if virt_use_execmem is enabled (sometimes needed)
            if command -v getsebool &>/dev/null; then
                VIRT_EXECMEM=$(getsebool virt_use_execmem 2>/dev/null | awk '{print $3}' || echo "unknown")
                log_info "virt_use_execmem: ${VIRT_EXECMEM}"
            fi
            log_success "SELinux enforcing - libvirt should work correctly"
        else
            log_info "SELinux not enforcing - virtualization will work without SELinux restrictions"
        fi
    fi
}

# 6. Hardware Inventory Check adapted for root config
check_hardware_inventory() {
    log_header "Hardware Inventory for Root Deployment"

    # Count nodes by role from root config
    MASTER_COUNT=$(grep -c 'role: master' "${SCRIPT_DIR}/${CONFIG_FILE}" || echo 0)
    WORKER_COUNT=$(grep -c 'role: worker' "${SCRIPT_DIR}/${CONFIG_FILE}" || echo 0)
    TOTAL_NODES=$((MASTER_COUNT + WORKER_COUNT))

    log_info "Hardware inventory from root configuration:"
    log_info "  Master nodes: ${MASTER_COUNT}"
    log_info "  Worker nodes: ${WORKER_COUNT}"
    log_info "  Total nodes: ${TOTAL_NODES}"

    # Validate minimum requirements
    if [[ ${MASTER_COUNT} -ge 3 ]]; then
        log_success "Master node count: ${MASTER_COUNT} (≥3 required for HA)"
    elif [[ ${MASTER_COUNT} -eq 1 ]]; then
        log_warning "Master node count: ${MASTER_COUNT} (single master - no HA)"
    else
        log_error "Master node count: ${MASTER_COUNT} (minimum 1 required, 3 recommended for HA)"
    fi

    if [[ ${WORKER_COUNT} -ge 3 ]]; then
        log_success "Worker node count: ${WORKER_COUNT} (≥3 optimal for ODF storage)"
    elif [[ ${WORKER_COUNT} -ge 2 ]]; then
        log_success "Worker node count: ${WORKER_COUNT} (adequate for testing)"
    else
        log_warning "Worker node count: ${WORKER_COUNT} (minimum 2 recommended for proper scheduling)"
    fi

    # Check for duplicate MAC addresses in root config
    if grep -q "mac:" "${SCRIPT_DIR}/${CONFIG_FILE}"; then
        MAC_ADDRESSES=$(grep -oP 'mac: "\K[^"]*' "${SCRIPT_DIR}/${CONFIG_FILE}")
        DUPLICATE_MACS=$(echo "${MAC_ADDRESSES}" | sort | uniq -d)

        if [[ -z "${DUPLICATE_MACS}" ]]; then
            log_success "No duplicate MAC addresses found in configuration"
        else
            log_error "Duplicate MAC addresses found in configuration:"
            echo "${DUPLICATE_MACS}" | while read -r mac; do
                log_error "  Duplicate MAC: ${mac}"
            done
        fi

        # Show MAC address assignments
        log_info "MAC address assignments:"
        grep -A1 -B1 "mac:" "${SCRIPT_DIR}/${CONFIG_FILE}" | grep -E "(name:|mac:)" | \
        paste - - | sed 's/.*name: /  /' | sed 's/.*mac: / → /'
    else
        log_info "No MAC addresses found in configuration (using KCLI auto-assignment)"
    fi

    # Check memory and CPU allocations from root config
    MASTER_MEMORY=$(grep -oP 'ctlplane_memory: \K\d+' "${SCRIPT_DIR}/${CONFIG_FILE}" || echo "0")
    WORKER_MEMORY=$(grep -oP 'worker_memory: \K\d+' "${SCRIPT_DIR}/${CONFIG_FILE}" || echo "0")
    MASTER_CPU=$(grep -oP 'ctlplane_numcpus: \K\d+' "${SCRIPT_DIR}/${CONFIG_FILE}" || echo "0")
    WORKER_CPU=$(grep -oP 'worker_numcpus: \K\d+' "${SCRIPT_DIR}/${CONFIG_FILE}" || echo "0")

    if [[ $MASTER_MEMORY -gt 0 && $WORKER_MEMORY -gt 0 ]]; then
        TOTAL_VM_MEMORY=$(( (MASTER_MEMORY * MASTER_COUNT) + (WORKER_MEMORY * WORKER_COUNT) ))
        TOTAL_VM_MEMORY_GB=$((TOTAL_VM_MEMORY / 1024))

        log_info "VM resource allocation:"
        log_info "  Masters: ${MASTER_COUNT} × ${MASTER_MEMORY}MB RAM, ${MASTER_CPU} vCPU each"
        log_info "  Workers: ${WORKER_COUNT} × ${WORKER_MEMORY}MB RAM, ${WORKER_CPU} vCPU each"
        log_info "  Total VM memory: ${TOTAL_VM_MEMORY_GB} GB"

        # Compare with host resources
        HOST_MEMORY_GB=$(free -g | awk '/^Mem:/ {print $2}')
        if [[ $TOTAL_VM_MEMORY_GB -lt $((HOST_MEMORY_GB * 80 / 100)) ]]; then
            log_success "VM memory allocation is conservative (${TOTAL_VM_MEMORY_GB}GB of ${HOST_MEMORY_GB}GB host RAM)"
        else
            log_warning "VM memory allocation is aggressive (${TOTAL_VM_MEMORY_GB}GB of ${HOST_MEMORY_GB}GB host RAM)"
        fi
    fi
}

# 7. Storage Configuration Check for Root
check_storage_configuration() {
    log_header "Storage Configuration for Root Deployment"

    # Check libvirt storage configuration
    if command -v virsh &>/dev/null; then
        if virsh pool-list | grep -q "default.*active"; then
            POOL_PATH=$(virsh pool-dumpxml default | grep -oP '<path>\K[^<]*')
            POOL_AVAILABLE=$(df -BG "${POOL_PATH}" | awk 'NR==2 {print $4}' | sed 's/G//')
            POOL_TOTAL=$(df -BG "${POOL_PATH}" | awk 'NR==2 {print $2}' | sed 's/G//')

            log_success "libvirt default storage pool is active: ${POOL_PATH}"
            log_info "Storage pool: ${POOL_AVAILABLE}GB available / ${POOL_TOTAL}GB total"

            if [[ ${POOL_AVAILABLE} -ge 500 ]]; then
                log_success "Storage pool space: ${POOL_AVAILABLE}GB (≥500GB recommended)"
            elif [[ ${POOL_AVAILABLE} -ge 300 ]]; then
                log_warning "Storage pool space: ${POOL_AVAILABLE}GB (300GB minimum, 500GB+ recommended)"
            else
                log_error "Storage pool space: ${POOL_AVAILABLE}GB (insufficient for cluster + ODF)"
            fi

            # Check pool permissions for root
            if [[ -w "${POOL_PATH}" ]]; then
                log_success "Root has write access to storage pool"
            else
                log_error "Root does not have write access to storage pool: ${POOL_PATH}"
            fi
        else
            log_info "libvirt default storage pool not active - will be configured during deployment"
        fi
    fi

    # Check if storage operators are enabled in root config
    if grep -q "storage_operators: true" "${SCRIPT_DIR}/${CONFIG_FILE}"; then
        log_success "Storage operators enabled in configuration"

        # Check ODF configuration
        ODF_SIZE=$(grep -oP 'odf_size: "\K[^"]*' "${SCRIPT_DIR}/${CONFIG_FILE}" || echo "unknown")
        if [[ "$ODF_SIZE" != "unknown" ]]; then
            log_info "ODF size configured: ${ODF_SIZE} per worker"

            # Calculate total ODF storage needed
            if [[ "$ODF_SIZE" =~ ^([0-9]+)Gi$ ]]; then
                ODF_SIZE_GB=$((${BASH_REMATCH[1]} * WORKER_COUNT))
                log_info "Total ODF storage required: ${ODF_SIZE_GB}GB (${ODF_SIZE} × ${WORKER_COUNT} workers)"
            fi
        fi

        # Check for local storage devices in config
        if grep -q "local_storage_devices:" "${SCRIPT_DIR}/${CONFIG_FILE}"; then
            log_info "Local storage devices configured:"
            grep -A5 "local_storage_devices:" "${SCRIPT_DIR}/${CONFIG_FILE}" | grep -E "^\s*-\s" | sed 's/^/  /'
        fi
    else
        log_info "Storage operators not enabled - manual storage setup will be required"
    fi

    # Check filesystem types and mount options
    log_info "Root filesystem information:"
    df -T / | grep -v "Filesystem" | awk '{print "  Filesystem: " $1 " (" $2 "), Mount: " $7 ", Available: " $5}'

    # Check if /var/lib/libvirt/images is on a separate mount or same as root
    LIBVIRT_MOUNT=$(df /var/lib/libvirt/images 2>/dev/null | tail -1 | awk '{print $6}' || echo "/")
    if [[ "$LIBVIRT_MOUNT" == "/" ]]; then
        log_info "libvirt images stored on root filesystem"
    else
        log_info "libvirt images on separate mount: ${LIBVIRT_MOUNT}"
    fi
}

# Summary
print_summary() {
    log_header "Root User Validation Summary"

    TOTAL_CHECKS=$((CHECKS_PASSED + CHECKS_FAILED + CHECKS_WARNING))

    echo -e "${GREEN}Passed: ${CHECKS_PASSED}${NC}"
    echo -e "${YELLOW}Warnings: ${CHECKS_WARNING}${NC}"
    echo -e "${RED}Failed: ${CHECKS_FAILED}${NC}"
    echo -e "Total checks: ${TOTAL_CHECKS}"

    echo -e "\n${BLUE}Root User Environment Summary:${NC}"
    echo -e "Host: $(hostname) running RHEL $(grep VERSION= /etc/os-release | cut -d'"' -f2)"
    echo -e "Root Home: ${ROOT_HOME}"
    echo -e "Configuration: ${CONFIG_FILE}"

    if [[ ${CHECKS_FAILED} -eq 0 ]]; then
        echo -e "\n${GREEN}✓ System is ready for OpenShift deployment as root!${NC}"
        if [[ ${CHECKS_WARNING} -gt 0 ]]; then
            echo -e "${YELLOW}⚠ Please review warnings above for optimal deployment${NC}"
        fi
        echo -e "\nNext steps for root deployment:"
        echo -e "  1. Review configuration file: ${CONFIG_FILE}"
        echo -e "  2. Ensure pull secret is available: $(grep -oP "pull_secret: '\K[^']*" "${SCRIPT_DIR}/${CONFIG_FILE}")"
        echo -e "  3. Run deployment: sudo ./deploy-kcli-cluster-root.sh ${CLUSTER_NAME:-kubernaut-e2e} ${CONFIG_FILE}"
        exit 0
    else
        echo -e "\n${RED}✗ System is not ready for deployment${NC}"
        echo -e "${RED}Please fix the failed checks above before proceeding${NC}"
        echo -e "\nCommon fixes for root deployment:"
        echo -e "  • Install missing packages: dnf install -y python3-pip libvirt-devel gcc pkg-config"
        echo -e "  • Install KCLI: pip3 install kcli[all]"
        echo -e "  • Start libvirt: systemctl enable --now libvirtd"
        echo -e "  • Generate SSH key: ssh-keygen -t rsa -b 4096 -f /root/.ssh/id_rsa"
        echo -e "  • Download pull secret to: $(grep -oP "pull_secret: '\K[^']*" "${SCRIPT_DIR}/${CONFIG_FILE}")"
        exit 1
    fi
}

# Main execution
main() {
    check_root_system_requirements
    check_root_software_dependencies
    check_root_authentication_files
    check_network_configuration
    check_root_virtualization_environment
    check_hardware_inventory
    check_storage_configuration
    print_summary
}

# Run main function
main "$@"
