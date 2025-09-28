# Integration Test Execution Guide

**Document Version**: 1.0
**Date**: January 2025
**Testing Strategy**: Automated integration tests with Go/Ginkgo + Manual validation
**Execution**: Automated test suites with optional manual exploration

---

## 🎯 **Hybrid Performance Testing Overview**

Our hybrid approach combines the best of both testing strategies:

### **Phase A: Business Requirement Validation** ✅
- **Purpose**: Validate system meets documented business requirements
- **Method**: Automated integration tests with Go/Ginkgo
- **Duration**: 15-30 minutes
- **Outcome**: Pass/fail compliance with business requirements

### **Phase B: Capacity Exploration** 📊
- **Purpose**: Understand system characteristics and operational limits
- **Method**: Progressive load testing to find breaking points
- **Duration**: Optional manual testing (4-5 hours)
- **Outcome**: Performance curves, capacity recommendations, scaling insights

---

## 🚀 **Pre-Execution Setup**

### **Step 1: Environment Readiness Verification**

```bash
# Navigate to kubernaut project
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Verify current service connectivity
echo "=== Checking Integration Environment Status ==="

# Check LLM service (required)
echo "Checking LLM service at 192.168.1.169:8080..."
curl -s http://192.168.1.169:8080/health 2>/dev/null && echo "✅ LLM service accessible" || echo "❌ LLM service not accessible"

# Check Redis (required with authentication)
echo "Checking Redis at localhost:6379..."
redis-cli -h localhost -p 6379 -a integration_redis_password ping 2>/dev/null && echo "✅ Redis accessible" || echo "❌ Redis not accessible"

# Check PostgreSQL (available via docker-compose)
echo "Checking PostgreSQL..."
podman ps | grep postgres && echo "✅ PostgreSQL container running" || echo "❌ PostgreSQL container not running"

# Check Kind cluster (optional for Phase 1)
echo "Checking Kind cluster (optional)..."
kubectl cluster-info 2>/dev/null && echo "✅ Kind cluster available" || echo "ℹ️  Kind cluster not available (using envtest for Phase 1)"

echo "=== Environment Check Complete ==="
```

### **Step 1a: Automated Phase 1 Integration Tests (Recommended)**

```bash
# Run the automated Phase 1 integration test suites
echo "=== Running Phase 1 Automated Integration Tests ==="

# Set environment variables for integration tests
export LLM_ENDPOINT="http://192.168.1.169:8080"
export LLM_MODEL="granite3.1-dense:8b"  # or your preferred model
export LLM_PROVIDER="ollama"  # or "ramalama"
export SKIP_SLOW_TESTS=true
export TEST_TIMEOUT=300s

# Run Alert Processing Performance tests
echo "Running Alert Processing Performance tests..."
go test -v -tags=integration ./test/integration/alert_processing/... -timeout=300s

# Run Kubernetes Operations Safety tests
echo "Running Kubernetes Operations Safety tests..."
go test -v -tags=integration ./test/integration/kubernetes_operations/... -timeout=300s

# Run Platform Operations Concurrent Execution tests
echo "Running Platform Operations tests..."
go test -v -tags=integration ./test/integration/platform_operations/... -timeout=300s

echo "=== Phase 1 Automated Tests Complete ==="
echo "Review test output above for PASS/FAIL status on each business requirement."
```

### **Step 2: Deploy Kubernaut Applications**

```bash
# Start Kubernaut applications (pointing to Kind cluster)
# Replace with actual deployment commands based on your applications

# Example deployment commands (adjust as needed):
export KUBECONFIG=~/.kube/config
export K8S_CLUSTER_ENDPOINT="https://127.0.0.1:6443"  # Kind cluster endpoint
export POSTGRES_HOST="localhost"
export POSTGRES_PORT="30432"  # PostgreSQL NodePort
export LLM_ENDPOINT="http://192.168.1.169:8080"
export REDIS_PASSWORD="integration_redis_password"

# Deploy kubernaut
cd /path/to/kubernaut/cmd/kubernaut
go build -o kubernaut main.go
./kubernaut --config-file=config.yaml &

# Deploy context-api-server
cd /path/to/kubernaut/cmd/context-api-server
go build -o context-api-server main.go
./context-api-server --port=8081 &

# Deploy kubernaut
cd /path/to/kubernaut/cmd/kubernaut
go build -o kubernaut main.go
./kubernaut --port=8082 &

# Return to test directory
cd ~/kubernaut-integration-test

# Verify services are accessible
curl -s http://192.168.1.169:8080/health || echo "LLM service not ready"
redis-cli -h localhost -p 6379 -a integration_redis_password ping || echo "Redis not ready"

# Check if Kind cluster is available (optional for Phase 1)
kubectl cluster-info 2>/dev/null || echo "Kind cluster not available (using envtest for Phase 1)"
```

