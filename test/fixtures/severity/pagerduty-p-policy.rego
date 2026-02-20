# PagerDuty Priority Scheme (P0-P4) - Test Fixture
# DD-SEVERITY-001 v1.1: Custom severity mapping for testing
#
# This policy demonstrates PagerDuty "P" priority scheme mapping
# to normalized severity values (critical, high, medium, low, unknown)
# using a map-based lookup with lower() normalization.
#
# Usage in tests:
#   - Load this policy into SignalProcessing classifier
#   - Send alerts with severity="P0", "P1", "P2", "P3", "P4"
#   - Verify normalized severity in SignalProcessing.Status.Severity

package signalprocessing.severity

import rego.v1

# Map-based lookup: all case variants handled via lower()
# PagerDuty P0 and P1 both map to critical (different impact levels)
severity_map := {
    "p0": "critical",
    "p1": "critical",
    "p2": "high",
    "p3": "medium",
    "p4": "low",
}

result := {"severity": severity_map[lower(input.signal.severity)], "source": "rego-policy"} if {
    lower(input.signal.severity) in object.keys(severity_map)
}

# Fallback: unmapped severity -> unknown
default result := {"severity": "unknown", "source": "fallback"}
