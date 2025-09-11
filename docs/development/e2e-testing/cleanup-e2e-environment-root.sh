#!/bin/bash
# Complete E2E Testing Environment Cleanup Script for Root User on RHEL 9.7
# Safely removes all components of the Kubernaut E2E testing environment

set -euo pipefail

# Root user validation
if [[ $EUID -ne 0 ]]; then
    echo -e "\033[0;31m[ERROR]\033[0m This script should be run as root for consistency with deployment"
    echo "Usage: sudo $0 [options]"
    exit 1
fi

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-kubernaut-e2e}"
FORCE_CLEANUP="${FORCE_CLEANUP:-false}"
PRESERVE_CLUSTER="${PRESERVE_CLUSTER:-false}"
PRESERVE_DATA="${PRESERVE_DATA:-false}"
ROOT_HOME="/root"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_header() { echo -e "\n${PURPLE}=== $1 ===${NC}"; }

# Progress tracking
TOTAL_STEPS=8
CURRENT_STEP=0

progress() {
    CURRENT_STEP=$((CURRENT_STEP + 1))
    echo -e "${CYAN}[STEP ${CURRENT_STEP}/${TOTAL_STEPS}]${NC} $1"
}

# Banner
echo -e "${PURPLE}"
cat << "EOF"
 _  __     _                                 _      _____ ____  _____    _____ _
| |/ /    | |                               | |    |  ___/ ___||  ___|  /  __ \ |
| ' /_   _| |__   ___ _ __ _ __   __ _ _   _  | |_   | |__ \___ \| |__    | /  \/ | ___  __ _ _ __  _   _ _ __
|  <| | | | '_ \ / _ \ '__| '_ \ / _` | | | | | __|  |  __| ___) |  __|   | |   | |/ _ \/ _` | '_ \| | | | '_ \
| . \ |_| | |_) |  __/ |  | | | | (_| | |_| | | |_   | |___\____/| |___   | \__/\ |  __/ (_| | | | | |_| | |_) |
|_|\_\__,_|_.__/ \___|_|  |_| |_|\__,_|\__,_|  \__|  \____/     \____/    \____/_|\___|\__,_|_| |_|\__,_| .__/
                                                                                                         | |
     ____             _      _   _                                                                      |_|
    |  _ \ ___   ___ | |_   | | | |___  ___ _ __
    | |_) / _ \ / _ \| __|  | | | / __|/ _ \ '__|
    |  _ < (_) | (_) | |_   | |_| \__ \  __/ |
    |_| \_\___/ \___/ \__|   \___/|___/\___|_|
EOF
echo -e "${NC}"

log_info "Starting Kubernaut E2E Testing Environment Cleanup (Root User)"
log_info "Host: $(hostname) (RHEL $(grep VERSION= /etc/os-release | cut -d'"' -f2 2>/dev/null || echo 'Unknown'))"
log_info "User: $(whoami)"
log_info "Cluster: ${CLUSTER_NAME}"
log_info "Preserve Cluster: ${PRESERVE_CLUSTER}"
log_info "Preserve Data: ${PRESERVE_DATA}"
echo ""

# Confirmation prompt
if [[ "$FORCE_CLEANUP" != "true" ]]; then
    echo -e "${YELLOW}WARNING: This will remove the following components:${NC}"
    echo "  • Kubernaut application and configurations"
    echo "  • Vector database and stored patterns"
    echo "  • LitmusChaos framework and experiments"
    echo "  • Test applications and data"
    echo "  • Monitoring stack components"
    if [[ "$PRESERVE_CLUSTER" != "true" ]]; then
        echo "  • OpenShift cluster (${CLUSTER_NAME})"
        echo "  • libvirt VMs and storage"
    fi
    echo ""
    read -p "Are you sure you want to continue? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "Cleanup cancelled by root user"
        exit 0
    fi
fi

# Set kubeconfig for cleanup operations
export KUBECONFIG="${ROOT_HOME}/.kcli/clusters/${CLUSTER_NAME}/auth/kubeconfig"

# Step 1: Cleanup Test Applications
cleanup_test_applications() {
    progress "Cleaning up Test Applications"

    if oc get namespace kubernaut-test-apps &>/dev/null; then
        log_info "Removing test applications..."
        oc delete namespace kubernaut-test-apps --timeout=120s &>/dev/null || log_warning "Failed to delete test applications namespace"
        log_success "Test applications cleaned up"
    else
        log_info "Test applications namespace not found"
    fi
}