### **Step 3: Initialize Test Environment**

```bash
# Create test session directory
export TEST_SESSION="test_session_$(date +%Y%m%d_%H%M%S)"
mkdir -p "results/$TEST_SESSION"

# Initialize monitoring
echo "Starting test session: $TEST_SESSION" > "results/$TEST_SESSION/test_log.txt"
date >> "results/$TEST_SESSION/test_log.txt"

# Set webhook URLs for testing
export PROMETHEUS_WEBHOOK="http://localhost:8080/webhook/prometheus"
export CONTEXT_API_ENDPOINT="http://localhost:8081/api/v1/context"
export TOOLSET_API_ENDPOINT="http://localhost:8082/api/v1/toolset"
```

---

## 🔬 **Phase A: Business Requirement Validation**

**Objective**: Validate exact compliance with documented business requirements
**Duration**: 3-4 hours
**Critical**: All tests must pass for Milestone 1 completion

### **Test A1: Alert Processing Performance (BR-PA-003, BR-PA-004)**

#### **A1.1: Response Time Validation (BR-PA-003)**
```bash
echo "=== Test A1.1: Response Time Validation ==="
echo "Business Requirement: BR-PA-003 - Process alerts within 5 seconds"
echo "Test Duration: 10 minutes"
echo "Target: 95th percentile < 5 seconds"

# Run response time test
python3 scripts/generate_synthetic_alerts.py \
    --webhook-url "$PROMETHEUS_WEBHOOK" \
    --rate 60 \
    --duration 10 \
    --types high_memory high_cpu pod_crash

# Analyze results
python3 << 'EOF'
import json
import glob
import statistics

# Load latest results
result_files = glob.glob("results/alert_generation_*.json")
latest_result = max(result_files, key=lambda x: os.path.getctime(x))

with open(latest_result, 'r') as f:
    data = json.load(f)

avg_response_time = data.get('avg_response_time', 0)
success_rate = data.get('success_rate', 0)

print(f"\n--- BR-PA-003 Validation Results ---")
print(f"Average Response Time: {avg_response_time:.3f}s")
print(f"Success Rate: {success_rate:.2f}%")

# Business requirement validation
if avg_response_time < 5.0 and success_rate >= 95.0:
    print("✅ BR-PA-003: PASS - Response time requirement met")
    exit(0)
else:
    print("❌ BR-PA-003: FAIL - Response time requirement not met")
    exit(1)
EOF

echo "Test A1.1 completed. Results saved to results/$TEST_SESSION/"
```

#### **A1.2: Concurrent Load Validation (BR-PA-004)**
```bash
echo "=== Test A1.2: Concurrent Load Validation ==="
echo "Business Requirement: BR-PA-004 - Support 100 concurrent requests"
echo "Test Duration: 5 minutes"
echo "Target: 100 simultaneous requests processed successfully"

# Run concurrent load test
./scripts/concurrent_load_test.sh "$PROMETHEUS_WEBHOOK" 100 300

# Results are automatically analyzed by the script
# Look for "BR-PA-004: Concurrent request handling - PASS/FAIL"
echo "Test A1.2 completed. Check output above for PASS/FAIL status."
```

### **Test A2: Availability Validation (BR-PA-001)**

