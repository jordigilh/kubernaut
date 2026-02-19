# Copyright 2026 Jordi Gil.
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

"""
Mock K8s client fixtures for DetectedLabels integration tests.

ADR-056 SoC: These fixtures provide mock K8s clients that simulate
various infrastructure configurations (PDB-protected, HPA-enabled,
ArgoCD-managed, etc.) for testing the on-demand label detection
flow in WorkflowDiscoveryToolset.

Each fixture returns a mock K8s client with:
- resolve_owner_chain(): Pod -> RS -> Deployment chain
- _get_resource_metadata(): Pod/Deployment metadata with labels/annotations
- get_namespace_metadata(): Namespace labels/annotations
- compute_spec_hash(): Deterministic hash for the root owner
- list_pdbs(): PodDisruptionBudgets in namespace
- list_hpas(): HorizontalPodAutoscalers in namespace
- list_network_policies(): NetworkPolicies in namespace
"""

from unittest.mock import AsyncMock, MagicMock
from typing import Any, Dict, List, Optional, Tuple


OWNER_CHAIN_POD_TO_DEPLOY = [
    {"kind": "Pod", "name": "api-pod-abc", "namespace": "production"},
    {"kind": "ReplicaSet", "name": "api-rs-xyz", "namespace": "production"},
    {"kind": "Deployment", "name": "api", "namespace": "production"},
]


def _make_pdb(name: str, match_labels: Dict[str, str]):
    """Create a mock PDB object."""
    pdb = MagicMock()
    pdb.metadata.name = name
    pdb.spec.selector.match_labels = match_labels
    return pdb


def _make_hpa(name: str, target_kind: str, target_name: str):
    """Create a mock HPA object."""
    hpa = MagicMock()
    hpa.metadata.name = name
    hpa.spec.scale_target_ref.kind = target_kind
    hpa.spec.scale_target_ref.name = target_name
    return hpa


def _make_netpol(name: str):
    """Create a mock NetworkPolicy object."""
    netpol = MagicMock()
    netpol.metadata.name = name
    return netpol


def _make_pod_metadata(name: str, labels: Dict, annotations: Optional[Dict] = None):
    """Create a mock Pod metadata."""
    pod = MagicMock()
    pod.metadata.name = name
    pod.metadata.labels = labels
    pod.metadata.annotations = annotations or {}
    return pod


def _make_deploy_metadata(name: str, labels: Dict):
    """Create a mock Deployment metadata."""
    deploy = MagicMock()
    deploy.metadata.name = name
    deploy.metadata.labels = labels
    return deploy


def create_mock_k8s_with_pdb() -> AsyncMock:
    """K8s client with a Deployment protected by a PDB.

    Resources: Deployment(api) + PDB(api-pdb) with matching selector.
    Expected labels: pdbProtected=true
    """
    mock_k8s = AsyncMock()
    mock_k8s.resolve_owner_chain.return_value = OWNER_CHAIN_POD_TO_DEPLOY
    mock_k8s.compute_spec_hash.return_value = "sha256:pdb-test"

    pod = _make_pod_metadata("api-pod-abc", {"app": "api"})
    deploy = _make_deploy_metadata("api", {"app": "api"})

    async def get_metadata(kind, name, namespace):
        if kind == "Pod":
            return pod
        if kind == "Deployment":
            return deploy
        return None

    mock_k8s._get_resource_metadata = AsyncMock(side_effect=get_metadata)
    mock_k8s.get_namespace_metadata = AsyncMock(
        return_value={"labels": {}, "annotations": {}}
    )
    mock_k8s.list_pdbs = AsyncMock(
        return_value=([_make_pdb("api-pdb", {"app": "api"})], None)
    )
    mock_k8s.list_hpas = AsyncMock(return_value=([], None))
    mock_k8s.list_network_policies = AsyncMock(return_value=([], None))
    return mock_k8s


