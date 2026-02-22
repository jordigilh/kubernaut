#!/usr/bin/env bash
# Shared platform deployment helpers for demo scenarios.
# Source this from run.sh:
#   source "$(dirname "$0")/../../scripts/platform-helper.sh"

PLATFORM_NS="${PLATFORM_NS:-kubernaut-system}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
CHART_DIR="${REPO_ROOT}/charts/kubernaut"
KIND_VALUES="${REPO_ROOT}/deploy/demo/helm/kubernaut-kind-values.yaml"
LLM_VALUES="${HOME}/.kubernaut/helm/llm-values.yaml"

ensure_platform() {
    if helm status kubernaut -n "${PLATFORM_NS}" &>/dev/null; then
        echo "  Kubernaut platform already installed."
        _check_llm_credentials
        return 0
    fi

    echo "==> Installing Kubernaut platform..."

    echo "  Applying CRDs..."
    kubectl apply -f "${CHART_DIR}/crds/" 2>&1 | sed 's/^/    /'

    local llm_flag=""
    if [ -f "${LLM_VALUES}" ]; then
        llm_flag="--values ${LLM_VALUES}"
        echo "  LLM config loaded from ${LLM_VALUES}"
    else
        echo "  WARNING: No LLM config found at ${LLM_VALUES}"
        echo "  Copy the example and fill in your values:"
        echo "    cp deploy/demo/helm/llm-values.yaml.example ~/.kubernaut/helm/llm-values.yaml"
    fi

    echo "  Installing Helm chart..."
    helm upgrade --install kubernaut "${CHART_DIR}" \
        --namespace "${PLATFORM_NS}" \
        --create-namespace \
        --values "${KIND_VALUES}" \
        ${llm_flag} \
        --skip-crds \
        --wait --timeout 10m

    echo "  Kubernaut platform installed in ${PLATFORM_NS}."
    _check_llm_credentials
}

_check_llm_credentials() {
    if ! kubectl get secret llm-credentials -n "${PLATFORM_NS}" &>/dev/null; then
        echo ""
        echo "  WARNING: LLM credentials not configured."
        echo "  AI analysis will not work until you create the llm-credentials Secret."
        echo ""
        echo "  Quick setup (Vertex AI):"
        echo "    cp deploy/demo/credentials/vertex-ai-example.yaml my-llm-credentials.yaml"
        echo "    # Edit with your provider credentials"
        echo "    kubectl apply -f my-llm-credentials.yaml"
        echo "    kubectl rollout restart deployment/holmesgpt-api -n ${PLATFORM_NS}"
        echo ""
    fi
}
