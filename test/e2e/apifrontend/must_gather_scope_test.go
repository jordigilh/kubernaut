package e2e_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("APIFrontend E2E must-gather scope [BR-TESTING-001]", func() {
	It("UT-E2E-AF-FLEET-MUSTGATHER-001: combined runs include local and Fleet clusters when Fleet setup was attempted", func() {
		Expect(apifrontendE2EClusterNames(afE2ELaneCombined, false)).To(Equal([]string{e2eClusterName}),
			"BR-TESTING-001: local AF failure diagnostics must target the local cluster")
		Expect(apifrontendE2EClusterNames(afE2ELaneCombined, true)).To(Equal([]string{e2eClusterName, fleetAFClusterName}),
			"BR-TESTING-001: Fleet-enabled AF failure diagnostics must target both clusters")
	})

	It("UT-E2E-AF-FLEET-MUSTGATHER-002: collects Envoy controller namespaces only from the Fleet cluster", func() {
		Expect(apifrontendMustGatherExtraNamespaces(e2eClusterName, true)).To(BeEmpty(),
			"BR-TESTING-001: local AF diagnostics must not request Fleet-only namespaces")
		Expect(apifrontendMustGatherExtraNamespaces(fleetAFClusterName, true)).To(Equal([]string{
			"envoy-gateway-system",
			"envoy-ai-gateway-system",
		}), "BR-TESTING-001: Fleet diagnostics must include both deployed Envoy controller namespaces")
		Expect(apifrontendMustGatherExtraNamespaces(fleetAFClusterName, false)).To(BeEmpty(),
			"BR-TESTING-001: local-only runs must not collect Fleet controller namespaces")
	})

	It("UT-E2E-AF-FLEET-LANE-001 [BR-FLEET-054]: local CI lane owns only the standalone AF cluster", func() {
		Expect(apifrontendE2EClusterNames(afE2ELaneLocal, false)).To(Equal([]string{e2eClusterName}))
		Expect(apifrontendE2EClusterNames(afE2ELaneLocal, true)).To(Equal([]string{e2eClusterName}),
			"the local lane must never claim or tear down the Fleet lane's cluster")
	})

	It("UT-E2E-AF-FLEET-LANE-002 [BR-INTEGRATION-065]: Fleet CI lane owns only its hub-only cluster", func() {
		Expect(apifrontendE2EClusterNames(afE2ELaneFleet, true)).To(Equal([]string{fleetAFClusterName}))
		Expect(apifrontendE2EClusterNames(afE2ELaneFleet, false)).To(BeEmpty(),
			"a Fleet-only setup failure before cluster creation must not target the local cluster")
	})
})
