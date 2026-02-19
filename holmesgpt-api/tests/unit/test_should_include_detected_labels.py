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
Unit tests for should_include_detected_labels safety gate.

Business Requirements:
  - BR-SP-101: DetectedLabels auto-detection
  - BR-HAPI-194: Honor failedDetections in workflow filtering

Design Decision: DD-WORKFLOW-001 v1.7

Test Matrix: 9 tests (UT-HAPI-056-068 through UT-HAPI-056-076)
  - Required data checks: 3 tests (068-070) -- GATE 1
  - Exact match: 1 test (071) -- GATE 2
  - Owner chain: 1 test (072) -- GATE 3
  - Fallback: 3 tests (073-075) -- GATE 4
  - Default exclude: 1 test (076) -- safe default
"""

from toolsets.workflow_discovery import should_include_detected_labels


class TestGate1RequiredDataChecks:
    """GATE 1: Required data must be present. Missing data -> safe exclude."""

    def test_ut_hapi_056_068_source_resource_missing(self):
        """UT-HAPI-056-068: Labels excluded when signal source resource is unknown.

        Given source_resource is None
        When should_include_detected_labels is called
        Then returns False (safe default)
        """
        rca_resource = {"kind": "Pod", "namespace": "prod", "name": "api-xyz"}

        result = should_include_detected_labels(None, rca_resource)

        assert result is False

    def test_ut_hapi_056_069_rca_resource_missing(self):
        """UT-HAPI-056-069: Labels excluded when LLM provides no RCA resource.

        Given rca_resource is None
        When should_include_detected_labels is called
        Then returns False (safe default)
        """
        source_resource = {"kind": "Pod", "namespace": "prod", "name": "api-xyz"}

        result = should_include_detected_labels(source_resource, None)

        assert result is False

    def test_ut_hapi_056_070_rca_resource_kind_missing(self):
        """UT-HAPI-056-070: Labels excluded when RCA resource has no kind field.

        Given rca_resource has no 'kind' key
        When should_include_detected_labels is called
        Then returns False (cannot validate relationship without kind)
        """
        source_resource = {"kind": "Pod", "namespace": "prod", "name": "api-xyz"}
        rca_resource = {"namespace": "prod", "name": "api"}

        result = should_include_detected_labels(source_resource, rca_resource)

        assert result is False


class TestGate2ExactMatch:
    """GATE 2: Exact resource match (same kind, namespace, name) -> include."""

    def test_ut_hapi_056_071_exact_resource_match(self):
        """UT-HAPI-056-071: Labels included when LLM identifies exact same resource.

        Given source_resource and rca_resource have identical kind, namespace, name
        When should_include_detected_labels is called
        Then returns True (proven match, no owner_chain needed)
        """
        source = {"kind": "Pod", "namespace": "prod", "name": "api-xyz"}
        rca = {"kind": "Pod", "namespace": "prod", "name": "api-xyz"}

        result = should_include_detected_labels(source, rca)

        assert result is True


class TestGate3OwnerChainMatch:
    """GATE 3: RCA resource found in owner chain -> proven relationship -> include."""

    def test_ut_hapi_056_072_owner_chain_match(self):
        """UT-HAPI-056-072: Labels included when RCA resource is a proven owner.

        Given Pod source, Deployment RCA, and owner_chain contains that Deployment
        When should_include_detected_labels is called
        Then returns True (K8s ownerReferences prove the relationship)
        """
        source = {"kind": "Pod", "namespace": "prod", "name": "api-xyz"}
        rca = {"kind": "Deployment", "namespace": "prod", "name": "api"}
        owner_chain = [
            {"kind": "ReplicaSet", "namespace": "prod", "name": "api-rs-abc"},
            {"kind": "Deployment", "namespace": "prod", "name": "api"},
        ]

        result = should_include_detected_labels(source, rca, owner_chain)

        assert result is True


class TestGate4FallbackRules:
    """GATE 4: Same namespace+kind fallback when owner_chain is provided."""

    def test_ut_hapi_056_073_same_namespace_kind_fallback(self):
        """UT-HAPI-056-073: Labels included for sibling resources (same namespace+kind).

        Given two different Pods in the same namespace with empty owner_chain
        When should_include_detected_labels is called
        Then returns True (conservative include for sibling resources)
        """
        source = {"kind": "Pod", "namespace": "prod", "name": "api-abc"}
        rca = {"kind": "Pod", "namespace": "prod", "name": "api-def"}
        owner_chain = []

        result = should_include_detected_labels(source, rca, owner_chain)

        assert result is True

    def test_ut_hapi_056_074_cluster_scoped_same_kind(self):
        """UT-HAPI-056-074: Labels included for cluster-scoped resources with same kind.

        Given two Nodes (cluster-scoped, no namespace) with empty owner_chain
        When should_include_detected_labels is called
        Then returns True (same kind is sufficient for cluster-scoped resources)
        """
        source = {"kind": "Node", "name": "worker-1"}
        rca = {"kind": "Node", "name": "worker-2"}
        owner_chain = []

        result = should_include_detected_labels(source, rca, owner_chain)

        assert result is True

    def test_ut_hapi_056_075_cross_scope_mismatch(self):
        """UT-HAPI-056-075: Labels excluded when scopes differ (namespaced vs cluster).

        Given namespaced Pod and cluster-scoped Node with empty owner_chain
        When should_include_detected_labels is called
        Then returns False (different scope, cannot be related)
        """
        source = {"kind": "Pod", "namespace": "prod", "name": "api-xyz"}
        rca = {"kind": "Node", "name": "worker-3"}
        owner_chain = []

        result = should_include_detected_labels(source, rca, owner_chain)

        assert result is False


class TestDefaultExclude:
    """Default: Cannot prove relationship -> EXCLUDE (100% safe)."""

    def test_ut_hapi_056_076_default_exclude_no_relationship(self):
        """UT-HAPI-056-076: Labels excluded when no relationship can be proven.

        Given Pod source and Deployment RCA with owner_chain=None (not provided)
        When should_include_detected_labels is called
        Then returns False (GATE 4 skipped because owner_chain is None, not empty)
        """
        source = {"kind": "Pod", "namespace": "prod", "name": "api-xyz"}
        rca = {"kind": "Deployment", "namespace": "prod", "name": "api"}

        result = should_include_detected_labels(source, rca, None)

        assert result is False
