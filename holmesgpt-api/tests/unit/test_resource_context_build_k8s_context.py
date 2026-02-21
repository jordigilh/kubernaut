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

E2E-AA-056-003: When the RCA target is a Deployment, pod_details must be
populated from the Deployment's pod template labels so that PDB detection
can match PDB selectors against the labels that would appear on Pods.
"""

import pytest
from unittest.mock import AsyncMock, MagicMock


def _make_deployment_with_pod_template(labels, annotations=None):
    """Build a mock Deployment with spec.template.metadata.labels."""
    deploy = MagicMock()
    deploy.metadata.labels = {"app": "api"}
    deploy.spec = MagicMock()
    deploy.spec.template = MagicMock()
    deploy.spec.template.metadata = MagicMock()
    deploy.spec.template.metadata.labels = labels or {}
    deploy.spec.template.metadata.annotations = annotations or {}
    return deploy


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
    mock_k8s.get_namespace_metadata = AsyncMock(
        return_value={"labels": {}, "annotations": {}}
    )

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
