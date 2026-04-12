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
Tests for K8s client label detection query methods.

ADR-056 / DD-HAPI-018: K8sResourceClient extensions for DetectedLabels detection.
These list methods provide the K8s API queries that LabelDetector needs:
  - list_pdbs(namespace) -> (list, error_str|None)
  - list_hpas(namespace) -> (list, error_str|None)
  - list_network_policies(namespace) -> (list, error_str|None)
  - list_resource_quotas(namespace) -> (list, error_str|None) [#366]

Business Requirements:
  - BR-SP-101: DetectedLabels Auto-Detection (K8s API access)
  - BR-SP-103: FailedDetections Tracking (error propagation)

Test Matrix: 15 tests
  - PDB queries: 3 tests (UT-HAPI-056-022 to UT-HAPI-056-024)
  - HPA queries: 3 tests (UT-HAPI-056-025 to UT-HAPI-056-027)
  - NetworkPolicy queries: 3 tests (UT-HAPI-056-028 to UT-HAPI-056-030)
  - Namespace metadata: 3 tests (UT-HAPI-056-031 to UT-HAPI-056-033)
  - ResourceQuota queries: 3 tests (UT-HAPI-366-001 to UT-HAPI-366-003)
"""

import pytest
from unittest.mock import MagicMock

from kubernetes.client.rest import ApiException


def _make_client(policy_v1=None, autoscaling_v2=None, networking_v1=None, core_v1=None, batch_v1=None):
    """Create a K8sResourceClient with mocked API clients, bypassing lazy init."""
    from clients.k8s_client import K8sResourceClient

    k8s = K8sResourceClient()
    k8s._initialized = True
    k8s._core_v1 = core_v1 or MagicMock()
    k8s._batch_v1 = batch_v1 or MagicMock()
    k8s._policy_v1 = policy_v1 or MagicMock()
    k8s._autoscaling_v2 = autoscaling_v2 or MagicMock()
    k8s._networking_v1 = networking_v1 or MagicMock()
    return k8s


def _make_list_response(items):
    """Create a mock K8s list response with .items attribute."""
    response = MagicMock()
    response.items = items
    return response


class TestListPDBs:
    """UT-HAPI-056-022 through UT-HAPI-056-024: PodDisruptionBudget list queries."""

    @pytest.mark.asyncio
    async def test_ut_hapi_056_022_list_pdbs_success(self):
        """UT-HAPI-056-022: list_pdbs returns PDB items and None error on success."""
        mock_policy = MagicMock()
        pdb1 = MagicMock()
        pdb1.metadata.name = "pdb-api"
        pdb2 = MagicMock()
        pdb2.metadata.name = "pdb-web"
        mock_policy.list_namespaced_pod_disruption_budget.return_value = (
            _make_list_response([pdb1, pdb2])
        )

        k8s = _make_client(policy_v1=mock_policy)
        items, error = await k8s.list_pdbs("production")

        assert error is None
        assert len(items) == 2
        assert items[0].metadata.name == "pdb-api"
        assert items[1].metadata.name == "pdb-web"
        mock_policy.list_namespaced_pod_disruption_budget.assert_called_once_with(
            namespace="production"
        )

    @pytest.mark.asyncio
    async def test_ut_hapi_056_023_list_pdbs_api_exception(self):
        """UT-HAPI-056-023: list_pdbs returns empty list and error string on ApiException (RBAC)."""
        mock_policy = MagicMock()
        mock_policy.list_namespaced_pod_disruption_budget.side_effect = ApiException(
            status=403, reason="Forbidden"
        )

        k8s = _make_client(policy_v1=mock_policy)
        items, error = await k8s.list_pdbs("production")

        assert items == []
        assert error is not None
        assert "403" in error or "Forbidden" in error

    @pytest.mark.asyncio
    async def test_ut_hapi_056_024_list_pdbs_unexpected_error(self):
        """UT-HAPI-056-024: list_pdbs returns empty list and error string on unexpected exception."""
        mock_policy = MagicMock()
        mock_policy.list_namespaced_pod_disruption_budget.side_effect = (
            ConnectionError("connection refused")
        )

        k8s = _make_client(policy_v1=mock_policy)
        items, error = await k8s.list_pdbs("production")

        assert items == []
        assert error is not None
        assert "connection refused" in error


class TestListHPAs:
    """UT-HAPI-056-025 through UT-HAPI-056-027: HorizontalPodAutoscaler list queries."""

    @pytest.mark.asyncio
    async def test_ut_hapi_056_025_list_hpas_success(self):
        """UT-HAPI-056-025: list_hpas returns HPA items and None error on success."""
        mock_autoscaling = MagicMock()
        hpa1 = MagicMock()
        hpa1.metadata.name = "hpa-api"
        mock_autoscaling.list_namespaced_horizontal_pod_autoscaler.return_value = (
            _make_list_response([hpa1])
        )

        k8s = _make_client(autoscaling_v2=mock_autoscaling)
        items, error = await k8s.list_hpas("production")

        assert error is None
        assert len(items) == 1
        assert items[0].metadata.name == "hpa-api"
        mock_autoscaling.list_namespaced_horizontal_pod_autoscaler.assert_called_once_with(
            namespace="production"
        )

    @pytest.mark.asyncio
    async def test_ut_hapi_056_026_list_hpas_api_exception(self):
        """UT-HAPI-056-026: list_hpas returns empty list and error string on ApiException (timeout)."""
        mock_autoscaling = MagicMock()
        mock_autoscaling.list_namespaced_horizontal_pod_autoscaler.side_effect = (
            ApiException(status=504, reason="Gateway Timeout")
        )

        k8s = _make_client(autoscaling_v2=mock_autoscaling)
        items, error = await k8s.list_hpas("production")

        assert items == []
        assert error is not None
        assert "504" in error or "Gateway Timeout" in error

    @pytest.mark.asyncio
    async def test_ut_hapi_056_027_list_hpas_unexpected_error(self):
        """UT-HAPI-056-027: list_hpas returns empty list and error string on unexpected exception."""
        mock_autoscaling = MagicMock()
        mock_autoscaling.list_namespaced_horizontal_pod_autoscaler.side_effect = (
            TimeoutError("context deadline exceeded")
        )

        k8s = _make_client(autoscaling_v2=mock_autoscaling)
        items, error = await k8s.list_hpas("production")

        assert items == []
        assert error is not None
        assert "context deadline exceeded" in error


class TestListNetworkPolicies:
    """UT-HAPI-056-028 through UT-HAPI-056-030: NetworkPolicy list queries."""

    @pytest.mark.asyncio
    async def test_ut_hapi_056_028_list_network_policies_success(self):
        """UT-HAPI-056-028: list_network_policies returns items and None error on success."""
        mock_networking = MagicMock()
        netpol1 = MagicMock()
        netpol1.metadata.name = "deny-all"
        mock_networking.list_namespaced_network_policy.return_value = (
            _make_list_response([netpol1])
        )

        k8s = _make_client(networking_v1=mock_networking)
        items, error = await k8s.list_network_policies("production")

        assert error is None
        assert len(items) == 1
        assert items[0].metadata.name == "deny-all"
        mock_networking.list_namespaced_network_policy.assert_called_once_with(
            namespace="production"
        )

    @pytest.mark.asyncio
    async def test_ut_hapi_056_029_list_network_policies_api_exception(self):
        """UT-HAPI-056-029: list_network_policies returns empty list and error on ApiException."""
        mock_networking = MagicMock()
        mock_networking.list_namespaced_network_policy.side_effect = ApiException(
            status=403, reason="Forbidden"
        )

        k8s = _make_client(networking_v1=mock_networking)
        items, error = await k8s.list_network_policies("production")

        assert items == []
        assert error is not None
        assert "403" in error or "Forbidden" in error

    @pytest.mark.asyncio
    async def test_ut_hapi_056_030_list_network_policies_unexpected_error(self):
        """UT-HAPI-056-030: list_network_policies returns empty list and error on unexpected exception."""
        mock_networking = MagicMock()
        mock_networking.list_namespaced_network_policy.side_effect = (
            OSError("network unreachable")
        )

        k8s = _make_client(networking_v1=mock_networking)
        items, error = await k8s.list_network_policies("production")

        assert items == []
        assert error is not None
        assert "network unreachable" in error


class TestGetNamespaceMetadata:
    """UT-HAPI-056-031 through UT-HAPI-056-033: Namespace metadata queries."""

    @pytest.mark.asyncio
    async def test_ut_hapi_056_031_get_namespace_metadata_success(self):
        """UT-HAPI-056-031: get_namespace_metadata returns labels and annotations on success."""
        mock_core = MagicMock()
        mock_ns = MagicMock()
        mock_ns.metadata.labels = {
            "argocd.argoproj.io/instance": "cluster-apps",
            "kubernetes.io/metadata.name": "production",
        }
        mock_ns.metadata.annotations = {
            "fluxcd.io/sync-status": "synced",
        }
        mock_core.read_namespace.return_value = mock_ns

        k8s = _make_client(core_v1=mock_core)
        result = await k8s.get_namespace_metadata("production")

        assert result is not None
        assert result["labels"] == {
            "argocd.argoproj.io/instance": "cluster-apps",
            "kubernetes.io/metadata.name": "production",
        }
        assert result["annotations"] == {"fluxcd.io/sync-status": "synced"}
        mock_core.read_namespace.assert_called_once_with(name="production")

    @pytest.mark.asyncio
    async def test_ut_hapi_056_032_get_namespace_metadata_api_exception(self):
        """UT-HAPI-056-032: get_namespace_metadata returns None on ApiException (not found/RBAC)."""
        mock_core = MagicMock()
        mock_core.read_namespace.side_effect = ApiException(
            status=404, reason="Not Found"
        )

        k8s = _make_client(core_v1=mock_core)
        result = await k8s.get_namespace_metadata("missing-ns")

        assert result is None

    @pytest.mark.asyncio
    async def test_ut_hapi_056_033_get_namespace_metadata_unexpected_error(self):
        """UT-HAPI-056-033: get_namespace_metadata returns None on unexpected exception."""
        mock_core = MagicMock()
        mock_core.read_namespace.side_effect = ConnectionError("connection refused")

        k8s = _make_client(core_v1=mock_core)
        result = await k8s.get_namespace_metadata("production")

        assert result is None


class TestListResourceQuotas:
    """UT-HAPI-366-001 through UT-HAPI-366-003: ResourceQuota list queries (#366)."""

    @pytest.mark.asyncio
    async def test_ut_hapi_366_001_list_resource_quotas_success(self):
        """UT-HAPI-366-001: list_resource_quotas returns quota items when quotas exist in namespace."""
        mock_core = MagicMock()
        quota1 = MagicMock()
        quota1.metadata.name = "compute-quota"
        quota1.spec.hard = {"cpu": "4", "memory": "8Gi"}
        quota1.status.hard = {"cpu": "4", "memory": "8Gi"}
        quota1.status.used = {"cpu": "2500m", "memory": "6Gi"}
        mock_core.list_namespaced_resource_quota.return_value = (
            _make_list_response([quota1])
        )

        k8s = _make_client(core_v1=mock_core)
        items, error = await k8s.list_resource_quotas("production")

        assert error is None
        assert len(items) == 1
        assert items[0].metadata.name == "compute-quota"
        assert items[0].spec.hard == {"cpu": "4", "memory": "8Gi"}
        mock_core.list_namespaced_resource_quota.assert_called_once_with(
            namespace="production"
        )

    @pytest.mark.asyncio
    async def test_ut_hapi_366_002_list_resource_quotas_empty(self):
        """UT-HAPI-366-002: list_resource_quotas returns empty list when no quotas exist."""
        mock_core = MagicMock()
        mock_core.list_namespaced_resource_quota.return_value = (
            _make_list_response([])
        )

        k8s = _make_client(core_v1=mock_core)
        items, error = await k8s.list_resource_quotas("default")

        assert error is None
        assert items == []

    @pytest.mark.asyncio
    async def test_ut_hapi_366_003_list_resource_quotas_api_exception(self):
        """UT-HAPI-366-003: list_resource_quotas returns error string on K8s API failure."""
        mock_core = MagicMock()
        mock_core.list_namespaced_resource_quota.side_effect = ApiException(
            status=403, reason="Forbidden"
        )

        k8s = _make_client(core_v1=mock_core)
        items, error = await k8s.list_resource_quotas("production")

        assert items == []
        assert error is not None
        assert "403" in error or "Forbidden" in error


class TestGetResourceMetadataNonWorkload:
    """UT-HAPI-676: _get_resource_metadata_sync support for non-workload resource kinds.

    Issue #676: _get_resource_metadata_sync only supports workload kinds (Deployment,
    StatefulSet, etc.) and returns None for ConfigMap, Secret, Service, Job.
    Business Requirement: BR-HAPI-250
    """

    @pytest.mark.asyncio
    async def test_ut_hapi_676_005_configmap_metadata(self):
        """UT-HAPI-676-005: _get_resource_metadata_sync returns ConfigMap (not None).

        BR-HAPI-250: K8s client must resolve ConfigMap metadata to enable
        label detection on ConfigMap root owners.
        """
        mock_core = MagicMock()
        mock_cm = MagicMock()
        mock_cm.metadata.labels = {"app.kubernetes.io/managed-by": "Helm"}
        mock_core.read_namespaced_config_map.return_value = mock_cm

        k8s = _make_client(core_v1=mock_core)
        result = await k8s._get_resource_metadata("ConfigMap", "worker-config", "default")

        assert result is not None
        assert result.metadata.labels["app.kubernetes.io/managed-by"] == "Helm"

    @pytest.mark.asyncio
    async def test_ut_hapi_676_006_secret_metadata(self):
        """UT-HAPI-676-006: _get_resource_metadata_sync returns Secret (not None).

        BR-HAPI-250: K8s client must resolve Secret metadata for label detection.
        """
        mock_core = MagicMock()
        mock_secret = MagicMock()
        mock_secret.metadata.labels = {"app.kubernetes.io/managed-by": "Helm"}
        mock_core.read_namespaced_secret.return_value = mock_secret

        k8s = _make_client(core_v1=mock_core)
        result = await k8s._get_resource_metadata("Secret", "db-creds", "default")

        assert result is not None
        assert result.metadata.labels["app.kubernetes.io/managed-by"] == "Helm"

    @pytest.mark.asyncio
    async def test_ut_hapi_676_007_service_metadata(self):
        """UT-HAPI-676-007: _get_resource_metadata_sync returns Service (not None).

        BR-HAPI-250: K8s client must resolve Service metadata for label detection.
        """
        mock_core = MagicMock()
        mock_svc = MagicMock()
        mock_svc.metadata.labels = {"app.kubernetes.io/managed-by": "Helm"}
        mock_core.read_namespaced_service.return_value = mock_svc

        k8s = _make_client(core_v1=mock_core)
        result = await k8s._get_resource_metadata("Service", "api-svc", "default")

        assert result is not None
        assert result.metadata.labels["app.kubernetes.io/managed-by"] == "Helm"

    @pytest.mark.asyncio
    async def test_ut_hapi_676_008_job_metadata(self):
        """UT-HAPI-676-008: _get_resource_metadata_sync returns Job (not None).

        BR-HAPI-250: K8s client must resolve Job metadata for label detection.
        """
        mock_batch = MagicMock()
        mock_job = MagicMock()
        mock_job.metadata.labels = {"app.kubernetes.io/managed-by": "Helm"}
        mock_batch.read_namespaced_job.return_value = mock_job

        k8s = _make_client(batch_v1=mock_batch)
        result = await k8s._get_resource_metadata("Job", "data-migration", "default")

        assert result is not None
        assert result.metadata.labels["app.kubernetes.io/managed-by"] == "Helm"