# Step 2: Cleanup Kubernaut Application
cleanup_kubernaut_application() {
    progress "Cleaning up Kubernaut Application"

    if oc get namespace kubernaut-system &>/dev/null; then
        if [[ "$PRESERVE_DATA" != "true" ]]; then
            log_info "Removing Kubernaut application and data..."
            oc delete namespace kubernaut-system --timeout=120s &>/dev/null || log_warning "Failed to delete kubernaut-system namespace"
            log_success "Kubernaut application and data cleaned up"
        else
            log_info "Preserving Kubernaut data, removing only application..."
            oc delete deployment kubernaut -n kubernaut-system &>/dev/null || log_warning "Failed to delete Kubernaut deployment"
            oc delete service kubernaut-service -n kubernaut-system &>/dev/null || log_warning "Failed to delete Kubernaut service"
            log_success "Kubernaut application cleaned up (data preserved)"
        fi
    else
        log_info "Kubernaut namespace not found"
    fi
}

# Step 3: Cleanup LitmusChaos
cleanup_chaos_testing() {
    progress "Cleaning up LitmusChaos Framework"

    # Cleanup chaos experiments and results first
    if oc get namespace chaos-testing &>/dev/null; then
        log_info "Cleaning up chaos experiments..."
        oc delete chaosengines --all -n chaos-testing --timeout=60s &>/dev/null || true
        oc delete chaosresults --all -n chaos-testing --timeout=60s &>/dev/null || true

        log_info "Removing chaos testing namespace..."
        oc delete namespace chaos-testing --timeout=120s &>/dev/null || log_warning "Failed to delete chaos-testing namespace"
        log_success "Chaos testing namespace cleaned up"
    fi

    # Cleanup LitmusChaos operator
    if oc get namespace litmus &>/dev/null; then
        log_info "Removing LitmusChaos operator..."
        oc delete namespace litmus --timeout=120s &>/dev/null || log_warning "Failed to delete litmus namespace"
        log_success "LitmusChaos operator cleaned up"
    else
        log_info "LitmusChaos operator not found"
    fi

    # Cleanup CRDs (optional, as they might be used by other components)
    log_info "Cleaning up LitmusChaos CRDs..."
    oc delete crd chaosengines.litmuschaos.io &>/dev/null || true
    oc delete crd chaosexperiments.litmuschaos.io &>/dev/null || true
    oc delete crd chaosresults.litmuschaos.io &>/dev/null || true
    log_info "LitmusChaos CRDs cleanup attempted"
}

# Step 4: Cleanup Monitoring Stack
cleanup_monitoring() {
    progress "Cleaning up Monitoring Stack"

    if oc get namespace kubernaut-monitoring &>/dev/null; then
        log_info "Removing monitoring stack..."
        oc delete namespace kubernaut-monitoring --timeout=120s &>/dev/null || log_warning "Failed to delete monitoring namespace"
        log_success "Monitoring stack cleaned up"
    else
        log_info "Monitoring namespace not found"
    fi

    # Cleanup ServiceMonitors if they exist
    if oc get servicemonitor kubernaut-metrics -n kubernaut-system &>/dev/null; then
        oc delete servicemonitor kubernaut-metrics -n kubernaut-system &>/dev/null || true
        log_info "Kubernaut ServiceMonitor cleaned up"
    fi
}

# Step 5: Cleanup Storage (if not preserving cluster)
cleanup_storage() {
    if [[ "$PRESERVE_CLUSTER" == "true" ]]; then
        log_info "Preserving cluster - skipping storage cleanup"
        return 0
    fi

    progress "Cleaning up Storage Infrastructure"

    # Only cleanup if we're removing the entire cluster
    # Storage operators are part of the cluster and will be removed with it
    log_info "Storage cleanup will be handled by cluster removal"
}

# Step 6: Cleanup RBAC and Network Resources
cleanup_rbac_and_network() {
    progress "Cleaning up RBAC and Network Resources"

    # Cleanup ClusterRoles and ClusterRoleBindings created for testing
    CLUSTER_ROLES=("kubernaut" "chaos-e2e-runner")
    CLUSTER_ROLE_BINDINGS=("kubernaut" "chaos-e2e-runner")

    for role in "${CLUSTER_ROLES[@]}"; do
        if oc get clusterrole "$role" &>/dev/null; then
            oc delete clusterrole "$role" &>/dev/null || log_warning "Failed to delete ClusterRole: $role"
            log_info "ClusterRole deleted: $role"
        fi
    done

    for binding in "${CLUSTER_ROLE_BINDINGS[@]}"; do
        if oc get clusterrolebinding "$binding" &>/dev/null; then
            oc delete clusterrolebinding "$binding" &>/dev/null || log_warning "Failed to delete ClusterRoleBinding: $binding"
            log_info "ClusterRoleBinding deleted: $binding"
        fi
    done

    log_success "RBAC cleanup completed"
}