```bash
echo "=== Test A2: Availability Validation ==="
echo "Business Requirement: BR-PA-001 - 99.9% availability"
echo "Test Duration: 30 minutes"
echo "Target: Max 18 seconds downtime in 30 minutes"

# Create availability monitoring script
cat > scripts/availability_monitor.sh << 'EOF'
#!/bin/bash
WEBHOOK_URL="$1"
DURATION_MINUTES="$2"
TEST_SESSION="$3"

end_time=$(($(date +%s) + ($DURATION_MINUTES * 60)))
total_checks=0
successful_checks=0
downtime_seconds=0
last_check_failed=false

echo "Starting availability monitoring for $DURATION_MINUTES minutes"
echo "Target: 99.9% availability (max $((DURATION_MINUTES * 60 / 1000)) seconds downtime)"

while [ $(date +%s) -lt $end_time ]; do
    start_check=$(date +%s)

    # Simple health check (lightweight alert)
    response=$(curl -s -w "%{http_code}" -o /dev/null \
                   -X POST \
                   -H "Content-Type: application/json" \
                   -d '{"alerts":[{"status":"firing","labels":{"alertname":"HealthCheck"}}]}' \
                   "$WEBHOOK_URL" \
                   --max-time 10 2>/dev/null || echo "000")

    total_checks=$((total_checks + 1))

    if [ "$response" = "200" ]; then
        successful_checks=$((successful_checks + 1))
        if [ "$last_check_failed" = true ]; then
            echo "Service recovered at $(date)"
            last_check_failed=false
        fi
    else
        if [ "$last_check_failed" = false ]; then
            echo "Service failure detected at $(date) (HTTP: $response)"
            last_check_failed=true
        fi
        downtime_seconds=$((downtime_seconds + 10))
    fi

    # Progress report every 100 checks
    if [ $((total_checks % 100)) -eq 0 ]; then
        availability=$(echo "scale=3; $successful_checks * 100 / $total_checks" | bc)
        echo "Progress: ${total_checks} checks, ${availability}% availability, ${downtime_seconds}s downtime"
    fi

    sleep 10
done

# Final calculation
availability=$(echo "scale=3; $successful_checks * 100 / $total_checks" | bc)
max_allowed_downtime=$((DURATION_MINUTES * 60 / 1000))

echo "--- BR-PA-001 Availability Test Results ---"
echo "Total Checks: $total_checks"
echo "Successful Checks: $successful_checks"
echo "Availability: ${availability}%"
echo "Downtime: ${downtime_seconds} seconds"
echo "Max Allowed Downtime: ${max_allowed_downtime} seconds"

if (( $(echo "$availability >= 99.9" | bc -l) )); then
    echo "✅ BR-PA-001: PASS - Availability requirement met"
    echo "$availability,PASS" > "results/$TEST_SESSION/availability_result.csv"
else
    echo "❌ BR-PA-001: FAIL - Availability requirement not met"
    echo "$availability,FAIL" > "results/$TEST_SESSION/availability_result.csv"
fi
EOF

chmod +x scripts/availability_monitor.sh

# Run availability test
./scripts/availability_monitor.sh "$PROMETHEUS_WEBHOOK" 30 "$TEST_SESSION"

echo "Test A2 completed."
```

### **Test A3: Kubernetes Actions Validation (BR-PA-011)**

```bash
echo "=== Test A3: Kubernetes Actions Validation ==="
echo "Business Requirement: BR-PA-011 - 25+ K8s actions with 95% success rate"
echo "Test Duration: 15 minutes"

# Create K8s action test script
cat > scripts/k8s_actions_test.sh << 'EOF'
#!/bin/bash
WEBHOOK_URL="$1"
TEST_SESSION="$2"

# K8s actions to test (based on business requirement BR-PA-011)
actions_to_test=(
    "restart_pod"
    "scale_deployment"
    "update_resource_limits"
    "drain_node"
    "cordon_node"
    "uncordon_node"
    "delete_failed_pods"
    "rollback_deployment"
    "patch_service"
    "update_configmap"
    "create_network_policy"
    "update_ingress"
    "scale_horizontal_pod_autoscaler"
    "update_persistent_volume_claim"
    "patch_node_labels"
    "update_service_account"
    "create_role_binding"
    "update_deployment_strategy"
    "patch_stateful_set"
    "update_daemon_set"
    "create_pod_disruption_budget"
    "update_resource_quota"
    "patch_namespace"
    "update_endpoint"
    "create_secret"
)

echo "Testing ${#actions_to_test[@]} Kubernetes actions"
echo "Target: 95% success rate (min $((${#actions_to_test[@]} * 95 / 100)) successful)"

total_actions=0
successful_actions=0

for action in "${actions_to_test[@]}"; do
    echo "Testing action: $action"

    # Generate alert that should trigger this specific action
    alert_payload=$(cat <<JSON
{
    "alerts": [{
        "status": "firing",
        "labels": {
            "alertname": "ActionTest_${action}",
            "severity": "warning",
            "namespace": "test-workloads",
            "action_required": "$action"
        },
        "annotations": {
            "description": "Test alert for $action validation",
            "recommended_action": "$action"
        }
    }]
}
JSON
)

    # Send alert and measure response
    response=$(curl -s -w "%{http_code}" \
                   -X POST \
                   -H "Content-Type: application/json" \
                   -d "$alert_payload" \
                   "$WEBHOOK_URL" \
                   --max-time 30 2>/dev/null || echo "000")

    total_actions=$((total_actions + 1))

    if [ "$response" = "200" ]; then
        successful_actions=$((successful_actions + 1))
        echo "✅ $action: SUCCESS"
    else
        echo "❌ $action: FAILED (HTTP: $response)"
    fi

    # Brief pause between actions
    sleep 2
done

# Calculate success rate
success_rate=$(echo "scale=2; $successful_actions * 100 / $total_actions" | bc)
required_successes=$((total_actions * 95 / 100))

echo "--- BR-PA-011 K8s Actions Test Results ---"
echo "Total Actions Tested: $total_actions"
echo "Successful Actions: $successful_actions"
echo "Success Rate: ${success_rate}%"
echo "Required for Pass: $required_successes successful actions"

if [ $successful_actions -ge $required_successes ]; then
    echo "✅ BR-PA-011: PASS - K8s actions success rate requirement met"
    echo "$success_rate,PASS" > "results/$TEST_SESSION/k8s_actions_result.csv"
else
    echo "❌ BR-PA-011: FAIL - K8s actions success rate requirement not met"
    echo "$success_rate,FAIL" > "results/$TEST_SESSION/k8s_actions_result.csv"
fi
EOF

chmod +x scripts/k8s_actions_test.sh

# Run K8s actions test
./scripts/k8s_actions_test.sh "$PROMETHEUS_WEBHOOK" "$TEST_SESSION"

echo "Test A3 completed."
```

