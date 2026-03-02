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
Application Constants

Centralized configuration constants for the HAPI service to avoid magic numbers
and improve maintainability.

These constants are referenced across multiple modules and define the behavior
of the service's AI analysis and validation logic.
"""

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# CONFIDENCE THRESHOLDS (BR-HAPI-197, BR-HAPI-200)
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# Minimum confidence score for workflow selection without human review.
# Business Requirement: BR-HAPI-197 (needs_human_review field)
# Values below this threshold trigger needs_human_review=True.
# Configurable via config.yaml: prompt.confidence_threshold_human_review
# This constant serves as the fallback default when not configured.
CONFIDENCE_THRESHOLD_HUMAN_REVIEW = 0.7  # 70%

# Default confidence for mock responses
# Used in mock_responses.py and test fixtures
CONFIDENCE_DEFAULT_MOCK = 0.75  # 75%

# Default confidence for postexec validation success
# Used in postexec.py when validation passes
CONFIDENCE_DEFAULT_POSTEXEC_SUCCESS = 0.75  # 75%

# Default confidence for postexec validation with warnings
# Used in postexec.py when validation passes but has warnings
CONFIDENCE_DEFAULT_POSTEXEC_WARNING = 0.7  # 70%

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# LLM CONFIGURATION
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# Default LLM temperature for structured output
# Business Requirement: BR-HAPI-002 (Incident Analysis)
# Lower temperature (0.7) produces more deterministic, factual responses
LLM_TEMPERATURE_DEFAULT = 0.7

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# AUDIT CONFIGURATION (BR-AUDIT-005, ADR-038)
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# AUDIT CONFIGURATION DEFAULTS (BR-AUDIT-005, ADR-038)
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Per ADR-030: These are fallback defaults only.
# Actual values come from config.yaml (audit section).
# Integration tests: Set audit.flush_interval_seconds=0.1 in config.yaml

# Audit buffer size (number of events before forced flush)
# Design Decision: ADR-038 (Async Buffered Audit Ingestion)
AUDIT_BUFFER_SIZE = 10000

# Audit batch size (events sent per batch to Data Storage)
AUDIT_BATCH_SIZE = 50

# Audit flush interval (seconds between automatic flushes)
# Default: 5.0 seconds (production)
# Integration tests override via config.yaml
AUDIT_FLUSH_INTERVAL_SECONDS = 5.0

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# VALIDATION CONFIGURATION (DD-HAPI-002 v1.2)
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# Maximum validation attempts for LLM self-correction loop
# Design Decision: DD-HAPI-002 v1.2 (Workflow Response Validation)
# Referenced in incident/constants.py but defined here for consistency
MAX_VALIDATION_ATTEMPTS = 3

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# PROMPT CONFIGURATION (Issue #224, BR-HAPI-016)
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# Number of completed remediations for the same workflow+signal before
# the prompt warns the LLM to escalate to human review.
# Issue #224: LLM should escalate when remediation history shows repeated
# ineffective completions. Decision: #214 Option C (LLM-driven escalation).
# Configurable via config.yaml: prompt.repeated_remediation_escalation_threshold
REPEATED_REMEDIATION_ESCALATION_THRESHOLD = 2





