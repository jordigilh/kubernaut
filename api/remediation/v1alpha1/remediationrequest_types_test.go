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

package v1alpha1_test

import (
	"encoding/json"
	"reflect"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

func TestRemediationRequestTypes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Remediation Request Types Suite")
}

var _ = Describe("RemediationRequestSpec Multi-Cluster Fields (ADR-065, BR-INTEGRATION-065)", func() {

	// Issue #1651: ClusterName was removed because it is non-unique and unsafe
	// for cluster disambiguation. ClusterID is the sole supported identifier.
	It("UT-CRD-1651-001: ClusterName field has been removed from RemediationRequestSpec", func() {
		_, found := reflect.TypeOf(v1alpha1.RemediationRequestSpec{}).FieldByName("ClusterName")
		Expect(found).To(BeFalse(), "RemediationRequestSpec.ClusterName must not exist (issue #1651: non-unique, unsafe for disambiguation)")
	})

	Describe("Backward Compatibility", func() {
		It("UT-CRD-065-001: deserializes pre-federation JSON without ClusterID", func() {
			oldJSON := `{
				"signalFingerprint": "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				"signalName": "CrashLoopBackOff",
				"severity": "critical",
				"signalType": "alert",
				"targetType": "kubernetes",
				"targetResource": {"kind": "Pod", "name": "web-1", "namespace": "default"},
				"firingTime": "2026-01-01T00:00:00Z",
				"receivedTime": "2026-01-01T00:00:01Z"
			}`

			var spec v1alpha1.RemediationRequestSpec
			err := json.Unmarshal([]byte(oldJSON), &spec)
			Expect(err).ToNot(HaveOccurred())

			Expect(spec.ClusterID).To(BeEmpty(), "ClusterID should default to empty for old payloads")
			Expect(spec.SignalFingerprint).To(Equal("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"))
			Expect(spec.TargetResource.Kind).To(Equal("Pod"))
		})

		It("UT-CRD-065-002: omits ClusterID from JSON when empty", func() {
			spec := v1alpha1.RemediationRequestSpec{
				SignalFingerprint: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				SignalName:        "CrashLoopBackOff",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource:    v1alpha1.ResourceIdentifier{Kind: "Pod", Name: "web-1", Namespace: "default"},
				FiringTime:        metav1.Now(),
				ReceivedTime:      metav1.Now(),
			}

			data, err := json.Marshal(spec)
			Expect(err).ToNot(HaveOccurred())

			Expect(string(data)).ToNot(ContainSubstring("clusterID"))
			Expect(string(data)).ToNot(ContainSubstring("clusterName"))
		})
	})

	Describe("Multi-Cluster Serialization", func() {
		It("UT-CRD-065-003: round-trips ClusterID through JSON", func() {
			spec := v1alpha1.RemediationRequestSpec{
				SignalFingerprint: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				SignalName:        "HighMemoryUsage",
				Severity:          "warning",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource:    v1alpha1.ResourceIdentifier{Kind: "Deployment", Name: "api-server", Namespace: "prod"},
				FiringTime:        metav1.Now(),
				ReceivedTime:      metav1.Now(),
				ClusterID:         "prod-east-1",
			}

			data, err := json.Marshal(spec)
			Expect(err).ToNot(HaveOccurred())

			var roundTripped v1alpha1.RemediationRequestSpec
			err = json.Unmarshal(data, &roundTripped)
			Expect(err).ToNot(HaveOccurred())

			Expect(roundTripped.ClusterID).To(Equal("prod-east-1"))
		})

		It("UT-CRD-065-004: includes ClusterID in JSON when populated", func() {
			spec := v1alpha1.RemediationRequestSpec{
				SignalFingerprint: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				SignalName:        "CrashLoopBackOff",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource:    v1alpha1.ResourceIdentifier{Kind: "Pod", Name: "web-1", Namespace: "default"},
				FiringTime:        metav1.Now(),
				ReceivedTime:      metav1.Now(),
				ClusterID:         "staging-west",
			}

			data, err := json.Marshal(spec)
			Expect(err).ToNot(HaveOccurred())

			Expect(string(data)).To(ContainSubstring(`"clusterID":"staging-west"`))
		})

		It("UT-CRD-065-005: empty ClusterID indicates local hub cluster", func() {
			spec := v1alpha1.RemediationRequestSpec{
				SignalFingerprint: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				SignalName:        "CrashLoopBackOff",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource:    v1alpha1.ResourceIdentifier{Kind: "Pod", Name: "web-1", Namespace: "default"},
				FiringTime:        metav1.Now(),
				ReceivedTime:      metav1.Now(),
			}

			Expect(spec.ClusterID).To(BeEmpty(), "empty ClusterID == local hub cluster")
		})
	})
})

