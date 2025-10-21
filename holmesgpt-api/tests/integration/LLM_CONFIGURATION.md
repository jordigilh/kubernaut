# LLM Configuration for Integration Tests

This guide explains how to configure LLM providers for HolmesGPT API integration tests.

## Test Modes

### Mode 1: Mock LLM (Default)
**Use Case**: Local development, CI/CD without LLM costs

```bash
# No configuration needed - tests run with stubs
cd holmesgpt-api
pytest tests/integration/ -v
```

**Behavior**: Tests use stub responses, fast execution, no LLM API calls

---

### Mode 2: Real LLM Integration
**Use Case**: Validate actual LLM behavior before production deployment

**Enable Real LLM Tests**:
```bash
# Set environment variable
export RUN_REAL_LLM=true

# Or use pytest flag
pytest tests/integration/test_real_llm_integration.py --run-real-llm -v
```

---

## LLM Provider Configuration

### Option A: Vertex AI (Claude via Google Cloud)

**Prerequisites**:
- Google Cloud project with Vertex AI API enabled
- Service account with Vertex AI User role
- Claude 3.5 Sonnet or Claude 3 Opus access

**Configuration**:
```bash
# 1. Authenticate with Google Cloud
gcloud auth application-default login

# 2. Set environment variables
export LLM_MODEL="vertex_ai/claude-3-5-sonnet@20240620"
export VERTEXAI_PROJECT="your-gcp-project-id"
export VERTEXAI_LOCATION="us-central1"

# 3. Run tests
export RUN_REAL_LLM=true
cd holmesgpt-api
pytest tests/integration/test_real_llm_integration.py --run-real-llm -v
```

**Cost**: ~$0.10-0.50 per test run (13 tests × 2-4 LLM calls each)

---

### Option B: Anthropic Direct (Claude)

**Prerequisites**:
- Anthropic API key
- Claude 3.5 Sonnet or Claude 3 Opus access

**Configuration**:
```bash
# 1. Set API key
export ANTHROPIC_API_KEY="sk-ant-..."

# 2. Set model (litellm format)
export LLM_MODEL="anthropic/claude-3-5-sonnet-20240620"

# 3. Run tests
export RUN_REAL_LLM=true
cd holmesgpt-api
pytest tests/integration/test_real_llm_integration.py --run-real-llm -v
```

**Cost**: ~$0.10-0.50 per test run

---

### Option C: OpenAI (GPT-4)

**Prerequisites**:
- OpenAI API key
- GPT-4 Turbo or GPT-4 access

**Configuration**:
```bash
# 1. Set API key
export OPENAI_API_KEY="sk-..."

# 2. Set model (litellm format)
export LLM_MODEL="openai/gpt-4-turbo-preview"

# 3. Run tests
export RUN_REAL_LLM=true
cd holmesgpt-api
pytest tests/integration/test_real_llm_integration.py --run-real-llm -v
```

**Cost**: ~$0.20-0.80 per test run (GPT-4 is more expensive)

---

### Option D: Local LLM (Ollama)

**Prerequisites**:
- Ollama installed locally or on accessible server
- Model with tool calling support (e.g., llama3.1:8b, mistral-large)

**Configuration**:
```bash
# 1. Start Ollama server
ollama serve

# 2. Pull model with tool calling support
ollama pull llama3.1:8b

# 3. Set environment variables
export LLM_MODEL="ollama/llama3.1:8b"
export LLM_ENDPOINT="http://localhost:11434"

# 4. Run tests (will be slower)
export RUN_REAL_LLM=true
cd holmesgpt-api
pytest tests/integration/test_real_llm_integration.py --run-real-llm -v -s
```

**Cost**: Free (local compute)
**Speed**: 5-10 minutes per test run (depends on hardware)

---

## Test Coverage

### Real LLM Integration Tests

**File**: `tests/integration/test_real_llm_integration.py`

| Test Suite | Tests | Purpose |
|------------|-------|---------|
| **TestRealRecoveryAnalysis** | 6 | Recovery strategy generation |
| **TestRealPostExecAnalysis** | 4 | Post-execution effectiveness |
| **TestRealLLMErrorHandling** | 2 | Error handling validation |
| **TestRealLLMPerformance** | 1 | Performance benchmarking |
| **Total** | **13** | **Complete LLM behavior validation** |

---

## Running Tests Selectively

### Run All Real LLM Tests
```bash
export RUN_REAL_LLM=true
export LLM_MODEL="vertex_ai/claude-3-5-sonnet@20240620"
cd holmesgpt-api
pytest tests/integration/test_real_llm_integration.py --run-real-llm -v
```

### Run Specific Test Class
```bash
# Only recovery analysis tests
pytest tests/integration/test_real_llm_integration.py::TestRealRecoveryAnalysis --run-real-llm -v

# Only post-exec analysis tests
pytest tests/integration/test_real_llm_integration.py::TestRealPostExecAnalysis --run-real-llm -v
```

### Run Single Test
```bash
pytest tests/integration/test_real_llm_integration.py::TestRealRecoveryAnalysis::test_recovery_analysis_with_real_llm --run-real-llm -v
```

---

## CI/CD Integration

