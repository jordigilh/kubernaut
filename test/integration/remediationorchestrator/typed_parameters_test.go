package remediationorchestrator

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rwv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/contenthash"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/testutil"
)

const (
	trustedRestartWorkflowName    = "wf-restart-pods"
	trustedAIOriginalWorkflowName = "wf-ai-original"
	trustedProactiveWorkflowName  = "wf-proactive-fix"
)

var (
	trustedRestartWorkflowID    string
	trustedAIOriginalWorkflowID string
	trustedProactiveWorkflowID  string
)

func registerTestWorkflow(name string, parameters []rwv1.RemediationWorkflowParameter) *rwv1.RemediationWorkflow {
	w := newTestWorkflow(name, parameters)
	Expect(k8sClient.Create(ctx, w)).To(Succeed())
	DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(context.Background(), w))).To(Succeed()) })
	setTestWorkflowIdentity(w)
	return w
}

func registerTrustedWorkflow(name string) string {
	w := newTestWorkflow(name, trustedWorkflowParameters())
	Expect(k8sClient.Create(ctx, w)).To(Succeed())
	setTestWorkflowIdentity(w)
	return w.Status.WorkflowID
}

func newTestWorkflow(name string, parameters []rwv1.RemediationWorkflowParameter) *rwv1.RemediationWorkflow {
	return &rwv1.RemediationWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ROControllerNamespace},
		Spec: rwv1.RemediationWorkflowSpec{
			Version: "1.0.0", ActionType: "RestartPod",
			Description: rwv1.RemediationWorkflowDescription{What: "integration fixture", WhenToUse: "integration test"},
			Labels:      rwv1.RemediationWorkflowLabels{Severity: []string{"critical"}, Environment: []string{"production"}, Component: []string{"v1/Pod"}, Priority: "P1"},
			Execution:   rwv1.RemediationWorkflowExecution{Engine: "job", Bundle: testutil.ValidBundleRef},
			Parameters:  parameters,
		},
	}
}

func setTestWorkflowIdentity(w *rwv1.RemediationWorkflow) {
	content, err := contenthash.MarshalCleanCRDContent(w)
	Expect(err).NotTo(HaveOccurred())
	w.Status.ContentHash = contenthash.ComputeContentHash(string(content))
	w.Status.WorkflowID = contenthash.DeterministicUUID(w.Status.ContentHash)
	Expect(k8sClient.Status().Update(ctx, w)).To(Succeed())
}

func trustedWorkflowParameters() []rwv1.RemediationWorkflowParameter {
	return []rwv1.RemediationWorkflowParameter{
		{Name: "NAMESPACE", Type: "string", Description: "target namespace"},
		{Name: "POD_NAME", Type: "string", Description: "target pod"},
	}
}

func seedTrustedWorkflowFixtures() {
	trustedRestartWorkflowID = registerTrustedWorkflow(trustedRestartWorkflowName)
	trustedAIOriginalWorkflowID = registerTrustedWorkflow(trustedAIOriginalWorkflowName)
	trustedProactiveWorkflowID = registerTrustedWorkflow(trustedProactiveWorkflowName)
}

func typedStringParameters(values map[string]string) sharedtypes.TypedParameterMap {
	result := make(sharedtypes.TypedParameterMap, len(values))
	for key, value := range values {
		v := value
		result[key] = sharedtypes.TypedParameterValue{Kind: "string", StringValue: &v}
	}
	return result
}

func typedStringParameter(value string) sharedtypes.TypedParameterValue {
	return sharedtypes.TypedParameterValue{Kind: "string", StringValue: &value}
}
