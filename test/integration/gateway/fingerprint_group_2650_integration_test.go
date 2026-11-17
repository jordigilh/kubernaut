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
// Fingerprint Group Dimension Integration Test (#2650)
// ========================================
//
// PURPOSE:
// Proves that two signals targeting the same namespace/kind/name in two
// different API groups traverse the full production dispatch path
// (kubernetes-event adapter -> ResolveFingerprintWithCluster -> ProcessSignal
// -> CRDCreator) and produce TWO RemediationRequests, not one collapsed RR.
//
// The standalone-resource case deliberately constructs the registry-miss gap:
// the real ownerchain.K8sOwnerResolver is constructed WITHOUT a registry, so
// resolveGVK's static kindToGroup map misses the grouped kind "Route" and
// resolution returns an empty APIVersion — the group must come from the M1
// seeding (input resource.APIVersion).
//
// BUSINESS REQUIREMENTS:
// - BR-GATEWAY-004: cross-adapter deduplication consistency (group dimension)
// ========================================

package gateway

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	gwTypes "github.com/jordigilh/kubernaut/pkg/gateway/types"
	"github.com/jordigilh/kubernaut/pkg/shared/k8s/ownerchain"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// k8sEventPayloadFor2650 builds a Warning Kubernetes Event JSON payload for a
// cross-group Route target, mirroring what kubernetes-event-exporter POSTs to
// the adapter's route.
func k8sEventPayloadFor2650(namespace, apiVersion, name string) []byte {
	payload := map[string]any{
		"type":   "Warning",
		"reason": "DegradedRoute",
		"message": fmt.Sprintf("route %s degraded", name),
		"involvedObject": map[string]any{
			"kind":            "Route",
			"namespace":       namespace,
			"name":            name,
			"apiVersion":      apiVersion,
			"uid":             "uid-2650",
			"resourceVersion": "1",
		},
		"source": map[string]any{"component": "route-controller"},
	}
	raw, err := json.Marshal(payload)
	Expect(err).NotTo(HaveOccurred())
	return raw
}

// crdClientFor2650 builds a controller-runtime client with an apiextensions
// scheme, for creating and watching the test-only Route CRDs.
func crdClientFor2650() ctrl.Client {
	scheme := runtime.NewScheme()
	Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
	crdClient, err := ctrl.New(k8sConfig, ctrl.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	return crdClient
}

// registerRouteCRDsFor2650 creates the two cross-group Route CRDs in the
// envtest API server. ProcessSignal's scope gate (BR-SCOPE-002) resolves the
// target via a PartialObjectMetadata Get keyed by the signal's GVK; without
// the CRDs the RESTMapper returns NoKindMatchError, which is NOT NotFound and
// surfaces as a scope infrastructure error instead of falling through to
// namespace-label inheritance. A production cluster serving Route signals has
// these CRDs — registering them reproduces that.
func registerRouteCRDsFor2650() {
	crdClient := crdClientFor2650()

	for _, group := range []string{"route.openshift.io", "serving.knative.dev"} {
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
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
						},
					},
				}},
			},
		}
		Expect(crdClient.Create(ctx, crd)).To(Succeed(), "creating CRD routes."+group)

		// Wait for Established so the RESTMapper serves the new kind.
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
		}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(), "CRD %s should become Established", crd.Name)
	}
}

// removeRouteCRDsFor2650 cleans up the test-only CRDs so the shared envtest
// API server is left as found for other specs.
func removeRouteCRDsFor2650() {
	crdClient := crdClientFor2650()
	for _, group := range []string{"route.openshift.io", "serving.knative.dev"} {
		crd := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "routes." + group}}
		_ = crdClient.Delete(ctx, crd)
	}
}

