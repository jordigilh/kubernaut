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
package aianalysis_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
)

// UT-AA-2029-008: K8sInvestigationSessionChecker.CorrelatedSessionID (#2029 Part B).
//
// Uses a fake client with the same field index main.go registers on the real
// manager (spec.remediationRequestRef.name), since CorrelatedSessionID (like
// HasActiveSession/FindSessionPhase) queries via that index.
var _ = Describe("K8sInvestigationSessionChecker.CorrelatedSessionID (#2029 Part B)", func() {

	const testNS = "default"

	newFakeChecker := func(objs ...crclient.Object) *handlers.K8sInvestigationSessionChecker {
		scheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(isv1alpha1.AddToScheme(scheme)).To(Succeed())

		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithIndex(&isv1alpha1.InvestigationSession{}, handlers.ISFieldIndexRRName,
				func(obj crclient.Object) []string {
					is := obj.(*isv1alpha1.InvestigationSession)
					if is.Spec.RemediationRequestRef.Name == "" {
						return nil
					}
					return []string{is.Spec.RemediationRequestRef.Name}
				}).
			WithObjects(objs...).
			WithStatusSubresource(&isv1alpha1.InvestigationSession{}).
			Build()

		return handlers.NewK8sInvestigationSessionChecker(c, testNS)
	}

	newIS := func(name, rrName, correlationID string, phase isv1alpha1.SessionPhase) *isv1alpha1.InvestigationSession {
		return &isv1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS},
			Spec: isv1alpha1.InvestigationSessionSpec{
				RemediationRequestRef: isv1alpha1.ObjectRef{Name: rrName, Namespace: testNS},
				A2ATaskID:             "task-" + name,
				UserIdentity:          isv1alpha1.SessionUser{Username: "u"},
				JoinMode:              isv1alpha1.SessionJoinModeStart,
			},
			Status: isv1alpha1.InvestigationSessionStatus{
				Phase:           phase,
				KACorrelationID: correlationID,
			},
		}
	}

	It("returns (\"\", false, nil) when no IS exists for the RR", func() {
		checker := newFakeChecker()
		id, active, err := checker.CorrelatedSessionID(context.Background(), "rr-none")
		Expect(err).NotTo(HaveOccurred())
		Expect(id).To(BeEmpty())
		Expect(active).To(BeFalse())
	})

	It("returns (\"\", false, nil) when the IS exists but no correlation has been written yet", func() {
		is := newIS("is-no-corr", "rr-no-corr", "", isv1alpha1.SessionPhaseActive)
		checker := newFakeChecker(is)
		id, active, err := checker.CorrelatedSessionID(context.Background(), "rr-no-corr")
		Expect(err).NotTo(HaveOccurred())
		Expect(id).To(BeEmpty())
		Expect(active).To(BeFalse())
	})

	It("returns the correlated ID with active=true when the IS is non-terminal", func() {
		is := newIS("is-active-corr", "rr-active-corr", "ka-session-new-001", isv1alpha1.SessionPhaseActive)
		checker := newFakeChecker(is)
		id, active, err := checker.CorrelatedSessionID(context.Background(), "rr-active-corr")
		Expect(err).NotTo(HaveOccurred())
		Expect(id).To(Equal("ka-session-new-001"))
		Expect(active).To(BeTrue())
	})

	DescribeTable("returns the correlated ID with active=false when the IS is terminal",
		func(phase isv1alpha1.SessionPhase) {
			is := newIS("is-terminal-corr", "rr-terminal-corr", "ka-session-old-002", phase)
			checker := newFakeChecker(is)
			id, active, err := checker.CorrelatedSessionID(context.Background(), "rr-terminal-corr")
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal("ka-session-old-002"),
				"the correlated ID is still reported even when terminal — callers decide what to do with active=false")
			Expect(active).To(BeFalse())
		},
		Entry("Completed", isv1alpha1.SessionPhaseCompleted),
		Entry("Cancelled", isv1alpha1.SessionPhaseCancelled),
		Entry("Failed", isv1alpha1.SessionPhaseFailed),
	)

	It("returns (\"\", false, nil) for an empty rrName without querying the API", func() {
		checker := newFakeChecker()
		id, active, err := checker.CorrelatedSessionID(context.Background(), "")
		Expect(err).NotTo(HaveOccurred())
		Expect(id).To(BeEmpty())
		Expect(active).To(BeFalse())
	})
})
