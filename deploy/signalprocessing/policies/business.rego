# DEPRECATED (BR-FLEET-003, #1511, ADR-060): This standalone per-domain policy
# file predates the unified SignalProcessing Rego policy. New deployments
# should use charts/kubernaut/examples/signalprocessing-policy.rego, which
# consolidates environment/severity/priority/labels/cluster classification
# into a single package evaluated once per reconcile (and is the only policy
# shape that supports the optional `cluster` dimension). This file is kept
# for existing deployments that have not yet migrated; it is not maintained
# with new classification dimensions.
#
# Business Classification Policy
# Per IMPLEMENTATION_PLAN_V1.22.md specification
#
# BR-SP-002: Business unit classification via Rego policy
#
# IMPORTANT: This policy is MANDATORY. Go code has NO fallback logic.
# Operators MUST customize this policy to match their business structure.
# The `default` rule at the bottom catches any unmatched combinations.
#
# Input Schema (per BR-SP-002, Issue #113):
# {
#   "namespace": { "name": "...", "labels": {...}, "annotations": {...} },
#   "workload": { "kind": "Deployment", "name": "...", "labels": {...}, "annotations": {...} },
#   "signal": { "severity": "...", "labels": {...} },
#   "environment": "production"
# }
#
# Output Schema:
# {
#   "business_unit": "platform",
#   "service_owner": "team-a",
#   "criticality": "high",
#   "sla": "tier-1",
#   "source": "namespace-labels"
# }

package signalprocessing.business

import rego.v1

# ============================================================================
# BUSINESS UNIT DETECTION
# ============================================================================
# Detect business unit from namespace labels

result := {
    "business_unit": bu,
    "service_owner": owner,
    "criticality": crit,
    "sla": sla,
    "source": "namespace-labels"
} if {
    bu := object.get(input.namespace.labels, "kubernaut.ai/business-unit", "")
    bu != ""
    owner := object.get(input.namespace.labels, "kubernaut.ai/service-owner", "")
    crit := object.get(input.namespace.labels, "kubernaut.ai/criticality", "medium")
    sla := object.get(input.namespace.labels, "kubernaut.ai/sla-tier", "tier-3")
}

# ============================================================================
# ADDITIONAL RULES (Operator Customization)
# ============================================================================
# Operators can add additional detection rules here.
# Examples:
#
# Infer from namespace name:
# result := {"business_unit": "payments", ...} if {
#     startswith(input.namespace.name, "payments-")
# }

# ============================================================================
# DEFAULT CATCH-ALL (MANDATORY)
# ============================================================================
# This is the AUTHORITATIVE default when no specific rule matches.
# Operators SHOULD customize this to match their organization structure.
# Go code has NO fallback - this default is the single source of truth.
#
# Example customizations:
# - default result := {"business_unit": "unassigned", ...}
# - default result := {"business_unit": "", ...} (empty = no classification)
#
default result := {
    "business_unit": "",
    "service_owner": "",
    "criticality": "",
    "sla": "",
    "source": "unclassified"
}
