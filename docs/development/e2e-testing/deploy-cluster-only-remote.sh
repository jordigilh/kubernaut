#!/bin/bash
# Remote OCP Cluster-Only Deployment Script
#
# This script deploys ONLY the OpenShift cluster on a remote host (helios08)
# For hybrid architecture where:
# - OCP Cluster: Remote (helios08)
# - AI Model: Local (localhost:8080)
# - Kubernaut: Local (connecting to remote cluster)
# - Tests: Local

set -euo pipefail

# Configuration
CLUSTER_NAME="${1:-kubernaut-e2e}"
CONFIG_FILE="${2:-kcli-baremetal-params-root.yml}"
REMOTE_HOST="${REMOTE_HOST:-helios08}"
REMOTE_USER="${REMOTE_USER:-root}"
REMOTE_PATH="/root/kubernaut-e2e"

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
 ____                      _          ____ _           _              ___        _
|  _ \ ___ _ __ ___   ___ | |_ ___   / ___| |_   _ ___| |_ ___ _ __   / _ \ _ __ | |_   _
| |_) / _ \ '_ ` _ \ / _ \| __/ _ \ | |   | | | | / __| __/ _ \ '__| | | | | '_ \| | | | |
|  _ <  __/ | | | | | (_) | ||  __/ | |___| | |_| \__ \ ||  __/ |    | |_| | | | | | |_| |
|_| \_\___|_| |_| |_|\___/ \__\___|  \____|_|\__,_|___/\__\___|_|     \___/|_| |_|_|\__, |
                                                                                    |___/
EOF
echo -e "${NC}"

log_info "Remote OpenShift Cluster-Only Deployment"
log_info "Target Host: ${REMOTE_USER}@${REMOTE_HOST}"
log_info "Cluster Name: ${CLUSTER_NAME}"
log_info "Architecture: Hybrid (cluster remote, AI+tests local)"

# Validate SSH connection
validate_remote_connection() {
    log_header "Validating Remote Connection"

    if ssh -o ConnectTimeout=10 -o BatchMode=yes "${REMOTE_USER}@${REMOTE_HOST}" "echo 'Connection verified'" 2>/dev/null; then
        log_success "SSH connection to ${REMOTE_HOST} verified"
    else
        log_error "Cannot connect to ${REMOTE_HOST}"
        log_info "Please ensure SSH key authentication is configured"
        exit 1
    fi

    # Get remote host information
    REMOTE_HOSTNAME=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "hostname" 2>/dev/null)
    REMOTE_OS=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "cat /etc/os-release | grep '^PRETTY_NAME=' | cut -d'\"' -f2" 2>/dev/null)
    REMOTE_MEMORY_GB=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "free -g | awk '/^Mem:/ {print \$2}'" 2>/dev/null)

    log_info "Remote Host: ${REMOTE_HOSTNAME}"
    log_info "OS: ${REMOTE_OS}"
    log_info "Memory: ${REMOTE_MEMORY_GB} GB"
}

# Copy deployment scripts to remote host
copy_deployment_scripts() {
    log_header "Copying Deployment Scripts to Remote Host"

    # Create remote directory
    ssh "${REMOTE_USER}@${REMOTE_HOST}" "mkdir -p ${REMOTE_PATH}"

    # Copy only cluster deployment related files
    log_info "Copying cluster deployment scripts..."
    scp deploy-kcli-cluster-root.sh "${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_PATH}/"
    scp validate-baremetal-setup-root.sh "${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_PATH}/"
    scp kcli-baremetal-params-root.yml "${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_PATH}/"
    scp setup-storage.sh "${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_PATH}/"

    # Copy storage configuration if it exists
    if [[ -d "storage" ]]; then
        scp -r storage "${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_PATH}/"
    fi

    # Make scripts executable on remote host
    ssh "${REMOTE_USER}@${REMOTE_HOST}" "cd ${REMOTE_PATH} && chmod +x *.sh"

    log_success "Deployment scripts copied to remote host"
}

