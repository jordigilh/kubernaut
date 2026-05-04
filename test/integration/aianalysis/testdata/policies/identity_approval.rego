package aianalysis.approval

import rego.v1

# Identity-aware approval policy fixture for IT-KA-774 integration tests.
# Auto-approves when acting user belongs to "sre" group.
# Requires approval for all other users/groups and autonomous flows.

default require_approval := true

default reason := "Approval required (no SRE group membership or autonomous flow)"

require_approval := false if {
    input.identity
    some g in input.identity.groups
    g == "sre"
}

reason := "Auto-approved: SRE group membership" if {
    input.identity
    some g in input.identity.groups
    g == "sre"
}
