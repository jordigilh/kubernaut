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
Unit Tests for Recovery Analysis Functions

Tests internal helper functions and logic for recovery analysis.
Target: Increase recovery.py coverage from 31% to 80%

Business Requirements: BR-HAPI-001 to 050

NOTE: TestCreateInvestigationPrompt and prompt-related edge cases were REMOVED
because the prompt format was refactored from "Recovery Analysis Request" to
ADR-041 v3.3 "Incident Analysis" format. The old tests were testing the obsolete
prompt structure. New prompt tests should be created if needed to test the
current ADR-041 v3.3 format.
"""

import pytest
import json
from unittest.mock import Mock, patch, AsyncMock
from src.extensions.recovery import (
    MinimalDAL,
    _get_holmes_config,
    _extract_strategies_from_analysis,
    _extract_warnings_from_analysis
)


# ========================================
# TEST SUITE 1: MinimalDAL
# ========================================

class TestMinimalDAL:
    """Test MinimalDAL stateless implementation"""

    def test_minimal_dal_initialization(self):
        """Test MinimalDAL initialization"""
        dal = MinimalDAL(cluster_name="test-cluster")

        assert dal.cluster == "test-cluster"
        assert dal.cluster_name == "test-cluster"
        assert dal.enabled is False

    def test_minimal_dal_initialization_no_cluster(self):
        """Test MinimalDAL initialization without cluster name"""
        dal = MinimalDAL()

        assert dal.cluster is None
        assert dal.cluster_name is None
        assert dal.enabled is False

    def test_get_issue_data_returns_none(self):
        """Test get_issue_data returns None (not used in Kubernaut)"""
        dal = MinimalDAL(cluster_name="test-cluster")

        result = dal.get_issue_data("issue-123")

        assert result is None

    def test_get_resource_instructions_returns_none(self):
        """Test get_resource_instructions returns None (not used in Kubernaut)"""
        dal = MinimalDAL(cluster_name="test-cluster")

        result = dal.get_resource_instructions("deployment", "oom")

        assert result is None

    def test_get_global_instructions_for_account_returns_none(self):
        """Test get_global_instructions_for_account returns None (not used in Kubernaut)"""
        dal = MinimalDAL(cluster_name="test-cluster")

        result = dal.get_global_instructions_for_account()

        assert result is None


# ========================================
# TEST SUITE 2: HolmesGPT Config
# ========================================
# NOTE: DEV_MODE tests removed as part of the DEV_MODE anti-pattern cleanup (DD-HAPI-001 v3.0).
# Testing is now done using the mock LLM server approach configured via LLM_* env vars.

class TestGetHolmesConfig:
    """Test HolmesGPT SDK configuration"""

    def test_get_holmes_config_with_model(self):
        """Test config creation with LLM_MODEL"""
        with patch.dict('os.environ', {
            'LLM_MODEL': 'gpt-4'
        }, clear=True):
            config = _get_holmes_config()

            assert config is not None
            # Config object exists

    def test_get_holmes_config_without_model_raises_error(self):
        """
        Test that config creation without LLM_MODEL raises HTTPException

        BR-HAPI-001: LLM_MODEL is required
        BEHAVIOR: Missing LLM_MODEL should raise HTTPException with 500 status
        """
        from fastapi import HTTPException

        with patch.dict('os.environ', {}, clear=True):
            # Ensure LLM_MODEL is not set
            import os
            if 'LLM_MODEL' in os.environ:
                del os.environ['LLM_MODEL']

            # Should raise HTTPException because LLM_MODEL is required
            with pytest.raises(HTTPException) as exc_info:
                _get_holmes_config()

            # Validate the exception details
            assert exc_info.value.status_code == 500
            assert "LLM_MODEL" in str(exc_info.value.detail) or "model" in str(exc_info.value.detail).lower()


# ========================================
# TEST SUITE 3: Strategy Extraction
# ========================================

class TestExtractStrategiesFromAnalysis:
    """Test strategy extraction from LLM analysis"""

    def test_extract_strategies_from_json_response(self):
        """Test extraction from well-formed JSON response"""
        analysis_text = """
Here's my analysis:

```json
{
  "strategies": [
    {
      "action_type": "scale-deployment",
      "confidence": 0.85,
      "rationale": "Increase replicas to handle load",
      "estimated_risk": "low"
    },
    {
      "action_type": "increase-memory",
      "confidence": 0.75,
      "rationale": "Address memory pressure",
      "estimated_risk": "medium"
    }
  ]
}
```
"""
        strategies = _extract_strategies_from_analysis(analysis_text)

        assert len(strategies) == 2
        assert strategies[0].action_type == "scale-deployment"
        assert strategies[0].confidence == 0.85
        assert strategies[1].action_type == "increase-memory"

    def test_extract_strategies_from_markdown_list(self):
        """Test extraction from markdown bullet list"""
        analysis_text = """
Recommended strategies:

