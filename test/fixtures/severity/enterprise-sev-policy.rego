# Enterprise Severity Scheme (Sev1-4) - Test Fixture
# DD-SEVERITY-001 v1.1: Custom severity mapping for testing
#
# This policy demonstrates Enterprise "Sev" severity scheme mapping
# to normalized severity values (critical, high, medium, low, unknown)
# using a map-based lookup with lower() normalization.
#
# Usage in tests:
#   - Load this policy into SignalProcessing classifier
#   - Send alerts with severity="Sev1", "SEV1", "sev1"
#   - Verify normalized severity in SignalProcessing.Status.Severity

package signalprocessing.severity

import rego.v1

# Map-based lookup: all case variants handled via lower()
severity_map := {
    "sev1": "critical",
    "sev2": "high",
    "sev3": "medium",
    "sev4": "low",
}

result := {"severity": severity_map[lower(input.signal.severity)], "source": "rego-policy"} if {
    lower(input.signal.severity) in object.keys(severity_map)
}

# Fallback: unmapped severity -> unknown
default result := {"severity": "unknown", "source": "fallback"}