### GitHub Actions Example
```yaml
name: Real LLM Integration Tests

on:
  schedule:
    - cron: '0 2 * * 1'  # Weekly on Mondays at 2 AM
  workflow_dispatch:  # Manual trigger

jobs:
  integration-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Python
        uses: actions/setup-python@v4
        with:
          python-version: '3.12'
      
      - name: Install dependencies
        run: |
          cd holmesgpt-api
          pip install -r requirements.txt
          pip install -r requirements-test.txt
      
      - name: Run Real LLM Tests
        env:
          RUN_REAL_LLM: "true"
          LLM_MODEL: "vertex_ai/claude-3-5-sonnet@20240620"
          VERTEXAI_PROJECT: ${{ secrets.VERTEXAI_PROJECT }}
          VERTEXAI_LOCATION: "us-central1"
          GOOGLE_APPLICATION_CREDENTIALS: ${{ secrets.GCP_SA_KEY }}
        run: |
          cd holmesgpt-api
          pytest tests/integration/test_real_llm_integration.py --run-real-llm -v --tb=short
```

### OpenShift Pipeline Example
```yaml
apiVersion: tekton.dev/v1beta1
kind: Pipeline
metadata:
  name: holmesgpt-api-integration
spec:
  tasks:
    - name: real-llm-tests
      taskSpec:
        steps:
          - name: test
            image: python:3.12
            env:
              - name: RUN_REAL_LLM
                value: "true"
              - name: LLM_MODEL
                valueFrom:
                  secretKeyRef:
                    name: llm-credentials
                    key: model
              - name: VERTEXAI_PROJECT
                valueFrom:
                  secretKeyRef:
                    name: llm-credentials
                    key: project
            script: |
              cd holmesgpt-api
              pip install -r requirements.txt -r requirements-test.txt
              pytest tests/integration/test_real_llm_integration.py --run-real-llm -v
```

---

## Troubleshooting

### Tests Skipped

**Problem**: Tests show as "skipped"

**Solution**:
```bash
# Verify RUN_REAL_LLM is set
echo $RUN_REAL_LLM
# Should output: true

# Verify LLM_MODEL is set
echo $LLM_MODEL
# Should output: vertex_ai/claude-3-5-sonnet@20240620 (or your model)

# Run with verbose output
pytest tests/integration/test_real_llm_integration.py --run-real-llm -v -s
```

---

### Authentication Errors

**Vertex AI**:
```bash
# Verify authentication
gcloud auth application-default login
gcloud auth application-default print-access-token
```

**Anthropic**:
```bash
# Verify API key is set
echo $ANTHROPIC_API_KEY | head -c 10
# Should output: sk-ant-...
```

**OpenAI**:
```bash
# Verify API key is set
echo $OPENAI_API_KEY | head -c 10
# Should output: sk-...
```

---

### Timeout Errors

**Problem**: Tests timeout (90s limit)

**Solutions**:
1. **Use faster LLM provider** (Claude 3 Haiku instead of GPT-4)
2. **Increase timeout** in test:
   ```python
   # In test_real_llm_integration.py
   assert duration < 120.0  # Increase from 90s
   ```
3. **Use local LLM** (no network latency)

---

### Cost Management

**Best Practices**:
1. **Run selectively** - Use specific test classes for targeted validation
2. **Use cheaper models** - Claude 3 Haiku or GPT-3.5 Turbo for development
3. **Mock for CI** - Only run real LLM tests on release candidates
4. **Budget alerts** - Set up cloud provider billing alerts

**Estimated Costs** (per full test run):
- **Claude 3.5 Sonnet**: ~$0.30
- **Claude 3 Haiku**: ~$0.05
- **GPT-4 Turbo**: ~$0.60
- **GPT-3.5 Turbo**: ~$0.02
- **Local Ollama**: $0.00

---

## Test Expectations

### Performance
- **Cloud LLM** (Vertex AI, Anthropic, OpenAI): 30-60 seconds per test
- **Local LLM** (Ollama): 2-10 minutes per test (hardware-dependent)
- **Stub Mode**: <1 second per test

### Success Criteria
- **Confidence**: ≥0.7 for recovery strategies
- **Strategies**: ≥1 recovery strategy per scenario
- **Rationale**: >20 characters (substantive analysis)
- **Risk Assessment**: Present in strategy metadata

### Quality Indicators (Logged, Not Required)
- Specific metric mentions (CPU, memory, latency)
- Multi-phase recovery strategies
- Root cause understanding
- Quantitative analysis

---

## Next Steps

After configuring LLM integration:

1. **Run Unit Tests** (baseline): `pytest tests/unit/ -v`
2. **Run Mock Integration Tests**: `pytest tests/integration/ -v`
3. **Run Real LLM Tests** (this guide): `pytest tests/integration/test_real_llm_integration.py --run-real-llm -v`
4. **Deploy to Development**: `oc apply -k deploy/holmesgpt-api/`
5. **Run E2E Tests**: Test against deployed service in cluster

---

## Related Documentation

- **Test Strategy**: `holmesgpt-api/tests/integration/README.md`
- **Deployment Guide**: `deploy/holmesgpt-api/README.md`
- **Context API Integration**: `holmesgpt-api/tests/integration/README.md#test-modes`

