# Severity Determination Policy - Example
# BR-SP-105: Severity Determination via Rego Policy
# DD-SEVERITY-001 v1.1: Strategy B - Policy-Defined Fallback
# ADR-066: 4-level severity model (critical > high > warning > info)
#
# This policy demonstrates severity determination with operator-controlled fallback.
# Operators MUST define fallback behavior (no system-imposed "unknown" fallback).
#
# Valid normalized values: "critical", "high", "warning", "info", "unknown"
#
# Two example strategies shown:
# 1. Conservative: unmapped severities escalate to "critical" (safety-first)
# 2. Permissive: unmapped severities downgrade to "unknown" (requires operator investigation)

package signalprocessing.severity

# ========================================
# CONSERVATIVE POLICY (Safety-First)
# Recommended for production environments
# ========================================
# Unmapped severity values escalate to "critical" for safety

# Standard normalized severity values (ADR-066: 4-level model)
# Uses lower() for case-insensitive matching of external monitoring severity values.
determine_severity := "critical" if {
	lower(input.signal.severity) == "critical"
}

determine_severity := "high" if {
	lower(input.signal.severity) == "high"
}

determine_severity := "warning" if {
	lower(input.signal.severity) == "warning"
}

determine_severity := "warning" if {
	lower(input.signal.severity) == "medium"
}

determine_severity := "info" if {
	lower(input.signal.severity) == "info"
}

determine_severity := "info" if {
	lower(input.signal.severity) == "low"
}

determine_severity := "unknown" if {
	lower(input.signal.severity) == "unknown"
}

# PagerDuty P0-P4 severity scheme
determine_severity := "critical" if {
	lower(input.signal.severity) == "p0"
}

determine_severity := "critical" if {
	lower(input.signal.severity) == "p1"
}

determine_severity := "high" if {
	lower(input.signal.severity) == "p2"
}

determine_severity := "warning" if {
	lower(input.signal.severity) == "p3"
}

determine_severity := "info" if {
	lower(input.signal.severity) == "p4"
}

# Enterprise Sev1-4 severity scheme
determine_severity := "critical" if {
	lower(input.signal.severity) == "sev1"
}

determine_severity := "high" if {
	lower(input.signal.severity) == "sev2"
}

determine_severity := "warning" if {
	lower(input.signal.severity) == "sev3"
}

determine_severity := "info" if {
	lower(input.signal.severity) == "sev4"
}

# CONSERVATIVE FALLBACK: Unmapped severities escalate to critical
# This ensures unknown alerts receive immediate attention
determine_severity := "critical" if {
	# Catch-all: any unmapped severity value maps to critical
	true
}

# ========================================
# ALTERNATIVE: PERMISSIVE POLICY
# For development/testing environments
# ========================================
# Uncomment the policy below and comment out the conservative policy above
# to use permissive fallback (unmapped → unknown)

# determine_severity := "critical" if {
# 	input.signal.severity == "critical"
# }
#
# determine_severity := "high" if {
# 	input.signal.severity == "high"
# }
#
# determine_severity := "warning" if {
# 	input.signal.severity == "warning"
# }
#
# determine_severity := "info" if {
# 	input.signal.severity == "info"
# }
#
# # PERMISSIVE FALLBACK: Unmapped severities downgrade to unknown
# # This requires operator investigation of unmapped values
# determine_severity := "unknown" if {
# 	# Catch-all: any unmapped severity value maps to unknown
# 	true
# }

# ========================================
# POLICY REQUIREMENTS (ADR-066)
# ========================================
# 1. MUST return one of: "critical", "high", "warning", "info", "unknown"
# 2. MUST include catch-all else clause (operator-defined fallback behavior)
# 3. MUST compile successfully (validated at controller startup)
# 4. Policy updates via ConfigMap hot-reload (5-second fsnotify)
# 5. ADR-066: 4-level model aligned with Prometheus vocabulary
#
# Example validation:
#   opa eval -d severity.rego 'data.signalprocessing.severity.determine_severity' \
#     -i '{"signal": {"severity": "P0"}}'
#   # Expected output: "critical"
