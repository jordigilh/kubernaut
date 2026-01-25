# PagerDuty Priority Scheme (P0-P4) - Test Fixture
# DD-SEVERITY-001 v1.1: Custom severity mapping for testing
#
# This policy demonstrates PagerDuty "P" priority scheme mapping
# to normalized severity values (critical, high, medium, low, unknown)
#
# Usage in tests:
#   - Load this policy into SignalProcessing classifier
#   - Send alerts with severity="P0", "P1", "P2", "P3", "P4"
#   - Verify normalized severity in SignalProcessing.Status.Severity

package signalprocessing.severity

import rego.v1

# PagerDuty P0 → Critical (all-hands production outage)
result := {"severity": "critical", "source": "rego-policy"} if {
	input.signal.severity in ["P0", "p0"]
}

# PagerDuty P1 → Critical (severe customer impact)
result := {"severity": "critical", "source": "rego-policy"} if {
	input.signal.severity in ["P1", "p1"]
}

# PagerDuty P2 → High (moderate impact)
result := {"severity": "high", "source": "rego-policy"} if {
	input.signal.severity in ["P2", "p2"]
}

# PagerDuty P3 → Medium (low priority)
result := {"severity": "medium", "source": "rego-policy"} if {
	input.signal.severity in ["P3", "p3"]
}

# PagerDuty P4 → Low (informational)
result := {"severity": "low", "source": "rego-policy"} if {
	input.signal.severity in ["P4", "p4"]
}

# Fallback: unmapped severity → unknown
default result := {"severity": "unknown", "source": "fallback"}
