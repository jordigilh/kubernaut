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

package remediationrequest

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
)

var _ = Describe("Issue #635: Display Helpers for RR kubectl Columns", func() {

	// ========================================
	// UT-RO-635-001: FormatResourceDisplay standard namespaced resource
	// ========================================

	Describe("FormatResourceDisplay", func() {
		It("UT-RO-635-001: should produce Kind/Name for standard namespaced resource", func() {
			result := remediationrequest.FormatResourceDisplay("Deployment", "web-frontend")
			Expect(result).To(Equal("Deployment/web-frontend"))
		})

		// UT-RO-635-002: FormatResourceDisplay with empty inputs
		It("UT-RO-635-002a: should return just name when Kind is empty", func() {
			result := remediationrequest.FormatResourceDisplay("", "web-frontend")
			Expect(result).To(Equal("web-frontend"))
		})

		It("UT-RO-635-002b: should return empty string when Name is empty", func() {
			result := remediationrequest.FormatResourceDisplay("Deployment", "")
			Expect(result).To(Equal(""))
		})

		It("UT-RO-635-002c: should return empty string when both are empty", func() {
			result := remediationrequest.FormatResourceDisplay("", "")
			Expect(result).To(Equal(""))
		})

		// UT-RO-635-007: cluster-scoped resource
		It("UT-RO-635-007: should produce Kind/Name for cluster-scoped resource (Node)", func() {
			result := remediationrequest.FormatResourceDisplay("Node", "worker-1")
			Expect(result).To(Equal("Node/worker-1"))
		})

		It("should handle StatefulSet resource", func() {
			result := remediationrequest.FormatResourceDisplay("StatefulSet", "redis-cluster")
			Expect(result).To(Equal("StatefulSet/redis-cluster"))
		})
	})

	// ========================================
	// UT-RO-635-003/004: FormatWorkflowDisplay
	// ========================================

	Describe("FormatWorkflowDisplay", func() {
		It("UT-RO-635-003: should produce ActionType:WorkflowID for standard workflow", func() {
			result := remediationrequest.FormatWorkflowDisplay("GitRevertCommit", "git-revert-v2")
			Expect(result).To(Equal("GitRevertCommit:git-revert-v2"))
		})

		It("UT-RO-635-004a: should return just WorkflowID when ActionType is empty", func() {
			result := remediationrequest.FormatWorkflowDisplay("", "git-revert-v2")
			Expect(result).To(Equal("git-revert-v2"))
		})

		It("UT-RO-635-004b: should return empty when WorkflowID is empty", func() {
			result := remediationrequest.FormatWorkflowDisplay("GitRevertCommit", "")
			Expect(result).To(Equal(""))
		})

		It("UT-RO-635-004c: should return empty when both are empty", func() {
			result := remediationrequest.FormatWorkflowDisplay("", "")
			Expect(result).To(Equal(""))
		})

		It("should handle RestartPod action type", func() {
			result := remediationrequest.FormatWorkflowDisplay("RestartPod", "restart-pod-v1")
			Expect(result).To(Equal("RestartPod:restart-pod-v1"))
		})
	})

	// ========================================
	// UT-RO-635-005/008: FormatConfidence
	// ========================================

	Describe("FormatConfidence", func() {
		It("UT-RO-635-005: should produce string from standard confidence value", func() {
			result := remediationrequest.FormatConfidence(0.97)
			Expect(result).To(Equal("0.97"))
		})

		It("UT-RO-635-008a: should produce 0.00 for zero confidence", func() {
			result := remediationrequest.FormatConfidence(0.0)
			Expect(result).To(Equal("0.00"))
		})

		It("UT-RO-635-008b: should produce 1.00 for perfect confidence", func() {
			result := remediationrequest.FormatConfidence(1.0)
			Expect(result).To(Equal("1.00"))
		})

		It("UT-RO-635-008c: should return empty string for negative confidence (invalid)", func() {
			result := remediationrequest.FormatConfidence(-1.0)
			Expect(result).To(Equal(""))
		})

		It("should produce 0.50 for mid-range confidence", func() {
			result := remediationrequest.FormatConfidence(0.5)
			Expect(result).To(Equal("0.50"))
		})
	})

	// ========================================
	// UT-RO-635-006: New status fields exist
	// ========================================

	Describe("New status fields on RemediationRequestStatus", func() {
		It("UT-RO-635-006: should accept TargetDisplay, Confidence, WorkflowDisplayName, SignalTargetDisplay", func() {
			status := remediationv1.RemediationRequestStatus{
				TargetDisplay:       "Deployment/web-frontend",
				Confidence:          "0.97",
				WorkflowDisplayName: "GitRevertCommit:git-revert-v2",
				SignalTargetDisplay:  "Pod/web-frontend-cdbdbc4f8-6kn6j",
			}
			Expect(status.TargetDisplay).To(Equal("Deployment/web-frontend"))
			Expect(status.Confidence).To(Equal("0.97"))
			Expect(status.WorkflowDisplayName).To(Equal("GitRevertCommit:git-revert-v2"))
			Expect(status.SignalTargetDisplay).To(Equal("Pod/web-frontend-cdbdbc4f8-6kn6j"))
		})
	})
})
