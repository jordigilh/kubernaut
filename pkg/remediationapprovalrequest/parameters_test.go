/*
Copyright 2026 Jordi Gil.

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

package remediationapprovalrequest_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("RemediationApprovalRequest parameter validation", func() {
	It("UT-AW-2570-100: BR-KA-191 accepts injected target API version absent from the workflow schema", func() {
		spec := &remediationv1.RemediationApprovalRequestSpec{
			Workflow: &sharedtypes.WorkflowSnapshot{
				WorkflowID:             "wf-target-api-version",
				DeclaredParameterNames: map[string]bool{"P": true},
			},
			Parameters: map[string]string{
				"P":                           "safe",
				"TARGET_RESOURCE_API_VERSION": "apps/v1",
			},
			ParameterSchema: []remediationworkflowv1.RemediationWorkflowParameter{
				{Name: "P", Type: "string"},
			},
			TargetContext: &remediationv1.ApprovalTargetContext{},
		}

		Expect(remediationapprovalrequest.ValidateParameterOverride(spec, nil)).To(Succeed())
	})

	It("UT-AW-2570-101: BR-KA-191 accepts an original parameter depending on an injected target value", func() {
		spec := managedDependencySpec()

		Expect(remediationapprovalrequest.ValidateParameterOverride(spec, nil)).To(Succeed())
	})

	It("UT-AW-2570-102: BR-KA-191 accepts an override when its dependency is an injected target value", func() {
		spec := managedDependencySpec()

		Expect(remediationapprovalrequest.ValidateParameterOverride(spec, map[string]string{
			"MEMORY_LIMIT_NEW": "1Gi",
		})).To(Succeed())
	})
})

func managedDependencySpec() *remediationv1.RemediationApprovalRequestSpec {
	return &remediationv1.RemediationApprovalRequestSpec{
		Workflow: &sharedtypes.WorkflowSnapshot{
			WorkflowID:             "wf-managed-dependency",
			DeclaredParameterNames: map[string]bool{"MEMORY_LIMIT_NEW": true},
		},
		Parameters: map[string]string{
			"MEMORY_LIMIT_NEW":     "512Mi",
			"TARGET_RESOURCE_KIND": "Deployment",
		},
		ParameterSchema: []remediationworkflowv1.RemediationWorkflowParameter{
			{
				Name:      "MEMORY_LIMIT_NEW",
				Type:      "string",
				Required:  true,
				DependsOn: []string{"TARGET_RESOURCE_KIND"},
			},
		},
		TargetContext: &remediationv1.ApprovalTargetContext{},
	}
}
