# AIAnalysis Approval Policy - Always Approve (Test Fixture)
# Used by unit tests that need the handler to proceed past Rego evaluation
# without requiring specific input field values.

package aianalysis.approval

import rego.v1

default require_approval := false
default reason := "Auto-approved by always-approve test fixture"
