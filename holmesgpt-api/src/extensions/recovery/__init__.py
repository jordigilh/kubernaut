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
Recovery Analysis Package

Business Requirements: BR-HAPI-001 to 050 (Recovery Analysis)
Design Decision: DD-RECOVERY-003 (Recovery Package Organization)

This package contains all recovery analysis functionality, split into:
- constants: MinimalDAL and shared constants
- prompt_builder: All prompt construction logic
- result_parser: Result parsing and extraction
- llm_integration: Main recovery logic and Holmes SDK integration
- endpoint: FastAPI router and endpoint

This __init__.py maintains backward compatibility by re-exporting all functions
that were previously in the monolithic recovery.py file.
"""

# ========================================
# CONSTANTS MODULE
# ========================================
from .constants import MinimalDAL

# ========================================
# PROMPT BUILDER MODULE
# ========================================
from .prompt_builder import (
    _get_failure_reason_guidance,
    _build_cluster_context_section,
    _build_mcp_filter_instructions,
    _create_recovery_investigation_prompt,
    _create_investigation_prompt,
)

# ========================================
# RESULT PARSER MODULE
# ========================================
from .result_parser import (
    _parse_investigation_result,
    _parse_recovery_specific_result,
    _extract_strategies_from_analysis,
    _extract_warnings_from_analysis,
)

# ========================================
# LLM INTEGRATION MODULE
# ========================================
from .llm_integration import (
    _get_holmes_config,
    analyze_recovery,
)

# ========================================
# ENDPOINT MODULE
# ========================================
from .endpoint import router

# ========================================
# PUBLIC API (Backward Compatibility)
# ========================================

# Main entry point
__all__ = [
    # Public API
    "analyze_recovery",
    "router",
    "MinimalDAL",

    # Private functions (for tests and internal use)
    "_get_failure_reason_guidance",
    "_build_cluster_context_section",
    "_build_mcp_filter_instructions",
    "_create_recovery_investigation_prompt",
    "_create_investigation_prompt",
    "_parse_investigation_result",
    "_parse_recovery_specific_result",
    "_extract_strategies_from_analysis",
    "_extract_warnings_from_analysis",
    "_get_holmes_config",
]

# Module-level docstring
__doc__ = """
Recovery Analysis Package

This package provides recovery analysis functionality for Kubernaut's AI-powered
remediation system. It analyzes failed workflow executions and recommends alternative
recovery strategies.

Key Components:
1. **LLM Integration**: Uses Holmes SDK to investigate failures and recommend alternatives
2. **Prompt Building**: Constructs recovery-specific investigation prompts
3. **Result Parsing**: Extracts structured recovery recommendations from LLM output
4. **Workflow Validation**: Ensures recommended workflows exist and are executable

Business Requirements: BR-HAPI-001 to 050 (Recovery Analysis)
Design Decision: DD-RECOVERY-003 (Recovery Architecture)
"""

