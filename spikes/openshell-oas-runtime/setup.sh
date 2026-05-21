#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CLUSTER_NAME="openshell-spike"
NAMESPACE="openshell"
OAS_IMAGE="oas-runtime:spike"

export KIND_EXPERIMENTAL_PROVIDER=podman

info()  { echo "==> $*"; }
error() { echo "ERROR: $*" >&2; exit 1; }

check_prerequisites() {
    info "Checking prerequisites..."
    for cmd in kind helm kubectl podman openshell; do
        if ! command -v "$cmd" &>/dev/null; then
            if [ "$cmd" = "openshell" ]; then
                info "OpenShell CLI not found. Installing..."
                curl -LsSf https://raw.githubusercontent.com/NVIDIA/OpenShell/main/install.sh | sh
                if ! command -v openshell &>/dev/null; then
                    error "Failed to install OpenShell CLI"
                fi
            else
                error "$cmd is required but not found"
            fi
        fi
    done
    info "All prerequisites met"
}

create_cluster() {
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        info "Kind cluster '${CLUSTER_NAME}' already exists, reusing"
    else
        info "Creating Kind cluster '${CLUSTER_NAME}'..."
        kind create cluster --config "${SCRIPT_DIR}/kind-config.yaml"
    fi
    kubectl cluster-info --context "kind-${CLUSTER_NAME}"
}

install_agent_sandbox() {
    if kubectl get crd sandboxes.agents.x-k8s.io &>/dev/null; then
        info "Agent Sandbox CRDs already installed"
        return
    fi
    info "Installing Agent Sandbox controller..."
    VERSION=$(curl -s https://api.github.com/repos/kubernetes-sigs/agent-sandbox/releases/latest | jq -r '.tag_name')
    if [ -z "$VERSION" ] || [ "$VERSION" = "null" ]; then
        error "Failed to fetch Agent Sandbox release version"
    fi
    info "Agent Sandbox version: ${VERSION}"
    kubectl apply -f "https://github.com/kubernetes-sigs/agent-sandbox/releases/download/${VERSION}/manifest.yaml"
    info "Waiting for Agent Sandbox controller..."
    kubectl -n agent-sandbox-system wait --for=condition=available deployment --all --timeout=120s
}

install_openshell() {
    if helm list -n "${NAMESPACE}" 2>/dev/null | grep -q openshell; then
        info "OpenShell Helm release already installed"
        return
    fi
    info "Installing OpenShell via Helm..."
    kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
    helm upgrade --install openshell \
        oci://ghcr.io/nvidia/openshell/helm-chart \
        --version 0.0.0-dev \
        --namespace "${NAMESPACE}" \
        --set server.sandboxImage="${OAS_IMAGE}" \
        --wait --timeout 5m
    info "Waiting for OpenShell gateway..."
    kubectl -n "${NAMESPACE}" rollout status statefulset/openshell --timeout=180s
}

build_and_load_image() {
    info "Building OAS Runtime BYOC image..."
    local oas_runtime_dir="${SCRIPT_DIR}/../oas-runtime"
    if [ ! -f "${oas_runtime_dir}/Dockerfile" ]; then
        error "OAS Runtime Dockerfile not found at ${oas_runtime_dir}/Dockerfile"
    fi
    podman build -t "${OAS_IMAGE}" "${oas_runtime_dir}"
    info "Loading image into Kind cluster..."
    kind load docker-image "${OAS_IMAGE}" --name "${CLUSTER_NAME}"
}

setup_gateway_access() {
    info "Extracting client mTLS certificates..."
    local mtls_dir="${HOME}/.config/openshell/gateways/k8s-spike/mtls"
    mkdir -p "${mtls_dir}"
    kubectl -n "${NAMESPACE}" get secret openshell-client-tls \
        -o jsonpath='{.data.ca\.crt}' | base64 -d > "${mtls_dir}/ca.crt"
    kubectl -n "${NAMESPACE}" get secret openshell-client-tls \
        -o jsonpath='{.data.tls\.crt}' | base64 -d > "${mtls_dir}/tls.crt"
    kubectl -n "${NAMESPACE}" get secret openshell-client-tls \
        -o jsonpath='{.data.tls\.key}' | base64 -d > "${mtls_dir}/tls.key"

    info "Starting port-forward to gateway (background)..."
    kubectl -n "${NAMESPACE}" port-forward svc/openshell 17670:8080 &
    local pf_pid=$!
    echo "${pf_pid}" > "${SCRIPT_DIR}/.port-forward.pid"
    sleep 3

    if ! kill -0 "${pf_pid}" 2>/dev/null; then
        error "Port-forward failed to start"
    fi

    info "Registering gateway with OpenShell CLI..."
    openshell gateway add https://127.0.0.1:17670 --local --name k8s-spike 2>/dev/null || true
    openshell gateway select k8s-spike 2>/dev/null || true
    openshell status
}

print_summary() {
    echo ""
    echo "=========================================="
    echo "  OpenShell + OAS Runtime Spike Ready"
    echo "=========================================="
    echo ""
    echo "Kind cluster:    ${CLUSTER_NAME}"
    echo "Namespace:       ${NAMESPACE}"
    echo "Gateway:         https://127.0.0.1:17670"
    echo "OAS Runtime:     ${OAS_IMAGE}"
    echo ""
    echo "Next steps:"
    echo "  1. Run test-sandbox.sh to create and test the sandbox"
    echo "  2. Run teardown.sh to clean up"
    echo ""
}

main() {
    info "Starting OpenShell + OAS Runtime spike setup"
    check_prerequisites
    create_cluster
    install_agent_sandbox
    build_and_load_image
    install_openshell
    setup_gateway_access
    print_summary
}

main "$@"