# Deploy cluster on remote host
deploy_remote_cluster() {
    log_header "Deploying OpenShift Cluster on Remote Host"

    log_info "Starting cluster deployment on ${REMOTE_HOST}..."
    log_info "This will take 45-60 minutes for a complete deployment"

    # Run cluster deployment on remote host
    ssh "${REMOTE_USER}@${REMOTE_HOST}" "cd ${REMOTE_PATH} && ./deploy-kcli-cluster-root.sh ${CLUSTER_NAME} ${CONFIG_FILE}" | while read line; do
        echo -e "${BLUE}[REMOTE]${NC} $line"
    done

    # Check if deployment was successful
    if ssh "${REMOTE_USER}@${REMOTE_HOST}" "test -f ${REMOTE_PATH}/kubeconfig"; then
        log_success "Cluster deployment completed successfully"
    else
        log_error "Cluster deployment failed - kubeconfig not found"
        exit 1
    fi
}

# Validate cluster deployment
validate_cluster_deployment() {
    log_header "Validating Remote Cluster Deployment"

    # Check cluster status from remote host
    log_info "Checking cluster status..."

    # Test cluster access from remote host
    CLUSTER_STATUS=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "cd ${REMOTE_PATH} && export KUBECONFIG=./kubeconfig && oc get nodes --no-headers 2>/dev/null | wc -l" || echo "0")

    if [[ "$CLUSTER_STATUS" -gt 0 ]]; then
        log_success "Cluster is accessible with ${CLUSTER_STATUS} nodes"

        # Get cluster details
        ssh "${REMOTE_USER}@${REMOTE_HOST}" "cd ${REMOTE_PATH} && export KUBECONFIG=./kubeconfig && echo 'Cluster Nodes:' && oc get nodes" | while read line; do
            echo -e "${GREEN}[CLUSTER]${NC} $line"
        done

        # Get cluster version
        CLUSTER_VERSION=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "cd ${REMOTE_PATH} && export KUBECONFIG=./kubeconfig && oc get clusterversion -o jsonpath='{.items[0].status.desired.version}' 2>/dev/null" || echo "unknown")
        log_info "Cluster Version: ${CLUSTER_VERSION}"

        # Get cluster URL
        CLUSTER_URL=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "cd ${REMOTE_PATH} && export KUBECONFIG=./kubeconfig && oc whoami --show-server 2>/dev/null" || echo "unknown")
        log_info "Cluster API URL: ${CLUSTER_URL}"

        # Get web console URL
        CONSOLE_URL=$(ssh "${REMOTE_USER}@${REMOTE_HOST}" "cd ${REMOTE_PATH} && export KUBECONFIG=./kubeconfig && oc get routes console -n openshift-console -o jsonpath='{.spec.host}' 2>/dev/null" || echo "unknown")
        if [[ "$CONSOLE_URL" != "unknown" ]]; then
            log_info "Web Console: https://${CONSOLE_URL}"
        fi

    else
        log_error "Cluster validation failed"
        exit 1
    fi
}

# Setup cluster for hybrid integration
setup_cluster_for_hybrid() {
    log_header "Configuring Cluster for Hybrid Architecture"

    log_info "Setting up cluster for hybrid integration..."

    # Create namespace for Kubernaut components (if needed for cluster-side resources)
    ssh "${REMOTE_USER}@${REMOTE_HOST}" "cd ${REMOTE_PATH} && export KUBECONFIG=./kubeconfig && oc new-project kubernaut-system 2>/dev/null || oc project kubernaut-system"

    # Create service account for remote management
    ssh "${REMOTE_USER}@${REMOTE_HOST}" "cd ${REMOTE_PATH} && export KUBECONFIG=./kubeconfig && cat << 'EOFSA' | oc apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernaut-remote
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-remote
rules:
- apiGroups: ['*']
  resources: ['*']
  verbs: ['*']
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-remote
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kubernaut-remote
subjects:
- kind: ServiceAccount
  name: kubernaut-remote
  namespace: kubernaut-system
EOFSA"

    # Create a note about the hybrid architecture
    ssh "${REMOTE_USER}@${REMOTE_HOST}" "cd ${REMOTE_PATH} && cat > HYBRID_ARCHITECTURE_README.md << 'EOFNOTE'
# Hybrid Kubernaut Architecture

This OpenShift cluster is part of a hybrid deployment:

## Architecture
- **OCP Cluster**: This host ($(hostname))
- **AI Model**: Developer machine (localhost:8080)
- **Kubernaut**: Developer machine (connects to this cluster)
- **Tests**: Developer machine
- **Vector DB**: Developer machine (PostgreSQL)

## Network Topology
- Developer machine can reach: cluster + AI model
- This cluster CANNOT reach: AI model on developer machine
- This is intentional for security isolation

## Cluster Access
- Kubeconfig: ${REMOTE_PATH}/kubeconfig
- Web Console: https://$(oc get routes console -n openshift-console -o jsonpath='{.spec.host}' 2>/dev/null || echo 'not-available')
- API Server: $(oc whoami --show-server 2>/dev/null || echo 'not-available')

## Management
- This cluster is managed remotely by Kubernaut running on the developer machine
- No AI components are deployed in this cluster
- All AI processing happens on the developer machine

## Storage
- OpenShift Data Foundation (ODF) is available for persistent storage
- Local Storage Operator (LSO) is configured for node-local storage
EOFNOTE"

    log_success "Cluster configured for hybrid architecture"
}

