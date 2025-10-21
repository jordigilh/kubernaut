# HolmesGPT API - Load Testing

This directory contains load testing scripts for validating HolmesGPT API performance under load.

## Prerequisites

```bash
# Install Locust
pip install locust
```

## Test Scenarios

### 1. Light Load (10 users)
**Use Case**: Baseline performance validation

```bash
# Port-forward to HolmesGPT API
oc port-forward -n kubernaut-system svc/holmesgpt-api 8080:80

# Run light load test
locust -f locustfile.py --host=http://localhost:8080 \
       --users 10 --spawn-rate 2 --run-time 5m --headless
```

**Expected Results**:
- Request rate: 5-10 req/sec
- P95 latency: <500ms
- Error rate: <1%

---

### 2. Medium Load (50 users)
**Use Case**: Normal production load

```bash
locust -f locustfile.py --host=http://localhost:8080 \
       --users 50 --spawn-rate 5 --run-time 10m --headless
```

**Expected Results**:
- Request rate: 25-50 req/sec
- P95 latency: <1s
- Error rate: <2%

---

### 3. Heavy Load (200 users)
**Use Case**: Stress testing and capacity planning

```bash
locust -f locustfile.py --host=http://localhost:8080 \
       --users 200 --spawn-rate 10 --run-time 15m --headless
```

**Expected Results**:
- Request rate: 100-200 req/sec
- P95 latency: <2s
- Error rate: <5%

---

## Interactive Mode

For manual control and real-time monitoring:

```bash
# Start Locust web UI
locust -f locustfile.py --host=http://localhost:8080

# Open browser to http://localhost:8089
# Configure users and spawn rate in web UI
```

---

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Load Testing

on:
  push:
    branches: [main]

jobs:
  load-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Install dependencies
        run: pip install locust
      
      - name: Port-forward to OCP
        run: |
          oc login ${{ secrets.OCP_API_URL }} --token=${{ secrets.OCP_TOKEN }}
          kubectl port-forward -n kubernaut-system svc/holmesgpt-api 8080:80 &
          sleep 5
      
      - name: Run load test
        run: |
          cd holmesgpt-api/tests/load
          locust -f locustfile.py --host=http://localhost:8080 \
                 --users 50 --spawn-rate 5 --run-time 5m --headless \
                 --csv=results/load-test
      
      - name: Upload results
        uses: actions/upload-artifact@v3
        with:
          name: load-test-results
          path: holmesgpt-api/tests/load/results/
```

---

## Metrics to Monitor

During load testing, monitor these Prometheus metrics:

### Investigation Performance
- `holmesgpt_investigations_total`: Total investigations
- `holmesgpt_investigations_duration_seconds`: Duration histogram
- `holmesgpt_active_requests`: Concurrent requests

### LLM Integration
- `holmesgpt_llm_calls_total`: LLM API calls
- `holmesgpt_llm_call_duration_seconds`: LLM call latency
- `holmesgpt_llm_token_usage_total`: Token consumption

### Error Rates
- `holmesgpt_http_requests_total{status=~"5.."}`: Server errors
- `holmesgpt_auth_failures_total`: Auth failures

### Resource Usage
- CPU and memory usage (from Kubernetes metrics)
- Network I/O
- Database connection pool

---

## Grafana Dashboard

Import the Grafana dashboard for real-time visualization:

```bash
# Apply Grafana dashboard ConfigMap
oc apply -f ../../deploy/holmesgpt-api/11-grafana-dashboard-configmap.yaml
```

Dashboard includes:
- Request rate over time
- Latency percentiles (p50, p95, p99)
- Error rate trends
- LLM cost estimation
- Active requests gauge

---

## Cost Considerations

### Mock LLM (No Cost)
**Recommended for infrastructure load testing**

```bash
# Ensure DEV_MODE=true or no LLM credentials configured
# API will use stub responses
```

### Real LLM (With Cost)
**Only for integration validation**

```bash
# Set LLM credentials in secret
# Monitor holmesgpt_llm_token_usage_total for cost tracking

# Estimated costs (example with Claude 3.5 Sonnet):
# - Light load (10 users, 5min): ~$2-5
# - Medium load (50 users, 10min): ~$10-20
# - Heavy load (200 users, 15min): ~$50-100
```

**Budget Control**:
```bash
# Set a token budget limit
# Stop test if budget exceeded
locust -f locustfile.py --host=http://localhost:8080 \
       --users 50 --spawn-rate 5 --run-time 10m \
       --stop-timeout 60
```

---

## Troubleshooting

### High Error Rate

**Check Prometheus metrics**:
```bash
# Check error breakdown
oc exec -n kubernaut-system deployment/holmesgpt-api -- \
  curl -s http://localhost:8080/metrics | grep holmesgpt_http_requests_total
```

**Check logs**:
```bash
oc logs -n kubernaut-system -l app.kubernetes.io/name=holmesgpt-api --tail=100
```

### High Latency

**Identify bottleneck**:
1. Check LLM call duration: `holmesgpt_llm_call_duration_seconds`
2. Check Context API duration: `holmesgpt_context_api_duration_seconds`
3. Check CPU/memory usage: `kubectl top pods -n kubernaut-system`

**Scale up if needed**:
```bash
oc scale deployment/holmesgpt-api --replicas=4 -n kubernaut-system
```

### Connection Refused

**Verify port-forward**:
```bash
# Check port-forward is running
ps aux | grep port-forward

# Restart if needed
oc port-forward -n kubernaut-system svc/holmesgpt-api 8080:80
```

---

## Results Analysis

### Locust Output

Locust generates CSV files with detailed metrics:

- `results/load-test_stats.csv`: Request statistics
- `results/load-test_stats_history.csv`: Time-series data
- `results/load-test_failures.csv`: Error details

### Example Analysis

```python
import pandas as pd

# Load results
stats = pd.read_csv('results/load-test_stats.csv')

# Calculate key metrics
print(f"Total requests: {stats['Request Count'].sum()}")
print(f"Average response time: {stats['Average Response Time'].mean():.2f}ms")
print(f"Failure rate: {stats['Failure Count'].sum() / stats['Request Count'].sum() * 100:.2f}%")

# P95 latency
print(f"P95 latency: {stats['95%'].mean():.2f}ms")
```

---

## Best Practices

1. **Start Small**: Begin with light load and gradually increase
2. **Monitor Costs**: Use mock LLM for infrastructure testing
3. **Watch Metrics**: Monitor Prometheus metrics in real-time
4. **Scale Gradually**: Don't overwhelm the system
5. **Document Results**: Save metrics for comparison
6. **Test Regularly**: Include in CI/CD for regression detection

---

## Business Requirements

- **BR-HAPI-104**: Performance validation and capacity planning
- **BR-HAPI-105**: Load testing framework for production readiness
- **BR-HAPI-106**: Cost-aware testing strategy

