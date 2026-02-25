# AI Analysis Approval Policy — Scored Risk Factors
# DD-WORKFLOW-001 v2.2: Approval determination based on environment and data quality
# Business Requirements: BR-AI-011 (policy evaluation), BR-AI-013 (approval scenarios), BR-AI-014 (graceful degradation)
# Issue #98: Refactored from exclusion chains to scored risk factors

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

# Check if a specific detection failed
detection_failed(field) if {
    field in input.failed_detections
}

# Check if any critical detection failed
has_critical_detection_failure if {
    detection_failed("gitOpsManaged")
}

has_critical_detection_failure if {
    detection_failed("pdbProtected")
}

# Check if stateful workload
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

# Check if warnings exist
has_warnings if {
    count(input.warnings) > 0
}

# Check if failed detections exist
has_failed_detections if {
    count(input.failed_detections) > 0
}

# Non-production environments
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

# ========================================
# APPROVAL RULES (unchanged — independent boolean checks)
# ========================================

# BR-AI-085-005: Default-deny when affected_resource is missing (ADR-055)
require_approval if {
    not has_affected_resource
}

# ADR-055: Production + sensitive resource kind requires approval
require_approval if {
    is_production
    is_sensitive_resource
}

# Production + failed detections requires approval (BR-AI-013)
require_approval if {
    is_production
    has_failed_detections
}

# Production + warnings requires approval (BR-AI-011)
require_approval if {
    is_production
    has_warnings
}

# Production + stateful workload requires approval (BR-AI-013)
require_approval if {
    is_production
    is_stateful
}

# Production environment requires approval (catch-all for production)
require_approval if {
    is_production
}

# ========================================
# SCORED RISK FACTORS FOR REASON GENERATION
# ========================================
# Each risk factor independently contributes a scored entry.
# The highest-scored reason wins. No exclusion chains needed —
# adding a new factor is a single rule addition.

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
}

risk_factors contains {"score": 60, "reason": "Production environment with warnings - requires manual approval"} if {
    is_production
    has_warnings
}

risk_factors contains {"score": 50, "reason": "Production environment with Stateful workload - requires manual approval"} if {
    is_production
    is_stateful
}

risk_factors contains {"score": 40, "reason": "Production environment requires manual approval"} if {
    is_production
}

# ========================================
# REASON AGGREGATION: Highest score wins
# ========================================

# Collect all scores from risk factors
all_scores contains f.score if {
    some f in risk_factors
}

# Find the maximum score among all risk factors
max_risk_score := max(all_scores) if {
    count(all_scores) > 0
}# Select the reason with the highest score
reason := f.reason if {
    some f in risk_factors
    f.score == max_risk_score
}
