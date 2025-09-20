#!/bin/bash
# OpenShift Storage Setup Automation Script
# Installs and configures Local Storage Operator (LSO) and OpenShift Data Foundation (ODF)

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KUBECONFIG="${KUBECONFIG:-$HOME/.kcli/clusters/ocp418-baremetal/auth/kubeconfig}"

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_header() { echo -e "\n${BLUE}=== $1 ===${NC}"; }

# Utility functions
wait_for_operator() {
    local operator_name=$1
    local namespace=$2
    local timeout=${3:-300}

    log_info "Waiting for ${operator_name} to be ready in namespace ${namespace}..."

    local count=0
    while [[ $count -lt $timeout ]]; do
        if oc get csv -n "$namespace" | grep -q "$operator_name.*Succeeded"; then
            log_success "${operator_name} is ready"
            return 0
        fi
        sleep 10
        count=$((count + 10))
        log_info "Waiting... (${count}/${timeout}s)"
    done

    log_error "Timeout waiting for ${operator_name}"
    return 1
}

wait_for_pods() {
    local namespace=$1
    local timeout=${2:-300}

    log_info "Waiting for pods in namespace ${namespace} to be ready..."

    if oc wait --for=condition=Ready pods --all -n "$namespace" --timeout="${timeout}s"; then
        log_success "All pods in ${namespace} are ready"
    else
        log_error "Timeout waiting for pods in ${namespace}"
        oc get pods -n "$namespace"
        return 1
    fi
}

# Pre-flight checks
preflight_checks() {
    log_header "Pre-flight Checks"

    # Check if oc is available
    if ! command -v oc &> /dev/null; then
        log_error "OpenShift CLI (oc) is not available"
        exit 1
    fi

    # Check cluster connectivity
    if ! oc whoami &> /dev/null; then
        log_error "Cannot connect to OpenShift cluster"
        log_info "Make sure KUBECONFIG is set: export KUBECONFIG=${KUBECONFIG}"
        exit 1
    fi

    # Check cluster status
    if ! oc get nodes &> /dev/null; then
        log_error "Cannot access cluster nodes"
        exit 1
    fi

    log_success "Pre-flight checks completed"
}

# Create required namespaces
create_namespaces() {
    log_header "Creating Namespaces"

    # Create openshift-local-storage namespace
    if ! oc get namespace openshift-local-storage &> /dev/null; then
        oc create namespace openshift-local-storage
        log_success "Created namespace: openshift-local-storage"
    else
        log_info "Namespace openshift-local-storage already exists"
    fi

    # Create openshift-storage namespace
    if ! oc get namespace openshift-storage &> /dev/null; then
        oc create namespace openshift-storage
        log_success "Created namespace: openshift-storage"
    else
        log_info "Namespace openshift-storage already exists"
    fi

    # Label nodes for storage
    log_info "Labeling worker nodes for storage..."
    oc label nodes -l node-role.kubernetes.io/worker= cluster.ocs.openshift.io/openshift-storage="" --overwrite
    log_success "Labeled worker nodes for storage"
}

# Install Local Storage Operator
install_lso() {
    log_header "Installing Local Storage Operator (LSO)"

    # Apply LSO configuration
    if [[ -f "${SCRIPT_DIR}/storage/local-storage-operator.yaml" ]]; then
        oc apply -f "${SCRIPT_DIR}/storage/local-storage-operator.yaml"
        log_success "Applied Local Storage Operator configuration"
    else
        log_error "Local Storage Operator configuration file not found"
        exit 1
    fi

    # Wait for LSO to be ready
    wait_for_operator "local-storage-operator" "openshift-local-storage" 300

    # Wait for LSO pods
    wait_for_pods "openshift-local-storage" 300

    log_success "Local Storage Operator installation completed"
}

