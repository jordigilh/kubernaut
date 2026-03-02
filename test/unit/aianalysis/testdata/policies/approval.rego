# AIAnalysis Approval Policy - Test Version (scored risk factors + confidence-based auto-approval)
# BR-AI-013: Determines if human approval is required for remediation
# Issue #98: Refactored from exclusion chains to scored risk factors
# Confidence-based auto-approval: High-confidence (>= threshold) production analyses
# auto-approve unless critical safety conditions are present.
# #225: Threshold configurable via input.confidence_threshold (default 0.8).

package aianalysis.approval

import rego.v1

# =============================================================================
# Helper Rules
# =============================================================================

is_production if {
    input.environment == "production"
}

is_high_severity if {
    input.severity == "critical"
}

is_high_severity if {
    input.severity == "P0"
}

# ADR-055: Check if affected_resource is present
has_affected_resource if {
    input.affected_resource
    input.affected_resource.kind != ""
}

has_warnings if {
    count(input.warnings) > 0
}

has_failed_detections if {
    count(input.failed_detections) > 0
}

# #225: Configurable confidence threshold — operators can override via input.confidence_threshold
default confidence_threshold := 0.8

confidence_threshold := input.confidence_threshold if {
    input.confidence_threshold
}

is_high_confidence if {
    input.confidence >= confidence_threshold
}

# =============================================================================
# Approval Rules (independent boolean checks)
# =============================================================================

default require_approval := false

# BR-AI-085-005: Default-deny when affected_resource is missing (ADR-055)
# Safety: ALWAYS require approval regardless of confidence
require_approval if {
    not has_affected_resource
}

# Production + low confidence → require approval
require_approval if {
    is_production
    not is_high_confidence
}

# Production + failed detections + low confidence → require approval
require_approval if {
    is_production
    has_failed_detections
    not is_high_confidence
}

# Production + warnings + low confidence → require approval
require_approval if {
    is_production
    has_warnings
    not is_high_confidence
}

# =============================================================================
# Scored Risk Factors for Reason Generation
# =============================================================================

risk_factors contains {"score": 90, "reason": "Missing affected resource - cannot determine remediation target (BR-AI-085-005)"} if {
    not has_affected_resource
}

risk_factors contains {"score": 60, "reason": "Data quality issues detected in production environment"} if {
    is_production
    has_failed_detections
    not is_high_confidence
}

risk_factors contains {"score": 70, "reason": "Data quality warnings in production environment"} if {
    is_production
    has_warnings
    not is_high_confidence
}

risk_factors contains {"score": 40, "reason": "Production environment requires manual approval"} if {
    is_production
    not is_high_confidence
}

# =============================================================================
# Reason Aggregation: Highest score wins
# =============================================================================

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

default reason := "Auto-approved by policy"
