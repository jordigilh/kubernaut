/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package authwebhook

import (
	"fmt"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// REFACTOR-AW-001: Decision validation extracted for reusability and testability
// Reference: BR-AUDIT-006, ADR-040 (RemediationApprovalRequest CRD)

// ValidateApprovalDecision validates that a decision is one of the allowed enum values.
// Returns an error if the decision is invalid.
func ValidateApprovalDecision(decision remediationv1.ApprovalDecision) error {
	validDecisions := map[remediationv1.ApprovalDecision]bool{
		remediationv1.ApprovalDecisionApproved: true,
		remediationv1.ApprovalDecisionRejected: true,
		remediationv1.ApprovalDecisionExpired:  true,
	}

	if !validDecisions[decision] {
		return fmt.Errorf("invalid decision: %s (must be Approved, Rejected, or Expired)", decision)
	}

	return nil
}
