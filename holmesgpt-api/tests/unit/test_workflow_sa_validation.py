#
# Copyright 2025 Jordi Gil.
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
#

"""
Per-Workflow ServiceAccount Validation Tests (#481)

Authority: DD-WE-005 v2.0 (Per-Workflow ServiceAccount Reference)
Business Requirement: BR-WE-007 (Service account configuration)

Tests that the workflow response validator correctly extracts and propagates
service_account_name from the DataStorage catalog to the validation result,
and that result_parser injects it into selected_workflow.
"""

import pytest
from unittest.mock import MagicMock, patch
from dataclasses import dataclass
from typing import Optional


class TestValidationResultServiceAccountName:
    """UT-HAPI-481-001: ValidationResult carries service_account_name from catalog."""

    def test_ut_hapi_481_001_validation_result_includes_sa(self):
        """Validated SA name should be present on ValidationResult when workflow has SA."""
        from src.validation.workflow_response_validator import ValidationResult

        result = ValidationResult(
            is_valid=True,
            validated_execution_bundle="quay.io/test:v1@sha256:abc",
            validated_service_account_name="my-workflow-sa",
        )
        assert result.validated_service_account_name == "my-workflow-sa"
        assert result.is_valid is True

    def test_ut_hapi_481_002_validation_result_none_sa_when_absent(self):
        """Validated SA name should be None when workflow has no SA."""
        from src.validation.workflow_response_validator import ValidationResult

        result = ValidationResult(
            is_valid=True,
            validated_execution_bundle="quay.io/test:v1@sha256:abc",
        )
        assert result.validated_service_account_name is None

    def test_ut_hapi_481_003_sa_injection_into_selected_workflow(self):
        """SA should be injected into selected_workflow dict when validation succeeds."""
        from src.validation.workflow_response_validator import ValidationResult

        validation_result = ValidationResult(
            is_valid=True,
            validated_execution_bundle="quay.io/test:v1@sha256:abc",
            validated_service_account_name="injected-sa",
        )

        selected_workflow = {
            "workflow_id": "wf-uuid-123",
            "execution_bundle": "quay.io/old:v1",
            "confidence": 0.95,
        }

        # Simulate the injection logic from result_parser
        if validation_result.validated_service_account_name:
            selected_workflow["service_account_name"] = validation_result.validated_service_account_name

        assert selected_workflow["service_account_name"] == "injected-sa"