# Step 7: Remove OpenShift Cluster
cleanup_cluster() {
    if [[ "$PRESERVE_CLUSTER" == "true" ]]; then
        progress "Preserving OpenShift Cluster"
        log_info "Cluster preservation enabled - skipping cluster removal"
        log_warning "Cluster '${CLUSTER_NAME}' is preserved and still running"
        log_info "To remove later: kcli delete cluster ${CLUSTER_NAME}"
        return 0
    fi

    progress "Removing OpenShift Cluster"

    if command -v kcli &>/dev/null; then
        if kcli list cluster | grep -q "^${CLUSTER_NAME}"; then
            log_info "Removing OpenShift cluster: ${CLUSTER_NAME}"
            log_warning "This will take several minutes..."

            if kcli delete cluster "${CLUSTER_NAME}" --yes; then
                log_success "OpenShift cluster removed successfully"
            else
                log_error "Failed to remove OpenShift cluster"
                log_info "You may need to manually remove the cluster: kcli delete cluster ${CLUSTER_NAME}"
            fi
        else
            log_info "Cluster '${CLUSTER_NAME}' not found in KCLI"
        fi
    else
        log_warning "KCLI not found - cannot remove cluster automatically"
        log_info "If cluster exists, remove manually with: kcli delete cluster ${CLUSTER_NAME}"
    fi

    # Clean up libvirt resources if we're root
    if [[ "$PRESERVE_CLUSTER" != "true" ]]; then
        log_info "Cleaning up libvirt resources..."

        # Remove any remaining VMs related to the cluster
        if command -v virsh &>/dev/null; then
            VM_LIST=$(virsh list --all | grep "$CLUSTER_NAME" | awk '{print $2}' || true)
            if [[ -n "$VM_LIST" ]]; then
                echo "$VM_LIST" | while read -r vm; do
                    if [[ -n "$vm" ]]; then
                        log_info "Removing VM: $vm"
                        virsh destroy "$vm" &>/dev/null || true
                        virsh undefine "$vm" --remove-all-storage &>/dev/null || true
                    fi
                done
            fi
        fi

        # Clean up KCLI configuration for this cluster
        if [[ -d "${ROOT_HOME}/.kcli/clusters/${CLUSTER_NAME}" ]]; then
            rm -rf "${ROOT_HOME}/.kcli/clusters/${CLUSTER_NAME}"
            log_info "Removed KCLI cluster configuration"
        fi
    fi
}

# Step 8: Cleanup temporary files and root-specific resources
cleanup_temp_files() {
    progress "Cleaning up Temporary Files and Root Resources"

    log_info "Cleaning up temporary files..."

    # Remove temporary files created during setup
    rm -f /tmp/models.json
    rm -f /tmp/ai-model-config.yaml
    rm -f /tmp/kubernaut-deployment*.yaml
    rm -f /tmp/postgres-*
    rm -f /tmp/kcli-deploy-*.log
    rm -f /tmp/kubernaut-e2e-*.pid

    # Remove log files in root directory
    rm -f "${ROOT_HOME}/kcli-deploy-${CLUSTER_NAME}.log"

    # Remove generated scripts if they exist and were auto-generated
    GENERATED_SCRIPTS=(
        "setup-litmus-chaos-root.sh"
        "setup-vector-database-root.sh"
        "run-e2e-tests-root.sh"
    )

    for script in "${GENERATED_SCRIPTS[@]}"; do
        if [[ -f "${SCRIPT_DIR}/${script}" ]]; then
            # Only remove if it was auto-generated (contains a specific marker or is very recent)
            if grep -q "set -euo pipefail" "${SCRIPT_DIR}/${script}" && [[ $(find "${SCRIPT_DIR}/${script}" -mmin -180 2>/dev/null) ]]; then
                rm -f "${SCRIPT_DIR}/${script}"
                log_info "Removed generated script: ${script}"
            fi
        fi
    done

    # Clean up KUBECONFIG from root's bashrc if it was added
    if [[ -f "${ROOT_HOME}/.bashrc" ]] && grep -q "kubernaut-e2e\|${CLUSTER_NAME}" "${ROOT_HOME}/.bashrc"; then
        log_info "Cleaning up KUBECONFIG from root's .bashrc..."
        sed -i "/kubernaut-e2e\|${CLUSTER_NAME}/d" "${ROOT_HOME}/.bashrc"
        log_info "Removed cluster references from root's .bashrc"
    fi

    log_success "Temporary files and root resources cleaned up"
}