### **Phase A Results Summary**

```bash
echo "=== Phase A: Business Requirement Validation Summary ==="

# Collect all Phase A results
phase_a_results="results/$TEST_SESSION/phase_a_summary.txt"
echo "Business Requirement Validation Results - $(date)" > "$phase_a_results"
echo "=============================================" >> "$phase_a_results"

# Check individual test results
tests_passed=0
total_tests=0

# BR-PA-003 (Response Time)
if grep -q "PASS" "results/alert_generation_"*.json 2>/dev/null; then
    echo "✅ BR-PA-003: Response Time - PASS" >> "$phase_a_results"
    tests_passed=$((tests_passed + 1))
else
    echo "❌ BR-PA-003: Response Time - FAIL" >> "$phase_a_results"
fi
total_tests=$((total_tests + 1))

# BR-PA-001 (Availability)
if [ -f "results/$TEST_SESSION/availability_result.csv" ]; then
    if grep -q "PASS" "results/$TEST_SESSION/availability_result.csv"; then
        echo "✅ BR-PA-001: Availability - PASS" >> "$phase_a_results"
        tests_passed=$((tests_passed + 1))
    else
        echo "❌ BR-PA-001: Availability - FAIL" >> "$phase_a_results"
    fi
fi
total_tests=$((total_tests + 1))

# BR-PA-011 (K8s Actions)
if [ -f "results/$TEST_SESSION/k8s_actions_result.csv" ]; then
    if grep -q "PASS" "results/$TEST_SESSION/k8s_actions_result.csv"; then
        echo "✅ BR-PA-011: K8s Actions - PASS" >> "$phase_a_results"
        tests_passed=$((tests_passed + 1))
    else
        echo "❌ BR-PA-011: K8s Actions - FAIL" >> "$phase_a_results"
    fi
fi
total_tests=$((total_tests + 1))

echo "" >> "$phase_a_results"
echo "Phase A Summary: $tests_passed/$total_tests tests passed" >> "$phase_a_results"

# Display results
cat "$phase_a_results"

if [ $tests_passed -eq $total_tests ]; then
    echo ""
    echo "🎉 Phase A: COMPLETE SUCCESS"
    echo "All business requirements validated. Ready for Phase B."
else
    echo ""
    echo "⚠️  Phase A: PARTIAL SUCCESS"
    echo "Some business requirements failed. Review before proceeding to Phase B."
    echo ""
    echo "❓ Do you want to proceed to Phase B anyway? (y/n)"
    read -r proceed_choice
    if [ "$proceed_choice" != "y" ]; then
        echo "Stopping test execution. Please resolve Phase A failures."
        exit 1
    fi
fi
```

---

## 📊 **Phase B: Capacity Exploration**

**Objective**: Understand system characteristics and operational limits
**Duration**: 4-5 hours
**Outcome**: Performance curves, scaling insights, capacity recommendations

### **Test B1: Alert Processing Capacity Exploration**

