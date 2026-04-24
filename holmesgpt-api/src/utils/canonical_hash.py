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
Canonical spec hashing for Kubernetes resource specs (Python port).

DD-EM-002: Cross-language deterministic hashing of Kubernetes resource specs.
BR-HAPI-016: Used by HAPI to compute currentSpecHash parameter.

This module is a faithful port of the Go implementation in
pkg/shared/hash/canonical.go. It MUST produce identical hashes for the same
input data.

Guarantees:
  - Idempotent: same logical content always produces the same hash
  - Dict-key-order independent: key iteration order does not affect output
  - List-order independent: element reordering does not affect output
  - Cross-language portable: produces identical hashes to the Go implementation
  - Format: "sha256:<64-lowercase-hex>" (71 characters total)
"""

import hashlib
import json
from typing import Any, Optional


def canonical_spec_hash(spec: Optional[dict]) -> str:
    """Compute a deterministic SHA-256 hash of a Kubernetes resource spec.

    The algorithm recursively normalizes the input:
      - Dicts: keys sorted alphabetically, values normalized recursively
      - Lists: elements sorted by their canonical JSON representation
      - Scalars: passed through unchanged

    A None spec is treated as an empty dict.

    Args:
        spec: The Kubernetes resource spec as a dict, or None.

    Returns:
        A string in the format "sha256:<64-lowercase-hex>" (71 characters total).
    """
    if spec is None:
        spec = {}

    normalized = _normalize_value(spec)

    # json.dumps with sort_keys=False because we've already sorted manually.
    # separators=(',', ':') matches Go's json.Marshal compact encoding.
    data = json.dumps(normalized, separators=(",", ":"), sort_keys=False).encode("utf-8")

    h = hashlib.sha256(data).hexdigest()
    return f"sha256:{h}"


def _normalize_value(v: Any) -> Any:
    """Recursively normalize a value for canonical serialization.

    Mirrors Go normalizeValue in pkg/shared/hash/canonical.go.
    """
    if isinstance(v, dict):
        # Sort keys alphabetically and normalize values recursively.
        # Return as a list of [key, value] pairs to preserve sort order
        # in JSON output. Go's encoding/json sorts map keys alphabetically.
        return {k: _normalize_value(v[k]) for k in sorted(v.keys())}
    elif isinstance(v, list):
        # Normalize each element, then sort by canonical JSON representation.
        normalized = [_normalize_value(child) for child in v]
        normalized.sort(key=lambda x: json.dumps(x, separators=(",", ":"), sort_keys=False))
        return normalized
    else:
        return v
