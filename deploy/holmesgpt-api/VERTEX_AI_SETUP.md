# Vertex AI Configuration for HolmesGPT API

## ⚠️ **DEPRECATED - DO NOT USE**

**This document is outdated and references an old architecture that no longer exists.**

**Status**: This guide uses environment variable-based configuration which was **replaced with YAML ConfigMap** in v1.0.3

**For current Vertex AI setup, see**:
- **Triage Report**: [`VERTEX_AI_SETUP_TRIAGE.md`](./VERTEX_AI_SETUP_TRIAGE.md) - Explains what's wrong with this document
- **Deployment Guide**: [`README.md`](./README.md) - Current deployment instructions
- **Configuration Guide**: [`../../docs/services/stateless/holmesgpt-api/CONFIGURATION_REFACTORING_SUMMARY.md`](../../docs/services/stateless/holmesgpt-api/CONFIGURATION_REFACTORING_SUMMARY.md) - New architecture explanation

**Following this guide will result in 100% deployment failure.**

---

## Overview (OUTDATED - DO NOT FOLLOW)

This guide explains how to configure HolmesGPT API to use Claude 3.5 Sonnet via Google Cloud Vertex AI.

## Prerequisites

1. **Google Cloud Project** with Vertex AI API enabled
2. **Claude 3.5 Sonnet** access enabled in your GCP project
3. **Service Account** with Vertex AI User role

---

## Step 1: Enable Vertex AI API

```bash
# Set your GCP project
export GCP_PROJECT_ID="your-project-id"

# Enable Vertex AI API
gcloud services enable aiplatform.googleapis.com --project=$GCP_PROJECT_ID

# Enable Claude on Vertex AI (if not already enabled)
# This may require contacting Google Cloud support or accessing through console
```

---

## Step 2: Create Service Account

```bash
# Create service account for HolmesGPT API
gcloud iam service-accounts create holmesgpt-api \
  --display-name="HolmesGPT API Service Account" \
  --project=$GCP_PROJECT_ID

# Grant Vertex AI User role
gcloud projects add-iam-policy-binding $GCP_PROJECT_ID \
  --member="serviceAccount:holmesgpt-api@${GCP_PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/aiplatform.user"

# Create and download service account key
gcloud iam service-accounts keys create holmesgpt-vertex-ai-key.json \
  --iam-account=holmesgpt-api@${GCP_PROJECT_ID}.iam.gserviceaccount.com
```

---

## Step 3: Configure Kubernetes Secret

### Option A: Using Service Account JSON (Recommended)

```bash
# Create secret with service account JSON
kubectl create secret generic holmesgpt-api-vertex-ai \
  --from-file=credentials.json=./holmesgpt-vertex-ai-key.json \
  -n kubernaut-system

# Update the main secret with Vertex AI configuration
kubectl create secret generic holmesgpt-api-secret \
  --from-literal=LLM_PROVIDER="vertex-ai" \
  --from-literal=LLM_MODEL="claude-3-5-sonnet@20241022" \
  --from-literal=LLM_ENDPOINT="https://us-central1-aiplatform.googleapis.com" \
  --from-literal=GCP_PROJECT_ID="$GCP_PROJECT_ID" \
  --from-literal=GCP_REGION="us-central1" \
  --from-literal=CONTEXT_API_URL="http://context-api.kubernaut-system.svc.cluster.local:8091" \
  --dry-run=client -o yaml | kubectl apply -f - -n kubernaut-system
```

### Option B: Using API Key (if available)

```bash
# If you have a Vertex AI API key
export VERTEX_AI_API_KEY="your-api-key-here"

kubectl create secret generic holmesgpt-api-secret \
  --from-literal=LLM_PROVIDER="vertex-ai" \
  --from-literal=LLM_API_KEY="$VERTEX_AI_API_KEY" \
  --from-literal=LLM_MODEL="claude-3-5-sonnet@20241022" \
  --from-literal=LLM_ENDPOINT="https://us-central1-aiplatform.googleapis.com" \
  --from-literal=GCP_PROJECT_ID="$GCP_PROJECT_ID" \
  --from-literal=GCP_REGION="us-central1" \
  --from-literal=CONTEXT_API_URL="http://context-api.kubernaut-system.svc.cluster.local:8091" \
  --dry-run=client -o yaml | kubectl apply -f - -n kubernaut-system
```

---

