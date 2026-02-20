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
DD-HAPI-018 Conformance Tests: DetectedLabels Detection Specification.

Cross-language contract extracted from SP's Go implementation.
Both SP (Go) and HAPI (Python) implementations MUST pass these test vectors.

Business Requirements:
  - BR-HAPI-250: DetectedLabels integration with Data Storage
  - BR-HAPI-252: DetectedLabels in workflow search
  - BR-SP-101:   DetectedLabels Auto-Detection (reference)
  - BR-SP-103:   FailedDetections Tracking

Test Matrix: 19 tests
  - Happy Path: 14 tests (UT-HAPI-056-001 through UT-HAPI-056-014)
  - Edge Cases: 3 tests (UT-HAPI-056-015 through UT-HAPI-056-017)
  - Error Handling: 4 tests (UT-HAPI-056-018 through UT-HAPI-056-021)

Reference: docs/architecture/decisions/DD-HAPI-018-detected-labels-detection-specification.md
"""

import asyncio

import pytest
from unittest.mock import AsyncMock, MagicMock


def _make_k8s_queries(
    pdbs=None, pdbs_error=None,
    hpas=None, hpas_error=None,
    netpols=None, netpols_error=None,
):
    """Build a mock K8s queries object for LabelDetector."""
    queries = AsyncMock()
    queries.list_pdbs = AsyncMock(return_value=(pdbs or [], pdbs_error))
    queries.list_hpas = AsyncMock(return_value=(hpas or [], hpas_error))
    queries.list_network_policies = AsyncMock(return_value=(netpols or [], netpols_error))
    return queries


def _make_pdb(selector_match_labels):
    """Build a mock PDB with the given selector matchLabels."""
    pdb = MagicMock()
    pdb.spec.selector.match_labels = selector_match_labels
    pdb.spec.selector.match_expressions = None
    return pdb


def _make_hpa(target_kind, target_name):
    """Build a mock HPA with the given scaleTargetRef."""
    hpa = MagicMock()
    hpa.spec.scale_target_ref.kind = target_kind
    hpa.spec.scale_target_ref.name = target_name
    return hpa


class TestLabelDetectorHappyPath:
    """UT-HAPI-056-001 through UT-HAPI-056-014: Happy path detection vectors."""

    @pytest.mark.asyncio
    async def test_ut_hapi_056_001_argocd_gitops(self):
        """UT-HAPI-056-001: Pod annotation argocd.argoproj.io/instance -> gitOpsManaged=true, gitOpsTool=argocd."""
        from detection.labels import LabelDetector

        queries = _make_k8s_queries()
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "pod_details": {
                "name": "api-pod-abc",
                "labels": {"app": "api"},
                "annotations": {"argocd.argoproj.io/instance": "my-app"},
            },
            "deployment_details": {
                "name": "api-deployment",
                "labels": {"app": "api"},
            },
        }
        owner_chain = []

        result = await detector.detect_labels(k8s_context, owner_chain)

        assert result is not None
        assert result["gitOpsManaged"] is True
        assert result["gitOpsTool"] == "argocd"
        assert result["failedDetections"] == []

    @pytest.mark.asyncio
    async def test_ut_hapi_056_003_flux_gitops(self):
        """UT-HAPI-056-003: Deployment label fluxcd.io/sync-gc-mark -> gitOpsManaged=true, gitOpsTool=flux."""
        from detection.labels import LabelDetector

        queries = _make_k8s_queries()
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "deployment_details": {
                "name": "api-deployment",
                "labels": {"fluxcd.io/sync-gc-mark": "sha256:abc123"},
            },
        }
        owner_chain = []

        result = await detector.detect_labels(k8s_context, owner_chain)

        assert result is not None
        assert result["gitOpsManaged"] is True
        assert result["gitOpsTool"] == "flux"

    @pytest.mark.asyncio
    async def test_ut_hapi_056_004_namespace_argocd_label(self):
        """UT-HAPI-056-004: Namespace label argocd.argoproj.io/instance -> gitOpsManaged=true, gitOpsTool=argocd."""
        from detection.labels import LabelDetector

        queries = _make_k8s_queries()
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "namespace_labels": {"argocd.argoproj.io/instance": "my-app"},
            "deployment_details": {"name": "api", "labels": {}},
        }

        result = await detector.detect_labels(k8s_context, [])

        assert result["gitOpsManaged"] is True
        assert result["gitOpsTool"] == "argocd"

    @pytest.mark.asyncio
    async def test_ut_hapi_056_005_namespace_flux_annotation(self):
        """UT-HAPI-056-005: Namespace annotation fluxcd.io/sync-status -> gitOpsManaged=true, gitOpsTool=flux."""
        from detection.labels import LabelDetector

        queries = _make_k8s_queries()
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "namespace_annotations": {"fluxcd.io/sync-status": "synced"},
            "deployment_details": {"name": "api", "labels": {}},
        }

        result = await detector.detect_labels(k8s_context, [])

        assert result["gitOpsManaged"] is True
        assert result["gitOpsTool"] == "flux"

    @pytest.mark.asyncio
    async def test_ut_hapi_056_006_namespace_precedence_label_over_annotation(self):
        """UT-HAPI-056-006: Namespace Flux label takes precedence over ArgoCD annotation (labels > annotations)."""
        from detection.labels import LabelDetector

        queries = _make_k8s_queries()
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "namespace_labels": {"fluxcd.io/sync-gc-mark": "sha256:xyz"},
            "namespace_annotations": {"argocd.argoproj.io/managed": "true"},
            "deployment_details": {"name": "api", "labels": {}},
        }

        result = await detector.detect_labels(k8s_context, [])

        assert result["gitOpsManaged"] is True
        assert result["gitOpsTool"] == "flux"

    @pytest.mark.asyncio
    async def test_ut_hapi_056_002_argocd_deployment_label(self):
        """UT-HAPI-056-002: Deployment label argocd.argoproj.io/instance -> gitOpsManaged=true, gitOpsTool=argocd.

        Covers the branch at labels.py line 112: deployment has ArgoCD label,
        pod has no ArgoCD annotation, and no Flux labels present.
        """
        from detection.labels import LabelDetector

        queries = _make_k8s_queries()
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "pod_details": {
                "name": "api-pod-xyz",
                "labels": {"app": "api"},
                "annotations": {},
            },
            "deployment_details": {
                "name": "api",
                "labels": {"argocd.argoproj.io/instance": "my-app"},
            },
        }

        result = await detector.detect_labels(k8s_context, [])

        assert result is not None
        assert result["gitOpsManaged"] is True
        assert result["gitOpsTool"] == "argocd"

    @pytest.mark.asyncio
    async def test_ut_hapi_056_007_pdb_protected(self):
        """UT-HAPI-056-007: PDB selector matches pod labels -> pdbProtected=true."""
        from detection.labels import LabelDetector

        pdb = _make_pdb({"app": "api"})
        queries = _make_k8s_queries(pdbs=[pdb])
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "pod_details": {
                "name": "api-pod-abc",
                "labels": {"app": "api"},
                "annotations": {},
            },
        }
        owner_chain = []

        result = await detector.detect_labels(k8s_context, owner_chain)

        assert result is not None
        assert result["pdbProtected"] is True
        assert "pdbProtected" not in result["failedDetections"]

    @pytest.mark.asyncio
    async def test_ut_hapi_056_008_hpa_enabled(self):
        """UT-HAPI-056-008: HPA targets Deployment -> hpaEnabled=true."""
        from detection.labels import LabelDetector

        hpa = _make_hpa("Deployment", "api-deployment")
        queries = _make_k8s_queries(hpas=[hpa])
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "deployment_details": {
                "name": "api-deployment",
                "labels": {},
            },
        }
        owner_chain = []

        result = await detector.detect_labels(k8s_context, owner_chain)

        assert result is not None
        assert result["hpaEnabled"] is True
        assert "hpaEnabled" not in result["failedDetections"]

    @pytest.mark.asyncio
    async def test_ut_hapi_056_009_statefulset_owner(self):
        """UT-HAPI-056-009: Owner chain contains StatefulSet -> stateful=true."""
        from detection.labels import LabelDetector

        queries = _make_k8s_queries()
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "pod_details": {
                "name": "db-pod-0",
                "labels": {},
                "annotations": {},
            },
        }
        owner_chain = [
            {"kind": "StatefulSet", "name": "db", "namespace": "prod"},
        ]

        result = await detector.detect_labels(k8s_context, owner_chain)

        assert result is not None
        assert result["stateful"] is True

    @pytest.mark.asyncio
    async def test_ut_hapi_056_010_helm_managed(self):
        """UT-HAPI-056-010: Deployment label app.kubernetes.io/managed-by=Helm -> helmManaged=true."""
        from detection.labels import LabelDetector

        queries = _make_k8s_queries()
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "deployment_details": {
                "name": "api-deployment",
                "labels": {
                    "app.kubernetes.io/managed-by": "Helm",
                    "helm.sh/chart": "api-1.0.0",
                },
            },
        }
        owner_chain = []

        result = await detector.detect_labels(k8s_context, owner_chain)

        assert result is not None
        assert result["helmManaged"] is True

    @pytest.mark.asyncio
    async def test_ut_hapi_056_011_helm_chart_label_only(self):
        """UT-HAPI-056-011: Deployment label helm.sh/chart (without managed-by=Helm) -> helmManaged=true.

        Covers the branch at labels.py line 224: only helm.sh/chart present,
        app.kubernetes.io/managed-by is absent or not 'Helm'.
        """
        from detection.labels import LabelDetector

        queries = _make_k8s_queries()
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "deployment_details": {
                "name": "api-deployment",
                "labels": {
                    "helm.sh/chart": "api-1.0.0",
                },
            },
        }

        result = await detector.detect_labels(k8s_context, [])

        assert result is not None
        assert result["helmManaged"] is True

    @pytest.mark.asyncio
    async def test_ut_hapi_056_012_network_isolated(self):
        """UT-HAPI-056-012: NetworkPolicy exists in namespace -> networkIsolated=true."""
        from detection.labels import LabelDetector

        netpol = MagicMock()
        netpol.metadata.name = "deny-all"
        queries = _make_k8s_queries(netpols=[netpol])
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
        }
        owner_chain = []

        result = await detector.detect_labels(k8s_context, owner_chain)

        assert result is not None
        assert result["networkIsolated"] is True
        assert "networkIsolated" not in result["failedDetections"]

    @pytest.mark.asyncio
    async def test_ut_hapi_056_013_istio_service_mesh(self):
        """UT-HAPI-056-013: Pod annotation sidecar.istio.io/status -> serviceMesh=istio."""
        from detection.labels import LabelDetector

        queries = _make_k8s_queries()
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "pod_details": {
                "name": "api-pod-abc",
                "labels": {},
                "annotations": {"sidecar.istio.io/status": '{"version":"1.18.0"}'},
            },
        }
        owner_chain = []

        result = await detector.detect_labels(k8s_context, owner_chain)

        assert result is not None
        assert result["serviceMesh"] == "istio"

    @pytest.mark.asyncio
    async def test_ut_hapi_056_014_linkerd_service_mesh(self):
        """UT-HAPI-056-014: Pod annotation linkerd.io/proxy-version -> serviceMesh=linkerd."""
        from detection.labels import LabelDetector

        queries = _make_k8s_queries()
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "pod_details": {
                "name": "api-pod-abc",
                "labels": {},
                "annotations": {"linkerd.io/proxy-version": "stable-2.14.0"},
            },
        }
        owner_chain = []

        result = await detector.detect_labels(k8s_context, owner_chain)

        assert result is not None
        assert result["serviceMesh"] == "linkerd"


class TestLabelDetectorEdgeCases:
    """UT-HAPI-056-015 through UT-HAPI-056-017: Edge case vectors."""

    @pytest.mark.asyncio
    async def test_ut_hapi_056_015_plain_deployment(self):
        """UT-HAPI-056-015: Plain deployment with no special features -> all false/empty."""
        from detection.labels import LabelDetector

        queries = _make_k8s_queries()
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "default",
            "deployment_details": {
                "name": "simple-app",
                "labels": {"app": "simple"},
            },
            "pod_details": {
                "name": "simple-app-pod",
                "labels": {"app": "simple"},
                "annotations": {},
            },
        }
        owner_chain = [
            {"kind": "ReplicaSet", "name": "simple-app-rs", "namespace": "default"},
            {"kind": "Deployment", "name": "simple-app", "namespace": "default"},
        ]

        result = await detector.detect_labels(k8s_context, owner_chain)

        assert result is not None
        assert result["gitOpsManaged"] is False
        assert result["gitOpsTool"] == ""
        assert result["pdbProtected"] is False
        assert result["hpaEnabled"] is False
        assert result["stateful"] is False
        assert result["helmManaged"] is False
        assert result["networkIsolated"] is False
        assert result["serviceMesh"] == ""
        assert result["failedDetections"] == []

    @pytest.mark.asyncio
    async def test_ut_hapi_056_016_none_context(self):
        """UT-HAPI-056-016: None KubernetesContext -> returns None."""
        from detection.labels import LabelDetector

        queries = _make_k8s_queries()
        detector = LabelDetector(queries)

        result = await detector.detect_labels(None, None)

        assert result is None

    @pytest.mark.asyncio
    async def test_ut_hapi_056_017_multiple_detections(self):
        """UT-HAPI-056-017: ArgoCD + PDB + HPA all present -> all three true."""
        from detection.labels import LabelDetector

        pdb = _make_pdb({"app": "api"})
        hpa = _make_hpa("Deployment", "api-deployment")
        queries = _make_k8s_queries(pdbs=[pdb], hpas=[hpa])
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "deployment_details": {
                "name": "api-deployment",
                "labels": {"app": "api"},
            },
            "pod_details": {
                "name": "api-pod",
                "labels": {"app": "api"},
                "annotations": {"argocd.argoproj.io/instance": "my-app"},
            },
        }
        owner_chain = []

        result = await detector.detect_labels(k8s_context, owner_chain)

        assert result is not None
        assert result["gitOpsManaged"] is True
        assert result["gitOpsTool"] == "argocd"
        assert result["pdbProtected"] is True
        assert result["hpaEnabled"] is True
        assert result["failedDetections"] == []


class TestLabelDetectorErrorHandling:
    """UT-HAPI-056-018 through UT-HAPI-056-021: Error handling vectors."""

    @pytest.mark.asyncio
    async def test_ut_hapi_056_018_pdb_rbac_forbidden(self):
        """UT-HAPI-056-018: PDB query returns RBAC forbidden -> pdbProtected=false, failedDetections=[pdbProtected]."""
        from detection.labels import LabelDetector

        queries = _make_k8s_queries(pdbs_error="forbidden: User cannot list poddisruptionbudgets")
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "pod_details": {
                "name": "api-pod",
                "labels": {"app": "api"},
                "annotations": {},
            },
        }
        owner_chain = []

        result = await detector.detect_labels(k8s_context, owner_chain)

        assert result is not None
        assert result["pdbProtected"] is False
        assert "pdbProtected" in result["failedDetections"]

    @pytest.mark.asyncio
    async def test_ut_hapi_056_019_hpa_timeout(self):
        """UT-HAPI-056-019: HPA query returns timeout -> hpaEnabled=false, failedDetections=[hpaEnabled]."""
        from detection.labels import LabelDetector

        queries = _make_k8s_queries(hpas_error="context deadline exceeded")
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "deployment_details": {
                "name": "api-deployment",
                "labels": {},
            },
        }
        owner_chain = []

        result = await detector.detect_labels(k8s_context, owner_chain)

        assert result is not None
        assert result["hpaEnabled"] is False
        assert "hpaEnabled" in result["failedDetections"]

    @pytest.mark.asyncio
    async def test_ut_hapi_056_020_all_queries_fail(self):
        """UT-HAPI-056-020: PDB + HPA + NetworkPolicy all fail -> failedDetections has all three."""
        from detection.labels import LabelDetector

        queries = _make_k8s_queries(
            pdbs_error="RBAC: access denied",
            hpas_error="context deadline exceeded",
            netpols_error="connection refused",
        )
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "deployment_details": {
                "name": "api-deployment",
                "labels": {},
            },
            "pod_details": {
                "name": "api-pod",
                "labels": {"app": "api"},
                "annotations": {},
            },
        }
        owner_chain = []

        result = await detector.detect_labels(k8s_context, owner_chain)

        assert result is not None
        assert result["pdbProtected"] is False
        assert result["hpaEnabled"] is False
        assert result["networkIsolated"] is False
        assert "pdbProtected" in result["failedDetections"]
        assert "hpaEnabled" in result["failedDetections"]
        assert "networkIsolated" in result["failedDetections"]

    @pytest.mark.asyncio
    async def test_ut_hapi_056_021_context_cancellation(self):
        """UT-HAPI-056-021: HPA query raises cancellation -> partial results, hpaEnabled in failedDetections."""
        from detection.labels import LabelDetector

        queries = _make_k8s_queries()
        queries.list_hpas = AsyncMock(side_effect=asyncio.CancelledError())
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "deployment_details": {
                "name": "api-deployment",
                "labels": {},
            },
        }
        owner_chain = []

        result = await detector.detect_labels(k8s_context, owner_chain)

        assert result is not None
        assert "hpaEnabled" in result["failedDetections"]


class TestLabelDetectorBranchGaps:
    """UT-HAPI-056-077 through UT-HAPI-056-080: Branch coverage gap fills."""

    @pytest.mark.asyncio
    async def test_ut_hapi_056_077_argocd_namespace_annotation(self):
        """UT-HAPI-056-077: Namespace annotation argocd.argoproj.io/managed -> gitOpsManaged=true (lowest precedence).

        Given namespace has ArgoCD annotation but no pod/deploy/ns-label markers
        When LabelDetector.detect_labels is called
        Then gitOpsManaged=true, gitOpsTool=argocd
        """
        from detection.labels import LabelDetector

        queries = _make_k8s_queries()
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "pod_details": {
                "name": "api-pod",
                "labels": {"app": "api"},
                "annotations": {},
            },
            "deployment_details": {
                "name": "api",
                "labels": {"app": "api"},
            },
            "namespace_labels": {},
            "namespace_annotations": {"argocd.argoproj.io/managed": "true"},
        }

        result = await detector.detect_labels(k8s_context, [])

        assert result is not None
        assert result["gitOpsManaged"] is True
        assert result["gitOpsTool"] == "argocd"

    @pytest.mark.asyncio
    async def test_ut_hapi_056_078_pdb_selector_no_match(self):
        """UT-HAPI-056-078: PDB exists but selector doesn't match pod labels -> pdbProtected=false.

        Given Pod labels {"app": "api"} and PDB selector {"app": "frontend"}
        When LabelDetector.detect_labels is called
        Then pdbProtected=false (no false positive), failedDetections empty
        """
        from detection.labels import LabelDetector

        pdb = _make_pdb({"app": "frontend"})
        queries = _make_k8s_queries(pdbs=[pdb])
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "pod_details": {
                "name": "api-pod",
                "labels": {"app": "api"},
                "annotations": {},
            },
        }

        result = await detector.detect_labels(k8s_context, [])

        assert result is not None
        assert result["pdbProtected"] is False
        assert "pdbProtected" not in result["failedDetections"]

    @pytest.mark.asyncio
    async def test_ut_hapi_056_079_pdb_selector_none(self):
        """UT-HAPI-056-079: PDB with selector=None -> pdbProtected=false, no crash.

        Given PDB exists but spec.selector is None
        When LabelDetector.detect_labels is called
        Then pdbProtected=false (gracefully skipped), no AttributeError
        """
        from detection.labels import LabelDetector

        pdb = MagicMock()
        pdb.spec.selector = None
        queries = _make_k8s_queries(pdbs=[pdb])
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "pod_details": {
                "name": "api-pod",
                "labels": {"app": "api"},
                "annotations": {},
            },
        }

        result = await detector.detect_labels(k8s_context, [])

        assert result is not None
        assert result["pdbProtected"] is False
        assert "pdbProtected" not in result["failedDetections"]

    @pytest.mark.asyncio
    async def test_ut_hapi_056_080_hpa_targets_different_deployment(self):
        """UT-HAPI-056-080: HPA exists but targets different deployment -> hpaEnabled=false.

        Given Deployment "api" and HPA targeting "frontend-deployment"
        When LabelDetector.detect_labels is called
        Then hpaEnabled=false (no false positive), failedDetections empty
        """
        from detection.labels import LabelDetector

        hpa = _make_hpa("Deployment", "frontend-deployment")
        queries = _make_k8s_queries(hpas=[hpa])
        detector = LabelDetector(queries)

        k8s_context = {
            "namespace": "prod",
            "deployment_details": {
                "name": "api",
                "labels": {"app": "api"},
            },
        }

        result = await detector.detect_labels(k8s_context, [])

        assert result is not None
        assert result["hpaEnabled"] is False
        assert "hpaEnabled" not in result["failedDetections"]