# Create local access instructions
create_local_access_guide() {
    log_header "Creating Local Access Guide"

    # Create local access instructions file
    cat > "./REMOTE_CLUSTER_ACCESS.md" << EOF
# Remote Cluster Access Guide

## Cluster Information
- **Remote Host**: ${REMOTE_USER}@${REMOTE_HOST}
- **Cluster Name**: ${CLUSTER_NAME}
- **Remote Path**: ${REMOTE_PATH}

## Quick Access Commands

### Copy kubeconfig to local machine:
\`\`\`bash
scp ${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_PATH}/kubeconfig ./kubeconfig-${REMOTE_HOST}
export KUBECONFIG=./kubeconfig-${REMOTE_HOST}
oc get nodes
\`\`\`

### Setup local Kubernaut for hybrid deployment:
\`\`\`bash
./setup-local-kubernaut-remote-cluster.sh ${REMOTE_HOST} ${REMOTE_USER}
\`\`\`

### Check remote cluster status:
\`\`\`bash
make status-e2e-remote
\`\`\`

### Access remote cluster directly:
\`\`\`bash
ssh ${REMOTE_USER}@${REMOTE_HOST}
cd ${REMOTE_PATH}
export KUBECONFIG=./kubeconfig
oc get nodes
\`\`\`

## Hybrid Architecture Setup

1. **Deploy cluster** (completed): \`make deploy-cluster-remote\`
2. **Setup local Kubernaut**: \`make setup-local-hybrid\`
3. **Start local AI model**: Start oss-gpt:20b on localhost:8080
4. **Run hybrid tests**: \`make test-e2e-hybrid\`

## Network Validation

The cluster cannot reach your local AI model (this is expected):
\`\`\`bash
# This should fail (which is correct):
oc run test-pod --image=curlimages/curl --restart=Never --rm -i --tty -- curl http://host.docker.internal:8080
\`\`\`

Your local machine can reach both the cluster and AI model:
\`\`\`bash
# Both should work:
oc get nodes                    # Cluster access
curl http://localhost:8080      # AI model access
\`\`\`
EOF

    log_success "Local access guide created: ./REMOTE_CLUSTER_ACCESS.md"
}

# Main execution
main() {
    log_info "Starting remote cluster-only deployment for hybrid architecture"

    # Validate connection
    validate_remote_connection

    # Copy deployment scripts
    copy_deployment_scripts

    # Deploy cluster
    deploy_remote_cluster

    # Validate deployment
    validate_cluster_deployment

    # Setup for hybrid integration
    setup_cluster_for_hybrid

    # Create local access guide
    create_local_access_guide

    log_success "Remote cluster deployment completed successfully!"

    # Display next steps
    log_header "Next Steps for Hybrid Setup"
    echo -e "${GREEN}1. Setup local Kubernaut:${NC} ./setup-local-kubernaut-remote-cluster.sh ${REMOTE_HOST} ${REMOTE_USER}"
    echo -e "${GREEN}2. Start local AI model:${NC} Ensure oss-gpt:20b is running on localhost:8080"
    echo -e "${GREEN}3. Run hybrid tests:${NC} make test-e2e-hybrid"
    echo -e "${GREEN}4. Access cluster locally:${NC} scp ${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_PATH}/kubeconfig ./kubeconfig"

    log_header "Hybrid Architecture Summary"
    echo -e "${CYAN}Deployment Complete:${NC}"
    echo -e "  🖥️  OpenShift Cluster: ${REMOTE_HOST} ✅"
    echo -e "  🤖 AI Model: localhost:8080 (setup required)"
    echo -e "  🔧 Kubernaut: localhost (setup required)"
    echo -e "  🧪 Tests: localhost (setup required)"

    log_info "Remote cluster is ready for hybrid E2E testing!"
}

# Execute main function
main "$@"
