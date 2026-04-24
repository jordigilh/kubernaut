# Running HolmesGPT API with Real LLM

## 🔒 Security First

**IMPORTANT**: The `run-with-llm.sh` script contains your LLM credentials and is automatically excluded from git. Never commit this file!

## 🚀 Quick Start

### Step 1: Create Your Local Configuration

```bash
cd holmesgpt-api
cp run-with-llm.sh.example run-with-llm.sh
chmod +x run-with-llm.sh
```

### Step 2: Edit Configuration

Open `run-with-llm.sh` and uncomment ONE of the provider options:

#### Option A: Cloud Provider with Project ID
```bash
export LLM_PROVIDER="your-provider"           # e.g., "provider-name"
export LLM_MODEL="your-model"                 # Provider-specific model
export LLM_PROJECT_ID="your-project-id"       # Your cloud project ID
export LLM_REGION="your-region"               # e.g., "region-name"
CREDS_MOUNT="-v ~/.config/your-provider/credentials.json:/tmp/creds.json:ro -e CREDENTIALS_FILE=/tmp/creds.json"
```

#### Option B: API Key-based Provider
```bash
export LLM_PROVIDER="your-provider"           # e.g., "provider-name"
export LLM_MODEL="your-model"                 # Provider-specific model
export LLM_API_KEY="your-api-key"             # API key
export LLM_ENDPOINT="https://api.provider.com/v1/endpoint"
CREDS_MOUNT=""
```

### Step 3: Run the Service

```bash
./run-with-llm.sh
```

The service will start on `http://localhost:8080`

## 🧪 Test the Real LLM Integration

### Health Check
```bash
curl http://localhost:8080/health
```

### Recovery Analysis (Real LLM)
```bash
curl -X POST http://localhost:8080/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "incident_id": "test-001",
    "failed_action": {
      "type": "scale_deployment",
      "target": "api-server",
      "desired_replicas": 10,
      "namespace": "production"
    },
    "failure_context": {
      "error": "OOMKilled",
      "memory_usage": "95%",
      "cpu_usage": "87%",
      "pod_restart_count": 15,
      "time_since_incident": "10m"
    },
    "investigation_result": {
      "root_cause": "memory_leak_in_cache",
      "affected_pods": ["api-server-a", "api-server-b"],
      "symptoms": ["high_memory", "slow_response_times"]
    },
    "context": {
      "namespace": "production",
      "cluster": "prod-cluster-1",
      "service_owner": "sre-team",
      "priority": "P0"
    },
    "constraints": {
      "max_attempts": 3,
      "timeout": "5m",
      "allowed_actions": ["rollback_deployment", "scale_down_deployment"]
    }
  }'
```

### Post-Execution Analysis (Real LLM)
```bash
curl -X POST http://localhost:8080/api/v1/postexec/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "execution_id": "test-exec-001",
    "action_id": "scale-up-001",
    "action_type": "scale_deployment",
    "action_details": {
      "deployment": "api-server",
      "replicas": 10,
      "namespace": "production"
    },
    "execution_success": true,
    "execution_result": {
      "status": "completed",
      "duration_ms": 5000,
      "message": "Deployment scaled successfully"
    },
    "pre_execution_state": {
      "cpu_usage": "95%",
      "memory_usage": "90%",
      "pod_count": 5
    },
    "post_execution_state": {
      "cpu_usage": "40%",
      "memory_usage": "35%",
      "pod_count": 10
    },
    "context": {
      "namespace": "production",
      "cluster": "prod-cluster-1",
      "service_owner": "sre-team",
      "priority": "P0"
    },
    "objectives": [
      "reduce_cpu_usage_below_50%",
      "increase_pod_count_to_10"
    ]
  }'
```

## 🔧 Troubleshooting

### Container Won't Start
- Verify credentials are valid
- Check `podman ps -a` for error logs
- Ensure port 8080 is available

### Authentication Errors
- For cloud providers: Verify credentials file path
- For API keys: Check environment variable is set
- Test credentials with provider CLI first

### LLM Timeout
- Check network connectivity
- Verify endpoint URL is correct
- Increase timeout in requests if needed

## 📊 Environment Variables Reference

| Variable | Required | Purpose | Example |
|----------|----------|---------|---------|
| `LLM_PROVIDER` | ✅ Yes | Provider name | `"provider-a"`, `"provider-b"` |
| `LLM_MODEL` | ✅ Yes | Model identifier | `"model-v4"`, `"model-opus"` |
| `LLM_PROJECT_ID` | ⚠️ Cloud only | Project/account ID | `"my-project-123"` |
| `LLM_REGION` | ⚠️ Cloud only | Cloud region | `"us-east1"` |
| `LLM_API_KEY` | ⚠️ API key only | Authentication key | `"sk-..."` |
| `LLM_ENDPOINT` | ⚠️ Optional | Custom endpoint | `"https://api.example.com"` |
| `DEV_MODE` | ⚠️ Optional | Enable dev features | `"true"` (default) |
| `AUTH_ENABLED` | ⚠️ Optional | Enable authentication | `"false"` (default) |

## 🔒 Security Checklist

- [x] `run-with-llm.sh` excluded from git (automatic)
- [x] No credentials in code or committed files
- [x] Environment variables for all secrets
- [x] Local-only configuration files
- [ ] Rotate credentials regularly
- [ ] Use read-only credentials when possible
- [ ] Never share `run-with-llm.sh` with others

## 📚 Next Steps

1. ✅ Service running locally with real LLM
2. 🧪 Run integration tests: `pytest tests/integration/test_real_llm_integration.py --run-real-llm`
3. 🐳 Deploy to Kubernetes: `kubectl apply -f deployment.yaml`
4. 📊 Configure monitoring and logging

