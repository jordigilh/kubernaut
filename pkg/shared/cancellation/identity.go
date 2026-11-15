// Package cancellation contains shared cancellation lifecycle contracts.
package cancellation

import (
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ErrIdentityMismatch indicates that a child does not belong to the live
// parent incarnation observed during cancellation.
var ErrIdentityMismatch = errors.New("cancellation identity mismatch")

// ParentIdentity identifies the live parent object observed by a child
// controller during cancellation.
type ParentIdentity struct {
	Name      string
	Namespace string
	UID       types.UID
}

// ReferenceIdentity is the identity copied into a child spec reference.
// Empty UID values are supported for legacy resources created before UID
// propagation was required.
type ReferenceIdentity struct {
	Name      string
	Namespace string
	UID       types.UID
}

// MatchesParentIdentity prevents a same-name resource from being treated as
// the parent of a child from an older RemediationRequest incarnation. Any
// non-empty UID must match. Resources with both UID sources absent retain the
// explicit legacy compatibility path.
func MatchesParentIdentity(
	parent ParentIdentity,
	reference ReferenceIdentity,
	ownerReferences []metav1.OwnerReference,
	ownerKind string,
) bool {
	if !referenceMatchesParent(parent, reference) {
		return false
	}

	owner, found := controllerOwner(ownerReferences)
	if reference.Name == "" && !found {
		return false
	}
	if !found {
		return true
	}
	return ownerMatchesParent(parent, owner, ownerKind)
}

// MatchesCancellationIdentity validates a child identity before its
// cancellation acknowledgement is trusted. Legacy UID-less references remain
// usable for creation/reuse through MatchesParentIdentity, but cancellation
// acknowledgement requires at least one incarnation binding. This prevents a
// fully UID-less, owner-less child from crossing same-name parent incarnations.
func MatchesCancellationIdentity(
	parent ParentIdentity,
	reference ReferenceIdentity,
	ownerReferences []metav1.OwnerReference,
	ownerKind string,
) bool {
	owner, found := controllerOwner(ownerReferences)
	if reference.UID == "" && (!found || owner.UID == "") {
		return false
	}
	return MatchesParentIdentity(parent, reference, ownerReferences, ownerKind)
}

func referenceMatchesParent(parent ParentIdentity, reference ReferenceIdentity) bool {
	if parent.Name == "" || parent.Namespace == "" {
		return false
	}
	if reference.Name != "" && reference.Name != parent.Name {
		return false
	}
	if reference.Namespace != "" && reference.Namespace != parent.Namespace {
		return false
	}
	return reference.UID == "" || reference.UID == parent.UID
}

func ownerMatchesParent(parent ParentIdentity, owner *metav1.OwnerReference, ownerKind string) bool {
	if owner == nil {
		return false
	}
	if ownerKind != "" && owner.Kind != ownerKind {
		return false
	}
	if owner.Name != parent.Name {
		return false
	}
	return owner.UID == "" || owner.UID == parent.UID
}

func controllerOwner(ownerReferences []metav1.OwnerReference) (*metav1.OwnerReference, bool) {
	for i := range ownerReferences {
		owner := &ownerReferences[i]
		if owner.Controller != nil && *owner.Controller {
			return owner, true
		}
	}
	return nil, false
}