var _ = Describe("Fingerprint Group Dimension (#2650, BR-GATEWAY-004)", Ordered, ContinueOnFailure, Label("fingerprint", "group-dimension", "integration"), func() {
	var (
		testLogger    logr.Logger
		testNamespace string
		gwServer      *gateway.Server
	)

	BeforeAll(func() {
		testLogger = logger.WithValues("test", "fingerprint-group-2650-int")
		testNamespace = helpers.CreateTestNamespace(ctx, k8sClient, "fp-group-int")
		registerRouteCRDsFor2650()
		gwConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
		var err error
		gwServer, err = createGatewayServer(gwConfig, testLogger, k8sClient, sharedAuditStore)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterAll(func() {
		removeRouteCRDsFor2650()
		if CurrentSpecReport().Failed() {
			return
		}
		helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
	})

	It("IT-GW-2650-001: two cross-group signals for the same ns/kind/name create two RRs (standalone resources, registry-miss gap)", func() {
		By("1. Wiring the production kubernetes-event adapter with the real ownerchain resolver (no registry: static kindToGroup misses Route)")

		// No WithRegistry: resolveGVK falls back to the static kindToGroup map,
		// which does not know "Route" — resolution.APIVersion comes back empty
		// for the standalone resource, so the group can only come from the
		// input resource's apiVersion (M1 seeding).
		resolver := ownerchain.NewK8sOwnerResolver(k8sClient, testLogger)
		adapter := adapters.NewKubernetesEventAdapter(resolver)

		By("2. Parsing the openshift-group signal (route.openshift.io/v1)")

		signalA, err := adapter.Parse(ctx, k8sEventPayloadFor2650(testNamespace, "route.openshift.io/v1", "shared-route"))
		Expect(err).NotTo(HaveOccurred())
		Expect(signalA.Fingerprint).NotTo(BeEmpty())

		By("3. Parsing the knative-group signal (serving.knative.dev/v1)")

		signalB, err := adapter.Parse(ctx, k8sEventPayloadFor2650(testNamespace, "serving.knative.dev/v1", "shared-route"))
		Expect(err).NotTo(HaveOccurred())
		Expect(signalB.Fingerprint).NotTo(BeEmpty())

		By("4. Asserting the fingerprints are distinct and carry each group")

		Expect(signalA.Fingerprint).NotTo(Equal(signalB.Fingerprint),
			"BR-GATEWAY-004: same ns/kind/name in two API groups must not share a fingerprint")

		expectedA := gwTypes.CalculateClusterAwareFingerprint("", gwTypes.ResourceIdentifier{
			Namespace: testNamespace, Kind: "Route", Name: "shared-route", APIVersion: "route.openshift.io/v1",
		})
		Expect(signalA.Fingerprint).To(Equal(expectedA),
			"M1 seeding: the registry-miss standalone resource's group must come from the input apiVersion")
		expectedB := gwTypes.CalculateClusterAwareFingerprint("", gwTypes.ResourceIdentifier{
			Namespace: testNamespace, Kind: "Route", Name: "shared-route", APIVersion: "serving.knative.dev/v1",
		})
		Expect(signalB.Fingerprint).To(Equal(expectedB))

		By("5. Processing both signals through Gateway -> two RRs, no cross-group dedup")

		responseA, err := gwServer.ProcessSignal(ctx, signalA)
		Expect(err).ToNot(HaveOccurred())
		Expect(responseA.Status).To(Equal("created"),
			"the openshift Route signal must create a new RemediationRequest")

		responseB, err := gwServer.ProcessSignal(ctx, signalB)
		Expect(err).ToNot(HaveOccurred())
		Expect(responseB.Status).To(Equal("created"),
			"the knative Route signal must create a SECOND RemediationRequest, not dedup onto the openshift group's RR")

		By("6. Verifying each fingerprint has exactly one RR")

		rrsA := listRRsByFingerprint(ctx, k8sClient, expectedA)
		Expect(rrsA).To(HaveLen(1), "exactly one RR for the openshift group fingerprint")
		rrsB := listRRsByFingerprint(ctx, k8sClient, expectedB)
		Expect(rrsB).To(HaveLen(1), "exactly one RR for the knative group fingerprint")
		Expect(rrsA[0].Name).NotTo(Equal(rrsB[0].Name))

		testLogger.Info("✅ Fingerprint group dimension validated: 2 cross-group signals -> 2 RRs",
			"rrA", rrsA[0].Name, "rrB", rrsB[0].Name)
	})
})
