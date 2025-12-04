// Package controller provides the Kubernetes controller implementation
// for the Remediation Orchestrator.
package controller

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// SetupWithManager sets up the controller with the Manager.
// It configures watches on RemediationRequest and all owned child CRDs.
//
// Watch configuration:
// - For(&RemediationRequest{}): Primary watch on our CRD
// - Owns(&SignalProcessing{}): Watch child CRDs we create
// - Owns(&AIAnalysis{}): Watch child CRDs we create
// - Owns(&WorkflowExecution{}): Watch child CRDs we create
// - Owns(&NotificationRequest{}): Watch child CRDs we create
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Validate all required CRD schemes are registered
	if err := r.validateSchemes(mgr); err != nil {
		return fmt.Errorf("scheme validation failed: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&remediationv1.RemediationRequest{}).
		Owns(&signalprocessingv1.SignalProcessing{}).
		Owns(&aianalysisv1.AIAnalysis{}).
		Owns(&workflowexecutionv1.WorkflowExecution{}).
		Owns(&notificationv1.NotificationRequest{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.Config.MaxConcurrentReconciles,
		}).
		Complete(r)
}

// validateSchemes ensures all required CRD types are registered in the scheme.
func (r *Reconciler) validateSchemes(mgr ctrl.Manager) error {
	scheme := mgr.GetScheme()

	// List of required GVKs
	requiredTypes := []struct {
		name string
		obj  any
	}{
		{"RemediationRequest", &remediationv1.RemediationRequest{}},
		{"SignalProcessing", &signalprocessingv1.SignalProcessing{}},
		{"AIAnalysis", &aianalysisv1.AIAnalysis{}},
		{"WorkflowExecution", &workflowexecutionv1.WorkflowExecution{}},
		{"NotificationRequest", &notificationv1.NotificationRequest{}},
	}

	for _, rt := range requiredTypes {
		gvks, _, err := scheme.ObjectKinds(rt.obj.(client.Object))
		if err != nil {
			return fmt.Errorf("CRD type %s not registered in scheme: %w", rt.name, err)
		}
		if len(gvks) == 0 {
			return fmt.Errorf("CRD type %s has no registered GVKs", rt.name)
		}
	}

	return nil
}

