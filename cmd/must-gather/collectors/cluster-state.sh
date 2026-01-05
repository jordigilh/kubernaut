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

# Kubernaut Must-Gather - Cluster State Collector
# BR-PLATFORM-001.6: Collect RBAC, Storage, Network resources

set -euo pipefail

COLLECTION_DIR="${1}"
CLUSTER_DIR="${COLLECTION_DIR}/cluster-scoped"

echo "Collecting cluster state..."

mkdir -p "${CLUSTER_DIR}"/{nodes,rbac,storage,network,config}

# Node information
echo "  - Collecting node information..."
kubectl get nodes -o yaml > "${CLUSTER_DIR}/nodes/nodes.yaml" 2>/dev/null || true
kubectl describe nodes > "${CLUSTER_DIR}/nodes/nodes-describe.txt" 2>/dev/null || true

# RBAC Resources (filter by kubernaut.ai labels)
echo "  - Collecting RBAC resources..."

# ServiceAccounts in Kubernaut namespaces
for namespace in "${KUBERNAUT_NAMESPACES[@]}"; do
    if kubectl get namespace "${namespace}" > /dev/null 2>&1; then
        kubectl get serviceaccounts -n "${namespace}" -o yaml \
            > "${CLUSTER_DIR}/rbac/serviceaccounts-${namespace}.yaml" 2>/dev/null || true
    fi
done

# ClusterRoles with kubernaut labels
kubectl get clusterroles -l app.kubernetes.io/part-of=kubernaut -o yaml \
    > "${CLUSTER_DIR}/rbac/clusterroles.yaml" 2>/dev/null || {
    # Fallback: get all clusterroles with 'kubernaut' in name
    kubectl get clusterroles -o yaml | grep -A 100 "name.*kubernaut" \
        > "${CLUSTER_DIR}/rbac/clusterroles.yaml" 2>/dev/null || true
}

# ClusterRoleBindings with kubernaut labels
kubectl get clusterrolebindings -l app.kubernetes.io/part-of=kubernaut -o yaml \
    > "${CLUSTER_DIR}/rbac/clusterrolebindings.yaml" 2>/dev/null || {
    # Fallback: get all clusterrolebindings with 'kubernaut' in name
    kubectl get clusterrolebindings -o yaml | grep -A 100 "name.*kubernaut" \
        > "${CLUSTER_DIR}/rbac/clusterrolebindings.yaml" 2>/dev/null || true
}

# Roles and RoleBindings in Kubernaut namespaces
for namespace in "${KUBERNAUT_NAMESPACES[@]}"; do
    if kubectl get namespace "${namespace}" > /dev/null 2>&1; then
        kubectl get roles -n "${namespace}" -o yaml \
            > "${CLUSTER_DIR}/rbac/roles-${namespace}.yaml" 2>/dev/null || true
        kubectl get rolebindings -n "${namespace}" -o yaml \
            > "${CLUSTER_DIR}/rbac/rolebindings-${namespace}.yaml" 2>/dev/null || true
    fi
done

# Storage Resources
echo "  - Collecting storage resources..."

# PersistentVolumeClaims in Kubernaut namespaces
for namespace in "${KUBERNAUT_NAMESPACES[@]}"; do
    if kubectl get namespace "${namespace}" > /dev/null 2>&1; then
        kubectl get pvc -n "${namespace}" -o yaml \
            > "${CLUSTER_DIR}/storage/pvc-${namespace}.yaml" 2>/dev/null || true
    fi
done

# PersistentVolumes (cluster-scoped)
kubectl get pv -o yaml > "${CLUSTER_DIR}/storage/persistentvolumes.yaml" 2>/dev/null || true

# StorageClasses
kubectl get storageclasses -o yaml > "${CLUSTER_DIR}/storage/storageclasses.yaml" 2>/dev/null || true

# Network Resources
echo "  - Collecting network resources..."

# Services in Kubernaut namespaces
for namespace in "${KUBERNAUT_NAMESPACES[@]}"; do
    if kubectl get namespace "${namespace}" > /dev/null 2>&1; then
        kubectl get services -n "${namespace}" -o yaml \
            > "${CLUSTER_DIR}/network/services-${namespace}.yaml" 2>/dev/null || true
        kubectl get endpoints -n "${namespace}" -o yaml \
            > "${CLUSTER_DIR}/network/endpoints-${namespace}.yaml" 2>/dev/null || true
    fi
done

# NetworkPolicies in Kubernaut namespaces
for namespace in "${KUBERNAUT_NAMESPACES[@]}"; do
    if kubectl get namespace "${namespace}" > /dev/null 2>&1; then
        kubectl get networkpolicies -n "${namespace}" -o yaml \
            > "${CLUSTER_DIR}/network/networkpolicies-${namespace}.yaml" 2>/dev/null || true
    fi
done

# Ingresses/Routes (if applicable)
for namespace in "${KUBERNAUT_NAMESPACES[@]}"; do
    if kubectl get namespace "${namespace}" > /dev/null 2>&1; then
        kubectl get ingresses -n "${namespace}" -o yaml \
            > "${CLUSTER_DIR}/network/ingresses-${namespace}.yaml" 2>/dev/null || true
        # OpenShift Routes
        kubectl get routes -n "${namespace}" -o yaml \
            > "${CLUSTER_DIR}/network/routes-${namespace}.yaml" 2>/dev/null || true
    fi
done

# Webhook Configurations (cluster-scoped)
echo "  - Collecting webhook configurations..."
kubectl get validatingwebhookconfigurations -o yaml \
    > "${CLUSTER_DIR}/config/validatingwebhookconfigurations.yaml" 2>/dev/null || true
kubectl get mutatingwebhookconfigurations -o yaml \
    > "${CLUSTER_DIR}/config/mutatingwebhookconfigurations.yaml" 2>/dev/null || true

# API Server information
echo "  - Collecting API server information..."
kubectl version --output=yaml > "${CLUSTER_DIR}/config/kubernetes-version.yaml" 2>/dev/null || true
kubectl api-resources > "${CLUSTER_DIR}/config/api-resources.txt" 2>/dev/null || true
kubectl api-versions > "${CLUSTER_DIR}/config/api-versions.txt" 2>/dev/null || true

echo "Cluster state collection complete"