def create_mock_k8s_with_hpa() -> AsyncMock:
    """K8s client with a Deployment managed by an HPA.

    Resources: Deployment(api) + HPA targeting it.
    Expected labels: hpaEnabled=true
    """
    mock_k8s = AsyncMock()
    mock_k8s.resolve_owner_chain.return_value = OWNER_CHAIN_POD_TO_DEPLOY
    mock_k8s.compute_spec_hash.return_value = "sha256:hpa-test"

    pod = _make_pod_metadata("api-pod-abc", {"app": "api"})
    deploy = _make_deploy_metadata("api", {"app": "api"})

    async def get_metadata(kind, name, namespace):
        if kind == "Pod":
            return pod
        if kind == "Deployment":
            return deploy
        return None

    mock_k8s._get_resource_metadata = AsyncMock(side_effect=get_metadata)
    mock_k8s.get_namespace_metadata = AsyncMock(
        return_value={"labels": {}, "annotations": {}}
    )
    mock_k8s.list_pdbs = AsyncMock(return_value=([], None))
    mock_k8s.list_hpas = AsyncMock(
        return_value=([_make_hpa("api-hpa", "Deployment", "api")], None)
    )
    mock_k8s.list_network_policies = AsyncMock(return_value=([], None))
    return mock_k8s


def create_mock_k8s_argocd_helm() -> AsyncMock:
    """K8s client with ArgoCD + Helm managed Deployment.

    Resources: Deployment(api) with ArgoCD + Helm labels.
    Expected labels: gitOpsManaged=true, gitOpsTool=argocd, helmManaged=true
    """
    mock_k8s = AsyncMock()
    mock_k8s.resolve_owner_chain.return_value = OWNER_CHAIN_POD_TO_DEPLOY
    mock_k8s.compute_spec_hash.return_value = "sha256:argocd-test"

    pod = _make_pod_metadata(
        "api-pod-abc",
        {"app": "api"},
        annotations={"argocd.argoproj.io/instance": "my-app"},
    )
    deploy = _make_deploy_metadata("api", {
        "app": "api",
        "app.kubernetes.io/managed-by": "Helm",
    })

    async def get_metadata(kind, name, namespace):
        if kind == "Pod":
            return pod
        if kind == "Deployment":
            return deploy
        return None

    mock_k8s._get_resource_metadata = AsyncMock(side_effect=get_metadata)
    mock_k8s.get_namespace_metadata = AsyncMock(
        return_value={
            "labels": {"argocd.argoproj.io/instance": "cluster-apps"},
            "annotations": {},
        }
    )
    mock_k8s.list_pdbs = AsyncMock(return_value=([], None))
    mock_k8s.list_hpas = AsyncMock(return_value=([], None))
    mock_k8s.list_network_policies = AsyncMock(return_value=([], None))
    return mock_k8s


def create_mock_k8s_rbac_denied() -> AsyncMock:
    """K8s client where PDB list returns RBAC 403 error.

    Expected labels: pdbProtected=false, failedDetections=[pdbProtected]
    """
    mock_k8s = AsyncMock()
    mock_k8s.resolve_owner_chain.return_value = OWNER_CHAIN_POD_TO_DEPLOY
    mock_k8s.compute_spec_hash.return_value = "sha256:rbac-test"

    pod = _make_pod_metadata("api-pod-abc", {"app": "api"})
    deploy = _make_deploy_metadata("api", {"app": "api"})

    async def get_metadata(kind, name, namespace):
        if kind == "Pod":
            return pod
        if kind == "Deployment":
            return deploy
        return None

    mock_k8s._get_resource_metadata = AsyncMock(side_effect=get_metadata)
    mock_k8s.get_namespace_metadata = AsyncMock(
        return_value={"labels": {}, "annotations": {}}
    )
    mock_k8s.list_pdbs = AsyncMock(
        return_value=([], "forbidden: User cannot list PodDisruptionBudgets")
    )
    mock_k8s.list_hpas = AsyncMock(return_value=([], None))
    mock_k8s.list_network_policies = AsyncMock(return_value=([], None))
    return mock_k8s


def create_mock_k8s_no_resources() -> AsyncMock:
    """K8s client where no K8s resources are found.

    Expected labels: all false, no failedDetections
    """
    mock_k8s = AsyncMock()
    mock_k8s.resolve_owner_chain.return_value = [
        {"kind": "Deployment", "name": "missing", "namespace": "production"},
    ]
    mock_k8s.compute_spec_hash.return_value = "sha256:empty-test"
    mock_k8s._get_resource_metadata = AsyncMock(return_value=None)
    mock_k8s.get_namespace_metadata = AsyncMock(
        return_value={"labels": {}, "annotations": {}}
    )
    mock_k8s.list_pdbs = AsyncMock(return_value=([], None))
    mock_k8s.list_hpas = AsyncMock(return_value=([], None))
    mock_k8s.list_network_policies = AsyncMock(return_value=([], None))
    return mock_k8s
