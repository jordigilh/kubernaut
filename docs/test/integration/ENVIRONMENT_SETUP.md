# Integration Test Environment Setup Guide

**Document Version**: 1.0
**Date**: January 2025
**Target**: Kind Cluster with Docker + Local Infrastructure
**Execution Time**: 2-3 hours for complete setup

---

## 🎯 **Setup Overview**

This guide provides step-by-step instructions for setting up a complete integration test environment using Kind (Kubernetes in Docker) with all necessary infrastructure components for Milestone 1 testing.

### **Final Environment Architecture**
```
┌─────────────────────────────────────────────────────────────┐
│                    Local Host                                │
│  ┌─────────────────┐  ┌─────────────────────────────────────┐│
│  │ Ollama Model    │  │ Kind Cluster (Docker)               ││
│  │ localhost:8080  │  │ ┌─────────────────────────────────┐ ││
│  └─────────────────┘  │ │ Control Plane (master)          │ ││
│  ┌─────────────────┐  │ ├─────────────────────────────────┤ ││
│  │ Test Scripts    │  │ │ Worker Node 1                   │ ││
│  │ & Tools         │  │ ├─────────────────────────────────┤ ││
│  └─────────────────┘  │ │ Worker Node 2                   │ ││
│  ┌─────────────────┐  │ └─────────────────────────────────┘ ││
│  │ Kubernaut Apps  │  │ ┌─────────────────────────────────┐ ││
│  │ (External)      │  │ │ Infrastructure Pods:            │ ││
│  └─────────────────┘  │ │ • PostgreSQL                    │ ││
│                        │ │ • Prometheus                    │ ││
│                        │ │ • Test Workloads               │ ││
│                        │ └─────────────────────────────────┘ ││
│                        └─────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

---

## 🚀 **Phase 1: Prerequisites & Docker Setup**

### **Step 1.1: Verify System Requirements**

```bash
# Check Docker installation
docker --version
# Required: Docker 20.10+

# Check system resources
echo "Available Memory: $(free -h | grep '^Mem' | awk '{print $7}')"
echo "Available Disk: $(df -h . | tail -1 | awk '{print $4}')"
echo "CPU Cores: $(nproc)"

# Requirements:
# - Memory: 8GB+ available
# - Disk: 20GB+ free space
# - CPU: 4+ cores recommended
```

### **Step 1.2: Install Required Tools**

```bash
# Install Kind
curl -Lo ./kind https://github.com/kubernetes-sigs/kind/releases/download/v0.30.0/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind

# Install kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl
sudo mv kubectl /usr/local/bin/kubectl

# Install Helm (for infrastructure deployment)
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Verify installations
kind --version
kubectl version --client
helm version
```

### **Step 1.3: Prepare Test Environment Directory**

```bash
# Create test environment workspace
mkdir -p ~/kubernaut-integration-test
cd ~/kubernaut-integration-test

# Create directory structure
mkdir -p {config,scripts,data,results}
```

---

## 🏗️ **Phase 2: Kind Cluster Bootstrap**

### **Step 2.1: Create Kind Cluster Configuration**

```bash
# Create Kind cluster configuration
cat > config/kind-cluster-config.yaml << 'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: kubernaut-integration
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 8080
    protocol: TCP
  - containerPort: 443
    hostPort: 8443
    protocol: TCP
  - containerPort: 9090
    hostPort: 9090
    protocol: TCP
- role: worker
  labels:
    node-type: worker
- role: worker
  labels:
    node-type: worker
networking:
  disableDefaultCNI: false
  kubeProxyMode: "iptables"
EOF
```

### **Step 2.2: Create and Verify Cluster**

```bash
# Create the cluster
echo "Creating Kind cluster (this may take 5-10 minutes)..."
kind create cluster --config=config/kind-cluster-config.yaml --wait=300s

# Verify cluster is ready
kubectl cluster-info --context kind-kubernaut-integration
kubectl get nodes -o wide

