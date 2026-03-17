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
read -rp "LLM provider (openai, anthropic): " LLM_PROVIDER
read -rp "LLM model (e.g., gpt-4o, claude-sonnet-4-20250514): " LLM_MODEL
read -rsp "API key: " API_KEY
echo ""

if [[ -z "$LLM_PROVIDER" || -z "$LLM_MODEL" || -z "$API_KEY" ]]; then
  echo "Error: provider, model, and API key are all required."
  exit 1
fi

# Determine the env var name for the API key
case "$LLM_PROVIDER" in
  openai)      KEY_NAME="OPENAI_API_KEY" ;;
  anthropic)   KEY_NAME="ANTHROPIC_API_KEY" ;;
  *)           KEY_NAME="LLM_API_KEY" ;;
esac

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

helm install "$RELEASE_NAME" "$CHART" \
  --namespace "$NAMESPACE" \
  --set "holmesgptApi.llm.provider=$LLM_PROVIDER" \
  --set "holmesgptApi.llm.model=$LLM_MODEL" \
  "${HELM_SLACK_ARGS[@]}"

echo ""
echo "=== Kubernaut installed ==="
echo "Run: kubectl get pods -n $NAMESPACE"
