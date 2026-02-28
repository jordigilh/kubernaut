"""
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

"""
Pydantic Model Validation Tests

Business Requirements: BR-HAPI-002, BR-HAPI-051 (Model validation)
"""

import pytest
from src.models.incident_models import IncidentRequest


class TestIncidentModels:
    """Tests for incident analysis models."""

    def test_incident_request_accepts_valid_data(self):
        """IncidentRequest accepts valid request data."""
        request = IncidentRequest(
            incident_id="inc-001",
            remediation_id="req-2025-11-27-abc123",
            signal_name="OOMKilled",
            severity="high",
            signal_source="prometheus",
            resource_namespace="production",
            resource_kind="Pod",
            resource_name="api-server",
            error_message="Container OOM",
            environment="production",
            priority="P0",
            risk_tolerance="low",
            business_category="critical",
            cluster_name="prod-cluster-1",
        )
        assert request.incident_id == "inc-001"
        assert request.remediation_id == "req-2025-11-27-abc123"
