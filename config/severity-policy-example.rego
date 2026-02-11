# Severity Determination Policy - Example
# BR-SP-105: Severity Determination via Rego Policy
# DD-SEVERITY-001 v1.1: Strategy B - Policy-Defined Fallback
#
# This policy demonstrates severity determination with operator-controlled fallback.
# Operators MUST define fallback behavior (no system-imposed "unknown" fallback).
#
# DD-SEVERITY-001 v1.1: Normalized severity values aligned with HAPI/workflow catalog
# Valid normalized values: "critical", "high", "medium", "low", "unknown"
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

# Standard normalized severity values (DD-SEVERITY-001 v1.1: aligned with HAPI/workflow catalog)
determine_severity := "critical" if {
	input.signal.severity == "critical"
}

determine_severity := "high" if {
	input.signal.severity == "high"
}

determine_severity := "medium" if {
	input.signal.severity == "medium"
}

determine_severity := "low" if {
	input.signal.severity == "low"
}

determine_severity := "unknown" if {
	input.signal.severity == "unknown"
}

# PagerDuty P0-P4 severity scheme
determine_severity := "critical" if {
	input.signal.severity == "P0"
}

determine_severity := "critical" if {
	input.signal.severity == "P1"
}

determine_severity := "high" if {
	input.signal.severity == "P2"
}

determine_severity := "medium" if {
	input.signal.severity == "P3"
}

determine_severity := "low" if {
	input.signal.severity == "P4"
}

# Enterprise Sev1-4 severity scheme
determine_severity := "critical" if {
	input.signal.severity == "Sev1"
}

determine_severity := "high" if {
	input.signal.severity == "Sev2"
}

determine_severity := "medium" if {
	input.signal.severity == "Sev3"
}

determine_severity := "low" if {
	input.signal.severity == "Sev4"
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
# determine_severity := "medium" if {
# 	input.signal.severity == "medium"
# }
#
# determine_severity := "low" if {
# 	input.signal.severity == "low"
# }
#
# # PERMISSIVE FALLBACK: Unmapped severities downgrade to unknown
# # This requires operator investigation of unmapped values
# determine_severity := "unknown" if {
# 	# Catch-all: any unmapped severity value maps to unknown
# 	true
# }

# ========================================
# POLICY REQUIREMENTS (DD-SEVERITY-001 v1.1)
# ========================================
# 1. MUST return one of: "critical", "high", "medium", "low", "unknown"
# 2. MUST include catch-all else clause (operator-defined fallback behavior)
# 3. MUST compile successfully (validated at controller startup)
# 4. Policy updates via ConfigMap hot-reload (5-second fsnotify)
# 5. DD-SEVERITY-001 v1.1: Values aligned with HAPI/workflow catalog
#
# Example validation:
#   opa eval -d severity.rego 'data.signalprocessing.severity.determine_severity' \
#     -i '{"signal": {"severity": "P0"}}'
#   # Expected output: "critical"
