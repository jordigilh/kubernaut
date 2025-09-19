#!/bin/bash
# OpenShift Cluster Networking Troubleshooting Script for helios08
# This script diagnoses and fixes common networking issues preventing
# bootstrap nodes from reaching the API VIP

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Expected configuration from kcli-baremetal-params-root.yml
API_VIP="192.168.122.100"
INGRESS_VIP="192.168.122.101"
NETWORK_CIDR="192.168.122.0/24"
LIBVIRT_NETWORK="default"

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_header() { echo -e "\n${CYAN}=== $1 ===${NC}"; }

# Banner
echo -e "${CYAN}"
cat << "EOF"
 _   _      _            _   _____                 _     _           _                 _
| \ | | ___| |___      _| |_|_   _|_ __ ___  _   _| |__ | | ___  ___| |__   ___   ___ | |_
|  \| |/ _ \ __\ \ /\ / / '_ \| || '__/ _ \| | | | '_ \| |/ _ \/ __| '_ \ / _ \ / _ \| __|
| |\  |  __/ |_ \ V  V /| |_) | || | | (_) | |_| | |_) | |  __/\__ \ | | | (_) | (_) | |_
|_| \_|\___|\__| \_/\_/ |_.__/|_||_|  \___/ \__,_|_.__/|_|\___||___/_| |_|\___/ \___/ \__|

EOF
echo -e "${NC}"

log_info "OpenShift Networking Troubleshooting for helios08"
log_info "Target API VIP: ${API_VIP}"
log_info "Expected Network: ${NETWORK_CIDR}"
log_info "This script will diagnose and attempt to fix networking issues"

# Function to check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root"
        log_info "Please run: sudo $0"
        exit 1
    fi
}

# Function to check libvirt service status
check_libvirt_status() {
    log_header "Checking libvirt Service Status"

    if systemctl is-active --quiet libvirtd; then
        log_success "libvirtd is running"
    else
        log_error "libvirtd is not running"
        log_info "Attempting to start libvirtd..."
        systemctl start libvirtd
        sleep 3
        if systemctl is-active --quiet libvirtd; then
            log_success "libvirtd started successfully"
        else
            log_error "Failed to start libvirtd"
            systemctl status libvirtd
            exit 1
        fi
    fi

    # Check related services
    for service in virtnetworkd virtqemud; do
        if systemctl is-active --quiet $service; then
            log_success "$service is running"
        else
            log_warning "$service is not running - attempting to start"
            systemctl start $service 2>/dev/null || log_info "$service not available on this system"
        fi
    done
}

# Function to check libvirt network configuration
check_libvirt_networks() {
    log_header "Checking libvirt Network Configuration"

    # List all networks
    log_info "Available libvirt networks:"
    virsh net-list --all

    # Check default network specifically
    if virsh net-list --all | grep -q "default"; then
        DEFAULT_STATUS=$(virsh net-list --all | grep default | awk '{print $2}')
        if [[ "$DEFAULT_STATUS" == "active" ]]; then
            log_success "Default network is active"
        else
            log_warning "Default network exists but is not active"
            log_info "Starting default network..."
            virsh net-start default
            virsh net-autostart default
            log_success "Default network started and set to autostart"
        fi
    else
        log_error "Default network does not exist"
        log_info "This is unusual - creating default network..."
        # Create basic default network
        cat > /tmp/default-network.xml << 'NETEOF'
<network>
  <name>default</name>
  <forward mode='nat'/>
  <bridge name='virbr0' stp='on' delay='0'/>
  <ip address='192.168.122.1' netmask='255.255.255.0'>
    <dhcp>
      <range start='192.168.122.2' end='192.168.122.254'/>
    </dhcp>
  </ip>
</network>
NETEOF
        virsh net-define /tmp/default-network.xml
        virsh net-start default
        virsh net-autostart default
        rm /tmp/default-network.xml
        log_success "Default network created and started"
    fi

    # Show network details
    log_info "Default network configuration:"
    virsh net-dumpxml default
}