```bash
echo "=== Test B1: Alert Processing Capacity Exploration ==="
echo "Objective: Find maximum alert processing throughput"
echo "Method: Progressive load increase until failure"

# Create capacity exploration script
cat > scripts/alert_capacity_exploration.sh << 'EOF'
#!/bin/bash
WEBHOOK_URL="$1"
TEST_SESSION="$2"

# Progressive load testing phases
phases=(
    "500:5"    # 500 alerts/min for 5 minutes (baseline)
    "1000:5"   # 1000 alerts/min for 5 minutes (requirement)
    "2000:5"   # 2000 alerts/min for 5 minutes (2x requirement)
    "5000:3"   # 5000 alerts/min for 3 minutes (stress test)
    "10000:2"  # 10000 alerts/min for 2 minutes (breaking point)
)

echo "Alert Processing Capacity Exploration"
echo "Phases: ${#phases[@]} progressive load increases"

results_file="results/$TEST_SESSION/capacity_exploration.csv"
echo "phase,alerts_per_minute,duration_minutes,sent_count,success_count,success_rate,avg_response_time,max_response_time" > "$results_file"

for phase_config in "${phases[@]}"; do
    IFS=':' read -r rate duration <<< "$phase_config"

    echo ""
    echo "--- Phase: ${rate} alerts/minute for ${duration} minutes ---"

    # Run load test for this phase
    python3 scripts/generate_synthetic_alerts.py \
        --webhook-url "$WEBHOOK_URL" \
        --rate "$rate" \
        --duration "$duration" \
        --types random

    # Get the latest results file
    latest_result=$(ls -t results/alert_generation_*.json | head -n1)

    # Extract results and append to capacity file
    python3 << PYTHON_EOF
import json
import sys

with open('$latest_result', 'r') as f:
    data = json.load(f)

# Calculate max response time (estimated from average + buffer)
max_response = data.get('avg_response_time', 0) * 1.5

results_line = f"$rate,$duration,{data.get('sent_count', 0)},{data.get('success_count', 0)},{data.get('success_rate', 0):.2f},{data.get('avg_response_time', 0):.3f},{max_response:.3f}"
print(results_line)

with open('$results_file', 'a') as f:
    f.write(results_line + '\n')

# Check if we should continue (if success rate drops below 50%, stop)
if data.get('success_rate', 0) < 50:
    print("BREAKING_POINT_REACHED")
    sys.exit(1)
PYTHON_EOF

    if [ $? -eq 1 ]; then
        echo "Breaking point reached. Stopping capacity exploration."
        break
    fi

    echo "Phase completed. Brief recovery period..."
    sleep 30  # Recovery period between phases
done

# Analyze capacity results
python3 << 'ANALYSIS_EOF'
import csv
import matplotlib.pyplot as plt
import numpy as np

# Read capacity results
results = []
with open('results/' + '$TEST_SESSION' + '/capacity_exploration.csv', 'r') as f:
    reader = csv.DictReader(f)
    results = list(reader)

if not results:
    print("No capacity results found")
    exit()

# Extract data for analysis
rates = [int(r['alerts_per_minute']) for r in results]
success_rates = [float(r['success_rate']) for r in results]
response_times = [float(r['avg_response_time']) for r in results]

print("\n--- Capacity Exploration Analysis ---")
print("Rate (alerts/min) | Success Rate | Avg Response Time")
print("-" * 50)

for i, result in enumerate(results):
    print(f"{rates[i]:>15} | {success_rates[i]:>10.1f}% | {response_times[i]:>13.3f}s")

# Determine maximum sustainable rate
sustainable_rates = [rates[i] for i in range(len(rates)) if success_rates[i] >= 95]
max_sustainable_rate = max(sustainable_rates) if sustainable_rates else 0

# Determine performance degradation point
degradation_point = 0
for i in range(len(rates)):
    if success_rates[i] < 90:
        degradation_point = rates[i]
        break

print(f"\n--- Capacity Analysis Results ---")
print(f"Maximum Sustainable Rate (95%+ success): {max_sustainable_rate} alerts/min")
print(f"Performance Degradation Point (90%+ success): {degradation_point} alerts/min")
print(f"Business Requirement (1000/min): {'SUSTAINABLE' if max_sustainable_rate >= 1000 else 'AT RISK'}")

# Recommendations
if max_sustainable_rate >= 2000:
    print("✅ Recommendation: System has excellent capacity headroom")
elif max_sustainable_rate >= 1500:
    print("✅ Recommendation: System has adequate capacity headroom")
elif max_sustainable_rate >= 1000:
    print("⚠️  Recommendation: System meets requirements but limited headroom")
else:
    print("❌ Recommendation: System may not sustain business requirements under load")

ANALYSIS_EOF

echo "Test B1 completed. Capacity analysis results above."
EOF

chmod +x scripts/alert_capacity_exploration.sh

# Run capacity exploration
./scripts/alert_capacity_exploration.sh "$PROMETHEUS_WEBHOOK" "$TEST_SESSION"
```

### **Test B2: Concurrent Request Capacity Exploration**

