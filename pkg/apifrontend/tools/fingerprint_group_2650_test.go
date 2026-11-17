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

package tools_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	gwtypes "github.com/jordigilh/kubernaut/pkg/gateway/types"
	"github.com/jordigilh/kubernaut/pkg/shared/k8s/ownerchain"
)

// Issue #2650: the AF-side fingerprint plumbing gains the API-group dimension
// so GW and AF agree on cross-group dedup targets (BR-GATEWAY-004, AC-4).
var _ = Describe("Fingerprint group dimension — AF tools (#2650, BR-GATEWAY-004)", func() {
	routeDeps := func() *tools.ToolDeps {
		tc := newTypedFakeClient()
		return &tools.ToolDeps{
			Client:       tc,
			ControllerNS: "prod",
			Triager:      defaultTestTriager("prod", "Route", "shared-route"),
			ScopeChecker: testAlwaysManagedScopeChecker(),
		}
	}

	createRoute := func(deps *tools.ToolDeps, apiVersion string) *remediationv1.RemediationRequest {
		result, err := tools.HandleCreateRR(context.Background(), deps, &tools.CreateRRArgs{
			Namespace:   "prod",
			Kind:        "Route",
			Name:        "shared-route",
			Description: "cross-group dedup probe",
			APIVersion:  apiVersion,
		}, "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.AlreadyExists).To(BeFalse(),
			"the second group's target must create a new RR, not dedup onto the first group's")
		return verifyTypedRR(deps.Client, "prod", extractRRName(result.RRID))
	}

	It("UT-AF-2650-001: two targets differing only in group create two RRs with distinct SignalFingerprint", func() {
		openshiftRR := createRoute(routeDeps(), "route.openshift.io/v1")
		knativeRR := createRoute(routeDeps(), "serving.knative.dev/v1")

		Expect(openshiftRR.Spec.SignalFingerprint).NotTo(Equal(knativeRR.Spec.SignalFingerprint),
			"BR-GATEWAY-004: same ns/kind/name in two API groups are different dedup targets on the AF path too")
	})

	It("UT-AF-2650-002: check_existing matching ns/kind/name but different group returns exists=false", func() {
		openshiftRR := createRoute(routeDeps(), "route.openshift.io/v1")
		tc := newTypedFakeClient(openshiftRR)

		result, err := tools.HandleCheckExistingRR(context.Background(), tc, "prod", tools.CheckExistingRRArgs{
			Namespace: "prod", Kind: "Route", Name: "shared-route", APIVersion: "serving.knative.dev/v1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Exists).To(BeFalse(),
			"a knative Route must not dedup onto an openshift Route's live remediation")

		sameGroup, err := tools.HandleCheckExistingRR(context.Background(), tc, "prod", tools.CheckExistingRRArgs{
			Namespace: "prod", Kind: "Route", Name: "shared-route", APIVersion: "route.openshift.io/v1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(sameGroup.Exists).To(BeTrue(),
			"the same-group lookup must still find the live remediation")
		Expect(sameGroup.RRID).To(Equal(openshiftRR.Name))
	})

	Describe("api_version input validation (SI-10)", func() {
		It("UT-AF-2650-003a: missing api_version is rejected with ErrInvalidInput", func() {
			_, err := tools.HandleCheckExistingRR(context.Background(), newTypedFakeClient(), "prod", tools.CheckExistingRRArgs{
				Namespace: "prod", Kind: "Route", Name: "shared-route",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"),
				"SI-10: api_version is a required input of kubernaut_check_existing_remediation")
			Expect(err.Error()).To(ContainSubstring("api_version"))
		})

		It("UT-AF-2650-003b: malformed api_version is rejected with ErrInvalidInput", func() {
			_, err := tools.HandleCheckExistingRR(context.Background(), newTypedFakeClient(), "prod", tools.CheckExistingRRArgs{
				Namespace: "prod", Kind: "Route", Name: "shared-route", APIVersion: "apps",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"),
				"SI-10: a slash-less group name is not a valid api_version")
		})
	})

	// IT-AF-2650-002 (AC-4, GW/AF parity): identical cluster+GVK standalone
	// resource -> identical fingerprint from the GW path
	// (ResolveFingerprintWithCluster with an OwnerResolverWithAPIVersion-capable
	// resolver, fleet_wiring_test.go UT-FLEET-FP-001 pattern) and from the AF
	// path (HandleCreateRR's RR spec.signalFingerprint, which is produced by
	// rrFingerprintWithCluster).
	It("IT-AF-2650-002: GW and AF produce identical fingerprints for identical cluster+GVK (AC-4 parity)", func() {
		const clusterID = "prod-east"
		deps := &tools.ToolDeps{
			Client:       newTypedFakeClient(),
			ControllerNS: "prod",
			Triager:      defaultTestTriager("prod", "Route", "shared-route", clusterID),
			ScopeChecker: testAlwaysManagedScopeChecker(),
		}
		result, err := tools.HandleCreateRR(context.Background(), deps, &tools.CreateRRArgs{
			ClusterID:   clusterID,
			Namespace:   "prod",
			Kind:        "Route",
			Name:        "shared-route",
			Description: "gw/af parity probe",
			APIVersion:  "route.openshift.io/v1",
		}, "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.AlreadyExists).To(BeFalse())
		rr := verifyTypedRR(deps.Client, "prod", extractRRName(result.RRID))
		Expect(rr.Spec.ClusterID).To(Equal(clusterID))

		resolver := &selfResolvingFakeResolver{apiVersion: "route.openshift.io/v1"}
		gwFingerprint, _, err := gwtypes.ResolveFingerprintWithCluster(
			context.Background(), clusterID, resolver,
			gwtypes.ResourceIdentifier{Namespace: "prod", Kind: "Route", Name: "shared-route", APIVersion: "route.openshift.io/v1"},
			logr.Discard(),
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(rr.Spec.SignalFingerprint).To(Equal(gwFingerprint),
			"AC-4: GW and AF must produce identical fingerprints for the same cluster+GVK top-level-owner resource (DD-GATEWAY-019 Alternative 2)")
	})
})

// selfResolvingFakeResolver resolves a standalone resource to itself,
// preserving its apiVersion the way the production K8sOwnerResolver does for
// top-level owners.
type selfResolvingFakeResolver struct{ apiVersion string }

func (f *selfResolvingFakeResolver) ResolveTopLevelOwner(_ context.Context, _, kind, name string) (string, string, error) {
	return kind, name, nil
}

func (f *selfResolvingFakeResolver) ResolveTopLevelOwnerWithAPIVersion(_ context.Context, _, kind, name string) (ownerchain.OwnerResolution, error) {
	return ownerchain.OwnerResolution{Kind: kind, Name: name, APIVersion: f.apiVersion}, nil
}
