#!/bin/bash
# Fix kubeconfig and complete deployment on helios08
# This script restores the correct kubeconfig and checks for remaining issues

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

log_header "Fixing Kubeconfig and Deployment Issues"

# Current cluster name (from the logs showing ocp418-baremetal)
CLUSTER_NAME="ocp418-baremetal"
CLUSTER_DIR="/root/.kcli/clusters/${CLUSTER_NAME}"

log_header "Checking Current Cluster Status"

# Check if cluster directory exists
if [ -d "$CLUSTER_DIR" ]; then
    log_info "Found cluster directory: $CLUSTER_DIR"
    ls -la "$CLUSTER_DIR" || true
else
    log_error "Cluster directory not found: $CLUSTER_DIR"
    log_info "Available cluster directories:"
    ls -la /root/.kcli/clusters/ 2>/dev/null || log_warning "No cluster directories found"
fi

log_header "Restoring Kubeconfig"

# Check if auth/kubeconfig exists in the cluster directory
KUBECONFIG_PATH="${CLUSTER_DIR}/auth/kubeconfig"
if [ -f "$KUBECONFIG_PATH" ]; then
    log_success "Found cluster kubeconfig: $KUBECONFIG_PATH"

    # Create .kube directory if it doesn't exist
    mkdir -p /root/.kube

    # Copy the kubeconfig
    cp "$KUBECONFIG_PATH" /root/.kube/config
    chmod 600 /root/.kube/config

    log_success "Kubeconfig restored to /root/.kube/config"

    # Test the connection
    log_info "Testing kubeconfig connection..."
    if oc version --client 2>/dev/null && oc cluster-info 2>/dev/null; then
        log_success "Kubeconfig is working - can connect to cluster"
    else
        log_warning "Kubeconfig restored but connection test failed (this may be normal during deployment)"
    fi
else
    log_error "Kubeconfig not found at: $KUBECONFIG_PATH"
fi

log_header "Checking for Remaining Old Cluster References"

log_info "Searching for any remaining 'stress' or 'parodos' references..."

# Check current environment variables
log_info "Checking environment variables..."
env | grep -i "stress\|parodos" || log_info "No stress/parodos environment variables found"

# Search for files containing stress or parodos in /root
log_info "Searching for files with content references..."
FOUND_FILES=$(find /root -type f -name "*.yaml" -o -name "*.yml" -o -name "*.json" -o -name "*config*" 2>/dev/null | xargs grep -l "stress\|parodos" 2>/dev/null || true)

if [ -n "$FOUND_FILES" ]; then
    log_warning "Found files still containing stress/parodos references:"
    echo "$FOUND_FILES"

    log_info "Contents of these files:"
    for file in $FOUND_FILES; do
        echo -e "\n${CYAN}=== $file ===${NC}"
        grep -n "stress\|parodos" "$file" 2>/dev/null || true
    done

    log_warning "These files may need to be removed or cleaned"
else
    log_success "No files found with stress/parodos content references"
fi

log_header "Checking KUBECONFIG Environment Variable"

if [ -n "${KUBECONFIG:-}" ]; then
    log_warning "KUBECONFIG environment variable is set to: $KUBECONFIG"
    if echo "$KUBECONFIG" | grep -q "stress\|parodos"; then
        log_error "KUBECONFIG contains old cluster references"
        log_info "Unsetting KUBECONFIG environment variable..."
        unset KUBECONFIG
        echo 'unset KUBECONFIG' >> /root/.bashrc
        log_success "KUBECONFIG unset - will use /root/.kube/config"
    fi
else
    log_info "KUBECONFIG environment variable is not set (good - will use /root/.kube/config)"
fi

log_header "Current Deployment Status"

# Check running VMs
log_info "Current running VMs:"
virsh list || log_warning "Could not list VMs"

# Check if we can determine the current cluster's API endpoint
if [ -f "/root/.kube/config" ]; then
    log_info "Current cluster API endpoint from kubeconfig:"
    grep "server:" /root/.kube/config | head -1 || log_warning "Could not determine API endpoint"
fi

log_header "Recommendations"

echo -e "${YELLOW}Next steps:${NC}"
echo "1. The cluster deployment appears to be in progress"
echo "2. The kubeconfig has been restored (if it existed)"
echo "3. Any remaining files with old cluster references should be removed"
echo "4. The deployment should be able to continue normally"
echo ""
echo "If the deployment is still running, you can monitor it with:"
echo "  tail -f /root/kcli-deploy-kubernaut-e2e.log"
echo ""
echo "To check cluster status:"
echo "  oc get nodes"
echo "  oc get clusteroperators"

log_success "Kubeconfig fix script completed"