# Function to check bridge configuration
check_bridge_configuration() {
    log_header "Checking Bridge Configuration"

    # Check if virbr0 exists
    if ip link show virbr0 &>/dev/null; then
        log_success "Bridge virbr0 exists"
        log_info "Bridge virbr0 details:"
        ip addr show virbr0
    else
        log_error "Bridge virbr0 does not exist"
        log_info "This indicates the libvirt default network is not properly configured"
        return 1
    fi

    # Check bridge status
    if ip link show virbr0 | grep -q "UP"; then
        log_success "Bridge virbr0 is UP"
    else
        log_warning "Bridge virbr0 is DOWN - attempting to bring up"
        ip link set virbr0 up
    fi
}

# Function to check firewall rules
check_firewall_rules() {
    log_header "Checking Firewall Configuration"

    # Check firewalld status
    if systemctl is-active --quiet firewalld; then
        log_info "firewalld is active"

        # Check libvirt zone
        if firewall-cmd --list-all-zones | grep -A 20 "libvirt" | grep -q "interfaces.*virbr0"; then
            log_success "virbr0 is properly assigned to libvirt zone"
        else
            log_warning "virbr0 may not be properly assigned to libvirt zone"
            log_info "libvirt zone configuration:"
            firewall-cmd --list-all --zone=libvirt || log_warning "libvirt zone does not exist"
        fi

        # Check if masquerading is enabled
        if firewall-cmd --query-masquerade --zone=libvirt &>/dev/null; then
            log_success "Masquerading is enabled in libvirt zone"
        else
            log_warning "Masquerading may not be enabled in libvirt zone"
            firewall-cmd --add-masquerade --zone=libvirt --permanent
            firewall-cmd --reload
            log_success "Enabled masquerading in libvirt zone"
        fi

    else
        log_info "firewalld is not active - checking iptables directly"

        # Check iptables rules
        if iptables -t nat -L | grep -q "192.168.122"; then
            log_success "Found libvirt NAT rules in iptables"
        else
            log_warning "No libvirt NAT rules found in iptables"
        fi
    fi
}

# Function to check DNS resolution
check_dns_resolution() {
    log_header "Checking DNS Resolution"

    local cluster_name="ocp418-baremetal"
    local domain="kubernaut.io"
    local api_fqdn="api.${cluster_name}.${domain}"

    log_info "Testing DNS resolution for: ${api_fqdn}"

    if nslookup "${api_fqdn}" &>/dev/null; then
        local resolved_ip=$(nslookup "${api_fqdn}" | grep -A 1 "Name:" | grep "Address:" | awk '{print $2}' | head -1)
        if [[ "$resolved_ip" == "$API_VIP" ]]; then
            log_success "DNS correctly resolves ${api_fqdn} to ${API_VIP}"
        else
            log_warning "DNS resolves ${api_fqdn} to ${resolved_ip}, expected ${API_VIP}"
        fi
    else
        log_warning "DNS resolution failed for ${api_fqdn}"
        log_info "This is expected during bootstrap phase - OpenShift will handle DNS internally"
    fi
}

# Function to check routing
check_routing() {
    log_header "Checking Routing Configuration"

    log_info "Checking route to API VIP:"
    if ip route get "$API_VIP" &>/dev/null; then
        local route_info=$(ip route get "$API_VIP")
        log_info "Route to $API_VIP: $route_info"

        # Check if route goes through virbr0
        if echo "$route_info" | grep -q "virbr0"; then
            log_success "Route to API VIP goes through virbr0 (correct)"
        else
            log_warning "Route to API VIP does not go through virbr0"
        fi
    else
        log_error "No route found to API VIP $API_VIP"
    fi

    log_info "Full routing table for 192.168.122.0/24:"
    ip route | grep "192.168.122" || log_info "No routes found for 192.168.122.0/24"
}

# Function to test connectivity
test_connectivity() {
    log_header "Testing Network Connectivity"

    # Test bridge connectivity
    log_info "Testing ping to bridge IP (192.168.122.1):"
    if ping -c 2 -W 3 192.168.122.1 &>/dev/null; then
        log_success "Bridge IP (192.168.122.1) is reachable"
    else
        log_error "Bridge IP (192.168.122.1) is NOT reachable"
        log_info "This indicates a fundamental networking problem"
    fi

    # Test API VIP (should not be reachable until cluster is up)
    log_info "Testing ping to API VIP ($API_VIP):"
    if ping -c 2 -W 3 "$API_VIP" &>/dev/null; then
        log_warning "API VIP ($API_VIP) is already responding"
        log_info "This might indicate a previous failed deployment still running"
    else
        log_success "API VIP ($API_VIP) is not responding (expected during bootstrap)"
    fi
}

