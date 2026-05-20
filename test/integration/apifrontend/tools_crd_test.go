package apifrontend_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("CRD Tools Integration (tools/ via envtest)", func() {

	rrGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}

	Describe("AC-24: CRD tools against real K8s API", func() {
		It("IT-AF-1195-035: list_remediations returns list from envtest", func() {
			ctx := context.Background()

			// Create a test RR CRD in envtest
			rr := &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "kubernaut.ai/v1alpha1",
					"kind":       "RemediationRequest",
					"metadata": map[string]any{
						"name":      "test-rr-035",
						"namespace": "default",
					},
					"spec": map[string]any{
						"signalName":        "TestAlert",
						"signalFingerprint": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
						"signalType":        "prometheus",
						"severity":          "warning",
						"firingTime":        "2025-01-01T00:00:00Z",
						"receivedTime":      "2025-01-01T00:00:01Z",
						"targetType":        "Deployment",
						"targetResource": map[string]any{
							"kind": "Deployment",
							"name": "test-app",
						},
					},
				},
			}
			_, err := dynamicClient.Resource(rrGVR).Namespace("default").Create(ctx, rr, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_ = dynamicClient.Resource(rrGVR).Namespace("default").Delete(ctx, "test-rr-035", metav1.DeleteOptions{})
			})

			result, err := tools.HandleListRemediations(ctx, dynamicClient, tools.ListRemediationsArgs{
				Namespace: "default",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(BeNumerically(">=", 1))
		})

		It("IT-AF-1195-036: get_pods returns pod list from envtest", func() {
			ctx := context.Background()

			result, err := tools.HandleGetPods(ctx, dynamicClient, tools.GetPodsArgs{
				Namespace: "default",
			})
			Expect(err).NotTo(HaveOccurred())
			// envtest may or may not have pods; the key is no error and valid result
			Expect(result.Pods).NotTo(BeNil())
		})

		It("IT-AF-1195-037: get_workloads returns workload list from envtest", func() {
			ctx := context.Background()

			result, err := tools.HandleGetWorkloads(ctx, dynamicClient, tools.GetWorkloadsArgs{
				Namespace: "default",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Workloads).NotTo(BeNil())
		})
	})

	Describe("AC-27: RBAC enforcement limits tools by user role", func() {
		It("IT-AF-1195-041: unauthorized user cannot list remediations", func() {
			identity := &auth.UserIdentity{
				Username: "unauthorized-user",
				Groups:   []string{"viewers"},
			}
			ctx := auth.WithUserIdentity(context.Background(), identity)

			factory := auth.NewImpersonatingDynamicFactory(restCfg)
			impersonatedClient, err := factory(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Impersonated user with no RBAC should fail
			_, err = impersonatedClient.Resource(rrGVR).Namespace("default").List(ctx, metav1.ListOptions{})
			Expect(err).To(HaveOccurred(), "unauthorized user should be denied by RBAC")
		})
	})
})
