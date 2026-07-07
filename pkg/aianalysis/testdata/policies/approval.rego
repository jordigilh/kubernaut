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
    lower(input.environment) == "production"
}

is_high_severity if {
    lower(input.severity) == "critical"
}

is_high_severity if {
    lower(input.severity) == "p0"
}

# ADR-055: Check if remediation_target is present
has_remediation_target if {
    input.remediation_target
    input.remediation_target.kind != ""
}

# #247: Infrastructure-provisioning action types always require approval,
# regardless of the (LLM-reported) remediation_target kind.
is_infrastructure_action if {
    input.action_type == "ProvisionNode"
}

# ADR-055: Check if remediation target is a sensitive kind.
# Added in REFACTOR (#247) to close a pre-existing divergence from the
# integration fixture (test/integration/aianalysis/testdata/policies/approval.rego),
# which already had this rule.
is_sensitive_resource if {
    input.remediation_target.kind == "Node"
}

is_sensitive_resource if {
    input.remediation_target.kind == "StatefulSet"
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

# BR-AI-085-005: Default-deny when remediation_target is missing (ADR-055)
# Safety: ALWAYS require approval regardless of confidence
require_approval if {
    not has_remediation_target
}

# #247: Infrastructure-provisioning actions always require approval
require_approval if {
    is_infrastructure_action
}

# ADR-055: Production + sensitive resource kind ALWAYS requires approval
# (REFACTOR #247: closes pre-existing divergence from the integration fixture)
require_approval if {
    is_production
    is_sensitive_resource
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

risk_factors contains {"score": 90, "reason": "Missing remediation target - cannot determine resource to remediate (BR-AI-085-005)"} if {
    not has_remediation_target
}

# #247: Highest score -- infrastructure-provisioning actions always require
# approval, independent of environment/confidence, so the reason must be
# unambiguous regardless of what other risk factors might also apply.
risk_factors contains {"score": 95, "reason": "Infrastructure-provisioning action type requires manual approval (#247)"} if {
    is_infrastructure_action
}

risk_factors contains {"score": 80, "reason": "Production environment with sensitive resource kind - requires manual approval"} if {
    is_production
    is_sensitive_resource
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
