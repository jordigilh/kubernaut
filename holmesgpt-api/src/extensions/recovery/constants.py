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
Recovery Analysis Constants

Business Requirement: BR-HAPI-001 to 050 (Recovery Analysis)
Design Decision: DD-HOLMESGPT-014 (MinimalDAL Stateless Architecture)

This module contains constants and shared classes for recovery analysis.
"""

import logging

logger = logging.getLogger(__name__)

# ========================================
# LLM SELF-CORRECTION CONSTANTS (DD-HAPI-002 v1.2)
# ========================================
MAX_VALIDATION_ATTEMPTS = 3  # BR-HAPI-017-004: Max attempts before human review


class MinimalDAL:
    """
    Minimal DAL for HolmesGPT SDK integration (no Robusta Platform)

    Architecture Decision (DD-HOLMESGPT-014):
    Kubernaut does NOT integrate with Robusta Platform.

    Kubernaut Provides Equivalent Features Via:
    - Workflow catalog → PostgreSQL with Data Storage Service (not Robusta Platform)
    - Historical data → Context API (not Supabase)
    - Custom investigation logic → Rego policies in RemediationExecution Controller
    - LLM credentials → Kubernetes Secrets (not database)
    - Remediation state → CRDs (RemediationRequest, AIAnalysis, RemediationExecution)

    Result: No Robusta Platform database integration needed.

    This MinimalDAL satisfies HolmesGPT SDK's DAL interface requirements
    without connecting to any Robusta Platform database.

    Note: We still install supabase/postgrest dependencies (~50MB) because
    the SDK requires them, but this class ensures they're never used at runtime.

    See: docs/decisions/DD-HOLMESGPT-014-MinimalDAL-Stateless-Architecture.md
    """
    def __init__(self, cluster_name=None):
        self.cluster = cluster_name
        self.cluster_name = cluster_name  # Backwards compatibility
        self.enabled = False  # Disable Robusta platform features
        logger.info(f"Using MinimalDAL (no Robusta Platform) for cluster={cluster_name}")

    def get_issue_data(self, issue_id):
        """
        Historical issue data (NOT USED)

        Kubernaut: Context API provides historical data via separate service
        """
        return None

    def get_resource_instructions(self, resource_type, issue_type):
        """
        Custom investigation runbooks (NOT USED)

        Kubernaut: Rego policies in RemediationExecution Controller provide custom logic

        Returns None to signal no custom runbooks (SDK will use defaults)
        """
        return None

    def get_global_instructions_for_account(self):
        """
        Account-level investigation guidelines (NOT USED)

        Kubernaut: RemediationExecution Controller manages investigation flow

        Returns None to signal no global instructions (SDK will use defaults)
        """
        return None