```bash
echo "=== Test B2: Concurrent Request Capacity Exploration ==="
echo "Objective: Find maximum concurrent request handling capacity"

cat > scripts/concurrent_capacity_exploration.sh << 'EOF'
#!/bin/bash
WEBHOOK_URL="$1"
TEST_SESSION="$2"

# Progressive concurrency levels
concurrency_levels=(50 100 200 500 1000 2000)

echo "Concurrent Request Capacity Exploration"
echo "Levels: ${concurrency_levels[@]}"

results_file="results/$TEST_SESSION/concurrent_capacity.csv"
echo "concurrency_level,total_requests,successful_requests,success_rate,avg_response_time" > "$results_file"

for concurrency in "${concurrency_levels[@]}"; do
    echo ""
    echo "--- Testing $concurrency concurrent requests ---"

    # Run concurrent test
    ./scripts/concurrent_load_test.sh "$WEBHOOK_URL" "$concurrency" 60

    # Extract results from the concurrent test output
    latest_concurrent_result=$(ls -t results/concurrent_test_*.csv | head -n1)

    # Analyze and append results
    python3 << PYTHON_EOF
import csv
import statistics

with open('$latest_concurrent_result', 'r') as f:
    reader = csv.DictReader(f)
    results = list(reader)

total_requests = len(results)
successful_requests = sum(1 for r in results if r.get('http_code') == '200')
success_rate = (successful_requests / total_requests * 100) if total_requests > 0 else 0

response_times = [float(r['duration']) for r in results if r.get('http_code') == '200']
avg_response_time = statistics.mean(response_times) if response_times else 0

result_line = f"$concurrency,{total_requests},{successful_requests},{success_rate:.2f},{avg_response_time:.3f}"
print(f"Results: {result_line}")

with open('$results_file', 'a') as f:
    f.write(result_line + '\n')

# Stop if success rate drops below 75%
if success_rate < 75:
    print("CONCURRENT_LIMIT_REACHED")
    exit(1)
PYTHON_EOF

    if [ $? -eq 1 ]; then
        echo "Concurrent request limit reached. Stopping exploration."
        break
    fi

    echo "Level completed. Recovery period..."
    sleep 30
done

# Analyze concurrent capacity results
python3 << 'CONCURRENT_ANALYSIS_EOF'
import csv

with open('results/' + '$TEST_SESSION' + '/concurrent_capacity.csv', 'r') as f:
    reader = csv.DictReader(f)
    results = list(reader)

print("\n--- Concurrent Capacity Analysis ---")
print("Concurrency | Success Rate | Avg Response Time")
print("-" * 45)

for result in results:
    concurrency = result['concurrency_level']
    success_rate = float(result['success_rate'])
    response_time = float(result['avg_response_time'])
    print(f"{concurrency:>10} | {success_rate:>10.1f}% | {response_time:>13.3f}s")

# Find maximum sustainable concurrency (95%+ success, <5s response)
sustainable_levels = []
for result in results:
    success_rate = float(result['success_rate'])
    response_time = float(result['avg_response_time'])
    if success_rate >= 95 and response_time < 5.0:
        sustainable_levels.append(int(result['concurrency_level']))

max_sustainable = max(sustainable_levels) if sustainable_levels else 0

print(f"\n--- Concurrent Capacity Results ---")
print(f"Maximum Sustainable Concurrency (95%+ success, <5s): {max_sustainable}")
print(f"Business Requirement (100 concurrent): {'SUSTAINABLE' if max_sustainable >= 100 else 'AT RISK'}")

if max_sustainable >= 500:
    print("✅ Excellent concurrent request handling capacity")
elif max_sustainable >= 200:
    print("✅ Good concurrent request handling capacity")
elif max_sustainable >= 100:
    print("⚠️  Adequate capacity, meets minimum requirements")
else:
    print("❌ Insufficient capacity for business requirements")

CONCURRENT_ANALYSIS_EOF

echo "Test B2 completed."
EOF

chmod +x scripts/concurrent_capacity_exploration.sh

# Run concurrent capacity exploration
./scripts/concurrent_capacity_exploration.sh "$PROMETHEUS_WEBHOOK" "$TEST_SESSION"
```

---

## 📈 **Results Analysis & Reporting**

### **Generate Comprehensive Report**

