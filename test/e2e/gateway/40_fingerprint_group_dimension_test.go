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

// ========================================
// Fingerprint Group Dimension E2E (#2650)
// ========================================
//
// PURPOSE:
// Proves on a deployed Gateway (Kind, full stack) that two Kubernetes events
// targeting the SAME namespace/kind/name in two different API groups
// (route.openshift.io/v1 Route vs serving.knative.dev/v1 Route) create TWO
// RemediationRequests with distinct signal fingerprints — not one collapsed
// RR (BR-GATEWAY-004, issue #2650 maintainer request for cross-group E2E
// coverage).
//
// The journey exercised: k8s event webhook -> KubernetesEventAdapter parse
// (involvedObject.apiVersion validated per #2592) -> real ownerchain
// resolution on the standalone Route CRs (M1 group seeding when the
// registry-miss gap fires) -> ProcessSignal scope gate (explicit-GVK
// PartialObjectMetadata lookup against the registered CRDs) -> dedup check
// -> CRD creation.
//
// The Route CRDs and CR instances exist so the scope gate's exact-GVK lookup
// resolves and the owner resolver does not treat the target as deleted
// (Tier 3c signal drop).
package gateway

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// routeGroups2650 are the two cross-group API groups whose Route kind collides
// on ns/kind/name — the motivating #2650 scenario.
var routeGroups2650 = []string{"route.openshift.io", "serving.knative.dev"}

