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

# Kubernaut Must-Gather - Database Infrastructure Collector
# BR-PLATFORM-001.6b: Collect PostgreSQL and Redis infrastructure

set -euo pipefail

COLLECTION_DIR="${1}"
DB_DIR="${COLLECTION_DIR}/database"

echo "Collecting database infrastructure..."

mkdir -p "${DB_DIR}"/{postgresql,redis}

# PostgreSQL Collection
echo "  - Collecting PostgreSQL infrastructure..."

# Find PostgreSQL pods in Kubernaut namespaces
POSTGRES_PODS=$(kubectl get pods -n kubernaut-system -l app=postgresql --no-headers 2>/dev/null | awk '{print $1}' || echo "")

if [ -z "${POSTGRES_PODS}" ]; then
    # Try alternate label
    POSTGRES_PODS=$(kubectl get pods -n kubernaut-system -l app.kubernetes.io/name=postgresql --no-headers 2>/dev/null | awk '{print $1}' || echo "")
fi

if [ -n "${POSTGRES_PODS}" ]; then
    while IFS= read -r pod; do
        echo "    Collecting logs from PostgreSQL pod: ${pod}"

        # Pod logs
        kubectl logs "${pod}" -n kubernaut-system \
            --since="${SINCE_DURATION}" \
            --tail=5000 \
            --timestamps \
            > "${DB_DIR}/postgresql/${pod}.log" 2>/dev/null || true

        # Pod description
        kubectl describe pod "${pod}" -n kubernaut-system \
            > "${DB_DIR}/postgresql/${pod}-describe.txt" 2>/dev/null || true

    done <<< "${POSTGRES_PODS}"

    # PostgreSQL ConfigMaps
    kubectl get configmaps -n kubernaut-system -l app=postgresql -o yaml \
        > "${DB_DIR}/postgresql/configmaps.yaml" 2>/dev/null || true

    # PostgreSQL version (if accessible)
    FIRST_PG_POD=$(echo "${POSTGRES_PODS}" | head -n 1)
    kubectl exec "${FIRST_PG_POD}" -n kubernaut-system -- psql --version \
        > "${DB_DIR}/postgresql/version.txt" 2>/dev/null || true

    echo "    PostgreSQL collection complete"
else
    echo "    Warning: No PostgreSQL pods found"
    echo "{ \"error\": \"PostgreSQL pods not found\" }" > "${DB_DIR}/postgresql/error.json"
fi

# Redis Collection
echo "  - Collecting Redis infrastructure..."

# Find Redis pods in Kubernaut namespaces
REDIS_PODS=$(kubectl get pods -n kubernaut-system -l app=redis --no-headers 2>/dev/null | awk '{print $1}' || echo "")

if [ -z "${REDIS_PODS}" ]; then
    # Try alternate label
    REDIS_PODS=$(kubectl get pods -n kubernaut-system -l app.kubernetes.io/name=redis --no-headers 2>/dev/null | awk '{print $1}' || echo "")
fi

if [ -n "${REDIS_PODS}" ]; then
    while IFS= read -r pod; do
        echo "    Collecting logs from Redis pod: ${pod}"

        # Pod logs
        kubectl logs "${pod}" -n kubernaut-system \
            --since="${SINCE_DURATION}" \
            --tail=5000 \
            --timestamps \
            > "${DB_DIR}/redis/${pod}.log" 2>/dev/null || true

        # Pod description
        kubectl describe pod "${pod}" -n kubernaut-system \
            > "${DB_DIR}/redis/${pod}-describe.txt" 2>/dev/null || true

    done <<< "${REDIS_PODS}"

    # Redis ConfigMaps
    kubectl get configmaps -n kubernaut-system -l app=redis -o yaml \
        > "${DB_DIR}/redis/configmaps.yaml" 2>/dev/null || true

    # Redis INFO (if accessible via redis-cli)
    FIRST_REDIS_POD=$(echo "${REDIS_PODS}" | head -n 1)
    kubectl exec "${FIRST_REDIS_POD}" -n kubernaut-system -- redis-cli INFO \
        > "${DB_DIR}/redis/info.txt" 2>/dev/null || true

    # Redis DBSIZE and memory info
    kubectl exec "${FIRST_REDIS_POD}" -n kubernaut-system -- redis-cli DBSIZE \
        > "${DB_DIR}/redis/dbsize.txt" 2>/dev/null || true
    kubectl exec "${FIRST_REDIS_POD}" -n kubernaut-system -- redis-cli MEMORY STATS \
        > "${DB_DIR}/redis/memory-stats.txt" 2>/dev/null || true

    echo "    Redis collection complete"
else
    echo "    Warning: No Redis pods found"
    echo "{ \"error\": \"Redis pods not found\" }" > "${DB_DIR}/redis/error.json"
fi

echo "Database infrastructure collection complete"

