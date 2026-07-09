package tools_test

import (
	"context"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	gwtypes "github.com/jordigilh/kubernaut/pkg/gateway/types"
)

func testFingerprint(ns, kind, name string) string {
	return gwtypes.CalculateClusterAwareFingerprint("", gwtypes.ResourceIdentifier{
		Namespace: ns,
		Kind:      kind,
		Name:      name,
	})
}

func newTypedRRWithFingerprint(namespace, name, phase, targetKind, targetName string) *remediationv1.RemediationRequest {
	fp := testFingerprint(namespace, targetKind, targetName)
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
		rr := newTypedRRWithFingerprint("prod", "rr-deploy-web-1", "Executing", "Deployment", "web")
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
		rr := newTypedRRWithFingerprint("prod", "rr-deploy-web-1", "Completed", "Deployment", "web")
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

	Describe("Cluster-aware dedup fingerprinting (#1409, AC-4: information-flow isolation)", func() {
		It("UT-AF-1409-005: a cluster_id=\"a\" lookup never matches state fingerprinted under cluster_id=\"b\"", func() {
			rrA := newTypedRRWithFingerprint("prod", "rr-cluster-a", "Executing", "Deployment", "web")
			rrA.Spec.ClusterID = "cluster-a"
			rrA.Spec.SignalFingerprint = gwtypes.CalculateClusterAwareFingerprint("cluster-a", gwtypes.ResourceIdentifier{
				Namespace: "prod", Kind: "Deployment", Name: "web",
			})
			client := newTypedFakeClient(rrA)

			result, err := tools.HandleCheckExistingRR(context.Background(), client, "prod", tools.CheckExistingRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", ClusterID: "cluster-b",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Exists).To(BeFalse(),
				"AC-4: cluster-b's dedup check must not match cluster-a's remediation state (no cross-cluster conflation)")
		})

		It("UT-AF-1409-005b: a cluster_id=\"a\" lookup matches state fingerprinted under the same cluster_id", func() {
			rrA := newTypedRRWithFingerprint("prod", "rr-cluster-a", "Executing", "Deployment", "web")
			rrA.Spec.ClusterID = "cluster-a"
			rrA.Spec.SignalFingerprint = gwtypes.CalculateClusterAwareFingerprint("cluster-a", gwtypes.ResourceIdentifier{
				Namespace: "prod", Kind: "Deployment", Name: "web",
			})
			client := newTypedFakeClient(rrA)

			result, err := tools.HandleCheckExistingRR(context.Background(), client, "prod", tools.CheckExistingRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", ClusterID: "cluster-a",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Exists).To(BeTrue())
			Expect(result.RRID).To(Equal("rr-cluster-a"))
			Expect(result.ClusterID).To(Equal("cluster-a"))
		})

		It("UT-AF-1409-005c: empty cluster_id (local hub) preserves pre-#1409 backward-compatible matching", func() {
			rr := newTypedRRWithFingerprint("prod", "rr-local", "Executing", "Deployment", "web")
			client := newTypedFakeClient(rr)

			result, err := tools.HandleCheckExistingRR(context.Background(), client, "prod", tools.CheckExistingRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Exists).To(BeTrue())
		})

		// IT-AF-1409-006 is the Wiring Manifest's designated proof for the
		// "cluster_id arg -> cluster-aware fingerprint -> dedup lookup result"
		// row. It calls HandleCheckExistingRR directly because that IS the
		// kubernaut_check_existing_remediation tool's production entry point
		// (NewCheckExistingRemediationTool's functiontool closure is a
		// zero-logic pass-through, same as the neighboring UT-AF-1409-005/005b
		// tests and IT-AF-1409-010 below) — there is no additional MCP/HTTP
		// transport layer to cross for this internal (ADK-only, non-MCP) tool.
		It("IT-AF-1409-006: AC-4 — kubernaut_check_existing_remediation enforces cluster-scoped information-flow isolation end-to-end", func() {
			rrEast := newTypedRRWithFingerprint("prod", "rr-east-006", "Executing", "Deployment", "web")
			rrEast.Spec.ClusterID = "cluster-east-006"
			rrEast.Spec.SignalFingerprint = gwtypes.CalculateClusterAwareFingerprint("cluster-east-006", gwtypes.ResourceIdentifier{
				Namespace: "prod", Kind: "Deployment", Name: "web",
			})
			client := newTypedFakeClient(rrEast)

			sameClusterResult, err := tools.HandleCheckExistingRR(context.Background(), client, "prod", tools.CheckExistingRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", ClusterID: "cluster-east-006",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(sameClusterResult.Exists).To(BeTrue(),
				"AC-4: a live, non-terminal remediation must be found when checked from its own cluster")
			Expect(sameClusterResult.ClusterID).To(Equal("cluster-east-006"))

			differentClusterResult, err := tools.HandleCheckExistingRR(context.Background(), client, "prod", tools.CheckExistingRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", ClusterID: "cluster-west-006",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(differentClusterResult.Exists).To(BeFalse(),
				"AC-4: a live, non-terminal remediation on cluster-east must never be reported as covering an identically-named target checked from cluster-west (no cross-cluster conflation)")
		})

		It("IT-AF-1409-010: SI-10 — oversized cluster_id rejected at the kubernaut_check_existing_remediation tool boundary before fingerprint computation", func() {
			client := newTypedFakeClient()

			_, err := tools.HandleCheckExistingRR(context.Background(), client, "prod", tools.CheckExistingRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", ClusterID: strings.Repeat("a", 254),
			})
			Expect(err).To(HaveOccurred(),
				"SI-10: validate.ClusterID must be wired into HandleCheckExistingRR, not dead code")
			Expect(err.Error()).To(ContainSubstring("cluster_id"))
		})
	})

	It("UT-AF-052-048: mismatched fingerprint not reported as existing", func() {
		rr := newTypedRRWithFingerprint("prod", "rr-deploy-web-1", "Executing", "Deployment", "web")
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
					SignalFingerprint: testFingerprint(workloadNS, "Deployment", "web"),
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
					SignalFingerprint: testFingerprint("staging", "Deployment", "web"),
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