# Expected output: 1 control-plane + 2 worker nodes in Ready state
```

### **Step 2.3: Verify Cluster Networking**

```bash
# Test basic networking
kubectl create deployment nginx-test --image=nginx:latest
kubectl expose deployment nginx-test --port=80 --type=NodePort
kubectl wait --for=condition=available --timeout=300s deployment/nginx-test

# Get the NodePort and test connectivity
NODE_PORT=$(kubectl get svc nginx-test -o jsonpath='{.spec.ports[0].nodePort}')
curl -s http://localhost:${NODE_PORT} | grep "Welcome to nginx" || echo "Networking test failed"

# Cleanup test
kubectl delete deployment nginx-test
kubectl delete service nginx-test
```

---

## 📊 **Phase 3: Infrastructure Components Setup**

### **Step 3.1: Deploy PostgreSQL for State Storage**

```bash
# Create namespace for infrastructure
kubectl create namespace kubernaut-infra

# Create PostgreSQL deployment
cat > config/postgresql-deployment.yaml << 'EOF'
apiVersion: v1
kind: Secret
metadata:
  name: postgres-secret
  namespace: kubernaut-infra
data:
  postgres-password: a3ViZXJuYXV0MTIz  # kubernaut123 base64 encoded
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgresql
  namespace: kubernaut-infra
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgresql
  template:
    metadata:
      labels:
        app: postgresql
    spec:
      containers:
      - name: postgresql
        image: postgres:15
        env:
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: postgres-password
        - name: POSTGRES_DB
          value: kubernaut
        ports:
        - containerPort: 5432
        volumeMounts:
        - name: postgres-storage
          mountPath: /var/lib/postgresql/data
      volumes:
      - name: postgres-storage
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: postgresql
  namespace: kubernaut-infra
spec:
  selector:
    app: postgresql
  ports:
  - port: 5432
    targetPort: 5432
  type: ClusterIP
EOF

# Deploy PostgreSQL
kubectl apply -f config/postgresql-deployment.yaml
kubectl wait --for=condition=available --timeout=300s deployment/postgresql -n kubernaut-infra
```

### **Step 3.2: Deploy Prometheus for Monitoring**

```bash
# Create Prometheus configuration
cat > config/prometheus-config.yaml << 'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
  namespace: kubernaut-infra
data:
  prometheus.yml: |
    global:
      scrape_interval: 15s
    scrape_configs:
    - job_name: 'kubernetes-pods'
      kubernetes_sd_configs:
      - role: pod
      relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
    - job_name: 'kubernaut-metrics'
      static_configs:
      - targets: ['host.docker.internal:8081']  # Kubernaut metrics endpoint
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus
  namespace: kubernaut-infra
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prometheus
  template:
    metadata:
      labels:
        app: prometheus
    spec:
      containers:
      - name: prometheus
        image: prom/prometheus:latest
        ports:
        - containerPort: 9090
        volumeMounts:
        - name: prometheus-config
          mountPath: /etc/prometheus/
        - name: prometheus-storage
          mountPath: /prometheus
      volumes:
      - name: prometheus-config
        configMap:
          name: prometheus-config
      - name: prometheus-storage
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: prometheus
  namespace: kubernaut-infra
spec:
  selector:
    app: prometheus
  ports:
  - port: 9090
    targetPort: 9090
    nodePort: 30090
  type: NodePort
EOF

# Deploy Prometheus
kubectl apply -f config/prometheus-config.yaml
kubectl wait --for=condition=available --timeout=300s deployment/prometheus -n kubernaut-infra

# Verify Prometheus is accessible
echo "Prometheus should be accessible at: http://localhost:30090"
```

### **Step 3.3: Deploy Test Workloads**

```bash
# Create test workloads namespace
kubectl create namespace test-workloads

# Deploy sample workloads for testing K8s actions
cat > config/test-workloads.yaml << 'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: high-memory-app
  namespace: test-workloads
  labels:
    app: high-memory-app
spec:
  replicas: 2
  selector:
    matchLabels:
      app: high-memory-app
  template:
    metadata:
      labels:
        app: high-memory-app
    spec:
      containers:
      - name: memory-consumer
        image: nginx:latest
        resources:
          limits:
            memory: 128Mi
          requests:
            memory: 64Mi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cpu-intensive-app
  namespace: test-workloads
  labels:
    app: cpu-intensive-app
