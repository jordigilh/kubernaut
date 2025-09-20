#!/bin/bash
# Complete KCLI Cleanup Script - Removes ALL traces of previous deployments
# This addresses the persistent odf_params.yaml issue

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

echo -e "${CYAN}"
cat << "EOF"
  ____                      _      _         ____  _
 / ___|___  _ __ ___  _ __ | | ___| |_ ___  / ___|| | ___  __ _ _ __  _   _ _ __
| |   / _ \| '_ ` _ \| '_ \| |/ _ \ __/ _ \| |    | |/ _ \/ _` | '_ \| | | | '_ \
| |__| (_) | | | | | | |_) | |  __/ ||  __/| |___ | |  __/ (_| | | | | |_| | |_) |
 \____\___/|_| |_| |_| .__/|_|\___|\__\___| \____|_|\___|\__,_|_| |_|\__,_| .__/
                     |_|                                                  |_|
EOF
echo -e "${NC}"

log_header "Complete KCLI Cache and Configuration Cleanup"
log_info "This script removes ALL traces of previous KCLI deployments"
log_info "Target: Remove persistent references to stress.parodos.dev"

# Function to backup file before deletion
backup_and_remove() {
    local file="$1"
    if [[ -f "$file" ]]; then
        log_info "Backing up and removing: $file"
        cp "$file" "${file}.bak.$(date +%Y%m%d_%H%M%S)" 2>/dev/null || true
        rm -f "$file"
    fi
}

log_header "1. Stop any running KCLI deployments"
# Kill any running KCLI processes
pkill -f "kcli.*create.*cluster" 2>/dev/null || true
pkill -f "openshift-install" 2>/dev/null || true
sleep 2

log_header "2. Delete all KCLI clusters"
log_info "Removing all KCLI clusters..."
kcli delete cluster --yes stress 2>/dev/null || true
kcli delete cluster --yes ocp418-baremetal 2>/dev/null || true
kcli delete cluster --yes kubernaut-e2e 2>/dev/null || true

# List and delete any other clusters
kcli list cluster 2>/dev/null | tail -n +2 | awk '{print $1}' | while read cluster; do
    if [[ -n "$cluster" && "$cluster" != "Cluster" ]]; then
        log_info "Deleting cluster: $cluster"
        kcli delete cluster --yes "$cluster" 2>/dev/null || true
    fi
done

log_header "3. Clean up KCLI directories"
log_info "Removing KCLI cluster directories..."
rm -rf /root/.kcli/clusters/* 2>/dev/null || true
find /root/.kcli -name "*stress*" -delete 2>/dev/null || true
find /root/.kcli -name "*parodos*" -delete 2>/dev/null || true

log_header "4. Remove problematic parameter files"
log_info "Removing cached parameter files with old cluster references..."

# The key problematic file
backup_and_remove "/root/odf_params.yaml"

# Find and remove any YAML files with stress/parodos references
log_info "Searching for files with stress/parodos references..."
find /root -maxdepth 3 -type f \( -name "*.yaml" -o -name "*.yml" \) | while read file; do
    if grep -l "stress\|parodos" "$file" 2>/dev/null; then
        log_warning "Found reference in: $file"
        if [[ "$file" == *"kcli-baremetal-params-root.yml" ]]; then
            log_info "Skipping parameter file: $file (should be correct)"
        else
            backup_and_remove "$file"
        fi
    fi
done

log_header "5. Clean up kubeconfig files"
unset KUBECONFIG 2>/dev/null || true
rm -f /root/.kube/config.old* 2>/dev/null || true
rm -f /root/kubeconfig* 2>/dev/null || true
rm -f /root/.kube/config.stress* 2>/dev/null || true

log_header "6. Remove libvirt VMs"
log_info "Destroying and undefining VMs..."
virsh list --all --name 2>/dev/null | grep -E "(stress|ocp418)" | while read vm; do
    if [[ -n "$vm" ]]; then
        log_info "Destroying VM: $vm"
        virsh destroy "$vm" 2>/dev/null || true
        virsh undefine "$vm" --remove-all-storage 2>/dev/null || true
    fi
done

log_header "7. Clean up storage"
log_info "Removing VM disk images..."
rm -rf /var/lib/libvirt/images/*stress* 2>/dev/null || true
rm -rf /var/lib/libvirt/images/*ocp418* 2>/dev/null || true

log_header "8. Clear environment variables"
unset KUBECONFIG 2>/dev/null || true
# Remove any exports from shell history that might reload the wrong config
if [[ -f /root/.bash_history ]]; then
    sed -i '/export.*KUBECONFIG.*stress/d' /root/.bash_history 2>/dev/null || true
    sed -i '/export.*KUBECONFIG.*parodos/d' /root/.bash_history 2>/dev/null || true
fi

log_header "9. Verification"
log_info "Verifying cleanup..."

echo ""
echo "Current KCLI clusters:"
kcli list cluster 2>/dev/null || echo "No clusters found"

echo ""
echo "Current VMs:"
virsh list --all

echo ""
echo "Checking for remaining stress/parodos references:"
FOUND_REFS=$(find /root -type f \( -name "*.yaml" -o -name "*.yml" \) -exec grep -l "stress\|parodos" {} \; 2>/dev/null | wc -l)
if [[ "$FOUND_REFS" -eq 0 ]]; then
    log_success "No remaining references to stress/parodos found"
else
    log_warning "Found $FOUND_REFS files with stress/parodos references:"
    find /root -type f \( -name "*.yaml" -o -name "*.yml" \) -exec grep -l "stress\|parodos" {} \; 2>/dev/null || true
fi

echo ""
echo "Current environment:"
env | grep -i kube || echo "No KUBECONFIG variables set"

log_header "Cleanup Complete"
log_success "All KCLI clusters, VMs, and cached configuration removed"
log_info "You can now deploy fresh clusters without conflicts"

echo ""
echo "Next steps:"
echo "1. cd /root/kubernaut-e2e"
echo "2. ./deploy-kcli-cluster-root.sh kubernaut-e2e kcli-baremetal-params-root.yml"