# Function to check for running VMs that might conflict
check_existing_vms() {
    log_header "Checking for Existing VMs"

    log_info "Currently running VMs:"
    virsh list --all

    # Check for OpenShift VMs
    if virsh list --all | grep -q "ocp418-baremetal"; then
        log_warning "Found existing OpenShift VMs from previous deployment"
        log_info "These may need to be cleaned up"
        virsh list --all | grep "ocp418-baremetal"

        log_info "To clean up, you can run:"
        log_info "  kcli delete cluster --yes ocp418-baremetal"
        log_info "  # or manually: virsh destroy <vm-name>; virsh undefine <vm-name>"
    else
        log_success "No conflicting OpenShift VMs found"
    fi
}

# Function to fix common issues
fix_common_issues() {
    log_header "Applying Common Fixes"

    log_info "Restarting libvirt services..."
    systemctl restart libvirtd
    sleep 3

    # Ensure default network is running
    if ! virsh net-list | grep -q "default.*active"; then
        log_info "Starting default network..."
        virsh net-start default 2>/dev/null || true
    fi

    # Ensure bridge is up
    if ip link show virbr0 &>/dev/null && ! ip link show virbr0 | grep -q "UP"; then
        log_info "Bringing up virbr0 bridge..."
        ip link set virbr0 up
    fi

    # Reload firewall rules if firewalld is running
    if systemctl is-active --quiet firewalld; then
        log_info "Reloading firewall rules..."
        firewall-cmd --reload
    fi

    log_success "Applied common fixes"
}

# Function to provide cleanup commands
provide_cleanup_commands() {
    log_header "Cluster Cleanup Commands"

    cat << 'CLEANUP_EOF'

If you need to clean up the failed deployment before retrying:

# Method 1: Use KCLI (recommended)
cd /root/kubernaut-e2e
kcli delete cluster --yes ocp418-baremetal

# Method 2: Manual cleanup if KCLI fails
virsh list --all | grep ocp418-baremetal | awk '{print $2}' | xargs -I {} virsh destroy {}
virsh list --all | grep ocp418-baremetal | awk '{print $2}' | xargs -I {} virsh undefine {} --remove-all-storage

# Clean up any remaining storage
rm -rf /root/.kcli/clusters/ocp418-baremetal
rm -rf /var/lib/libvirt/images/*ocp418-baremetal*

CLEANUP_EOF
}

# Function to provide retry commands
provide_retry_commands() {
    log_header "Retry Deployment Commands"

    cat << 'RETRY_EOF'

After fixing networking issues, retry the deployment:

# Navigate to deployment directory
cd /root/kubernaut-e2e

# Retry cluster deployment
./deploy-kcli-cluster-root.sh kubernaut-e2e kcli-baremetal-params-root.yml

# Monitor deployment progress
watch 'kcli list vm; echo ""; kcli info cluster ocp418-baremetal'

RETRY_EOF
}

# Main execution
main() {
    check_root

    log_info "Starting comprehensive networking diagnostics..."

    # Run all checks
    check_libvirt_status
    check_libvirt_networks
    check_bridge_configuration
    check_firewall_rules
    check_dns_resolution
    check_routing
    test_connectivity
    check_existing_vms

    # Apply fixes
    fix_common_issues

    # Wait a moment for changes to take effect
    sleep 2

    # Test again
    log_header "Post-Fix Connectivity Test"
    test_connectivity

    # Provide guidance
    provide_cleanup_commands
    provide_retry_commands

    log_header "Summary"
    log_info "Diagnostics complete. Key points:"
    log_info "1. Verify bridge virbr0 is UP and has IP 192.168.122.1"
    log_info "2. Ensure libvirt default network is active"
    log_info "3. Check firewall/iptables are not blocking traffic"
    log_info "4. Clean up any existing VMs before retry"
    log_info "5. The API VIP should NOT respond until cluster is fully up"

    log_success "Run the retry commands above after addressing any issues found"
}

# Execute main function
main "$@"