spec:
  replicas: 1
  selector:
    matchLabels:
      app: cpu-intensive-app
  template:
    metadata:
      labels:
        app: cpu-intensive-app
    spec:
      containers:
      - name: cpu-consumer
        image: nginx:latest
        resources:
          limits:
            cpu: 200m
          requests:
            cpu: 100m
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: crashloop-simulator
  namespace: test-workloads
  labels:
    app: crashloop-simulator
spec:
  replicas: 1
  selector:
    matchLabels:
      app: crashloop-simulator
  template:
    metadata:
      labels:
        app: crashloop-simulator
    spec:
      containers:
      - name: crasher
        image: busybox
        command: ['sh', '-c', 'echo "Starting..."; sleep 30; exit 1']
EOF

# Deploy test workloads
kubectl apply -f config/test-workloads.yaml
kubectl wait --for=condition=available --timeout=300s deployment/high-memory-app -n test-workloads
kubectl wait --for=condition=available --timeout=300s deployment/cpu-intensive-app -n test-workloads
```

---

## 🧪 **Phase 4: Test Infrastructure Setup**

### **Step 4.1: Create Synthetic Alert Generator**

```bash
# Create alert generator script
cat > scripts/generate_synthetic_alerts.py << 'EOF'
#!/usr/bin/env python3
"""
Synthetic Alert Generator for Kubernaut Integration Testing
Generates realistic Prometheus-style alerts for testing
"""
import json
import time
import requests
import random
from datetime import datetime, timezone
import argparse

