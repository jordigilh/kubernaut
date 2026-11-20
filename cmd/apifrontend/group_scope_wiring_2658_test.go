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

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	discoveryfake "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
)

// nopEmitter satisfies audit.Emitter without side effects.
type nopEmitter struct{}

func (nopEmitter) Emit(context.Context, *audit.Event) {}

// routeSiblingMapper registers Route in two API groups, the #2658 sibling case
// that backend_deps' RESTMapper must resolve.
func routeSiblingMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper(nil)
	for _, group := range []string{"route.openshift.io", "serving.knative.dev"} {
		gvk := schema.GroupVersionKind{Group: group, Version: "v1", Kind: "Route"}
		gvr := schema.GroupVersionResource{Group: group, Version: "v1", Resource: "routes"}
		mapper.AddSpecific(gvk, gvr, gvr, meta.RESTScopeNamespace)
	}
	return mapper
}

func knativeRoute(namespace, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("serving.knative.dev/v1")
	obj.SetKind("Route")
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return obj
}

// promAlertsServer serves a single firing alert correlated to
// prod/Route/web, so buildSeverityTriageDeps' real Prometheus client resolves
// a positive severity and the group-scope gate is actually reached.
func promAlertsServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/alerts":
			_, _ = fmt.Fprint(w, `{"status":"success","data":{"alerts":[{"labels":{"alertname":"RouteDegraded","namespace":"prod","kind":"Route","name":"web","severity":"critical"},"state":"firing"}]}}`)
		case "/api/v1/rules":
			_, _ = fmt.Fprint(w, `{"status":"success","data":{"groups":[]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// discoveryFake serves the group/resource lists the production checker reads.
func discoveryFake(lists []*metav1.APIResourceList) discovery.DiscoveryInterface {
	fake := &discoveryfake.FakeDiscovery{Fake: &k8stesting.Fake{}}
	fake.Resources = lists
	return fake
}

var _ = Describe("IT-AF-2658-102: severity triage wires the cross-group sibling checker", func() {
	var (
		promSrv *httptest.Server
		cfg     *config.Config
	)

	BeforeEach(func() {
		promSrv = promAlertsServer()
		cfg = config.DefaultConfig()
		cfg.SeverityTriage.Enabled = true
		cfg.SeverityTriage.PrometheusURL = promSrv.URL
	})

	AfterEach(func() {
		promSrv.Close()
	})

	input := func() severity.TriageInput {
		return severity.TriageInput{
			Namespace:     "prod",
			Kind:          "Route",
			Name:          "web",
			APIGroup:      "route.openshift.io",
			APIGroupKnown: true,
			ClusterID:     "hub",
			Labels:        map[string]string{"namespace": "prod", "kind": "Route", "name": "web"},
		}
	}

	It("fails closed for a cross-group sibling resolved through the wired RESTMapper", func() {
		deps := &backendDeps{
			Mapper: routeSiblingMapper(),
			// The checker reads through the direct (non-circuit-breaker)
			// client: a CB-open state must not masquerade as "no sibling".
			k8sRawDynClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), knativeRoute("prod", "web")),
		}

		Expect(buildSeverityTriageDeps(context.Background(), cfg, deps, nopEmitter{}, logr.Discard())).To(Succeed())
		Expect(deps.Triager).NotTo(BeNil())

		result, err := deps.Triager.Triage(context.Background(), input())
		var amb *severity.AmbiguousSeverityError
		Expect(errors.As(err, &amb)).To(BeTrue(), "a real sibling through the real mapper must fail closed")
		Expect(amb.Candidate.AmbiguityReason).To(Equal(severity.AmbiguityReasonGroupUnscopable))
		Expect(amb.Candidate.CandidateAPIGroups).To(Equal([]string{"serving.knative.dev"}))
		Expect(result.Ambiguous).To(BeTrue())
	})

	It("resolves a Kind whose real plural the guess cannot derive (Gateway), via discovery", func() {
		deps := &backendDeps{
			k8sDiscovery: discoveryFake([]*metav1.APIResourceList{
				{GroupVersion: "gateway.networking.k8s.io/v1", APIResources: []metav1.APIResource{
					{Name: "gateways", Kind: "Gateway", Namespaced: true},
				}},
				{GroupVersion: "gateway.networking.x-k8s.io/v1alpha2", APIResources: []metav1.APIResource{
					{Name: "gateways", Kind: "Gateway", Namespaced: true},
				}},
			}),
			k8sRawDynClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
		}
		Expect(buildSeverityTriageDeps(context.Background(), cfg, deps, nopEmitter{}, logr.Discard())).To(Succeed())

		// The discriminating assertion is the absence of an error: with the
		// plural-guess mapper path this same input fails closed with
		// group_unscopable, because "gatewaies" enumerates zero groups.
		result, err := deps.Triager.Triage(context.Background(), severity.TriageInput{
			Namespace: "prod", Kind: "Gateway", Name: "web",
			APIGroup: "gateway.networking.k8s.io", APIGroupKnown: true,
			ClusterID: "hub",
			Labels:    map[string]string{"namespace": "prod", "kind": "Gateway", "name": "web"},
		})
		var ambiguous *severity.AmbiguousSeverityError
		Expect(errors.As(err, &ambiguous)).To(BeFalse(),
			"an irregular plural must not fail closed when discovery data is available")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Severity).NotTo(BeEmpty())
	})

	It("adjudicates when no sibling exists", func() {
		deps := &backendDeps{
			Mapper:          routeSiblingMapper(),
			k8sRawDynClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
		}

		Expect(buildSeverityTriageDeps(context.Background(), cfg, deps, nopEmitter{}, logr.Discard())).To(Succeed())
		result, err := deps.Triager.Triage(context.Background(), input())
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Severity).To(Equal("critical"))
	})
})