```bash
echo "=== Generating Final Integration Test Report ==="

cat > "results/$TEST_SESSION/FINAL_TEST_REPORT.md" << 'EOF'
# Kubernaut Milestone 1 Integration Test Report

**Test Session**: TEST_SESSION_PLACEHOLDER
**Date**: DATE_PLACEHOLDER
**Testing Strategy**: Hybrid Performance Testing
**Environment**: envtest + LLM service + Redis + Optional Kind cluster

---

## Executive Summary

**Phase A: Business Requirement Validation**
- Target: Validate compliance with documented business requirements
- Duration: 3-4 hours
- Results: [PHASE_A_RESULTS_PLACEHOLDER]

**Phase B: Capacity Exploration**
- Target: Understand system characteristics and operational limits
- Duration: 4-5 hours
- Results: [PHASE_B_RESULTS_PLACEHOLDER]

---

## Business Requirement Compliance

### Critical Requirements (Must Pass for Milestone 1)

| Requirement | Test Result | Measured Value | Pass Criteria | Status |
|-------------|-------------|---------------|---------------|--------|
| BR-PA-001 | Availability | [AVAILABILITY_VALUE]% | ≥99.9% | [AVAILABILITY_STATUS] |
| BR-PA-003 | Response Time | [RESPONSE_TIME_VALUE]s | <5s (95th percentile) | [RESPONSE_TIME_STATUS] |
| BR-PA-004 | Concurrent Load | [CONCURRENT_VALUE] requests | ≥100 concurrent | [CONCURRENT_STATUS] |
| BR-PA-011 | K8s Actions | [K8S_SUCCESS_RATE]% | ≥95% success rate | [K8S_STATUS] |

---

## Capacity Analysis

### Alert Processing Capacity
- **Maximum Sustainable Rate**: [MAX_ALERT_RATE] alerts/minute
- **Performance Degradation Point**: [DEGRADATION_POINT] alerts/minute
- **Business Requirement Headroom**: [HEADROOM_ANALYSIS]

### Concurrent Request Capacity
- **Maximum Sustainable Concurrency**: [MAX_CONCURRENCY] requests
- **Response Time Under Load**: [LOAD_RESPONSE_TIME] seconds
- **Scalability Assessment**: [SCALABILITY_ASSESSMENT]

---

## Production Readiness Assessment

**Overall Confidence Rating**: [CONFIDENCE_RATING]%

### Strengths
- [IDENTIFIED_STRENGTHS]

### Areas of Concern
- [IDENTIFIED_CONCERNS]

### Recommendations
- [RECOMMENDATIONS]

---

## Pilot Deployment Recommendation

**Go/No-Go Decision**: [GO_NO_GO_DECISION]

**Rationale**: [DECISION_RATIONALE]

EOF

# Populate the template with actual results
# ... (Add result population logic here)

echo "Final report generated: results/$TEST_SESSION/FINAL_TEST_REPORT.md"
echo ""
echo "🎯 Integration Testing Complete!"
echo "📊 Review the final report for production readiness assessment"
echo "🚀 Use results to make pilot deployment decision"
```

---

## 🎉 **Test Execution Summary**

### **Total Estimated Time**: 8-10 hours
- **Setup & Preparation**: 1 hour
- **Phase A (Requirement Validation)**: 3-4 hours
- **Phase B (Capacity Exploration)**: 4-5 hours
- **Results Analysis & Reporting**: 1 hour

### **Expected Outcomes**
1. **Business Requirement Compliance**: Pass/fail status for all critical requirements
2. **Performance Characteristics**: Response time curves, throughput limits, concurrency capacity
3. **Production Readiness**: Confidence assessment with go/no-go recommendation
4. **Capacity Planning**: Scaling recommendations for future milestones

### **Success Criteria for Milestone 1 Completion**
- ✅ All Phase A business requirements pass
- ✅ System demonstrates stable performance under load
- ✅ No critical failures or data corruption observed
- ✅ Confidence rating ≥90% for production deployment

---

## 🔧 **Troubleshooting Guide for Integration Environment**

### **Common Issues and Solutions**

#### **LLM Service Issues**

**Problem**: LLM service not accessible at 192.168.1.169:8080
```bash
curl -s http://192.168.1.169:8080/health
# Returns: curl: (7) Failed to connect
```

**Solutions**:
```bash
# Check if LLM service is running
ps aux | grep -E "(ollama|ramalama)" || echo "No LLM service found"

# Start LLM service if needed
ollama serve &  # For Ollama
# OR
ramalama serve --port 7070 &  # For Ramalama

# Verify LLM service starts correctly
sleep 5 && curl -s http://192.168.1.169:8080/health
```

**Problem**: LLM service running but models not available
```bash
# Check available models
ollama list
# OR
curl -s http://192.168.1.169:8080/api/tags
```

**Solutions**:
```bash
# Pull required model (adjust model name as needed)
ollama pull granite3.1-dense:8b
# OR
ollama pull granite3.1-dense:2b  # Smaller model for testing
```

#### **Redis Authentication Issues**

**Problem**: Redis connection refused or authentication failed
```bash
redis-cli -h localhost -p 6379 ping
# Returns: (error) NOAUTH Authentication required
```

**Solutions**:
```bash
# Use correct password from docker-compose.integration.yml
redis-cli -h localhost -p 6379 -a integration_redis_password ping
# Should return: PONG

# Check if Redis is running
podman ps | grep redis || echo "Redis container not running"

# Start Redis if needed (with docker-compose)
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
podman-compose -f test/integration/docker-compose.integration.yml up -d redis
```

#### **PostgreSQL Database Issues**

**Problem**: PostgreSQL not accessible
```bash
# Check PostgreSQL status
podman ps | grep postgres
```