# Configure local storage devices
configure_local_storage() {
    log_header "Configuring Local Storage Devices"

    # Wait for local volume discovery to complete
    log_info "Waiting for device discovery to complete..."
    sleep 30

    # Check discovered devices
    if oc get localvolumediscoveryresults -n openshift-local-storage &> /dev/null; then
        log_info "Discovered storage devices:"
        oc get localvolumediscoveryresults -n openshift-local-storage -o wide
    else
        log_warning "No devices discovered yet, this may take some time"
    fi

    # Wait for local volumes to be created
    log_info "Waiting for local volumes to be provisioned..."
    sleep 60

    # Verify storage classes
    if oc get storageclass local-block &> /dev/null; then
        log_success "Local block storage class created"
    else
        log_warning "Local block storage class not yet available"
    fi

    if oc get storageclass local-filesystem &> /dev/null; then
        log_success "Local filesystem storage class created"
    else
        log_warning "Local filesystem storage class not yet available"
    fi
}

# Install ODF Operator
install_odf() {
    log_header "Installing OpenShift Data Foundation (ODF)"

    # Apply ODF configuration
    if [[ -f "${SCRIPT_DIR}/storage/odf-operator.yaml" ]]; then
        oc apply -f "${SCRIPT_DIR}/storage/odf-operator.yaml"
        log_success "Applied ODF Operator configuration"
    else
        log_error "ODF Operator configuration file not found"
        exit 1
    fi

    # Wait for ODF operator to be ready
    wait_for_operator "odf-operator" "openshift-storage" 600

    # Wait for additional operators to be installed
    log_info "Waiting for additional ODF operators to be ready..."
    sleep 30

    # Check for other ODF-related operators
    local odf_operators=("ocs-operator" "noobaa-operator" "csi-addons")
    for op in "${odf_operators[@]}"; do
        if oc get csv -n openshift-storage | grep -q "$op"; then
            wait_for_operator "$op" "openshift-storage" 300 || log_warning "Operator $op may not be fully ready"
        fi
    done

    log_success "ODF Operator installation completed"
}

# Configure ODF storage cluster
configure_odf_cluster() {
    log_header "Configuring ODF Storage Cluster"

    # Wait for local storage to be available
    log_info "Ensuring local storage is available for ODF..."
    local max_wait=300
    local count=0

    while [[ $count -lt $max_wait ]]; do
        if oc get pv | grep -q "local-block.*Available"; then
            log_success "Local block storage is available for ODF"
            break
        fi
        sleep 10
        count=$((count + 10))
        log_info "Waiting for local storage... (${count}/${max_wait}s)"
    done

    if [[ $count -ge $max_wait ]]; then
        log_error "Timeout waiting for local storage to be available"
        log_info "Available persistent volumes:"
        oc get pv
        return 1
    fi

    # Wait for storage cluster to be ready
    log_info "Waiting for ODF Storage Cluster to be ready (this may take 10-15 minutes)..."
    local storage_wait=900  # 15 minutes
    count=0

    while [[ $count -lt $storage_wait ]]; do
        if oc get storagecluster ocs-storagecluster -n openshift-storage -o jsonpath='{.status.phase}' 2>/dev/null | grep -q "Ready"; then
            log_success "ODF Storage Cluster is ready"
            break
        fi
        sleep 30
        count=$((count + 30))
        log_info "Storage cluster status: $(oc get storagecluster ocs-storagecluster -n openshift-storage -o jsonpath='{.status.phase}' 2>/dev/null || echo "Not ready")"
        log_info "Waiting... (${count}/${storage_wait}s)"
    done

    if [[ $count -ge $storage_wait ]]; then
        log_error "Timeout waiting for ODF Storage Cluster to be ready"
        oc describe storagecluster ocs-storagecluster -n openshift-storage
        return 1
    fi

    # Wait for ODF pods to be ready
    wait_for_pods "openshift-storage" 600

    log_success "ODF Storage Cluster configuration completed"
}