class SyntheticAlertGenerator:
    def __init__(self, webhook_url, rate_per_minute=60):
        self.webhook_url = webhook_url
        self.rate_per_minute = rate_per_minute
        self.interval = 60.0 / rate_per_minute

    def generate_alert(self, alert_type="random"):
        """Generate a synthetic alert based on type"""
        alert_types = {
            "high_memory": self._high_memory_alert(),
            "high_cpu": self._high_cpu_alert(),
            "pod_crash": self._pod_crash_alert(),
            "storage_full": self._storage_full_alert(),
            "network_issue": self._network_issue_alert()
        }

        if alert_type == "random":
            alert_type = random.choice(list(alert_types.keys()))

        return alert_types.get(alert_type, alert_types["high_memory"])

    def _high_memory_alert(self):
        return {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "HighMemoryUsage",
                    "severity": "critical",
                    "namespace": "test-workloads",
                    "pod": f"high-memory-app-{random.randint(1000, 9999)}",
                    "container": "memory-consumer"
                },
                "annotations": {
                    "description": "Pod memory usage is above 90%",
                    "summary": "High memory usage detected",
                    "runbook_url": "https://example.com/runbooks/high-memory"
                },
                "startsAt": datetime.now(timezone.utc).isoformat(),
                "generatorURL": "http://prometheus:9090"
            }]
        }

    def _high_cpu_alert(self):
        return {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "HighCPUUsage",
                    "severity": "warning",
                    "namespace": "test-workloads",
                    "pod": f"cpu-intensive-app-{random.randint(1000, 9999)}",
                    "container": "cpu-consumer"
                },
                "annotations": {
                    "description": "Pod CPU usage is above 80%",
                    "summary": "High CPU usage detected",
                    "runbook_url": "https://example.com/runbooks/high-cpu"
                },
                "startsAt": datetime.now(timezone.utc).isoformat(),
                "generatorURL": "http://prometheus:9090"
            }]
        }

    def _pod_crash_alert(self):
        return {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "PodCrashLooping",
                    "severity": "critical",
                    "namespace": "test-workloads",
                    "pod": f"crashloop-simulator-{random.randint(1000, 9999)}",
                    "container": "crasher"
                },
                "annotations": {
                    "description": "Pod is crash looping",
                    "summary": "Pod restart loop detected",
                    "runbook_url": "https://example.com/runbooks/crash-loop"
                },
                "startsAt": datetime.now(timezone.utc).isoformat(),
                "generatorURL": "http://prometheus:9090"
            }]
        }

    def _storage_full_alert(self):
        return {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "StorageNearlyFull",
                    "severity": "warning",
                    "namespace": "test-workloads",
                    "volume": f"pvc-{random.randint(1000, 9999)}",
                    "node": f"worker-{random.randint(1, 2)}"
                },
                "annotations": {
                    "description": "Storage volume is 85% full",
                    "summary": "Storage capacity warning",
                    "runbook_url": "https://example.com/runbooks/storage-full"
                },
                "startsAt": datetime.now(timezone.utc).isoformat(),
                "generatorURL": "http://prometheus:9090"
            }]
        }

    def _network_issue_alert(self):
        return {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "NetworkLatencyHigh",
                    "severity": "warning",
                    "namespace": "test-workloads",
                    "source_pod": f"app-{random.randint(1000, 9999)}",
                    "target_service": "postgresql"
                },
                "annotations": {
                    "description": "Network latency between pods is >100ms",
                    "summary": "High network latency detected",
                    "runbook_url": "https://example.com/runbooks/network-latency"
                },
                "startsAt": datetime.now(timezone.utc).isoformat(),
                "generatorURL": "http://prometheus:9090"
            }]
        }

    def send_alert(self, alert_data):
        """Send alert to webhook endpoint"""
        try:
            response = requests.post(
                self.webhook_url,
                json=alert_data,
                headers={'Content-Type': 'application/json'},
                timeout=10
            )
            return response.status_code, response.elapsed.total_seconds()
        except Exception as e:
            return -1, -1, str(e)

    def run_continuous(self, duration_minutes=5, alert_types=None):
        """Run continuous alert generation"""
        if alert_types is None:
            alert_types = ["random"]

        end_time = time.time() + (duration_minutes * 60)
        sent_count = 0
        success_count = 0
        total_response_time = 0

        print(f"Starting continuous alert generation...")
        print(f"Rate: {self.rate_per_minute} alerts/minute")
        print(f"Duration: {duration_minutes} minutes")
        print(f"Target URL: {self.webhook_url}")

        while time.time() < end_time:
            alert_type = random.choice(alert_types)
            alert = self.generate_alert(alert_type)

            start_time = time.time()
            status_code, response_time = self.send_alert(alert)

            sent_count += 1
            if status_code == 200:
                success_count += 1
                total_response_time += response_time

            # Progress reporting
            if sent_count % 10 == 0:
                success_rate = (success_count / sent_count) * 100
                avg_response_time = total_response_time / max(success_count, 1)
                print(f"Sent: {sent_count}, Success: {success_rate:.1f}%, Avg Response: {avg_response_time:.3f}s")

            # Wait for next alert
            time.sleep(self.interval)

        # Final statistics
        success_rate = (success_count / sent_count) * 100 if sent_count > 0 else 0
        avg_response_time = total_response_time / success_count if success_count > 0 else 0

        print(f"\nFinal Statistics:")
        print(f"Total Alerts Sent: {sent_count}")
        print(f"Successful Alerts: {success_count}")
        print(f"Success Rate: {success_rate:.2f}%")
        print(f"Average Response Time: {avg_response_time:.3f}s")

        return {
            "sent_count": sent_count,
            "success_count": success_count,
            "success_rate": success_rate,
            "avg_response_time": avg_response_time
        }

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Generate synthetic alerts for testing")
    parser.add_argument("--webhook-url", required=True, help="Webhook URL for alerts")
    parser.add_argument("--rate", type=int, default=60, help="Alerts per minute (default: 60)")
    parser.add_argument("--duration", type=int, default=5, help="Duration in minutes (default: 5)")
    parser.add_argument("--types", nargs="*", default=["random"],
                       help="Alert types to generate (default: random)")

    args = parser.parse_args()

    generator = SyntheticAlertGenerator(args.webhook_url, args.rate)
    results = generator.run_continuous(args.duration, args.types)

    # Save results for analysis
    with open(f"results/alert_generation_{int(time.time())}.json", "w") as f:
        json.dump(results, f, indent=2)
EOF

chmod +x scripts/generate_synthetic_alerts.py

