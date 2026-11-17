package cancellation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/jordigilh/kubernaut/pkg/shared/cancellation"
)

var _ = Describe("Cancellation identity binding", func() {
	parent := cancellation.ParentIdentity{
		Name:      "rr-current",
		Namespace: "system",
		UID:       types.UID("rr-current-uid"),
	}

	It("UT-ORCH-2588-009: rejects a same-name child with a stale parent UID", func() {
		Expect(cancellation.MatchesParentIdentity(
			parent,
			cancellation.ReferenceIdentity{Name: parent.Name, Namespace: parent.Namespace, UID: types.UID("rr-old-uid")},
			nil,
			"RemediationRequest",
		)).To(BeFalse())
	})

	It("UT-ORCH-2588-010: rejects a child owned by a stale parent UID", func() {
		controller := true
		owner := metav1.OwnerReference{
			Kind:       "RemediationRequest",
			Name:       parent.Name,
			UID:        types.UID("rr-old-uid"),
			Controller: &controller,
		}

		Expect(cancellation.MatchesParentIdentity(
			parent,
			cancellation.ReferenceIdentity{Name: parent.Name, Namespace: parent.Namespace},
			[]metav1.OwnerReference{owner},
			"RemediationRequest",
		)).To(BeFalse())
	})

	It("UT-ORCH-2588-011: accepts matching and legacy UID-less identities", func() {
		controller := true
		owner := metav1.OwnerReference{
			Kind:       "RemediationRequest",
			Name:       parent.Name,
			UID:        parent.UID,
			Controller: &controller,
		}

		Expect(cancellation.MatchesParentIdentity(
			parent,
			cancellation.ReferenceIdentity(parent),
			[]metav1.OwnerReference{owner},
			"RemediationRequest",
		)).To(BeTrue())
		Expect(cancellation.MatchesParentIdentity(
			parent,
			cancellation.ReferenceIdentity{Name: parent.Name, Namespace: parent.Namespace},
			nil,
			"RemediationRequest",
		)).To(BeTrue())
	})

	It("UT-ORCH-2588-027: cancellation acknowledgement rejects fully UID-less identities", func() {
		Expect(cancellation.MatchesCancellationIdentity(
			parent,
			cancellation.ReferenceIdentity{Name: parent.Name, Namespace: parent.Namespace},
			nil,
			"RemediationRequest",
		)).To(BeFalse())

		controller := true
		owner := metav1.OwnerReference{
			Kind:       "RemediationRequest",
			Name:       parent.Name,
			UID:        parent.UID,
			Controller: &controller,
		}
		Expect(cancellation.MatchesCancellationIdentity(
			parent,
			cancellation.ReferenceIdentity{Name: parent.Name, Namespace: parent.Namespace},
			[]metav1.OwnerReference{owner},
			"RemediationRequest",
		)).To(BeTrue())
	})

	It("UT-ORCH-2588-013: rejects a child with no parent reference or controller owner", func() {
		Expect(cancellation.MatchesParentIdentity(parent, cancellation.ReferenceIdentity{}, nil, "RemediationRequest")).To(BeFalse())
	})
})
