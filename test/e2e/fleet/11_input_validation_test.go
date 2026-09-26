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

package fleet

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// E2E-FLEET-SI10-001: Fleet input validation (malformed payloads)
// Authority: BR-GATEWAY-003, BR-GATEWAY-043, Issue #54
// FedRAMP: SI-10 (Information input validation)
// SOC2: CC7.2 (Internal controls -- input validation)
//
// Validates that the Gateway rejects malformed payloads in a fleet
// context, returning appropriate HTTP error codes without creating
// RemediationRequests from invalid data.
var _ = Describe("E2E-FLEET-SI10-001 [SI-10]: Fleet input validation rejects malformed payloads (BR-GATEWAY-003)", Label("fleet"), func() {

	gatewayURL := urlLocalhost30080

	It("should return 400 for syntactically invalid JSON [SI-10]", func() {
		malformed := `{"alerts": [{"status": "firing", INVALID}]}`

		resp, err := postWithFleetAuth(
			gatewayURL+"/api/v1/signals/prometheus",
			strings.NewReader(malformed))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
			"SI-10: syntactically invalid JSON must be rejected with 400")
	})

	It("should return 400 for empty alerts array [SI-10]", func() {
		emptyAlerts := `{"version": "4", "status": "firing", "alerts": []}`

		resp, err := postWithFleetAuth(
			gatewayURL+"/api/v1/signals/prometheus",
			strings.NewReader(emptyAlerts))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
			"SI-10: empty alerts array must be rejected -- no signals to process")
	})

	It("should return 400 for alert missing required labels [SI-10]", func() {
		missingLabels := `{
			"version": "4",
			"status": "firing",
			"alerts": [{
				"status": "firing",
				"labels": {},
				"annotations": {},
				"startsAt": "2026-06-29T12:00:00Z"
			}]
		}`

		resp, err := postWithFleetAuth(
			gatewayURL+"/api/v1/signals/prometheus",
			strings.NewReader(missingLabels))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
			"SI-10: alert without required labels (alertname) must be rejected")
	})

	It("should return 400 for empty request body [SI-10]", func() {
		resp, err := postWithFleetAuth(
			gatewayURL+"/api/v1/signals/prometheus",
			strings.NewReader(""))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
			"SI-10: empty body must be rejected")
	})

	It("should accept well-formed fleet alert with cluster_id and return success [SI-10, AC-4]", func() {
		// Issue #54 bug fix: "validation-test-app" was a fake resource name that never
		// existed as a K8s object. Gateway's owner resolution (see the detailed note in
		// 01_signal_ingestion_test.go) does a live lookup and drops signals targeting
		// resources that don't exist, so this "well-formed" alert was always rejected
		// with a misleading "Failed to parse batch payload" 400 (the batch-parse error
		// path is shared with the all-alerts-failed-owner-resolution path). Create the
		// target as a real (zero-replica) Deployment so resolution succeeds.
		const targetName = "validation-test-app"
		targetNS := fleetWorkloadNamespace("fleet-input-validation")
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
				Namespace: targetNS,
				// BR-SCOPE-001/ADR-053: label the resource directly (see the detailed
				// note in 01_signal_ingestion_test.go for why the namespace-level
				// fallback alone was not sufficient).
				Labels: map[string]string{"kubernaut.ai/managed": "true"},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](0),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": targetName}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": targetName}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Image: "busybox:1.36"}},
					},
				},
			},
		}
		// Created on the REMOTE cluster (DD-TEST-013): see the equivalent note
		// in 01_signal_ingestion_test.go.
		if createErr := remoteK8sClient.Create(ctx, dep); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
			Expect(createErr).NotTo(HaveOccurred(), "Failed to create %s fixture", targetName)
		}
		DeferCleanup(func() { _ = remoteK8sClient.Delete(context.Background(), dep) })

		payload := buildPrometheusAlertWithClusterInNamespace("FleetSI10Valid", "warning",
			targetName, targetNS, "prod-west")

		postFleetAlertUntilAccepted(gatewayURL, payload, http.StatusCreated, http.StatusAccepted)
	})

	It("E2E-FLEET-2394-002 [AC-6, SI-10]: should reject an unattributed alert instead of falling back to a matching hub target (BR-FLEET-054)", func() {
		suffix := uuid.NewString()[:8]
		targetName := fmt.Sprintf("unattributed-%s", suffix)
		alertName := fmt.Sprintf("FleetMissingCluster-%s", suffix)
		targetNS := fleetWorkloadNamespace("fleet-input-validation")
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
				Namespace: targetNS,
				Labels:    map[string]string{"kubernaut.ai/managed": "true"},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](0),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": targetName}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": targetName}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "busybox:1.36"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, dep)).To(Succeed(), "matching hub target must exist to expose unsafe local fallback")
		DeferCleanup(func() { _ = k8sClient.Delete(context.Background(), dep) })

		By("Posting a valid signal with an empty cluster label while the named resource exists on the hub")
		payload := buildPrometheusAlertWithClusterInNamespace(alertName, "warning", targetName, targetNS, "")
		resp, err := postWithFleetAuth(gatewayURL+"/api/v1/signals/prometheus", strings.NewReader(string(payload)))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(BeNumerically(">=", http.StatusBadRequest),
			"fleet Gateway must reject an unattributed signal even though its target exists locally")
		Expect(resp.StatusCode).To(BeNumerically("<", http.StatusInternalServerError),
			"missing cluster attribution is invalid input, not an internal error")

		By("Verifying the rejected signal did not create a RemediationRequest via local fallback")
		Consistently(func(g Gomega) {
			var rrList remediationv1.RemediationRequestList
			g.Expect(k8sClient.List(ctx, &rrList, client.InNamespace(namespace))).To(Succeed())
			for i := range rrList.Items {
				rr := &rrList.Items[i]
				g.Expect(rr.Spec.SignalName == alertName &&
					rr.Spec.TargetResource.Name == targetName &&
					rr.Spec.TargetResource.Namespace == targetNS).To(BeFalse(),
					"AC-4/AC-6: fleet mode must not infer hub identity from a local Kubernetes match")
			}
		}, 10*time.Second, 1*time.Second).Should(Succeed())
	})
})