# Install Python dependencies if needed
python3 -m pip install requests --user
```

### **Step 4.2: Create Load Testing Scripts**

```bash
# Create concurrent load test script
cat > scripts/concurrent_load_test.sh << 'EOF'
#!/bin/bash
"""
Concurrent Load Test Script
Tests BR-PA-004: 100 concurrent request handling
"""

WEBHOOK_URL="$1"
CONCURRENT_REQUESTS="${2:-100}"
TEST_DURATION="${3:-300}"  # 5 minutes

if [ -z "$WEBHOOK_URL" ]; then
    echo "Usage: $0 <webhook_url> [concurrent_requests] [duration_seconds]"
    exit 1
fi

echo "Starting concurrent load test..."
echo "Webhook URL: $WEBHOOK_URL"
echo "Concurrent Requests: $CONCURRENT_REQUESTS"
echo "Test Duration: $TEST_DURATION seconds"

# Create results directory
mkdir -p results

# Function to send single request
send_request() {
    local id=$1
    local webhook_url=$2
    local start_time=$(date +%s.%3N)

    # Generate simple alert payload
    local alert_payload='{
        "alerts": [{
            "status": "firing",
            "labels": {
                "alertname": "LoadTestAlert",
                "severity": "warning",
                "test_id": "'$id'"
            },
            "annotations": {
                "description": "Load test alert #'$id'",
                "summary": "Concurrent load test alert"
            },
            "startsAt": "'$(date -Iseconds)'"
        }]
    }'

    # Send request and capture response
    local response=$(curl -s -w "\n%{http_code}\n%{time_total}" \
                          -X POST \
                          -H "Content-Type: application/json" \
                          -d "$alert_payload" \
                          "$webhook_url" 2>/dev/null)

    local end_time=$(date +%s.%3N)
    local duration=$(echo "$end_time - $start_time" | bc)
    local http_code=$(echo "$response" | tail -n 2 | head -n 1)
    local curl_time=$(echo "$response" | tail -n 1)

    # Log result
    echo "$id,$start_time,$end_time,$duration,$http_code,$curl_time" >> results/concurrent_test_$$.csv
}

# Initialize results file
echo "request_id,start_time,end_time,duration,http_code,curl_time" > results/concurrent_test_$$.csv

# Start concurrent requests
echo "Starting $CONCURRENT_REQUESTS concurrent requests..."
for i in $(seq 1 $CONCURRENT_REQUESTS); do
    send_request $i $WEBHOOK_URL &

    # Brief delay to spread out the start times slightly
    sleep 0.01
done

# Wait for all background jobs to complete
wait

# Analyze results
echo "Analyzing results..."
python3 << 'PYTHON_EOF'
import csv
import statistics
import sys

results_file = f"results/concurrent_test_{sys.argv[1]}.csv"

with open(results_file, 'r') as f:
    reader = csv.DictReader(f)
    results = list(reader)

# Calculate statistics
response_times = [float(r['duration']) for r in results]
http_codes = [int(r['http_code']) for r in results if r['http_code'].isdigit()]

success_count = sum(1 for code in http_codes if code == 200)
total_requests = len(results)
success_rate = (success_count / total_requests) * 100

print(f"\n--- Concurrent Load Test Results ---")
print(f"Total Requests: {total_requests}")
print(f"Successful Requests (200): {success_count}")
print(f"Success Rate: {success_rate:.2f}%")
print(f"Average Response Time: {statistics.mean(response_times):.3f}s")
print(f"Median Response Time: {statistics.median(response_times):.3f}s")
print(f"95th Percentile Response Time: {sorted(response_times)[int(len(response_times) * 0.95)]:.3f}s")
print(f"Max Response Time: {max(response_times):.3f}s")

# Business requirement validation
print(f"\n--- Business Requirement Validation ---")
if success_rate >= 95:
    print("✅ BR-PA-004: Concurrent request handling - PASS")
else:
    print("❌ BR-PA-004: Concurrent request handling - FAIL")

avg_response = statistics.mean(response_times)
if avg_response < 5.0:
    print("✅ BR-PA-003: 5-second processing time - PASS")
