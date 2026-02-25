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
Incident Analysis Package

Business Requirements: BR-HAPI-002 (Incident Analysis)
Design Decision: DD-RECOVERY-003 (DetectedLabels for workflow filtering)

Provides AI-powered Root Cause Analysis (RCA) and workflow selection for initial incidents.

This package was refactored from a single 1,593-line file into focused modules
for improved maintainability. All imports are re-exported for backward compatibility.

Module Structure:
- constants.py: Configuration constants and LLM self-correction settings
- prompt_builder.py: LLM prompt construction and context building
- result_parser.py: Investigation result parsing and validation
- llm_integration.py: Core HolmesGPT SDK integration and self-correction loop
- endpoint.py: FastAPI endpoint definition

Refactoring Date: 2025-12-14
"""

# Re-export everything for backward compatibility
from .endpoint import router, incident_analyze_endpoint
from .llm_integration import analyze_incident, MinimalDAL, create_data_storage_client
from .constants import MAX_VALIDATION_ATTEMPTS
from src.audit import get_audit_store  # Shared factory (no longer in llm_integration)
from .prompt_builder import (
    create_incident_investigation_prompt,
    build_cluster_context_section,
    build_mcp_filter_instructions,
    build_validation_error_feedback
)
from .result_parser import (
    parse_and_validate_investigation_result,
    determine_human_review_reason,
    parse_investigation_result  # DEPRECATED but kept for backward compatibility
)

# Backward compatibility: Export private function names with _ prefix for existing tests
_create_incident_investigation_prompt = create_incident_investigation_prompt
_build_cluster_context_section = build_cluster_context_section
_build_mcp_filter_instructions = build_mcp_filter_instructions
_build_validation_error_feedback = build_validation_error_feedback
_parse_and_validate_investigation_result = parse_and_validate_investigation_result
_determine_human_review_reason = determine_human_review_reason
_parse_investigation_result = parse_investigation_result
_create_data_storage_client = create_data_storage_client

__all__ = [
    # FastAPI endpoint
    "router",
    "incident_analyze_endpoint",

    # Core business logic
    "analyze_incident",

    # Audit and infrastructure
    "get_audit_store",
    "MinimalDAL",
    "create_data_storage_client",
    "_create_data_storage_client",  # Backward compatibility

    # Constants
    "MAX_VALIDATION_ATTEMPTS",

    # Prompt building (public API)
    "create_incident_investigation_prompt",
    "build_cluster_context_section",
    "build_mcp_filter_instructions",
    "build_validation_error_feedback",

    # Prompt building (backward compatibility with _ prefix)
    "_create_incident_investigation_prompt",
    "_build_cluster_context_section",
    "_build_mcp_filter_instructions",
    "_build_validation_error_feedback",

    # Result parsing (public API)
    "parse_and_validate_investigation_result",
    "determine_human_review_reason",
    "parse_investigation_result",  # DEPRECATED

    # Result parsing (backward compatibility with _ prefix)
    "_parse_and_validate_investigation_result",
    "_determine_human_review_reason",
    "_parse_investigation_result",
]