// RemediationRequestStatus god-struct decomposition (issue #2206, following #2205's
// AIAnalysisStatus precedent): 40 of 50 top-level fields are grouped into 5 pointer
// sub-structs (PhaseProgress, RoutingStatus, CompletionStatus, OperatorAudit,
// WorkflowSelection), each with a GetX()/EnsureX() nil-safe accessor pair.
var _ = Describe("RemediationRequestStatus God-Struct Decomposition (BR-COMMON-001, issue #2206)", func() {
	Describe("GetPhaseProgress / EnsurePhaseProgress", func() {
		It("UT-CRD-2206-001: GetPhaseProgress returns zero-value without mutating Status when nil", func() {
			status := v1alpha1.RemediationRequestStatus{}

			pp := status.GetPhaseProgress()

			Expect(pp).ToNot(BeNil())
			Expect(*pp).To(Equal(v1alpha1.PhaseProgress{}))
			Expect(status.PhaseProgress).To(BeNil(), "GetPhaseProgress must not initialize the underlying pointer")
		})

		It("UT-CRD-2206-002: EnsurePhaseProgress lazily initializes and returns the same instance on repeated calls", func() {
			status := v1alpha1.RemediationRequestStatus{}

			first := status.EnsurePhaseProgress()
			Expect(first).ToNot(BeNil())
			Expect(status.PhaseProgress).ToNot(BeNil(), "EnsurePhaseProgress must initialize the underlying pointer")

			first.AIAnalysisRef = &corev1.ObjectReference{Name: "ai-1"}
			second := status.EnsurePhaseProgress()
			Expect(second).To(BeIdenticalTo(first), "EnsurePhaseProgress must return the same instance, not reallocate")
			Expect(second.AIAnalysisRef).ToNot(BeNil())
			Expect(second.AIAnalysisRef.Name).To(Equal("ai-1"))
		})
	})

	Describe("GetRoutingStatus / EnsureRoutingStatus", func() {
		It("UT-CRD-2206-003: GetRoutingStatus returns zero-value without mutating Status when nil", func() {
			status := v1alpha1.RemediationRequestStatus{}

			rs := status.GetRoutingStatus()

			Expect(rs).ToNot(BeNil())
			Expect(*rs).To(Equal(v1alpha1.RoutingStatus{}))
			Expect(status.RoutingStatus).To(BeNil(), "GetRoutingStatus must not initialize the underlying pointer")
		})

		It("UT-CRD-2206-004: EnsureRoutingStatus lazily initializes and returns the same instance on repeated calls", func() {
			status := v1alpha1.RemediationRequestStatus{}

			first := status.EnsureRoutingStatus()
			Expect(first).ToNot(BeNil())
			Expect(status.RoutingStatus).ToNot(BeNil(), "EnsureRoutingStatus must initialize the underlying pointer")

			first.DuplicateOf = "rr-original"
			second := status.EnsureRoutingStatus()
			Expect(second).To(BeIdenticalTo(first), "EnsureRoutingStatus must return the same instance, not reallocate")
			Expect(second.DuplicateOf).To(Equal("rr-original"))
		})
	})

	Describe("GetCompletionStatus / EnsureCompletionStatus", func() {
		It("UT-CRD-2206-005: GetCompletionStatus returns zero-value without mutating Status when nil", func() {
			status := v1alpha1.RemediationRequestStatus{}

			cs := status.GetCompletionStatus()

			Expect(cs).ToNot(BeNil())
			Expect(*cs).To(Equal(v1alpha1.CompletionStatus{}))
			Expect(status.CompletionStatus).To(BeNil(), "GetCompletionStatus must not initialize the underlying pointer")
		})

		It("UT-CRD-2206-006: EnsureCompletionStatus lazily initializes and returns the same instance on repeated calls", func() {
			status := v1alpha1.RemediationRequestStatus{}

			first := status.EnsureCompletionStatus()
			Expect(first).ToNot(BeNil())
			Expect(status.CompletionStatus).ToNot(BeNil(), "EnsureCompletionStatus must initialize the underlying pointer")

			first.RequiresManualReview = true
			second := status.EnsureCompletionStatus()
			Expect(second).To(BeIdenticalTo(first), "EnsureCompletionStatus must return the same instance, not reallocate")
			Expect(second.RequiresManualReview).To(BeTrue())
		})
	})

	Describe("GetOperatorAudit / EnsureOperatorAudit", func() {
		It("UT-CRD-2206-007: GetOperatorAudit returns zero-value without mutating Status when nil", func() {
			status := v1alpha1.RemediationRequestStatus{}

			oa := status.GetOperatorAudit()

			Expect(oa).ToNot(BeNil())
			Expect(*oa).To(Equal(v1alpha1.OperatorAudit{}))
			Expect(status.OperatorAudit).To(BeNil(), "GetOperatorAudit must not initialize the underlying pointer")
		})

		It("UT-CRD-2206-008: EnsureOperatorAudit lazily initializes and returns the same instance on repeated calls", func() {
			status := v1alpha1.RemediationRequestStatus{}

			first := status.EnsureOperatorAudit()
			Expect(first).ToNot(BeNil())
			Expect(status.OperatorAudit).ToNot(BeNil(), "EnsureOperatorAudit must initialize the underlying pointer")

			first.LastModifiedBy = "alice"
			second := status.EnsureOperatorAudit()
			Expect(second).To(BeIdenticalTo(first), "EnsureOperatorAudit must return the same instance, not reallocate")
			Expect(second.LastModifiedBy).To(Equal("alice"))
		})
	})

	Describe("GetWorkflowSelection / EnsureWorkflowSelection", func() {
		It("UT-CRD-2206-009: GetWorkflowSelection returns zero-value without mutating Status when nil", func() {
			status := v1alpha1.RemediationRequestStatus{}

			ws := status.GetWorkflowSelection()

			Expect(ws).ToNot(BeNil())
			Expect(*ws).To(Equal(v1alpha1.WorkflowSelection{}))
			Expect(status.WorkflowSelection).To(BeNil(), "GetWorkflowSelection must not initialize the underlying pointer")
		})

		It("UT-CRD-2206-010: EnsureWorkflowSelection lazily initializes and returns the same instance on repeated calls", func() {
			status := v1alpha1.RemediationRequestStatus{}

			first := status.EnsureWorkflowSelection()
			Expect(first).ToNot(BeNil())
			Expect(status.WorkflowSelection).ToNot(BeNil(), "EnsureWorkflowSelection must initialize the underlying pointer")

			first.TargetDisplay = "Deployment/web-frontend"
			second := status.EnsureWorkflowSelection()
			Expect(second).To(BeIdenticalTo(first), "EnsureWorkflowSelection must return the same instance, not reallocate")
			Expect(second.TargetDisplay).To(Equal("Deployment/web-frontend"))
		})
	})
})