**Solutions**:
```bash
# Start PostgreSQL with docker-compose
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
podman-compose -f test/integration/docker-compose.integration.yml up -d postgres

# Check logs if issues persist
podman logs kubernaut-postgres-test
```

#### **Integration Test Failures**

**Problem**: Tests failing with timeout errors
```bash
# Example error:
panic: test timed out after 2m0s
```

**Solutions**:
```bash
# Increase test timeout
export TEST_TIMEOUT=600s  # 10 minutes
go test -v -tags=integration ./test/integration/... -timeout=600s

# Enable slow test skipping for faster execution
export SKIP_SLOW_TESTS=true
```

**Problem**: LLM client connection errors in tests
```bash
# Example error:
Error: failed to connect to LLM service: dial tcp 127.0.0.1:7070: connect: connection refused
```

**Solutions**:
```bash
# Verify LLM service is accessible before running tests
curl -s http://192.168.1.169:8080/health || echo "Start LLM service first"

# Set correct environment variables
export LLM_ENDPOINT="http://192.168.1.169:8080"
export LLM_MODEL="granite3.1-dense:8b"
export LLM_PROVIDER="ollama"
```

**Problem**: Kubernetes envtest setup failures
```bash
# Example error:
unable to start test environment: unable to setup envtest
```

**Solutions**:
```bash
# Install or update envtest binaries
make setup-envtest
# OR
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
setup-envtest use 1.33.0

# Check envtest assets availability
ls -la bin/k8s/1.33.0-darwin-arm64/
```

#### **Performance and Resource Issues**

**Problem**: Tests running very slowly or timing out
```bash
# System resource monitoring
top -l 1 | grep -E "(CPU usage|PhysMem)"
```

**Solutions**:
```bash
# Reduce test scale for resource-constrained environments
export SKIP_SLOW_TESTS=true

# Monitor resource usage during tests
# In separate terminal:
watch -n 5 'ps aux | grep -E "(go test|ollama|redis|postgres)" | head -10'

# Use smaller LLM model if memory is limited
export LLM_MODEL="granite3.1-dense:2b"  # Instead of 8b model
```

### **Environment Reset Procedures**

#### **Complete Environment Reset**
```bash
# Stop all running services
pkill -f "ollama serve"
pkill -f "ramalama serve"
pkill -f "go test"

# Reset containers
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
podman-compose -f test/integration/docker-compose.integration.yml down
podman-compose -f test/integration/docker-compose.integration.yml up -d

# Wait for services to be ready
sleep 10

# Start LLM service
ollama serve &
sleep 5

# Verify environment
curl -s http://192.168.1.169:8080/health && echo "✅ LLM OK"
redis-cli -h localhost -p 6379 -a integration_redis_password ping && echo "✅ Redis OK"
```

#### **Clean Test State**
```bash
# Clean Go test cache
go clean -testcache

# Clean temporary test files
rm -rf /tmp/envtest-*
rm -rf /tmp/integration-test-*

# Reset Kubernetes test assets if needed
setup-envtest use 1.33.0 --bin-dir bin/k8s
```

### **Debug Information Collection**

**When reporting issues, collect this information:**

```bash
echo "=== Kubernaut Integration Environment Debug Info ==="
echo "Date: $(date)"
echo "Platform: $(uname -a)"
echo ""

echo "=== Go Environment ==="
go version
echo "GOPATH: $GOPATH"
echo "GOOS: $GOOS"
echo "GOARCH: $GOARCH"
echo ""

echo "=== Service Status ==="
curl -s http://192.168.1.169:8080/health && echo "LLM: OK" || echo "LLM: FAILED"
redis-cli -h localhost -p 6379 -a integration_redis_password ping 2>/dev/null && echo "Redis: OK" || echo "Redis: FAILED"
podman ps | grep postgres && echo "PostgreSQL: OK" || echo "PostgreSQL: FAILED"
kubectl cluster-info 2>/dev/null && echo "Kind: OK" || echo "Kind: NOT AVAILABLE"
echo ""

echo "=== Environment Variables ==="
echo "LLM_ENDPOINT: $LLM_ENDPOINT"
echo "LLM_MODEL: $LLM_MODEL"
echo "LLM_PROVIDER: $LLM_PROVIDER"
echo "SKIP_SLOW_TESTS: $SKIP_SLOW_TESTS"
echo "TEST_TIMEOUT: $TEST_TIMEOUT"
echo ""

echo "=== Recent Test Logs ==="
ls -la /tmp/envtest-* 2>/dev/null | tail -5 || echo "No envtest logs found"
```

---

**Next**: Execute this plan manually, analyze results, and make informed decision about pilot deployment readiness.
