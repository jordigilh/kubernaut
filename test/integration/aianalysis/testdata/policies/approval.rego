# AI Analysis Approval Policy — Integration Test Fixture
# Comprehensive scored risk factors + confidence-based auto-approval.
# This fixture is owned by integration tests and decoupled from the production
# policy (config/rego/aianalysis/approval.rego) so that operational changes
# (e.g. demo simplifications) do not break test assertions.
#
# Business Requirements exercised:
#   BR-AI-011 (policy evaluation), BR-AI-013 (approval scenarios),
#   BR-AI-014 (graceful degradation), BR-AI-028/029 (confidence gating)
#
# Confidence-based auto-approval: high-confidence (>= threshold) production
# analyses auto-approve unless critical safety conditions are present
# (missing remediation_target, sensitive resources).

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

# ADR-055: Check if remediation_target is present (required LLM output)
has_remediation_target if {
    input.remediation_target
    input.remediation_target.kind != ""
}

# ADR-055: Check if remediation target is a sensitive kind
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

# Configurable confidence threshold (default 0.8)
default confidence_threshold := 0.8

confidence_threshold := input.confidence_threshold if {
    input.confidence_threshold
}

is_high_confidence if {
    input.confidence >= confidence_threshold
}

# ========================================
# APPROVAL RULES
# ========================================
# Critical safety rules: ALWAYS require approval regardless of confidence.
# Production environment rules: Only require approval when confidence < confidence_threshold.

# BR-AI-085-005: Default-deny when remediation_target is missing (ADR-055)
require_approval if {
    not has_remediation_target
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

risk_factors contains {"score": 90, "reason": "Missing remediation target - cannot determine resource to remediate (BR-AI-085-005)"} if {
    not has_remediation_target
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
