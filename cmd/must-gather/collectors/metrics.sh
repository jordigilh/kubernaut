#!/bin/bash
# Copyright 2025 Jordi Gil
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Kubernaut Must-Gather - Metrics Collector
# BR-PLATFORM-001.6c: Collect Prometheus metrics snapshots

set -euo pipefail

COLLECTION_DIR="${1}"
METRICS_DIR="${COLLECTION_DIR}/metrics"

echo "Collecting metrics..."

mkdir -p "${METRICS_DIR}"

# Service metrics endpoints (from /metrics)
# V1.0 Services expose Prometheus metrics on port 9090 or 8080

SERVICE_ENDPOINTS=(
    "http://gateway.kubernaut-system:8080/metrics"
    "http://data-storage.kubernaut-system:8080/metrics"
    "http://holmesgpt-api.kubernaut-system:8080/metrics"
    "http://notification-controller-metrics.kubernaut-notifications:8080/metrics"
)

for endpoint in "${SERVICE_ENDPOINTS[@]}"; do
    service_name=$(echo "${endpoint}" | cut -d'/' -f3 | cut -d':' -f1 | cut -d'.' -f1)

    echo "  - Collecting metrics from: ${service_name}"

    # Collect current metrics snapshot
    curl -s -f -m 10 "${endpoint}" \
        > "${METRICS_DIR}/${service_name}-metrics.txt" 2>/dev/null || {
        echo "    Warning: Failed to collect metrics from ${endpoint}"
        echo "# Error: Failed to scrape metrics" > "${METRICS_DIR}/${service_name}-metrics.txt"
    }
done

# CRD Controller metrics (if exposed via ServiceMonitor)
for namespace in "${KUBERNAUT_NAMESPACES[@]}"; do
    if kubectl get namespace "${namespace}" > /dev/null 2>&1; then
        echo "  - Checking for ServiceMonitor in ${namespace}..."

        # Get ServiceMonitor objects
        kubectl get servicemonitors -n "${namespace}" -o yaml \
            > "${METRICS_DIR}/servicemonitor-${namespace}.yaml" 2>/dev/null || true
    fi
done

# Kubernetes metrics-server data (if available)
echo "  - Collecting metrics-server data..."
kubectl top nodes > "${METRICS_DIR}/nodes-resource-usage.txt" 2>/dev/null || {
    echo "Warning: metrics-server not available or not configured"
}

for namespace in "${KUBERNAUT_NAMESPACES[@]}"; do
    if kubectl get namespace "${namespace}" > /dev/null 2>&1; then
        kubectl top pods -n "${namespace}" \
            > "${METRICS_DIR}/pods-resource-usage-${namespace}.txt" 2>/dev/null || true
    fi
done

# Prometheus query (if Prometheus is accessible)
echo "  - Attempting to query Prometheus (if available)..."

PROMETHEUS_URL="http://prometheus-operated.kubernaut-system:9090"

if curl -s -f -m 5 "${PROMETHEUS_URL}/-/healthy" > /dev/null 2>&1; then
    echo "    Prometheus is accessible, collecting query samples..."

    # Query key metrics (last 24h)
    METRICS_QUERIES=(
        "kubernaut_gateway_requests_total"
        "kubernaut_remediation_requests_total"
        "kubernaut_workflow_executions_total"
        "up{job=~\".*kubernaut.*\"}"
    )

    for query in "${METRICS_QUERIES[@]}"; do
        query_name=$(echo "${query}" | tr '{}' '_' | tr -d '"' | cut -c1-50)

        curl -s -f -m 10 "${PROMETHEUS_URL}/api/v1/query?query=${query}" \
            > "${METRICS_DIR}/prometheus-query-${query_name}.json" 2>/dev/null || true
    done
else
    echo "    Prometheus not accessible at ${PROMETHEUS_URL}"
fi

echo "Metrics collection complete"

