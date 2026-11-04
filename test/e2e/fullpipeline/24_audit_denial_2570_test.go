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

package fullpipeline

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-AUD-2570-008, BR-AUDIT-005/006: authenticated callers without read
// permission cannot retrieve a production approval or its correlated audit.
var _ = Describe("Approval audit read authorization [E2E-AUD-2570-008]", Label("e2e", "fullpipeline", "issue-2570"), func() {
	It("denies a valid unprivileged identity access to the RAR and decision audit", func() {
		id := fmt.Sprintf("%d", time.Now().UnixNano())
		targetNS := "fp-aud-2570-" + id
		saName := "aud-2570-denied-" + id
		marker := "private-review-" + id
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNS, Labels: map[string]string{
			"kubernaut.ai/managed": "true", "kubernaut.ai/environment": "production",
		}}})).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNS}}))).To(Succeed())
		})
		deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "memory-eater", Namespace: targetNS}, Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](0), Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "memory-eater"}},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "memory-eater"}}, Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "app", Image: "busybox:1.36", Command: []string{"sleep", "3600"}}},
			}},
		}}
		Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "memory-eater", Namespace: targetNS, Labels: map[string]string{"app": "memory-eater"},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: deployment.Name, UID: deployment.UID, Controller: ptr.To(true)}}},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "busybox:1.36", Command: []string{"sleep", "3600"}}}}}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		rrName := fpPostSignalToGateway("OOMKilled", pod.Name, targetNS)
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &remediationv1.RemediationRequest{ObjectMeta: metav1.ObjectMeta{Name: rrName, Namespace: namespace}}, client.PropagationPolicy(metav1.DeletePropagationBackground)))).To(Succeed())
		})
		rarKey := client.ObjectKey{Namespace: namespace, Name: "rar-" + rrName}
		var rar remediationv1.RemediationApprovalRequest
		Eventually(func(g Gomega) {
			g.Expect(apiReader.Get(ctx, rarKey, &rar)).To(Succeed())
			g.Expect(rar.Status.Decision).To(Equal(remediationv1.ApprovalDecisionPending))
			g.Expect(rar.Spec.RemediationRequestRef.Name).To(Equal(rrName))
		}, timeout, interval).Should(Succeed())

		By("Recording identifiable review text through deployed AF and admission")
		token := getAFToken()
		Expect(token).NotTo(BeEmpty())
		session, err := fpInitMCPSessionExplicit(afHTTPClient, afBaseURL, token)
		Expect(err).NotTo(HaveOccurred())
		body := fpBuildJSONRPC("e2e-aud-2570-008", "tools/call", map[string]interface{}{"name": "kubernaut_approve", "arguments": map[string]interface{}{
			"rar_name": rarKey.Name, "decision": "Rejected", "reason": marker,
		}})
		_, code, err := fpMCPPOST(afHTTPClient, afBaseURL, token, session, body)
		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(http.StatusOK))
		Eventually(func(g Gomega) {
			g.Expect(apiReader.Get(ctx, rarKey, &rar)).To(Succeed())
			g.Expect(rar.Status.Decision).To(Equal(remediationv1.ApprovalDecisionRejected))
			g.Expect(rar.Status.DecisionMessage).To(Equal(marker))
		}, timeout, interval).Should(Succeed())

		By("Creating a real identity with no read grants and validating its token")
		Expect(infrastructure.CreateServiceAccount(ctx, namespace, kubeconfigPath, saName, GinkgoWriter)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: saName, Namespace: namespace}}))).To(Succeed())
		})
		deniedToken, err := infrastructure.GetServiceAccountToken(ctx, namespace, saName, kubeconfigPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(deniedToken).NotTo(BeEmpty())
		clientset, err := kubernetes.NewForConfig(mustGetKubeConfig())
		Expect(err).NotTo(HaveOccurred())
		review, err := clientset.AuthenticationV1().TokenReviews().Create(ctx, &authenticationv1.TokenReview{Spec: authenticationv1.TokenReviewSpec{Token: deniedToken}}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(review.Status.Authenticated).To(BeTrue(), "token must authenticate rather than merely fail authentication")
		Expect(review.Status.User.Username).To(Equal(fmt.Sprintf("system:serviceaccount:%s:%s", namespace, saName)))

		kubeConfig := rest.AnonymousClientConfig(mustGetKubeConfig())
		kubeHTTP, err := rest.HTTPClientFor(kubeConfig)
		Expect(err).NotTo(HaveOccurred())
		rarURL := fmt.Sprintf("%s/apis/kubernaut.ai/v1alpha1/namespaces/%s/remediationapprovalrequests/%s", kubeConfig.Host, namespace, rarKey.Name)
		dsURL := infrastructure.OperatorExposureURL(operatorExposure, "datastorage") + "/api/v1/audit/events?" + url.Values{
			"correlation_id": {rrName}, "event_type": {"webhook.remediationapprovalrequest.decided"}, "limit": {"50"},
		}.Encode()
		tlsTransport, err := infrastructure.NewTLSAwareTransport(kubeconfigPath)
		Expect(err).NotTo(HaveOccurred())
		dsHTTP := &http.Client{Transport: tlsTransport, Timeout: 20 * time.Second}
		read := func(c *http.Client, endpoint, bearer string) (int, string) {
			req, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			Expect(requestErr).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+bearer)
			resp, requestErr := c.Do(req)
			Expect(requestErr).NotTo(HaveOccurred())
			defer func() { Expect(resp.Body.Close()).To(Succeed()) }()
			data, readErr := io.ReadAll(resp.Body)
			Expect(readErr).NotTo(HaveOccurred())
			return resp.StatusCode, string(data)
		}

		By("Proving the authorized audit response contains the same protected text")
		Eventually(func(g Gomega) {
			status, data := read(dsHTTP, dsURL, e2eAuthToken)
			g.Expect(status).To(Equal(http.StatusOK))
			g.Expect(data).To(ContainSubstring(marker))
			g.Expect(data).To(ContainSubstring(rarKey.Name))
		}, 120*time.Second, 3*time.Second).Should(Succeed())

		By("Checking both protected reads return authorization denial without review contents")
		status, data := read(kubeHTTP, rarURL, deniedToken)
		Expect(status).To(Equal(http.StatusForbidden), "RAR read must be forbidden, not an authentication failure")
		Expect(data).NotTo(ContainSubstring(marker))
		Expect(data).NotTo(ContainSubstring("decisionMessage"))
		status, data = read(dsHTTP, dsURL, deniedToken)
		Expect(status).To(Equal(http.StatusForbidden), "audit read must be forbidden, not an authentication failure")
		Expect(data).To(ContainSubstring("Insufficient permissions"))
		Expect(data).NotTo(ContainSubstring(marker))
		Expect(data).NotTo(ContainSubstring("decision_message"))

		By("Observing the DataStorage authorization denial for this identity")
		var dsService string
		for _, adapter := range operatorExposure.Adapters {
			if adapter.Component == "datastorage" {
				dsService = adapter.ServiceName
			}
		}
		Expect(dsService).NotTo(BeEmpty())
		svc, err := clientset.CoreV1().Services(namespace).Get(ctx, dsService, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(svc.Spec.Selector).NotTo(BeEmpty())
		Eventually(func(g Gomega) {
			pods, listErr := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(svc.Spec.Selector).String()})
			g.Expect(listErr).NotTo(HaveOccurred())
			g.Expect(pods.Items).NotTo(BeEmpty())
			found := false
			for _, dsPod := range pods.Items {
				for _, container := range dsPod.Spec.Containers {
					logs, logErr := clientset.CoreV1().Pods(namespace).GetLogs(dsPod.Name, &corev1.PodLogOptions{Container: container.Name}).DoRaw(ctx)
					g.Expect(logErr).NotTo(HaveOccurred())
					for _, line := range strings.Split(string(logs), "\n") {
						if strings.Contains(line, "security_event") && strings.Contains(line, "authorization_denied") && strings.Contains(line, review.Status.User.Username) && strings.Contains(line, "/api/v1/audit/events") {
							found = true
						}
					}
				}
			}
			g.Expect(found).To(BeTrue(), "DataStorage must log the denied audit read and caller")
		}, 30*time.Second, time.Second).Should(Succeed())
	})
})
