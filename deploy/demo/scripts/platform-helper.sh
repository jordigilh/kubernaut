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

# Seed the workflow for a specific scenario into DataStorage.
# Call after ensure_platform. Fails fast if seeding returns an unexpected HTTP code.
# Args: $1=scenario directory name (e.g., "crashloop")
seed_scenario_workflow() {
    local scenario="$1"
    local seed_script="${REPO_ROOT}/deploy/demo/scripts/seed-workflows.sh"

    if [ ! -f "$seed_script" ]; then
        echo "ERROR: seed-workflows.sh not found at ${seed_script}"
        return 1
    fi

    echo "==> Seeding workflow for scenario: ${scenario}"

    local output
    output=$("$seed_script" --scenario "$scenario" 2>&1)
    local exit_code=$?

    echo "$output" | sed 's/^/    /'

    if [ $exit_code -ne 0 ]; then
        echo "ERROR: Workflow seeding failed for scenario '${scenario}'"
        return 1
    fi

    if echo "$output" | grep -q "HTTP 502\|HTTP 500\|HTTP 503\|HTTP 504"; then
        echo "ERROR: DataStorage returned a server error while seeding '${scenario}'."
        echo "  The schema image may not be accessible (private repo or not yet pushed)."
        return 1
    fi

    echo "  Workflow seeded for ${scenario}."
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
