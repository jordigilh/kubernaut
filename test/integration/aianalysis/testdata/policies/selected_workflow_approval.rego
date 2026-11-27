package aianalysis.approval

import rego.v1

# Trust-ladder Stage 3 fixture for #2714 / BR-AI-085 FR-AI-085-007 tests.
# Auto-approves only a specifically reviewed workflow identity read from the
# catalog-authoritative input.selected_workflow object (id, name, version).
# Requires approval for every other workflow and for flows where the field
# is absent (autonomous default-deny).

default require_approval := true

default reason := "Approval required (workflow identity not in trusted set)"

trusted_workflows := {
	"increase-memory-limits-gitops-v1": {"1.4.1"},
}

require_approval := false if {
	input.selected_workflow
	input.selected_workflow.id != ""
	trusted_workflows[input.selected_workflow.name]
	input.selected_workflow.version in trusted_workflows[input.selected_workflow.name]
}

reason := "Auto-approved: trusted workflow identity" if {
	input.selected_workflow
	input.selected_workflow.id != ""
	trusted_workflows[input.selected_workflow.name]
	input.selected_workflow.version in trusted_workflows[input.selected_workflow.name]
}