# Print cleanup summary for root user
print_cleanup_summary() {
    log_header "Root User E2E Environment Cleanup Summary"

    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  CLEANUP COMPLETED (ROOT USER)${NC}"
    echo -e "${GREEN}========================================${NC}"

    echo -e "\n${BLUE}Components Removed:${NC}"
    echo -e "  ✓ Kubernaut application"
    if [[ "$PRESERVE_DATA" != "true" ]]; then
        echo -e "  ✓ Vector database and stored patterns"
    else
        echo -e "  • Vector database data (preserved)"
    fi
    echo -e "  ✓ LitmusChaos framework"
    echo -e "  ✓ Test applications and data"
    echo -e "  ✓ Monitoring stack"
    echo -e "  ✓ RBAC and network resources"
    if [[ "$PRESERVE_CLUSTER" != "true" ]]; then
        echo -e "  ✓ OpenShift cluster (${CLUSTER_NAME})"
        echo -e "  ✓ libvirt VMs and storage"
    else
        echo -e "  • OpenShift cluster (preserved)"
    fi
    echo -e "  ✓ Temporary files and root configurations"

    echo -e "\n${BLUE}Root User Environment:${NC}"
    echo -e "Host: $(hostname)"
    echo -e "User: root"
    echo -e "Home: ${ROOT_HOME}"

    if [[ "$PRESERVE_CLUSTER" == "true" ]]; then
        echo -e "\n${YELLOW}Preserved Resources:${NC}"
        echo -e "  • OpenShift cluster: ${CLUSTER_NAME}"
        if [[ "$PRESERVE_DATA" == "true" ]]; then
            echo -e "  • Vector database data in kubernaut-system namespace"
        fi
        echo -e "\n${BLUE}To access preserved cluster as root:${NC}"
        echo -e "  export KUBECONFIG=${ROOT_HOME}/.kcli/clusters/${CLUSTER_NAME}/auth/kubeconfig"
        echo -e "  oc get nodes"
        echo -e "\n${BLUE}To remove preserved cluster later:${NC}"
        echo -e "  kcli delete cluster ${CLUSTER_NAME}"
    fi

    echo -e "\n${BLUE}To redeploy environment as root:${NC}"
    echo -e "  sudo ./setup-complete-e2e-environment-root.sh"

    echo -e "\n${BLUE}libvirt Status:${NC}"
    if systemctl is-active --quiet libvirtd; then
        echo -e "  • libvirtd: running"
        if command -v virsh &>/dev/null; then
            VM_COUNT=$(virsh list --all | grep -c "running\|shut off" || echo "0")
            echo -e "  • VMs: ${VM_COUNT} total"
        fi
    else
        echo -e "  • libvirtd: stopped"
    fi

    echo -e "${GREEN}========================================${NC}\n"

    log_success "Kubernaut E2E Testing Environment Cleanup Completed for Root User!"
}

# Main execution function
main() {
    log_info "Starting comprehensive cleanup of Kubernaut E2E testing environment for root"
    echo ""

    # Execute cleanup steps
    cleanup_test_applications
    cleanup_kubernaut_application
    cleanup_chaos_testing
    cleanup_monitoring
    cleanup_storage
    cleanup_rbac_and_network
    cleanup_cluster
    cleanup_temp_files

    # Print summary
    print_cleanup_summary
}

# Handle script arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --force)
            FORCE_CLEANUP="true"
            shift
            ;;
        --preserve-cluster)
            PRESERVE_CLUSTER="true"
            shift
            ;;
        --preserve-data)
            PRESERVE_DATA="true"
            shift
            ;;
        --cluster-name)
            CLUSTER_NAME="$2"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --force              Skip confirmation prompt"
            echo "  --preserve-cluster   Keep OpenShift cluster running"
            echo "  --preserve-data      Keep vector database data"
            echo "  --cluster-name NAME  Specify cluster name (default: kubernaut-e2e)"
            echo "  --help               Show this help message"
            echo ""
            echo "Environment Variables:"
            echo "  CLUSTER_NAME         Cluster name to cleanup"
            echo "  FORCE_CLEANUP        Skip confirmation (true/false)"
            echo "  PRESERVE_CLUSTER     Preserve cluster (true/false)"
            echo "  PRESERVE_DATA        Preserve data (true/false)"
            echo ""
            echo "NOTE: This script should be run as root for consistency with deployment"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            log_info "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Check if script is being sourced or executed
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
