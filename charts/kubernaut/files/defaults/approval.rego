# AI Analysis Default Approval Policy
#
# Shipped as the Helm chart default when no user-supplied policy is provided.
# Based on the integration test fixture (gold standard) with case-insensitive
# matching for all environment comparisons.
#
# Business Requirements:
#   BR-AI-011 (policy evaluation), BR-AI-013 (approval scenarios),
#   BR-AI-014 (graceful degradation), BR-AI-028/029 (confidence gating)
#
# Issue #604: All environment comparisons use lower() to match SP PascalCase output.
#
# Customization:
#   Override via Helm:
#     helm install kubernaut kubernaut/kubernaut \
#       --set-file aianalysis.policies.content=my-approval.rego
#   Or use an existing ConfigMap:
#     helm install kubernaut kubernaut/kubernaut \
#       --set aianalysis.policies.existingConfigMap=my-configmap

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

has_remediation_target if {
    input.remediation_target
    input.remediation_target.kind != ""
}

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

# Issue #604: lower() ensures PascalCase SP output ("Production") matches.
is_production if {
    lower(input.environment) == "production"
}

not_production if {
    lower(input.environment) == "development"
}

not_production if {
    lower(input.environment) == "staging"
}

not_production if {
    lower(input.environment) == "qa"
}

not_production if {
    lower(input.environment) == "test"
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
