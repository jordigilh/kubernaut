# Severity Determination Policy - Example
# BR-SP-105: Severity Determination via Rego Policy
# DD-SEVERITY-001: Strategy B - Policy-Defined Fallback
#
# This policy demonstrates severity determination with operator-controlled fallback.
# Operators MUST define fallback behavior (no system-imposed "unknown" fallback).
#
# Two example strategies shown:
# 1. Conservative: unmapped severities escalate to "critical" (safety-first)
# 2. Permissive: unmapped severities downgrade to "info" (ignore unknown)

package signalprocessing.severity

# ========================================
# CONSERVATIVE POLICY (Safety-First)
# Recommended for production environments
# ========================================
# Unmapped severity values escalate to "critical" for safety

# Standard Prometheus severity values
determine_severity := "critical" {
	input.signal.severity == "critical"
}

determine_severity := "warning" {
	input.signal.severity == "warning"
}

determine_severity := "info" {
	input.signal.severity == "info"
}

# PagerDuty P0-P4 severity scheme
determine_severity := "critical" {
	input.signal.severity == "P0"
}

determine_severity := "critical" {
	input.signal.severity == "P1"
}

determine_severity := "warning" {
	input.signal.severity == "P2"
}

determine_severity := "warning" {
	input.signal.severity == "P3"
}

determine_severity := "info" {
	input.signal.severity == "P4"
}

# Enterprise Sev1-4 severity scheme
determine_severity := "critical" {
	input.signal.severity == "Sev1"
}

determine_severity := "warning" {
	input.signal.severity == "Sev2"
}

determine_severity := "warning" {
	input.signal.severity == "Sev3"
}

determine_severity := "info" {
	input.signal.severity == "Sev4"
}

# CONSERVATIVE FALLBACK: Unmapped severities escalate to critical
# This ensures unknown alerts receive immediate attention
determine_severity := "critical" {
	# Catch-all: any unmapped severity value maps to critical
	true
}

# ========================================
# ALTERNATIVE: PERMISSIVE POLICY
# For development/testing environments
# ========================================
# Uncomment the policy below and comment out the conservative policy above
# to use permissive fallback (unmapped → info)

# determine_severity := "critical" {
# 	input.signal.severity == "critical"
# }
#
# determine_severity := "warning" {
# 	input.signal.severity == "warning"
# }
#
# determine_severity := "info" {
# 	input.signal.severity == "info"
# }
#
# # PERMISSIVE FALLBACK: Unmapped severities downgrade to info
# # This treats unknown alerts as informational only
# determine_severity := "info" {
# 	# Catch-all: any unmapped severity value maps to info
# 	true
# }

# ========================================
# POLICY REQUIREMENTS (DD-SEVERITY-001)
# ========================================
# 1. MUST return one of: "critical", "warning", "info"
# 2. MUST include catch-all else clause (no system fallback to "unknown")
# 3. MUST compile successfully (validated at controller startup)
# 4. Policy updates via ConfigMap hot-reload (5-second fsnotify)
#
# Example validation:
#   opa eval -d severity.rego 'data.signalprocessing.severity.determine_severity' \
#     -i '{"signal": {"severity": "P0"}}'
#   # Expected output: "critical"
