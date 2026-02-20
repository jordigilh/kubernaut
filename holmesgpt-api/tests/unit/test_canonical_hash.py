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
Tests for canonical spec hash (Python port of Go pkg/shared/hash/canonical.go).

BR-HAPI-016: Remediation history context for LLM prompt enrichment.
DD-HAPI-016 v1.1: Python HAPI needs canonical hash for currentSpecHash parameter.
DD-EM-002: Cross-language deterministic hashing of Kubernetes resource specs.

Test vectors are computed from the Go implementation to ensure cross-language
compatibility. The Python implementation MUST produce identical hashes.
"""

import pytest


class TestCanonicalSpecHash:
    """UT-HASH-001 through UT-HASH-007: Canonical spec hash cross-language tests."""

    def test_empty_spec_matches_go(self):
        """UT-HASH-001: Empty/None spec produces same hash as Go nil spec."""
        from src.utils.canonical_hash import canonical_spec_hash

        result = canonical_spec_hash(None)
        assert result == "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"

    def test_empty_dict_matches_go(self):
        """UT-HASH-002: Empty dict produces same hash as Go empty map."""
        from src.utils.canonical_hash import canonical_spec_hash

        result = canonical_spec_hash({})
        assert result == "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"

    def test_simple_spec_matches_go(self):
        """UT-HASH-003: Simple spec with nested map produces same hash as Go."""
        from src.utils.canonical_hash import canonical_spec_hash

        spec = {
            "replicas": 3,
            "selector": {
                "matchLabels": {
                    "app": "nginx",
                },
            },
        }
        result = canonical_spec_hash(spec)
        assert result == "sha256:d9acfeef7efdd936aba3136e9e05bc4078a79c7200be4e2c0c4d3ec5c3f2e360"

    def test_map_order_independence(self):
        """UT-HASH-004: Different insertion order produces identical hash (Go compatibility)."""
        from src.utils.canonical_hash import canonical_spec_hash

        spec_a = {
            "replicas": 3,
            "selector": {"matchLabels": {"app": "nginx"}},
        }
        spec_b = {
            "selector": {"matchLabels": {"app": "nginx"}},
            "replicas": 3,
        }
        assert canonical_spec_hash(spec_a) == canonical_spec_hash(spec_b)
        assert canonical_spec_hash(spec_a) == "sha256:d9acfeef7efdd936aba3136e9e05bc4078a79c7200be4e2c0c4d3ec5c3f2e360"

    def test_slice_order_independence(self):
        """UT-HASH-005: Different list order produces identical hash (Go compatibility)."""
        from src.utils.canonical_hash import canonical_spec_hash

        spec_ab = {
            "containers": [
                {"name": "sidecar", "image": "envoy:1.0"},
                {"name": "main", "image": "nginx:1.21"},
            ],
        }
        spec_ba = {
            "containers": [
                {"name": "main", "image": "nginx:1.21"},
                {"name": "sidecar", "image": "envoy:1.0"},
            ],
        }
        assert canonical_spec_hash(spec_ab) == canonical_spec_hash(spec_ba)
        assert canonical_spec_hash(spec_ab) == "sha256:612b5be97a8a0885785c82ba173a9c601579ee4fc8726158cb733f10407597c3"

    def test_hash_format(self):
        """UT-HASH-006: Hash format is 'sha256:<64-hex>' (71 chars total)."""
        from src.utils.canonical_hash import canonical_spec_hash

        result = canonical_spec_hash({"key": "value"})
        assert result.startswith("sha256:")
        assert len(result) == 71

    def test_idempotent(self):
        """UT-HASH-007: Same input always produces same hash."""
        from src.utils.canonical_hash import canonical_spec_hash

        spec = {"replicas": 3, "image": "nginx:1.21"}
        h1 = canonical_spec_hash(spec)
        h2 = canonical_spec_hash(spec)
        assert h1 == h2
