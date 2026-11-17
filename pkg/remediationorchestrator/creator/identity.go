package creator

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/cancellation"
)

func validateExistingChildIdentity(
	parent *remediationv1.RemediationRequest,
	reference cancellation.ReferenceIdentity,
	ownerReferences []metav1.OwnerReference,
	childKind string,
) error {
	if cancellation.MatchesParentIdentity(
		cancellation.ParentIdentity{
			Name:      parent.Name,
			Namespace: parent.Namespace,
			UID:       parent.UID,
		},
		reference,
		ownerReferences,
		"RemediationRequest",
	) {
		return nil
	}
	return fmt.Errorf("existing %s identity mismatch: %w", childKind, cancellation.ErrIdentityMismatch)
}
