package apifrontend_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rwv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/contenthash"
)

// createTrustedWorkflow supplies the live schema and its content-derived identity.
func createTrustedWorkflow(ctx context.Context, name string, parameters []rwv1.RemediationWorkflowParameter) *rwv1.RemediationWorkflow {
	w := &rwv1.RemediationWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultFixture},
		Spec: rwv1.RemediationWorkflowSpec{
			Version: "1.0.0", ActionType: "RestartPod",
			Description: rwv1.RemediationWorkflowDescription{What: "approval test", WhenToUse: "approval test"},
			Labels:      rwv1.RemediationWorkflowLabels{Severity: []string{"critical"}, Environment: []string{"production"}, Component: []string{"v1/Pod"}, Priority: "P1"},
			Execution:   rwv1.RemediationWorkflowExecution{Engine: "job", Bundle: "example.test/workflow:v1"},
			Parameters:  parameters,
		},
	}
	Expect(k8sClient.Create(ctx, w)).To(Succeed())
	DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(context.Background(), w))).To(Succeed()) })
	content, err := contenthash.MarshalCleanCRDContent(w)
	Expect(err).NotTo(HaveOccurred())
	w.Status.ContentHash = contenthash.ComputeContentHash(string(content))
	w.Status.WorkflowID = contenthash.DeterministicUUID(w.Status.ContentHash)
	Expect(k8sClient.Status().Update(ctx, w)).To(Succeed())
	return w
}
