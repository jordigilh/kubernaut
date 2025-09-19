#!/bin/bash
# KCLI Cache Cleanup Script for helios08
# This script removes all traces of previous KCLI deployments that might interfere
# with the new kubernaut-e2e cluster deployment

set -euo pipefail

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
 _  ______ _     ___    ____              _
| |/ / ___| |   |_ _|  / ___|__ _  ___ ___| |__   ___
| ' / |   | |    | |  | |   / _` |/ __/ _ \ '_ \ / _ \
| . \ |___| |___ | |  | |__| (_| | (_|  __/ | | |  __/
|_|\_\____|_____|___|  \____\__,_|\___\___|_| |_|\___|

 ____ _
/ ___| | ___  __ _ _ __  _   _ _ __
| |   | |/ _ \/ _` | '_ \| | | | '_ \
| |___| |  __/ (_| | | | | |_| | |_) |
\____|_|\___|\__,_|_| |_|\__,_| .__/
                             |_|
EOF
echo -e "${NC}"

log_info "KCLI Cache Cleanup for helios08"
log_info "Removing traces of previous deployments (stress.parodos.dev, etc.)"
log_info "Preparing for fresh kubernaut-e2e deployment"

# Function to check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root"
        log_info "Please run: sudo $0"
        exit 1
    fi
}

# Function to clean up KCLI cluster configurations
cleanup_kcli_clusters() {
    log_header "Cleaning Up KCLI Cluster Configurations"

    # List existing KCLI clusters
    log_info "Current KCLI clusters:"
    kcli list cluster || log_info "No clusters found or KCLI not responding"

    # Try to delete known problematic clusters
    local old_clusters=("stress" "ocp418-baremetal")
    for cluster in "${old_clusters[@]}"; do
        if kcli list cluster 2>/dev/null | grep -q "$cluster"; then
            log_warning "Found existing cluster: $cluster"
            log_info "Attempting to delete cluster: $cluster"
            kcli delete cluster --yes "$cluster" || log_warning "Failed to delete cluster $cluster (may not exist)"
        fi
    done

    # Clean up KCLI configuration directory
    if [[ -d "/root/.kcli" ]]; then
        log_info "Cleaning KCLI configuration directory..."

        # Back up the main config file
        if [[ -f "/root/.kcli/config.yml" ]]; then
            cp "/root/.kcli/config.yml" "/root/.kcli/config.yml.backup.$(date +%s)"
            log_info "Backed up KCLI config to /root/.kcli/config.yml.backup.*"
        fi

        # Remove cluster-specific directories
        rm -rf /root/.kcli/clusters/stress* || true
        rm -rf /root/.kcli/clusters/ocp418-baremetal* || true
        rm -rf /root/.kcli/clusters/kubernaut-e2e* || true

        # Remove any cached images or profiles related to old deployments
        find /root/.kcli -name "*stress*" -type f -delete 2>/dev/null || true
        find /root/.kcli -name "*parodos*" -type f -delete 2>/dev/null || true

        log_success "Cleaned KCLI configuration directory"
    fi
}

# Function to clean up libvirt VMs
cleanup_libvirt_vms() {
    log_header "Cleaning Up libvirt VMs"

    log_info "Current VMs:"
    virsh list --all || log_warning "Could not list VMs"

    # Find and destroy VMs from old deployments
    local vm_patterns=("stress" "ocp418-baremetal" "kubernaut-e2e")

    for pattern in "${vm_patterns[@]}"; do
        log_info "Looking for VMs matching pattern: $pattern"

        # Get list of VMs matching the pattern
        local vms=$(virsh list --all --name 2>/dev/null | grep -i "$pattern" || true)

        if [[ -n "$vms" ]]; then
            while IFS= read -r vm; do
                if [[ -n "$vm" ]]; then
                    log_warning "Found VM: $vm"

                    # Destroy if running
                    if virsh list --name | grep -q "^${vm}$"; then
                        log_info "Destroying running VM: $vm"
                        virsh destroy "$vm" || log_warning "Failed to destroy $vm"
                    fi

                    # Undefine VM and remove storage
                    log_info "Undefining VM: $vm"
                    virsh undefine "$vm" --remove-all-storage || log_warning "Failed to undefine $vm"
                fi
            done <<< "$vms"
        else
            log_success "No VMs found matching pattern: $pattern"
        fi
    done
}

# Function to clean up storage
cleanup_storage() {
    log_header "Cleaning Up Storage"

    # Clean up libvirt images
    local image_dir="/var/lib/libvirt/images"
    if [[ -d "$image_dir" ]]; then
        log_info "Cleaning up old VM images in $image_dir"

        # Remove images from old deployments
        local patterns=("stress" "ocp418-baremetal" "kubernaut-e2e")
        for pattern in "${patterns[@]}"; do
            find "$image_dir" -name "*${pattern}*" -type f -exec rm -f {} \; 2>/dev/null || true
            log_info "Removed images matching pattern: $pattern"
        done

        # List remaining images
        log_info "Remaining images:"
        ls -la "$image_dir" | grep -v "total" || log_info "No images found"
    fi
}

# Function to clean up kubeconfig files
cleanup_kubeconfig() {
    log_header "Cleaning Up Kubeconfig Files"

    # Common kubeconfig locations
    local kubeconfig_locations=(
        "/root/.kube/config"
        "/root/kubeconfig"
        "/root/.kcli/clusters/*/auth/kubeconfig"
        "/root/kubernaut-e2e/kubeconfig"
        "$KUBECONFIG"
    )

    for location in "${kubeconfig_locations[@]}"; do
        if [[ -n "$location" && -f "$location" ]]; then
            log_info "Checking kubeconfig: $location"

            # Check if it contains references to old clusters
            if grep -q "parodos.dev\|stress" "$location" 2>/dev/null; then
                log_warning "Found references to old cluster in: $location"
                log_info "Backing up and removing: $location"
                mv "$location" "${location}.old.$(date +%s)"
            else
                log_success "Kubeconfig clean: $location"
            fi
        fi
    done

    # Clean up any kubeconfig environment variables
    if [[ -n "${KUBECONFIG:-}" ]]; then
        log_warning "KUBECONFIG environment variable is set: $KUBECONFIG"
        log_info "Consider unsetting it: unset KUBECONFIG"
    fi
}

# Function to clean up environment variables and shell history
cleanup_environment() {
    log_header "Cleaning Up Environment"

    # Check for problematic environment variables
    local env_vars=(
        "KUBECONFIG"
        "KCLI_CLIENT"
        "OPENSHIFT_CLIENT"
    )

    for var in "${env_vars[@]}"; do
        if [[ -n "${!var:-}" ]]; then
            log_warning "Environment variable $var is set: ${!var}"
            log_info "Consider unsetting: unset $var"
        fi
    done

    # Clean up shell history entries that might contain old cluster references
    local history_files=(
        "/root/.bash_history"
        "/root/.zsh_history"
    )

    for hist_file in "${history_files[@]}"; do
        if [[ -f "$hist_file" ]]; then
            # Create backup
            cp "$hist_file" "${hist_file}.backup.$(date +%s)"

            # Remove lines containing old cluster references
            sed -i.tmp '/stress\.parodos\.dev/d; /stress parodos/d' "$hist_file" 2>/dev/null || true
            rm -f "${hist_file}.tmp" 2>/dev/null || true

            log_info "Cleaned history file: $hist_file"
        fi
    done
}

# Function to reset DNS cache
reset_dns_cache() {
    log_header "Resetting DNS Cache"

    # Flush DNS cache
    if command -v systemd-resolve &>/dev/null; then
        systemd-resolve --flush-caches
        log_success "Flushed systemd-resolved DNS cache"
    fi

    # Clear nscd cache if present
    if command -v nscd &>/dev/null && systemctl is-active --quiet nscd; then
        nscd -i hosts
        log_success "Cleared nscd DNS cache"
    fi

    # Test DNS resolution for new cluster
    local new_api="api.ocp418-baremetal.kubernaut.io"
    log_info "Testing DNS for new cluster: $new_api"
    if nslookup "$new_api" &>/dev/null; then
        log_success "DNS resolves for new cluster"
    else
        log_info "DNS does not resolve for new cluster (expected during initial deployment)"
    fi
}

# Function to verify KCLI configuration
verify_kcli_config() {
    log_header "Verifying KCLI Configuration"

    log_info "Current KCLI version:"
    kcli version || log_warning "Could not get KCLI version"

    log_info "KCLI hosts:"
    kcli list host || log_warning "Could not list KCLI hosts"

    log_info "KCLI clusters after cleanup:"
    kcli list cluster || log_info "No clusters found (good for fresh start)"

    # Check that the parameter file exists and has correct values
    local param_file="/root/kubernaut-e2e/kcli-baremetal-params-root.yml"
    if [[ -f "$param_file" ]]; then
        log_info "Checking parameter file: $param_file"

        # Extract key values
        local cluster_name=$(grep -oP 'cluster: \K\S+' "$param_file" || echo "unknown")
        local domain=$(grep -oP 'domain: \K\S+' "$param_file" || echo "unknown")

        log_info "  Cluster name: $cluster_name"
        log_info "  Domain: $domain"

        if [[ "$cluster_name" == "ocp418-baremetal" && "$domain" == "kubernaut.io" ]]; then
            log_success "Parameter file has correct values"
        else
            log_warning "Parameter file may have incorrect values"
        fi
    else
        log_warning "Parameter file not found: $param_file"
    fi
}

# Function to provide post-cleanup verification
provide_verification_commands() {
    log_header "Post-Cleanup Verification Commands"

    cat << 'VERIFY_EOF'

Run these commands to verify the cleanup was successful:

# 1. Verify no old clusters exist
kcli list cluster

# 2. Verify no old VMs exist
virsh list --all

# 3. Check environment is clean
env | grep -i kube
env | grep -i kcli

# 4. Verify parameter file is correct
cat /root/kubernaut-e2e/kcli-baremetal-params-root.yml | grep -E "(cluster|domain):"

# 5. Test fresh deployment
cd /root/kubernaut-e2e
./deploy-kcli-cluster-root.sh kubernaut-e2e kcli-baremetal-params-root.yml

VERIFY_EOF
}

# Main execution function
main() {
    check_root

    log_info "Starting comprehensive KCLI cache cleanup..."
    log_warning "This will remove ALL existing KCLI deployments and configurations"
    log_info "Press Ctrl+C within 5 seconds to cancel..."
    sleep 5

    # Run all cleanup functions
    cleanup_kcli_clusters
    cleanup_libvirt_vms
    cleanup_storage
    cleanup_kubeconfig
    cleanup_environment
    reset_dns_cache

    # Verify configuration
    verify_kcli_config

    # Provide next steps
    provide_verification_commands

    log_header "Cleanup Complete"
    log_success "All traces of old deployments have been removed"
    log_info "You can now run a fresh deployment with the correct cluster name and domain"
    log_info "Expected new cluster: ocp418-baremetal.kubernaut.io"

    log_header "Next Steps"
    log_info "1. Verify cleanup with the commands above"
    log_info "2. Run: cd /root/kubernaut-e2e"
    log_info "3. Run: ./deploy-kcli-cluster-root.sh kubernaut-e2e kcli-baremetal-params-root.yml"
    log_info "4. Monitor deployment with: watch 'kcli list vm; kcli info cluster ocp418-baremetal'"
}

# Execute main function
main "$@"