# Verify storage setup
verify_storage() {
    log_header "Verifying Storage Setup"

    # Check storage classes
    log_info "Available storage classes:"
    oc get storageclass

    # Check if default storage class is set
    if oc get storageclass | grep -q "(default)"; then
        DEFAULT_SC=$(oc get storageclass | grep "(default)" | awk '{print $1}')
        log_success "Default storage class: ${DEFAULT_SC}"
    else
        log_warning "No default storage class is set"
    fi

    # Check persistent volumes
    log_info "Available persistent volumes:"
    oc get pv

    # Check Ceph cluster status (if ODF is installed)
    if oc get cephcluster -n openshift-storage &> /dev/null; then
        log_info "Ceph cluster status:"
        oc get cephcluster -n openshift-storage -o wide

        # Check Ceph health
        if oc get cephcluster -n openshift-storage -o jsonpath='{.items[0].status.ceph.health}' | grep -q "HEALTH_OK"; then
            log_success "Ceph cluster is healthy"
        else
            log_warning "Ceph cluster may not be fully healthy yet"
        fi
    fi

    # Test storage with a simple PVC
    log_info "Testing storage with a test PVC..."
    cat << EOF | oc apply -f -
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: storage-test-pvc
  namespace: default
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
EOF

    # Wait for PVC to be bound
    if oc wait --for=condition=Bound pvc/storage-test-pvc -n default --timeout=60s; then
        log_success "Test PVC bound successfully"
        oc delete pvc storage-test-pvc -n default
        log_info "Test PVC cleaned up"
    else
        log_warning "Test PVC did not bind within timeout"
        oc describe pvc storage-test-pvc -n default
    fi
}

# Print storage information
print_storage_info() {
    log_header "Storage Configuration Summary"

    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  STORAGE CONFIGURATION SUMMARY${NC}"
    echo -e "${GREEN}========================================${NC}"

    # Storage Classes
    echo -e "\n${BLUE}Storage Classes:${NC}"
    oc get storageclass --no-headers | while read -r line; do
        sc_name=$(echo "$line" | awk '{print $1}')
        sc_provisioner=$(echo "$line" | awk '{print $2}')
        is_default=$(echo "$line" | grep -q "(default)" && echo " (default)" || echo "")
        echo -e "  • ${sc_name}: ${sc_provisioner}${is_default}"
    done

    # Operators
    echo -e "\n${BLUE}Installed Storage Operators:${NC}"
    if oc get csv -n openshift-local-storage | grep -q "local-storage-operator"; then
        echo -e "  ✓ Local Storage Operator (LSO)"
    fi
    if oc get csv -n openshift-storage | grep -q "odf-operator"; then
        echo -e "  ✓ OpenShift Data Foundation (ODF)"
    fi

    # Ceph Status
    if oc get cephcluster -n openshift-storage &> /dev/null; then
        CEPH_HEALTH=$(oc get cephcluster -n openshift-storage -o jsonpath='{.items[0].status.ceph.health}' 2>/dev/null || echo "Unknown")
        echo -e "\n${BLUE}Ceph Cluster Health:${NC} ${CEPH_HEALTH}"
    fi

    # Usage Examples
    echo -e "\n${BLUE}Usage Examples:${NC}"
    echo -e "  # Create a PVC using the default storage class:"
    echo -e "  kubectl apply -f - <<EOF"
    echo -e "  apiVersion: v1"
    echo -e "  kind: PersistentVolumeClaim"
    echo -e "  metadata:"
    echo -e "    name: my-pvc"
    echo -e "  spec:"
    echo -e "    accessModes: [ReadWriteOnce]"
    echo -e "    resources:"
    echo -e "      requests:"
    echo -e "        storage: 10Gi"
    echo -e "  EOF"

    echo -e "${GREEN}========================================${NC}\n"
}

# Main execution function
main() {
    log_info "Starting OpenShift storage setup automation"

    preflight_checks
    create_namespaces
    install_lso
    configure_local_storage
    install_odf
    configure_odf_cluster
    verify_storage
    print_storage_info

    log_success "OpenShift storage setup completed successfully!"
}

# Check if script is being sourced or executed
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