## Step 4: Update Deployment to Mount Credentials

If using service account JSON, update the deployment to mount the credentials:

```yaml
# Add to deployment spec
apiVersion: apps/v1
kind: Deployment
metadata:
  name: holmesgpt-api
spec:
  template:
    spec:
      containers:
      - name: holmesgpt-api
        env:
        - name: GOOGLE_APPLICATION_CREDENTIALS
          value: /var/secrets/google/credentials.json
        - name: LLM_PROVIDER
          valueFrom:
            secretKeyRef:
              name: holmesgpt-api-secret
              key: LLM_PROVIDER
        - name: GCP_PROJECT_ID
          valueFrom:
            secretKeyRef:
              name: holmesgpt-api-secret
              key: GCP_PROJECT_ID
        - name: GCP_REGION
          valueFrom:
            secretKeyRef:
              name: holmesgpt-api-secret
              key: GCP_REGION
        volumeMounts:
        - name: google-cloud-key
          mountPath: /var/secrets/google
          readOnly: true
      volumes:
      - name: google-cloud-key
        secret:
          secretName: holmesgpt-api-vertex-ai
```

---

## Step 5: Restart Deployment

```bash
# Restart pods to pick up new configuration
kubectl rollout restart deployment/holmesgpt-api -n kubernaut-system

# Watch rollout
kubectl rollout status deployment/holmesgpt-api -n kubernaut-system

# Verify pods are running
kubectl get pods -n kubernaut-system -l app.kubernetes.io/name=holmesgpt-api
```

---

## Step 6: Test Configuration

```bash
# Port-forward to API
kubectl port-forward -n kubernaut-system svc/holmesgpt-api 8080:8080 &

# Test health endpoint
curl http://localhost:8080/health | jq .

# Test investigation endpoint (will use Claude 3.5 Sonnet)
curl -X POST http://localhost:8080/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "incident_id": "test-001",
    "namespace": "production",
    "alert_name": "HighCPUUsage",
    "resource_type": "deployment",
    "resource_name": "api-server",
    "investigation_results": {
      "cpu_usage": 95.5
    }
  }' | jq .
```

---

## Claude 3.5 Sonnet Model Versions

Vertex AI provides different versions of Claude 3.5 Sonnet:

| Model ID | Release Date | Description |
|----------|--------------|-------------|
| `claude-3-5-sonnet@20241022` | Oct 2024 | Latest version (recommended) |
| `claude-3-5-sonnet@20240620` | June 2024 | Earlier version |
| `claude-3-5-sonnet-v2@20241022` | Oct 2024 | Alternative naming |

**Recommended**: Use `claude-3-5-sonnet@20241022` for best performance.

---

## Pricing (Vertex AI - US Region)

| Metric | Price |
|--------|-------|
| **Input tokens** | $3.00 / 1M tokens |
| **Output tokens** | $15.00 / 1M tokens |
| **Context caching** | $0.30 / 1M tokens (90% discount) |

### Cost Estimation

**Light Load Test** (10 users, 2 minutes):
- ~50 investigations
- ~500 tokens/request input, ~200 tokens/response
- Cost: ~$0.22

**Medium Load Test** (50 users, 10 minutes):
- ~250 investigations
- Cost: ~$1.10

**Production Usage** (1000 investigations/day):
- ~700k tokens/day
- Cost: ~$15-20/day = ~$450-600/month

---

## Monitoring

After configuration, monitor these metrics:

```promql
# LLM call rate
rate(holmesgpt_llm_calls_total{provider="vertex-ai"}[5m])

# LLM latency
histogram_quantile(0.95, rate(holmesgpt_llm_call_duration_seconds_bucket{provider="vertex-ai"}[5m]))

# Token usage
rate(holmesgpt_llm_token_usage_total{provider="vertex-ai"}[1h])

# Estimated cost (approximate)
(
  rate(holmesgpt_llm_token_usage_total{provider="vertex-ai",type="prompt"}[1h]) * 3 / 1000000 +
  rate(holmesgpt_llm_token_usage_total{provider="vertex-ai",type="completion"}[1h]) * 15 / 1000000
)
```

---

## Troubleshooting

### Error: "Permission Denied"

**Cause**: Service account lacks Vertex AI permissions

**Fix**:
```bash
gcloud projects add-iam-policy-binding $GCP_PROJECT_ID \
  --member="serviceAccount:holmesgpt-api@${GCP_PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/aiplatform.user"
```

