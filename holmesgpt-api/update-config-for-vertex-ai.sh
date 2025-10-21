#!/bin/bash
# Update HolmesGPT API ConfigMap with Vertex AI settings
# This script updates the ConfigMap to use your existing Vertex AI credentials

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}🔧 Updating HolmesGPT API ConfigMap for Vertex AI${NC}"
echo ""

# Get GCP project from environment or gcloud
GCP_PROJECT_ID="${ANTHROPIC_VERTEX_PROJECT_ID:-$(gcloud config get-value project 2>/dev/null)}"
GCP_REGION="${GCP_REGION:-us-central1}"

if [ -z "$GCP_PROJECT_ID" ]; then
    echo -e "${RED}❌ Error: Could not determine GCP project ID${NC}"
    echo "Please set ANTHROPIC_VERTEX_PROJECT_ID or configure gcloud"
    exit 1
fi

echo -e "${GREEN}✅ GCP Project: ${GCP_PROJECT_ID}${NC}"
echo -e "${GREEN}✅ GCP Region: ${GCP_REGION}${NC}"
echo ""

# Create temporary config file with actual values
TEMP_CONFIG="${SCRIPT_DIR}/config-temp.yaml"

cat > "$TEMP_CONFIG" <<EOF
# HolmesGPT API Configuration - Vertex AI
# Auto-generated from update-config-for-vertex-ai.sh

# Application Settings
log_level: INFO
dev_mode: false
auth_enabled: true
api_host: "0.0.0.0"
api_port: 8080

# LLM Provider Configuration
llm:
  provider: vertex-ai
  model: claude-3-5-sonnet@20241022
  endpoint: https://${GCP_REGION}-aiplatform.googleapis.com

  # GCP Configuration
  gcp_project_id: ${GCP_PROJECT_ID}
  gcp_region: ${GCP_REGION}

  # Rate limiting and timeouts
  max_retries: 3
  timeout_seconds: 60

  # Cost controls
  max_tokens_per_request: 4096
  temperature: 0.7

# Context API Configuration
context_api:
  url: http://context-api.kubernaut-system.svc.cluster.local:8091
  timeout_seconds: 10
  max_retries: 2

# Kubernetes API Configuration
kubernetes:
  service_host: kubernetes.default.svc
  service_port: 443
  token_reviewer_enabled: true

# Public endpoints (no auth required)
public_endpoints:
  - /health
  - /ready
  - /metrics

# Metrics Configuration
metrics:
  enabled: true
  endpoint: /metrics
  scrape_interval: 30s
EOF

echo -e "${YELLOW}📋 Generated configuration:${NC}"
cat "$TEMP_CONFIG"
echo ""

# Update ConfigMap
echo -e "${YELLOW}🔄 Updating ConfigMap...${NC}"

kubectl create configmap holmesgpt-api-config \
    --from-file=config.yaml="$TEMP_CONFIG" \
    --namespace=kubernaut-system \
    --dry-run=client -o yaml | kubectl apply -f -

echo -e "${GREEN}✅ ConfigMap updated${NC}"

# Clean up temp file
rm "$TEMP_CONFIG"

# Check if we have Application Default Credentials
echo ""
echo -e "${YELLOW}🔑 Checking Vertex AI credentials...${NC}"

if gcloud auth application-default print-access-token >/dev/null 2>&1; then
    echo -e "${GREEN}✅ Application Default Credentials (ADC) configured${NC}"
    echo ""
    echo -e "${YELLOW}📦 Creating service account key from ADC...${NC}"

    # Get the ADC credentials path
    if [ -n "$GOOGLE_APPLICATION_CREDENTIALS" ]; then
        ADC_PATH="$GOOGLE_APPLICATION_CREDENTIALS"
    else
        # Default ADC path
        ADC_PATH="$HOME/.config/gcloud/application_default_credentials.json"
    fi

    if [ -f "$ADC_PATH" ]; then
        echo -e "${GREEN}✅ Found ADC at: ${ADC_PATH}${NC}"

        # Create Kubernetes secret from ADC (generic name, not provider-specific)
        kubectl create secret generic holmesgpt-api-llm-credentials \
            --from-file=credentials.json="$ADC_PATH" \
            --namespace=kubernaut-system \
            --dry-run=client -o yaml | kubectl apply -f -

        echo -e "${GREEN}✅ Created holmesgpt-api-llm-credentials secret from ADC${NC}"
    else
        echo -e "${YELLOW}⚠️  ADC file not found at ${ADC_PATH}${NC}"
        echo -e "${YELLOW}   Run: gcloud auth application-default login${NC}"
    fi
else
    echo -e "${YELLOW}⚠️  Application Default Credentials not configured${NC}"
    echo -e "${YELLOW}   Run: gcloud auth application-default login${NC}"
fi

echo ""
echo -e "${GREEN}✅ Configuration update complete!${NC}"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo "  1. Restart deployment to pick up new config:"
echo "     kubectl rollout restart deployment/holmesgpt-api -n kubernaut-system"
echo ""
echo "  2. Watch rollout:"
echo "     kubectl rollout status deployment/holmesgpt-api -n kubernaut-system"
echo ""
echo "  3. Check logs:"
echo "     kubectl logs -f deployment/holmesgpt-api -n kubernaut-system | grep config_loaded"
echo ""
echo "  4. Test with real LLM:"
echo "     kubectl port-forward -n kubernaut-system svc/holmesgpt-api 8080:8080"
echo "     curl http://localhost:8080/health | jq ."
echo ""
echo -e "${YELLOW}💰 Cost Monitoring:${NC}"
echo "  - Monitor metrics: kubectl port-forward -n kubernaut-system svc/prometheus 9090:9090"
echo "  - Query: holmesgpt_llm_token_usage_total{provider=\"vertex-ai\"}"
echo ""

