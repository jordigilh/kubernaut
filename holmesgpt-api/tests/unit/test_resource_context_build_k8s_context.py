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
Tests for _build_k8s_context in GetResourceContextTool.

DD-HAPI-018 v1.2 conformance tests for context building across target kinds.
Validates that _build_k8s_context produces the correct KubernetesContext
so that LabelDetector can detect labels for any RCA target resource.
"""

import pytest
from unittest.mock import AsyncMock, MagicMock


def _make_deployment_with_pod_template(labels, annotations=None, deploy_annotations=None):
    """Build a mock Deployment with spec.template.metadata.labels."""
    deploy = MagicMock()
    deploy.metadata.labels = {"app": "api"}
    deploy.metadata.annotations = deploy_annotations or {}
    deploy.spec = MagicMock()
    deploy.spec.template = MagicMock()
    deploy.spec.template.metadata = MagicMock()
    deploy.spec.template.metadata.labels = labels or {}
    deploy.spec.template.metadata.annotations = annotations or {}
    return deploy


def _make_pdb(name, match_labels):
    """Build a mock PDB with spec.selector.matchLabels."""
    pdb = MagicMock()
    pdb.metadata.name = name
    pdb.spec.selector.match_labels = match_labels
    return pdb


def _make_pod(name, labels, annotations=None):
    """Build a mock Pod with metadata labels and annotations."""
    pod = MagicMock()
    pod.metadata.name = name
    pod.metadata.labels = labels or {}
    pod.metadata.annotations = annotations or {}
    return pod


def _make_ns_metadata():
    return {"labels": {}, "annotations": {}}


@pytest.mark.asyncio
async def test_e2e_aa_056_003_deployment_target_populates_pod_details_for_pdb():
    """E2E-AA-056-003: Deployment target populates pod_details from pod template for PDB detection.

    When get_resource_context is called with kind=Deployment, _build_k8s_context
    must populate pod_details from spec.template.metadata.labels so that
    LabelDetector._detect_pdb can match PDB selectors against pod labels.
    """
    from toolsets.resource_context import GetResourceContextTool

    deploy = _make_deployment_with_pod_template({"app": "app-e2e-003"})
    mock_k8s = AsyncMock()
    mock_k8s.resolve_owner_chain.return_value = [
        {"kind": "Deployment", "name": "app-e2e-003", "namespace": "test-ns"},
    ]
    mock_k8s._get_resource_metadata = AsyncMock(return_value=deploy)
    mock_k8s.get_namespace_metadata = AsyncMock(return_value=_make_ns_metadata())

    tool = GetResourceContextTool(k8s_client=mock_k8s, session_state={})
    k8s_context = await tool._build_k8s_context(
        kind="Deployment",
        name="app-e2e-003",
        namespace="test-ns",
        owner_chain=[{"kind": "Deployment", "name": "app-e2e-003", "namespace": "test-ns"}],
    )

    assert "pod_details" in k8s_context
    assert k8s_context["pod_details"]["labels"] == {"app": "app-e2e-003"}
    assert "deployment_details" in k8s_context
    assert k8s_context["deployment_details"]["name"] == "app-e2e-003"


@pytest.mark.asyncio
async def test_dl_hp_10_pdb_target_populates_pod_details_from_selector():
    """DL-HP-10: PDB target resolves pod context from PDB selector.

    DD-HAPI-018 v1.2: When the RCA target is a PodDisruptionBudget,
    _build_k8s_context reads the PDB's spec.selector.matchLabels, lists
    pods matching that selector, and populates pod_details from the first
    matched pod. This allows LabelDetector._detect_pdb to correctly set
    pdbProtected=true.
    """
    from toolsets.resource_context import GetResourceContextTool

    pdb = _make_pdb("my-pdb", {"app": "api"})
    matched_pod = _make_pod("api-pod-abc", {"app": "api", "version": "v1"}, {"sidecar.istio.io/status": "{}"})

    mock_k8s = AsyncMock()
    mock_k8s.list_pdbs = AsyncMock(return_value=([pdb], None))
    mock_k8s.list_pods_by_selector = AsyncMock(return_value=([matched_pod], None))
    mock_k8s.get_namespace_metadata = AsyncMock(return_value=_make_ns_metadata())

    tool = GetResourceContextTool(k8s_client=mock_k8s, session_state={})
    k8s_context = await tool._build_k8s_context(
        kind="PodDisruptionBudget",
        name="my-pdb",
        namespace="test-ns",
        owner_chain=[],
    )

    assert "pod_details" in k8s_context, "PDB target must populate pod_details from matched pods"
    assert k8s_context["pod_details"]["labels"]["app"] == "api"


@pytest.mark.asyncio
async def test_dl_ec_04_pdb_target_no_matching_pods():
    """DL-EC-04: PDB target with no matching pods yields no pod_details.

    DD-HAPI-018 v1.2: When the RCA target is a PDB but no pods match the
    PDB's selector, pod_details is absent. PDB detection returns
    pdbProtected=false with no failedDetections (no query failure).
    """
    from toolsets.resource_context import GetResourceContextTool

    pdb = _make_pdb("my-pdb", {"app": "api"})

    mock_k8s = AsyncMock()
    mock_k8s.list_pdbs = AsyncMock(return_value=([pdb], None))
    mock_k8s.list_pods_by_selector = AsyncMock(return_value=([], None))
    mock_k8s.get_namespace_metadata = AsyncMock(return_value=_make_ns_metadata())

    tool = GetResourceContextTool(k8s_client=mock_k8s, session_state={})
    k8s_context = await tool._build_k8s_context(
        kind="PodDisruptionBudget",
        name="my-pdb",
        namespace="test-ns",
        owner_chain=[],
    )

    assert "pod_details" not in k8s_context, "No matching pods means no pod_details"


@pytest.mark.asyncio
async def test_dl_ec_04_pdb_target_no_selector():
    """DL-EC-04 variant: PDB target with nil selector yields no pod_details.

    DD-HAPI-018 v1.2: When the PDB has no spec.selector, pod_details is
    absent. This is safe (pdbProtected=false).
    """
    from toolsets.resource_context import GetResourceContextTool

    pdb_no_selector = MagicMock()
    pdb_no_selector.metadata.name = "my-pdb"
    pdb_no_selector.spec.selector = None

    mock_k8s = AsyncMock()
    mock_k8s.list_pdbs = AsyncMock(return_value=([pdb_no_selector], None))
    mock_k8s.get_namespace_metadata = AsyncMock(return_value=_make_ns_metadata())

    tool = GetResourceContextTool(k8s_client=mock_k8s, session_state={})
    k8s_context = await tool._build_k8s_context(
        kind="PodDisruptionBudget",
        name="my-pdb",
        namespace="test-ns",
        owner_chain=[],
    )

    assert "pod_details" not in k8s_context


@pytest.mark.asyncio
async def test_pdb_target_not_found_in_namespace():
    """When the named PDB is not found in namespace, pod_details is absent.

    DD-HAPI-018 v1.2: _build_k8s_context finds PDBs by listing and matching
    by name. If the target PDB is not in the list (deleted between RCA and
    context building), context building proceeds without pod_details.
    """
    from toolsets.resource_context import GetResourceContextTool

    other_pdb = _make_pdb("other-pdb", {"app": "other"})

    mock_k8s = AsyncMock()
    mock_k8s.list_pdbs = AsyncMock(return_value=([other_pdb], None))
    mock_k8s.get_namespace_metadata = AsyncMock(return_value=_make_ns_metadata())

    tool = GetResourceContextTool(k8s_client=mock_k8s, session_state={})
    k8s_context = await tool._build_k8s_context(
        kind="PodDisruptionBudget",
        name="my-pdb",
        namespace="test-ns",
        owner_chain=[],
    )

    assert "pod_details" not in k8s_context


@pytest.mark.asyncio
async def test_regression_deployment_details_includes_annotations():
    """Regression: deployment_details must include annotations for ArgoCD v3 detection.

    Bug: resource_context.py built deployment_details with only {name, labels},
    omitting annotations. This caused LabelDetector._detect_gitops to miss
    ArgoCD v3 tracking-id on the Deployment, returning gitOpsManaged=false for
    ArgoCD-managed resources in the gitops-drift scenario.

    Fix: deployment_details now includes annotations from deploy.metadata.annotations.

    BR-HAPI-255, DD-HAPI-018 v1.3
    """
    from toolsets.resource_context import GetResourceContextTool

    argocd_annotations = {
        "argocd.argoproj.io/tracking-id": "demo-gitops-app:apps/Deployment:demo-gitops/web-frontend",
    }
    deploy = _make_deployment_with_pod_template(
        labels={"app": "web-frontend"},
        deploy_annotations=argocd_annotations,
    )

    mock_k8s = AsyncMock()
    mock_k8s.resolve_owner_chain.return_value = [
        {"kind": "Deployment", "name": "web-frontend", "namespace": "demo-gitops"},
    ]
    mock_k8s._get_resource_metadata = AsyncMock(return_value=deploy)
    mock_k8s.get_namespace_metadata = AsyncMock(return_value=_make_ns_metadata())

    tool = GetResourceContextTool(k8s_client=mock_k8s, session_state={})
    k8s_context = await tool._build_k8s_context(
        kind="Deployment",
        name="web-frontend",
        namespace="demo-gitops",
        owner_chain=[{"kind": "Deployment", "name": "web-frontend", "namespace": "demo-gitops"}],
    )

    assert "deployment_details" in k8s_context
    assert "annotations" in k8s_context["deployment_details"], (
        "deployment_details must include annotations for ArgoCD v3 tracking-id detection"
    )
    assert k8s_context["deployment_details"]["annotations"] == argocd_annotations
