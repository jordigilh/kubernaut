package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

var _ = Describe("AF scope fail-closed E2E (#2025)", Label("e2e", "phase4", "scope", "issue-2025"), func() {
	It("E2E-AF-2025-001: kubernaut_remediate rejects an unmanaged target and persists no RR", func() {
		ctx := context.Background()
		authToken, err := fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())

		suffix := strings.ToLower(fmt.Sprintf("%x", GinkgoRandomSeed()))
		namespace := "af-unmanaged-" + suffix
		targetName := "scope-target-" + suffix
		unmanagedNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, unmanagedNamespace)).To(Succeed())
		DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, unmanagedNamespace)) })

		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: namespace},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": targetName}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": targetName}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "target", Image: "busybox:1.36"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
		DeferCleanup(func() { _ = client.IgnoreNotFound(k8sClient.Delete(ctx, deployment)) })

		taskID := "scope-fail-closed-" + suffix
		resp, err := a2aInvoke(httpClient, baseURL, authToken, a2aTasksSendWithContext(
			taskID,
			"scope-fail-closed-context-"+suffix,
			fmt.Sprintf("Create a remediation request for deployment %s in %s namespace", targetName, namespace),
		))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		rpc, err := parseRPCResponse(resp)
		Expect(err).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil())
		Expect(rpc.Result).NotTo(BeNil())

		rrList := &remediationv1alpha1.RemediationRequestList{}
		Expect(k8sClient.List(ctx, rrList, client.InNamespace(e2eNamespace))).To(Succeed())
		for _, rr := range rrList.Items {
			if rr.Spec.TargetResource.Namespace == namespace && rr.Spec.TargetResource.Name == targetName {
				Fail(fmt.Sprintf("unmanaged target unexpectedly created RR %s/%s", rr.Namespace, rr.Name))
			}
		}
	})
})
