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
Incident Analysis Constants

Business Requirements: BR-HAPI-002 (Incident Analysis)
Design Decision: DD-HAPI-002 v1.2 (LLM Self-Correction)

This module contains constants used throughout the incident analysis workflow.
"""

# ========================================
# LLM SELF-CORRECTION CONSTANTS (DD-HAPI-002 v1.2)
# ========================================
MAX_VALIDATION_ATTEMPTS = 3  # BR-HAPI-197: Max attempts before human review


# ========================================
# PRIORITY DESCRIPTIONS
# ========================================
PRIORITY_DESCRIPTIONS = {
    "P0": "P0 (highest priority) - This is a {business_category} service requiring immediate attention",
    "P1": "P1 (high priority) - This service requires prompt attention",
    "P2": "P2 (medium priority) - This service requires timely resolution",
    "P3": "P3 (low priority) - This service can be addressed during normal operations"
}


# ========================================
# RISK TOLERANCE GUIDANCE
# ========================================
RISK_GUIDANCE = {
    "low": "low (conservative remediation required - avoid aggressive restarts or scaling)",
    "medium": "medium (balanced approach - standard remediation actions permitted)",
    "high": "high (aggressive remediation permitted - prioritize recovery speed)"
}


# ========================================
# SEVERITY LEVEL DESCRIPTIONS
# ========================================
SEVERITY_DESCRIPTIONS = {
    "critical": {
        "title": "critical - Immediate remediation required",
        "indicators": [
            "Production service completely unavailable",
            "Data loss or corruption occurring",
            "Security breach actively exploited",
            "SLA violation in progress",
            "Revenue-impacting outage",
            "Affects >50% of users"
        ]
    },
    "high": {
        "title": "high - Urgent remediation needed",
        "indicators": [
            "Significant service degradation (>50% performance loss)",
            "High error rate (>10% of requests failing)",
            "Production issue escalating toward critical",
            "Affects 10-50% of users",
            "SLA at risk"
        ]
    },
    "medium": {
        "title": "medium - Remediation recommended",
        "indicators": [
            "Minor service degradation (<50% performance loss)",
            "Moderate error rate (1-10% of requests failing)",
            "Non-production critical issues",
            "Affects <10% of users",
            "Staging/development critical issues"
        ]
    },
    "low": {
        "title": "low - Remediation optional",
        "indicators": [
            "Informational issues",
            "Optimization opportunities",
            "Development environment issues",
            "No user impact",
            "Capacity planning alerts"
        ]
    }
}


# ========================================
# CANONICAL SIGNAL TYPES (Examples)
# ========================================
CANONICAL_SIGNAL_TYPES = [
    "OOMKilled",
    "CrashLoopBackOff",
    "ImagePullBackOff",
    "Evicted",
    "NodeNotReady",
    "PodPending",
    "FailedScheduling",
    "BackoffLimitExceeded",
    "DeadlineExceeded",
    "FailedMount"
]