1. **Scale deployment** (confidence: 0.85, risk: low)
   - Rationale: Increase replicas to handle load

2. **Increase memory** (confidence: 0.75, risk: medium)
   - Rationale: Address memory pressure
"""
        strategies = _extract_strategies_from_analysis(analysis_text)

        # Should extract at least one strategy
        assert len(strategies) >= 1

    def test_extract_strategies_empty_response(self):
        """Test extraction from empty response returns fallback"""
        analysis_text = ""

        strategies = _extract_strategies_from_analysis(analysis_text)

        # Implementation returns fallback strategy for empty response
        assert isinstance(strategies, list)
        assert len(strategies) >= 1  # Fallback strategy
        if len(strategies) > 0:
            assert strategies[0].action_type == "manual_intervention_required"

    def test_extract_strategies_no_strategies_found(self):
        """Test extraction when no strategies are present returns fallback"""
        analysis_text = "This is just general text without strategies."

        strategies = _extract_strategies_from_analysis(analysis_text)

        # Implementation returns fallback strategy when none found
        assert isinstance(strategies, list)
        assert len(strategies) >= 1  # Fallback strategy

    def test_extract_strategies_malformed_json(self):
        """Test extraction from malformed JSON"""
        analysis_text = """
```json
{
  "strategies": [
    {"action_type": "scale-deployment", "confidence": 0.85
  ]
}
```
"""
        strategies = _extract_strategies_from_analysis(analysis_text)

        # Should gracefully handle malformed JSON
        # May return empty list or partial strategies
        assert isinstance(strategies, list)


# ========================================
# TEST SUITE 4: Warning Extraction
# ========================================

class TestExtractWarningsFromAnalysis:
    """Test warning extraction from LLM analysis"""

    def test_extract_explicit_warnings(self):
        """Test extraction of explicit warning statements"""
        analysis_text = """
Analysis summary:

Warning: System under high load - proceed with caution
Caution: Database connection pool exhausted
Note: Consider off-peak hours for maintenance
"""
        warnings = _extract_warnings_from_analysis(analysis_text)

        # Implementation extracts warnings based on keyword patterns
        assert len(warnings) >= 1
        assert any("high load" in w.lower() or "resource" in w.lower() for w in warnings)

    def test_extract_implicit_warnings_high_load(self):
        """Test extraction of implicit high load warnings"""
        analysis_text = """
The cluster is experiencing high load with CPU usage at 95%.
"""
        warnings = _extract_warnings_from_analysis(analysis_text)

        # Should detect high load pattern
        assert len(warnings) > 0

    def test_extract_implicit_warnings_resource_constraints(self):
        """Test extraction of resource constraint warnings"""
        analysis_text = """
Limited memory resources available in the cluster.
"""
        warnings = _extract_warnings_from_analysis(analysis_text)

        # Should detect resource constraint pattern
        assert len(warnings) > 0

    def test_extract_no_warnings(self):
        """Test extraction when no warnings present"""
        analysis_text = """
Everything looks good. Proceed with the remediation.
"""
        warnings = _extract_warnings_from_analysis(analysis_text)

        assert warnings == []

    def test_extract_warnings_empty_text(self):
        """Test extraction from empty text"""
        analysis_text = ""

        warnings = _extract_warnings_from_analysis(analysis_text)

        assert warnings == []


# ========================================
# TEST SUITE 5: Edge Cases
# ========================================
# NOTE: TestStubRecoveryAnalysis was removed as _stub_recovery_analysis was
# removed as part of the DEV_MODE anti-pattern cleanup (DD-HAPI-001 v3.0).
# Testing is now done using the mock LLM server approach.

class TestRecoveryAnalysisEdgeCases:
    """Test edge cases and boundary conditions for non-prompt functions"""

    def test_extract_strategies_with_negative_confidence(self):
        """Test strategy extraction handles negative confidence"""
        analysis_text = """
```json
{
  "strategies": [
    {"action_type": "test", "confidence": -0.5}
  ]
}
```
"""
        strategies = _extract_strategies_from_analysis(analysis_text)

        # Should handle gracefully (may skip or clamp)
        assert isinstance(strategies, list)

    def test_extract_strategies_with_confidence_over_1(self):
        """Test strategy extraction handles confidence > 1.0"""
        analysis_text = """
```json
{
  "strategies": [
    {"action_type": "test", "confidence": 1.5}
  ]
}
```
"""
        strategies = _extract_strategies_from_analysis(analysis_text)

        # Should handle gracefully
        assert isinstance(strategies, list)

    def test_extract_warnings_with_mixed_case(self):
        """Test warning extraction is case-insensitive"""
        analysis_text = """
WARNING: System under load
Caution: Database issue
NOTE: Maintenance window
"""
        warnings = _extract_warnings_from_analysis(analysis_text)

        # Implementation extracts based on keyword patterns
        assert len(warnings) >= 1