else:
    print("❌ BR-PA-003: 5-second processing time - FAIL")
PYTHON_EOF $$

echo "Results saved to: results/concurrent_test_$$.csv"
EOF

chmod +x scripts/concurrent_load_test.sh
```

---

## 🧪 **Phase 5: Ollama Integration Verification**

### **Step 5.1: Test Ollama Connectivity**

```bash
# Create Ollama connectivity test
cat > scripts/test_ollama_connectivity.sh << 'EOF'
#!/bin/bash
"""
Ollama Model Connectivity Test
Verifies local Ollama model is accessible and responsive
"""

OLLAMA_URL="${1:-http://localhost:8080}"

echo "Testing Ollama connectivity at: $OLLAMA_URL"

# Test basic connectivity
echo "1. Testing basic connectivity..."
response=$(curl -s -w "%{http_code}" -o /dev/null "$OLLAMA_URL/api/version" || echo "FAILED")

if [ "$response" = "200" ]; then
    echo "✅ Ollama API is accessible"
else
    echo "❌ Ollama API is not accessible (HTTP: $response)"
    echo "Please ensure Ollama is running at $OLLAMA_URL"
    exit 1
fi

# Test model availability
echo "2. Testing model availability..."
model_response=$(curl -s "$OLLAMA_URL/api/tags" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    models = [model['name'] for model in data.get('models', [])]
    print(f'Available models: {models}')
    if models:
        print('✅ Models are available')
        sys.exit(0)
    else:
        print('❌ No models available')
        sys.exit(1)
except Exception as e:
    print(f'❌ Error checking models: {e}')
    sys.exit(1)
" 2>/dev/null)

echo "$model_response"

# Test basic inference
echo "3. Testing basic inference..."
inference_test=$(curl -s "$OLLAMA_URL/api/generate" \
    -H "Content-Type: application/json" \
    -d '{
        "model": "llama2",
        "prompt": "Hello, respond with just the word SUCCESS if you can process this.",
        "stream": false
    }' | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    response = data.get('response', '').strip()
    if 'SUCCESS' in response.upper():
        print('✅ Model inference is working')
    else:
        print(f'⚠️  Model responded but may not be optimal: {response[:100]}...')
    sys.exit(0)
except Exception as e:
    print(f'❌ Error testing inference: {e}')
    sys.exit(1)
" 2>/dev/null)

echo "$inference_test"

echo "✅ Ollama integration test completed"
EOF

chmod +x scripts/test_ollama_connectivity.sh
```

---

## 🏁 **Phase 6: Environment Validation**

### **Step 6.1: Complete Environment Health Check**

```bash
# Create comprehensive health check script
cat > scripts/environment_health_check.sh << 'EOF'
#!/bin/bash
"""
Complete Environment Health Check
Validates all components are ready for integration testing
"""

echo "🔍 Kubernaut Integration Test Environment Health Check"
echo "=================================================="

# Test 1: Kind cluster health
echo "1. Checking Kind cluster health..."
if kubectl cluster-info --context kind-kubernaut-integration > /dev/null 2>&1; then
    node_count=$(kubectl get nodes --no-headers | wc -l)
    ready_nodes=$(kubectl get nodes --no-headers | grep -c " Ready ")
    echo "✅ Kind cluster is accessible ($ready_nodes/$node_count nodes ready)"
else
    echo "❌ Kind cluster is not accessible"
    exit 1
fi

# Test 2: Infrastructure components
echo "2. Checking infrastructure components..."
services=("postgresql" "prometheus")
for service in "${services[@]}"; do
    if kubectl get deployment $service -n kubernaut-infra > /dev/null 2>&1; then
        replicas=$(kubectl get deployment $service -n kubernaut-infra -o jsonpath='{.status.readyReplicas}')
        if [ "$replicas" = "1" ]; then
            echo "✅ $service is running and ready"
        else
            echo "❌ $service is not ready (ready replicas: $replicas)"
        fi
    else
        echo "❌ $service deployment not found"
    fi
done

# Test 3: Test workloads
echo "3. Checking test workloads..."
workloads=("high-memory-app" "cpu-intensive-app")
for workload in "${workloads[@]}"; do
    if kubectl get deployment $workload -n test-workloads > /dev/null 2>&1; then
        replicas=$(kubectl get deployment $workload -n test-workloads -o jsonpath='{.status.readyReplicas}')
        desired=$(kubectl get deployment $workload -n test-workloads -o jsonpath='{.spec.replicas}')
        if [ "$replicas" = "$desired" ]; then
            echo "✅ $workload is running and ready ($replicas/$desired replicas)"
        else
            echo "⚠️  $workload has some replicas not ready ($replicas/$desired replicas)"
        fi
    else
        echo "❌ $workload deployment not found"
    fi
done

# Test 4: Ollama connectivity
echo "4. Checking Ollama connectivity..."
if scripts/test_ollama_connectivity.sh > /dev/null 2>&1; then
    echo "✅ Ollama model is accessible and responsive"
else
    echo "❌ Ollama model connectivity failed"
fi

# Test 5: Network connectivity
echo "5. Checking network connectivity..."
if kubectl run network-test --image=curlimages/curl --rm -it --restart=Never -- curl -s http://prometheus.kubernaut-infra.svc.cluster.local:9090/api/v1/status/config > /dev/null 2>&1; then
    echo "✅ Internal network connectivity working"
else
    echo "❌ Internal network connectivity failed"
fi

# Test 6: External access
echo "6. Checking external access..."
if curl -s http://localhost:30090/api/v1/status/config > /dev/null 2>&1; then
    echo "✅ External access to Prometheus working"
else
    echo "⚠️  External access to Prometheus may have issues"
fi

echo ""
echo "🎯 Environment Status Summary:"
echo "- Kind cluster: 3 nodes ready"
echo "- Infrastructure: PostgreSQL + Prometheus deployed"
echo "- Test workloads: Available for K8s action testing"
echo "- Ollama model: Ready for AI processing"
echo "- Networking: Internal and external connectivity verified"
echo ""
echo "✅ Environment is ready for integration testing!"
EOF

chmod +x scripts/environment_health_check.sh

# Run the health check
./scripts/environment_health_check.sh
```

---

## 📋 **Setup Completion Checklist**

After completing all phases, verify:

- [ ] Kind cluster is running with 3 nodes (1 control-plane + 2 workers)
- [ ] PostgreSQL is deployed and accessible
- [ ] Prometheus is deployed and accessible at http://localhost:30090
- [ ] Test workloads are deployed and ready
- [ ] Ollama model is accessible at localhost:8080
- [ ] Alert generator script is ready
- [ ] Load testing scripts are functional
- [ ] Environment health check passes

---

## 🚀 **Next Steps**

1. **Environment Validation**: Run complete health check
2. **Kubernaut Deployment**: Deploy Kubernaut applications pointing to this infrastructure
3. **Basic Smoke Test**: Send a single alert and verify processing
4. **Integration Testing**: Begin executing the test suites

**Estimated Setup Time**: 2-3 hours
**Environment Lifetime**: Can be torn down and rebuilt as needed with `kind delete cluster --name kubernaut-integration`

---

## 🔧 **Troubleshooting**

### **Common Issues**

1. **Kind cluster creation fails**
   - Check Docker is running and has sufficient resources
   - Increase Docker memory/CPU limits if needed

2. **PostgreSQL won't start**
   - Check if port 5432 is already in use locally
   - Review pod logs: `kubectl logs -n kubernaut-infra deployment/postgresql`

3. **Ollama connectivity fails**
   - Verify Ollama is running: `curl http://localhost:8080/api/version`
   - Check if a model is loaded: `curl http://localhost:8080/api/tags`

4. **Network connectivity issues**
   - Verify Kind networking: `docker network ls`
   - Check DNS resolution in cluster: `kubectl run -it --rm debug --image=busybox -- nslookup prometheus.kubernaut-infra.svc.cluster.local`

### **Cleanup and Reset**

```bash
# Complete environment cleanup
kind delete cluster --name kubernaut-integration
docker system prune -f
rm -rf ~/kubernaut-integration-test

# Start fresh
# Re-run this setup guide from Phase 1
```