### Error: "Model Not Found"

**Cause**: Claude not enabled in your project or wrong region

**Fix**:
1. Check Claude is available in your GCP project
2. Verify region supports Claude (us-central1, us-east5, europe-west1)
3. Check model ID matches available versions

### Error: "Quota Exceeded"

**Cause**: Vertex AI quota limits reached

**Fix**:
1. Check quota in GCP Console: APIs & Services → Vertex AI API → Quotas
2. Request quota increase if needed
3. Implement rate limiting in application

### High Latency

**Cause**: Cold starts or high concurrent requests

**Fix**:
1. Use context caching to reduce token costs and latency
2. Implement request pooling
3. Consider reserved capacity for production

---

## Security Best Practices

1. **Use Service Account JSON** instead of API keys when possible
2. **Rotate credentials** regularly (every 90 days)
3. **Limit permissions** to only Vertex AI User role
4. **Enable audit logging** for API calls
5. **Use VPC Service Controls** for additional security
6. **Monitor costs** via GCP billing alerts

---

## Next Steps

1. ✅ Configure Vertex AI credentials
2. ✅ Update Kubernetes secrets
3. ✅ Restart deployment
4. ✅ Test with real investigation
5. ✅ Run load tests with budget limits
6. ✅ Set up cost alerts in GCP
7. ✅ Monitor performance via Grafana

---

## Quick Setup Script

```bash
#!/bin/bash
# quick-vertex-ai-setup.sh

set -e

# Configuration
export GCP_PROJECT_ID="${1:-your-project-id}"
export GCP_REGION="${2:-us-central1}"
export SA_NAME="holmesgpt-api"

echo "Setting up Vertex AI for HolmesGPT API..."
echo "Project: $GCP_PROJECT_ID"
echo "Region: $GCP_REGION"

# Enable API
echo "Enabling Vertex AI API..."
gcloud services enable aiplatform.googleapis.com --project=$GCP_PROJECT_ID

# Create service account
echo "Creating service account..."
gcloud iam service-accounts create $SA_NAME \
  --display-name="HolmesGPT API Service Account" \
  --project=$GCP_PROJECT_ID || true

# Grant permissions
echo "Granting Vertex AI User role..."
gcloud projects add-iam-policy-binding $GCP_PROJECT_ID \
  --member="serviceAccount:${SA_NAME}@${GCP_PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/aiplatform.user"

# Create key
echo "Creating service account key..."
gcloud iam service-accounts keys create holmesgpt-vertex-ai-key.json \
  --iam-account=${SA_NAME}@${GCP_PROJECT_ID}.iam.gserviceaccount.com

# Create Kubernetes secret
echo "Creating Kubernetes secret..."
kubectl create secret generic holmesgpt-api-vertex-ai \
  --from-file=credentials.json=./holmesgpt-vertex-ai-key.json \
  -n kubernaut-system \
  --dry-run=client -o yaml | kubectl apply -f -

# Update main secret
echo "Updating main secret..."
kubectl create secret generic holmesgpt-api-secret \
  --from-literal=LLM_PROVIDER="vertex-ai" \
  --from-literal=LLM_MODEL="claude-3-5-sonnet@20241022" \
  --from-literal=LLM_ENDPOINT="https://${GCP_REGION}-aiplatform.googleapis.com" \
  --from-literal=GCP_PROJECT_ID="$GCP_PROJECT_ID" \
  --from-literal=GCP_REGION="$GCP_REGION" \
  --from-literal=CONTEXT_API_URL="http://context-api.kubernaut-system.svc.cluster.local:8091" \
  -n kubernaut-system \
  --dry-run=client -o yaml | kubectl apply -f -

echo "✅ Vertex AI setup complete!"
echo ""
echo "Next steps:"
echo "1. Update deployment to mount credentials (see VERTEX_AI_SETUP.md)"
echo "2. Restart deployment: kubectl rollout restart deployment/holmesgpt-api -n kubernaut-system"
echo "3. Test configuration"

# Clean up local key file
echo ""
read -p "Delete local key file? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    rm holmesgpt-vertex-ai-key.json
    echo "✅ Local key file deleted"
fi
```

**Usage**:
```bash
chmod +x quick-vertex-ai-setup.sh
./quick-vertex-ai-setup.sh your-gcp-project-id us-central1
```


