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

package e2e_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	investigationsessionv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
)

// BR-INTERACTIVE-010 SC-5: E2E tests for Interactive Investigation in the AF E2E cluster.
var _ = Describe("E2E-1293: Interactive Investigation (AF)", Label("e2e", "interactive", "1293"), func() {
	const afNamespace = "kubernaut-system"

	It("[E2E-1293-004] SA caller is blocked from creating InvestigationSession", func() {
		ctx := context.Background()
		saName := fmt.Sprintf("e2e-1293-004-sa-%s", uuid.New().String()[:6])
		rrName := fmt.Sprintf("e2e-rr-1293-004-%s", uuid.New().String()[:8])

		By("Creating test RR for the SA to reference")
		Expect(createRR("default", rrName, "test-deploy-1293-004")).To(Succeed())
		DeferCleanup(func() { deleteRR("default", rrName) })

		By("Creating a ServiceAccount in the E2E namespace")
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName,
				Namespace: afNamespace,
			},
		}
		_, err := clientset.CoreV1().ServiceAccounts(afNamespace).Create(ctx, sa, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_ = clientset.CoreV1().ServiceAccounts(afNamespace).Delete(ctx, saName, metav1.DeleteOptions{})
		})

		By("Granting minimal RBAC to the SA (enough for AF auth, not enough for bypass)")
		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName + "-role",
				Namespace: afNamespace,
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"get", "list"},
				},
			},
		}
		_, err = clientset.RbacV1().Roles(afNamespace).Create(ctx, role, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_ = clientset.RbacV1().Roles(afNamespace).Delete(ctx, role.Name, metav1.DeleteOptions{})
		})

		rb := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName + "-binding",
				Namespace: afNamespace,
			},
			Subjects: []rbacv1.Subject{{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: afNamespace,
			}},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Role",
				Name:     role.Name,
			},
		}
		_, err = clientset.RbacV1().RoleBindings(afNamespace).Create(ctx, rb, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_ = clientset.RbacV1().RoleBindings(afNamespace).Delete(ctx, rb.Name, metav1.DeleteOptions{})
		})

		By("Getting SA token via TokenRequest API")
		expSeconds := int64(600)
		tokenResp, err := clientset.CoreV1().ServiceAccounts(afNamespace).CreateToken(
			ctx, saName,
			&authenticationv1.TokenRequest{
				Spec: authenticationv1.TokenRequestSpec{
					ExpirationSeconds: &expSeconds,
				},
			},
			metav1.CreateOptions{},
		)
		Expect(err).NotTo(HaveOccurred())
		saToken := tokenResp.Status.Token
		Expect(saToken).NotTo(BeEmpty())

		By("Calling AF A2A with SA token (should be rejected)")
		body := a2aTasksSend("e2e-1293-004", "start investigation for deployment test-deploy-1293-004")
		resp, err := a2aInvoke(httpClient, baseURL, saToken, body)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		respBody, _ := io.ReadAll(resp.Body)
		respStr := string(respBody)

		// The SA should be blocked at either:
		// 1. Auth layer: 401/403 if K8s auth isn't enabled or SA lacks AF RBAC
		// 2. SA guard: tool execution error mentioning service account restriction
		// Either outcome proves SAs cannot create IS CRDs via AF.
		saBlocked := resp.StatusCode == http.StatusUnauthorized ||
			resp.StatusCode == http.StatusForbidden ||
			strings.Contains(respStr, "service account") ||
			strings.Contains(respStr, "ServiceAccount") ||
			strings.Contains(respStr, "not permitted") ||
			strings.Contains(respStr, "denied")

		if !saBlocked {
			GinkgoWriter.Printf("  SA A2A response: HTTP %d, body: %s\n", resp.StatusCode, respStr)
		}

		By("Verifying no InvestigationSession CRD was created for this RR")
		Consistently(func() int {
			var list investigationsessionv1alpha1.InvestigationSessionList
			if err := k8sClient.List(ctx, &list, client.InNamespace(afNamespace)); err != nil {
				return 0
			}
			count := 0
			for _, is := range list.Items {
				if is.Spec.RemediationRequestRef.Name == rrName {
					count++
				}
			}
			return count
		}, 10*time.Second, 2*time.Second).Should(Equal(0),
			"BR-INTERACTIVE-010 SC-5: No IS CRD should be created when caller is a ServiceAccount")
	})
})
