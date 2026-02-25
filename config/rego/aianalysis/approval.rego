# AI Analysis Approval Policy — Scored Risk Factors + Confidence-Based Auto-Approval
# DD-WORKFLOW-001 v2.2: Approval determination based on environment, data quality, and confidence
# Business Requirements: BR-AI-011 (policy evaluation), BR-AI-013 (approval scenarios), BR-AI-014 (graceful degradation)
# Issue #98: Refactored from exclusion chains to scored risk factors
# Confidence-based auto-approval: High-confidence (>= 0.9) production analyses
# auto-approve unless critical safety conditions are present (missing affected_resource,
# sensitive resources).

package aianalysis.approval

import rego.v1

# ========================================
# DEFAULT VALUES
# ========================================

default require_approval := false
default reason := "Auto-approved"

# ========================================
# HELPER FUNCTIONS
# ========================================

detection_failed(field) if {
    field in input.failed_detections
}

has_critical_detection_failure if {
    detection_failed("gitOpsManaged")
}

has_critical_detection_failure if {
    detection_failed("pdbProtected")
}

is_stateful if {
    input.detected_labels["stateful"] == true
}

# ADR-055: Check if affected_resource is present (required LLM output)
has_affected_resource if {
    input.affected_resource
    input.affected_resource.kind != ""
}

# ADR-055: Check if affected resource is a sensitive kind
is_sensitive_resource if {
    input.affected_resource.kind == "Node"
}

is_sensitive_resource if {
    input.affected_resource.kind == "StatefulSet"
}

has_warnings if {
    count(input.warnings) > 0
}

has_failed_detections if {
    count(input.failed_detections) > 0
}

is_production if {
    input.environment == "production"
}

not_production if {
    input.environment == "development"
}

not_production if {
    input.environment == "staging"
}

not_production if {
    input.environment == "qa"
}

not_production if {
    input.environment == "test"
}

is_high_confidence if {
    input.confidence >= 0.9
}

# ========================================
# APPROVAL RULES
# ========================================
# Critical safety rules: ALWAYS require approval regardless of confidence.
# Production environment rules: Only require approval when confidence < 0.9.

# BR-AI-085-005: Default-deny when affected_resource is missing (ADR-055)
require_approval if {
    not has_affected_resource
}

# ADR-055: Production + sensitive resource kind ALWAYS requires approval
require_approval if {
    is_production
    is_sensitive_resource
}

# Production + failed detections + low confidence
require_approval if {
    is_production
    has_failed_detections
    not is_high_confidence
}

# Production + warnings + low confidence
require_approval if {
    is_production
    has_warnings
    not is_high_confidence
}

# Production + stateful workload + low confidence
require_approval if {
    is_production
    is_stateful
    not is_high_confidence
}

# Production catch-all: only for low confidence
require_approval if {
    is_production
    not is_high_confidence
}

# ========================================
# SCORED RISK FACTORS FOR REASON GENERATION
# ========================================

risk_factors contains {"score": 90, "reason": "Missing affected resource - cannot determine remediation target (BR-AI-085-005)"} if {
    not has_affected_resource
}

risk_factors contains {"score": 80, "reason": "Production environment with sensitive resource kind - requires manual approval"} if {
    is_production
    is_sensitive_resource
}

risk_factors contains {"score": 70, "reason": "Production environment with failed detections - requires manual approval"} if {
    is_production
    has_failed_detections
    not is_high_confidence
}

risk_factors contains {"score": 60, "reason": "Production environment with warnings - requires manual approval"} if {
    is_production
    has_warnings
    not is_high_confidence
}

risk_factors contains {"score": 50, "reason": "Production environment with Stateful workload - requires manual approval"} if {
    is_production
    is_stateful
    not is_high_confidence
}

risk_factors contains {"score": 40, "reason": "Production environment requires manual approval"} if {
    is_production
    not is_high_confidence
}

# ========================================
# REASON AGGREGATION: Highest score wins
# ========================================

all_scores contains f.score if {
    some f in risk_factors
}

max_risk_score := max(all_scores) if {
    count(all_scores) > 0
}

reason := f.reason if {
    some f in risk_factors
    f.score == max_risk_score
}
