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

package authwebhook

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/contenthash"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// createTrustedWorkflow creates the live catalog object required by approval
// admission and stamps the same content-derived identity as AuthWebhook.
func createTrustedWorkflow(ctx context.Context, name string, parameters []remediationworkflowv1.RemediationWorkflowParameter) *remediationworkflowv1.RemediationWorkflow {
	rw := &remediationworkflowv1.RemediationWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultFixture},
		Spec: remediationworkflowv1.RemediationWorkflowSpec{
			Version:    "1.0.0",
			ActionType: "RestartPod",
			Description: remediationworkflowv1.RemediationWorkflowDescription{
				What:      "AuthWebhook integration fixture",
				WhenToUse: "For AuthWebhook approval testing",
			},
			Labels: remediationworkflowv1.RemediationWorkflowLabels{
				Severity:    []string{"critical"},
				Environment: []string{"production"},
				Component:   []string{"v1/Pod"},
				Priority:    "P1",
			},
			Execution:  remediationworkflowv1.RemediationWorkflowExecution{Engine: "job", Bundle: "example.test/workflow:v1"},
			Parameters: parameters,
		},
	}
	Expect(k8sClient.Create(ctx, rw)).To(Succeed(), "trusted workflow fixture should be created")
	DeferCleanup(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(context.Background(), rw))).To(Succeed())
	})

	content, err := contenthash.MarshalCleanCRDContent(rw)
	Expect(err).NotTo(HaveOccurred())
	rw.Status.ContentHash = contenthash.ComputeContentHash(string(content))
	rw.Status.WorkflowID = contenthash.DeterministicUUID(rw.Status.ContentHash)
	rw.Status.CatalogStatus = sharedtypes.CatalogStatusActive
	Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed(), "trusted workflow identity should be persisted")
	return rw
}
