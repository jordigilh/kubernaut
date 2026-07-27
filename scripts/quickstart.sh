#!/usr/bin/env bash
set -euo pipefail

# Kubernaut Quickstart Setup
# Creates the LLM credentials Secret and runs helm install with sensible defaults.
# Optional: configure Slack notifications.

NAMESPACE="${KUBERNAUT_NAMESPACE:-kubernaut-system}"
RELEASE_NAME="${KUBERNAUT_RELEASE:-kubernaut}"
CHART="${KUBERNAUT_CHART:-oci://quay.io/kubernaut-ai/charts/kubernaut}"

echo "=== Kubernaut Quickstart ==="
echo ""

# --- LLM Configuration ---
# DD-PLATFORM-007: the chart now consumes LLM settings via a named profile
# (global.llmProfiles.<name>) referenced by kubernautAgent.llmProfileRef,
# not a literal kubernautAgent.llm.* block.
read -rp "LLM provider (openai, anthropic): " LLM_PROVIDER
read -rp "LLM model (e.g., gpt-4o, claude-sonnet-4-20250514): " LLM_MODEL
read -rsp "API key: " API_KEY
echo ""

if [[ -z "$LLM_PROVIDER" || -z "$LLM_MODEL" || -z "$API_KEY" ]]; then
  echo "Error: provider, model, and API key are all required."
  exit 1
fi

# api_key is the fixed Secret key name the chart's mounted-credential-file
# convention expects for every non-vertex_ai provider (see
# kubernaut.llm.credFile in charts/kubernaut/templates/_helpers.tpl) --
# it is not provider-specific, unlike the old OPENAI_API_KEY/ANTHROPIC_API_KEY
# env-var convention this replaced.
KEY_NAME="api_key"

# API Frontend's LLMConfig.Validate() (pkg/shared/types/llm.go) requires a
# non-empty endpoint for provider=openai -- kubernaut-agent's client does
# not enforce this, but apifrontend.llmProfileRef falls back to this same
# profile by default, so every openai profile must supply one up front.
LLM_ENDPOINT=""
if [[ "$LLM_PROVIDER" == "openai" ]]; then
  read -rp "LLM endpoint (default: https://api.openai.com/v1): " LLM_ENDPOINT
  LLM_ENDPOINT="${LLM_ENDPOINT:-https://api.openai.com/v1}"
fi

# --- Optional Slack ---
HELM_SLACK_ARGS=()
read -rp "Enable Slack notifications? (y/N): " ENABLE_SLACK
if [[ "$ENABLE_SLACK" =~ ^[Yy]$ ]]; then
  read -rp "Slack webhook URL: " SLACK_WEBHOOK
  read -rp "Slack channel (default: #kubernaut-alerts): " SLACK_CHANNEL
  SLACK_CHANNEL="${SLACK_CHANNEL:-#kubernaut-alerts}"

  if [[ -n "$SLACK_WEBHOOK" ]]; then
    HELM_SLACK_ARGS=(
      --set notification.slack.secretName=slack-webhook
      --set "notification.slack.channel=$SLACK_CHANNEL"
    )
  fi
fi

echo ""
echo "--- Creating namespace and secrets ---"

kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret generic llm-credentials \
  --from-literal="$KEY_NAME=$API_KEY" \
  -n "$NAMESPACE" \
  --dry-run=client -o yaml | kubectl apply -f -

if [[ ${#HELM_SLACK_ARGS[@]} -gt 0 ]]; then
  kubectl create secret generic slack-webhook \
    --from-literal="webhook-url=$SLACK_WEBHOOK" \
    -n "$NAMESPACE" \
    --dry-run=client -o yaml | kubectl apply -f -
fi

echo ""
echo "--- Installing Kubernaut ---"

HELM_LLM_ARGS=(
  --set "global.llmProfiles.primary.provider=$LLM_PROVIDER"
  --set "global.llmProfiles.primary.model=$LLM_MODEL"
  --set "global.llmProfiles.primary.credentialsSecretName=llm-credentials"
  --set "kubernautAgent.llmProfileRef=primary"
)
if [[ -n "$LLM_ENDPOINT" ]]; then
  HELM_LLM_ARGS+=(--set "global.llmProfiles.primary.endpoint=$LLM_ENDPOINT")
fi

helm install "$RELEASE_NAME" "$CHART" \
  --namespace "$NAMESPACE" \
  "${HELM_LLM_ARGS[@]}" \
  "${HELM_SLACK_ARGS[@]}"

echo ""
echo "=== Kubernaut installed ==="
echo "Run: kubectl get pods -n $NAMESPACE"