var _ = Describe("Fingerprint Group Dimension E2E (#2650, BR-GATEWAY-004)", Ordered, Label("e2e", "fingerprint", "group-dimension"), func() {
	var (
		testNamespace string
		crdClient     ctrl.Client // apiextensions + unstructured capable
	)

	// crdClientFor2650 builds a client with an apiextensions + RBAC capable
	// scheme. KUBECONFIG is set suite-wide, so config.GetConfig resolves the
	// cluster.
	crdClientFor2650 := func() ctrl.Client {
		scheme := runtime.NewScheme()
		Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
		Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
		cfg, err := config.GetConfig()
		Expect(err).NotTo(HaveOccurred())
		c, err := ctrl.New(cfg, ctrl.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())
		return c
	}

	// grantRouteReadRBAC2650 grants the deployed Gateway's ServiceAccount
	// get/list/watch on both Route CRD groups. ADR-068 (#2592): the local
	// scope Manager performs a PartialObjectMetadata informer read for ANY
	// kind carrying an explicit GVK, and "Gateway/RO RBAC must grant list+watch
	// on the GroupVersionKinds their adapters can legitimately emit" — without
	// this grant the exact-GVK scope lookup times out waiting for the informer
	// to sync and the event is rejected with a 500. This mirrors what a
	// cluster operator does to make a CRD group remediable.
	grantRouteReadRBAC2650 := func() {
		cr := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: "fp-group-e2e-route-reader"},
			Rules: []rbacv1.PolicyRule{
				{APIGroups: routeGroups2650, Resources: []string{"routes"}, Verbs: []string{"get", "list", "watch"}},
			},
		}
		Expect(crdClient.Create(ctx, cr)).To(Succeed())
		binding := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "fp-group-e2e-route-reader"},
			Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "gateway", Namespace: "kubernaut-system"}},
			RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "fp-group-e2e-route-reader"},
		}
		Expect(crdClient.Create(ctx, binding)).To(Succeed())
	}

	revokeRouteReadRBAC2650 := func() {
		_ = crdClient.Delete(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "fp-group-e2e-route-reader"}})
		_ = crdClient.Delete(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "fp-group-e2e-route-reader"}})
	}

	BeforeAll(func() {
		testNamespace = helpers.CreateTestNamespaceAndWait(k8sClient, "fp-group-e2e")
		crdClient = crdClientFor2650()
		grantRouteReadRBAC2650()

		// Register both Route CRDs and wait for Established so the Gateway's
		// exact-GVK scope lookup and owner resolution resolve the kinds.
		for _, group := range routeGroups2650 {
			crd := &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "routes." + group},
				Spec: apiextensionsv1.CustomResourceDefinitionSpec{
					Group: group,
					Names: apiextensionsv1.CustomResourceDefinitionNames{
						Plural: "routes", Singular: "route", Kind: "Route", ListKind: "RouteList",
					},
					Scope: "Namespaced",
					Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
						Name: "v1", Served: true, Storage: true,
						Schema: &apiextensionsv1.CustomResourceValidation{
							OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{Type: "object"},
						},
					}},
				},
			}
			Expect(crdClient.Create(ctx, crd)).To(Succeed(), "creating CRD routes."+group)
			Eventually(func() bool {
				fresh := &apiextensionsv1.CustomResourceDefinition{}
				if err := crdClient.Get(ctx, ctrl.ObjectKey{Name: crd.Name}, fresh); err != nil {
					return false
				}
				for _, cond := range fresh.Status.Conditions {
					if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
						return true
					}
				}
				return false
			}, 60*time.Second, time.Second).Should(BeTrue(), "CRD routes."+group+" should become Established")

			// Create the standalone Route CR (no ownerReferences): same name in
			// both groups, in the managed test namespace.
			route := &unstructured.Unstructured{}
			route.SetGroupVersionKind(schema.GroupVersionKind{Group: group, Version: "v1", Kind: "Route"})
			route.SetName("shared-route")
			route.SetNamespace(testNamespace)
			Expect(crdClient.Create(ctx, route)).To(Succeed(), "creating Route CR in group "+group)
		}
	})

	AfterAll(func() {
		if CurrentSpecReport().Failed() {
			return // preserve state for post-mortem
		}
		revokeRouteReadRBAC2650()
		for _, group := range routeGroups2650 {
			route := &unstructured.Unstructured{}
			route.SetGroupVersionKind(schema.GroupVersionKind{Group: group, Version: "v1", Kind: "Route"})
			route.SetName("shared-route")
			route.SetNamespace(testNamespace)
			if err := crdClient.Delete(ctx, route); err != nil && !apierrors.IsNotFound(err) {
				logger.Error(fmt.Errorf("delete Route CR: %w", err), "cleanup", "group", group)
			}
			crd := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "routes." + group}}
			if err := crdClient.Delete(ctx, crd); err != nil && !apierrors.IsNotFound(err) {
				logger.Error(fmt.Errorf("delete CRD: %w", err), "cleanup", "group", group)
			}
		}
		helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
	})

	sendRouteEvent2650 := func(apiVersion string) *WebhookResponse {
		payload := map[string]interface{}{
			"type":    "Warning",
			"reason":  "DegradedRoute",
			"message": "cross-group dedup probe",
			"involvedObject": map[string]interface{}{
				"apiVersion": apiVersion,
				"kind":       "Route",
				"name":       "shared-route",
				"namespace":  testNamespace,
			},
			"metadata":       map[string]interface{}{"name": fmt.Sprintf("fp-group-%s-%d", apiVersion, time.Now().UnixNano()), "namespace": testNamespace},
			"firstTimestamp": time.Now().Format(time.RFC3339),
			"lastTimestamp":  time.Now().Format(time.RFC3339),
			"count":          1,
			"source":         map[string]interface{}{"component": "route-controller"},
		}
		payloadBytes, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())
		return sendWebhookExpectCreated(gatewayURL, "/api/v1/signals/kubernetes-event", payloadBytes)
	}

	It("E2E-GW-2650-001: cross-group same-named Route events create two RRs with distinct fingerprints", func() {
		By("posting the openshift-group event (route.openshift.io/v1)")
		respA := sendRouteEvent2650("route.openshift.io/v1")
		var gwRespA GatewayResponse
		Expect(json.Unmarshal(respA.Body, &gwRespA)).To(Succeed())
		Expect(gwRespA.RemediationRequestName).NotTo(BeEmpty(),
			"the openshift Route event must create a RemediationRequest")

		By("posting the knative-group event (serving.knative.dev/v1) for the SAME ns/kind/name")
		respB := sendRouteEvent2650("serving.knative.dev/v1")
		var gwRespB GatewayResponse
		Expect(json.Unmarshal(respB.Body, &gwRespB)).To(Succeed())
		Expect(gwRespB.RemediationRequestName).NotTo(BeEmpty(),
			"BR-GATEWAY-004: the knative Route event must create a SECOND RR, not dedup onto the openshift group's")
		Expect(gwRespB.RemediationRequestName).NotTo(Equal(gwRespA.RemediationRequestName),
			"the two cross-group targets are different dedup subjects (#2650)")

		By("verifying both RRs carry distinct group-aware fingerprints and the correct target apiVersion")
		rrA := getRRByName2650(gwRespA.RemediationRequestName)
		rrB := getRRByName2650(gwRespB.RemediationRequestName)
		Expect(rrA.Spec.SignalFingerprint).NotTo(Equal(rrB.Spec.SignalFingerprint),
			"the group dimension must separate the fingerprints end to end")
		Expect(rrA.Spec.TargetResource.APIVersion).To(Equal("route.openshift.io/v1"))
		Expect(rrB.Spec.TargetResource.APIVersion).To(Equal("serving.knative.dev/v1"))
	})
})

// getRRByName2650 fetches an RR from the shared gateway namespace by name,
// waiting for it to appear (creation is asynchronous on the webhook path).
func getRRByName2650(name string) *remediationv1alpha1.RemediationRequest {
	var rr *remediationv1alpha1.RemediationRequest
	Eventually(func() error {
		fresh := &remediationv1alpha1.RemediationRequest{}
		err := k8sClient.Get(ctx, ctrl.ObjectKey{Namespace: gatewayNamespace, Name: name}, fresh)
		if err == nil {
			rr = fresh
		}
		return err
	}, 30*time.Second, time.Second).Should(Succeed(), "RR %s should exist", name)
	return rr
}
