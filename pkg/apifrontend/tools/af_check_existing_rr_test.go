package tools_test

import (
	"context"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	gwtypes "github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// testFingerprint computes a cluster-aware fingerprint for a "Deployment"
// named "web" (the only kind/name combination used across all callers), in
// the given namespace.
func testFingerprint(ns string) string {
	return gwtypes.CalculateClusterAwareFingerprint("", gwtypes.ResourceIdentifier{
		Namespace: ns,
		Kind:      "Deployment",
		Name:      "web",
	})
}

// newTypedRRWithFingerprint builds a RemediationRequest in the fixed "prod"
// namespace, targeting a "Deployment" named "web" (the only namespace/target
// combination used across all callers of this helper).
func newTypedRRWithFingerprint(name, phase string) *remediationv1.RemediationRequest {
	const (
		namespace  = "prod"
		targetKind = "Deployment"
		targetName = "web"
	)
	fp := testFingerprint(namespace)
	return &remediationv1.RemediationRequest{
		ObjectMeta: objMeta(namespace, name),
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: fp,
			TargetResource: remediationv1.ResourceIdentifier{
				Kind: targetKind,
				Name: targetName,
			},
		},
		Status: remediationv1.RemediationRequestStatus{
			OverallPhase: remediationv1.RemediationPhase(phase),
		},
	}
}

var _ = Describe("kubernaut_check_existing_remediation", func() {
	It("UT-AF-052-040: finds active RR for matching fingerprint", func() {
		rr := newTypedRRWithFingerprint("rr-deploy-web-1", "Executing")
		client := newTypedFakeClient(rr)

		result, err := tools.HandleCheckExistingRR(context.Background(), client, "prod", tools.CheckExistingRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "web",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Exists).To(BeTrue())
		Expect(result.RRID).To(Equal("rr-deploy-web-1"))
		Expect(result.Phase).To(Equal("Executing"))
	})

	It("UT-AF-052-041: terminal RR not reported as existing", func() {
		rr := newTypedRRWithFingerprint("rr-deploy-web-1", "Completed")
		client := newTypedFakeClient(rr)

		result, err := tools.HandleCheckExistingRR(context.Background(), client, "prod", tools.CheckExistingRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "web",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Exists).To(BeFalse())
	})

	It("UT-AF-052-042: no RRs at all returns exists=false", func() {
		client := newTypedFakeClient()

		result, err := tools.HandleCheckExistingRR(context.Background(), client, "prod", tools.CheckExistingRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "web",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Exists).To(BeFalse())
	})

	It("UT-AF-052-043: empty namespace rejected", func() {
		client := newTypedFakeClient()

		_, err := tools.HandleCheckExistingRR(context.Background(), client, "prod", tools.CheckExistingRRArgs{
			Namespace: "", Kind: "Deployment", Name: "web",
		})
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-044: empty kind rejected", func() {
		client := newTypedFakeClient()

		_, err := tools.HandleCheckExistingRR(context.Background(), client, "prod", tools.CheckExistingRRArgs{
			Namespace: "prod", Kind: "", Name: "web",
		})
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-045: empty name rejected", func() {
		client := newTypedFakeClient()

		_, err := tools.HandleCheckExistingRR(context.Background(), client, "prod", tools.CheckExistingRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "",
		})
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-046: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleCheckExistingRR(context.Background(), nil, "prod", tools.CheckExistingRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "web",
		})
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-052-047: concurrent calls safe", func() {
		client := newTypedFakeClient()

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := tools.HandleCheckExistingRR(context.Background(), client, "prod", tools.CheckExistingRRArgs{
					Namespace: "prod", Kind: "Deployment", Name: "web",
				})
				Expect(err).NotTo(HaveOccurred())
			}()
		}
		wg.Wait()
	})

	It("UT-AF-052-048: mismatched fingerprint not reported as existing", func() {
		rr := newTypedRRWithFingerprint("rr-deploy-web-1", "Executing")
		client := newTypedFakeClient(rr)

		result, err := tools.HandleCheckExistingRR(context.Background(), client, "prod", tools.CheckExistingRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "other-target",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Exists).To(BeFalse())
	})

	Describe("ADR-057 namespace split (F-NS-SPLIT)", func() {
		It("UT-AF-1292-NS-006: lists in controllerNS, fingerprints with workload NS (BR-SAFETY-001, SI-10)", func() {
			controllerNS := "kubernaut-system"
			workloadNS := "prod"

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: objMeta(controllerNS, "rr-deploy-web-existing"),
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: testFingerprint(workloadNS),
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "web",
						Namespace: workloadNS,
					},
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: "Executing",
				},
			}
			client := newTypedFakeClient(rr)

			result, err := tools.HandleCheckExistingRR(context.Background(), client, controllerNS, tools.CheckExistingRRArgs{
				Namespace: workloadNS, Kind: "Deployment", Name: "web",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Exists).To(BeTrue(),
				"must find RR in controllerNS using workload NS fingerprint")
			Expect(result.RRID).To(Equal("rr-deploy-web-existing"))
		})

		It("UT-AF-1292-NS-007: returns false when fingerprint uses wrong workload NS (BR-SAFETY-001, SI-10)", func() {
			controllerNS := "kubernaut-system"

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: objMeta(controllerNS, "rr-deploy-web-staging"),
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: testFingerprint("staging"),
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "web",
						Namespace: "staging",
					},
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: "Executing",
				},
			}
			client := newTypedFakeClient(rr)

			result, err := tools.HandleCheckExistingRR(context.Background(), client, controllerNS, tools.CheckExistingRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Exists).To(BeFalse(),
				"fingerprint(prod/Deployment/web) must NOT match fingerprint(staging/Deployment/web)")
		})
	})
})
