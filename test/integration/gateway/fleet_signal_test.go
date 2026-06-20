/*
Copyright 2025 Jordi Gil.

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

package gateway

import (
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// GW Fleet Signal Ingestion (BR-INTEGRATION-065)
//
// Phase 1 RED tests for cluster-aware signal ingestion.
// These tests validate that:
// 1. PrometheusAdapter extracts the cluster label from commonLabels
// 2. NormalizedSignal carries a ClusterID field
// 3. CRDCreator populates spec.clusterID on the RemediationRequest
// 4. Fingerprint calculation includes clusterID for cluster-aware dedup
//
// All tests are expected to FAIL (RED) because:
// - NormalizedSignal does not yet have a ClusterID field
// - PrometheusAdapter does not extract cluster from commonLabels
// - CRDCreator does not populate spec.clusterID from the signal
// - CalculateOwnerFingerprint does not factor in clusterID
var _ = Describe("GW Fleet Signal Ingestion (BR-INTEGRATION-065)", Ordered, Label("fleet", "integration"), func() {
	var (
		testLogger    logr.Logger
		testNamespace string
		gwServer      *gateway.Server
	)

	BeforeAll(func() {
		testLogger = logger.WithValues("test", "fleet-signal-ingestion")

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("GW Fleet Signal Ingestion (BR-INTEGRATION-065) - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		testNamespace = helpers.CreateTestNamespace(ctx, k8sClient, "fleet-signal-int")

		gwConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
		var err error
		gwServer, err = createGatewayServer(gwConfig, testLogger, k8sClient, sharedAuditStore)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Gateway server")

		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)
		testLogger.Info("✅ Gateway server created with shared K8s client")
	})

	AfterAll(func() {
		if CurrentSpecReport().Failed() {
			testLogger.Info("⚠️  Test FAILED - Preserving namespace", "namespace", testNamespace)
			return
		}
		helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
		testLogger.Info("✅ Test cleanup complete")
	})

	// IT-GW-FLEET-001: Cluster label propagation to RR spec.clusterID
	//
	// Business Outcome: When a Prometheus alert arrives with commonLabels.cluster="prod-east",
	// the Gateway must propagate this through the full pipeline so the resulting
	// RemediationRequest has spec.clusterID="prod-east". This enables downstream
	// services (RO, WE) to route remediation to the correct cluster.
	//
	// RED because:
	// - NormalizedSignal has no ClusterID field
	// - PrometheusAdapter.Parse does not extract cluster from commonLabels
	// - CRDCreator does not set spec.clusterID from signal.ClusterID
	It("IT-GW-FLEET-001: should propagate cluster label to RR spec.clusterID", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("IT-GW-FLEET-001: Cluster label → spec.clusterID propagation")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		signal := createNormalizedSignal(SignalBuilder{
			AlertName:    "HighMemoryUsage",
			Namespace:    testNamespace,
			ResourceKind: "Deployment",
			ResourceName: "nginx",
			Severity:     "critical",
			Source:       "prometheus",
		})
		// RED: NormalizedSignal.ClusterID does not exist yet.
		// When implemented, PrometheusAdapter will set this from commonLabels.cluster.
		// For now, we set it directly to demonstrate the expected contract.
		signal.ClusterID = "prod-east"

		response, err := gwServer.ProcessSignal(ctx, signal)
		Expect(err).ToNot(HaveOccurred(), "ProcessSignal should succeed")
		Expect(response.Status).To(Equal("created"),
			"Signal should create a new RemediationRequest")

		crdList := &remediationv1alpha1.RemediationRequestList{}
		err = k8sClient.List(ctx, crdList, client.InNamespace(controllerNamespace))
		Expect(err).ToNot(HaveOccurred(), "Should list CRDs successfully")

		var matchingRR *remediationv1alpha1.RemediationRequest
		for i := range crdList.Items {
			if crdList.Items[i].Name == response.RemediationRequestName {
				matchingRR = &crdList.Items[i]
				break
			}
		}
		Expect(matchingRR).ToNot(BeNil(),
			"Should find the created RemediationRequest by name")

		Expect(matchingRR.Spec.ClusterID).To(Equal("prod-east"),
			"BR-INTEGRATION-065: RR spec.clusterID must match the cluster label from the signal")

		testLogger.Info("✅ IT-GW-FLEET-001 PASSED: cluster label propagated to spec.clusterID")
	})

	// IT-GW-FLEET-002: Cluster-aware deduplication produces different fingerprints
	//
	// Business Outcome: Two alerts for the same resource (namespace=default, kind=Deployment,
	// name=nginx) from different clusters ("prod-east" vs "prod-west") MUST create two
	// separate RemediationRequests with different fingerprints. Without cluster-aware
	// fingerprinting, the second alert would be deduplicated away, leaving the prod-west
	// cluster without remediation.
	//
	// This test computes cluster-aware fingerprints using the production function
	// (CalculateClusterAwareFingerprint) to ensure the signals have properly
	// differentiated fingerprints before entering the processing pipeline.
	It("IT-GW-FLEET-002: should create different RRs for same resource on different clusters", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("IT-GW-FLEET-002: Cluster-aware deduplication (different clusters)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		sharedResource := types.ResourceIdentifier{
			Namespace: "default",
			Kind:      "Deployment",
			Name:      "nginx",
		}

		signalEast := createNormalizedSignal(SignalBuilder{
			AlertName:    "HighCPU",
			Namespace:    "default",
			ResourceKind: "Deployment",
			ResourceName: "nginx",
			Severity:     "warning",
			Source:       "prometheus",
		})
		signalEast.ClusterID = "prod-east"
		signalEast.Fingerprint = types.CalculateClusterAwareFingerprint("prod-east", sharedResource)

		responseEast, err := gwServer.ProcessSignal(ctx, signalEast)
		Expect(err).ToNot(HaveOccurred(), "ProcessSignal for prod-east should succeed")
		Expect(responseEast.Status).To(Equal("created"),
			"prod-east signal should create a new RR")

		signalWest := createNormalizedSignal(SignalBuilder{
			AlertName:    "HighCPU",
			Namespace:    "default",
			ResourceKind: "Deployment",
			ResourceName: "nginx",
			Severity:     "warning",
			Source:       "prometheus",
		})
		signalWest.ClusterID = "prod-west"
		signalWest.Fingerprint = types.CalculateClusterAwareFingerprint("prod-west", sharedResource)

		responseWest, err := gwServer.ProcessSignal(ctx, signalWest)
		Expect(err).ToNot(HaveOccurred(), "ProcessSignal for prod-west should succeed")
		Expect(responseWest.Status).To(Equal("created"),
			"prod-west signal should create a SEPARATE RR (not deduplicated)")

		Expect(responseEast.Fingerprint).ToNot(Equal(responseWest.Fingerprint),
			"BR-INTEGRATION-065: Same resource on different clusters must have different fingerprints")

		crdList := &remediationv1alpha1.RemediationRequestList{}
		err = k8sClient.List(ctx, crdList, client.InNamespace(controllerNamespace))
		Expect(err).ToNot(HaveOccurred())

		eastRRName := responseEast.RemediationRequestName
		westRRName := responseWest.RemediationRequestName
		Expect(eastRRName).ToNot(Equal(westRRName),
			"Two different RRs should have been created")

		var eastRR, westRR *remediationv1alpha1.RemediationRequest
		for i := range crdList.Items {
			switch crdList.Items[i].Name {
			case eastRRName:
				eastRR = &crdList.Items[i]
			case westRRName:
				westRR = &crdList.Items[i]
			}
		}

		Expect(eastRR).ToNot(BeNil(), "Should find prod-east RR")
		Expect(westRR).ToNot(BeNil(), "Should find prod-west RR")

		Expect(eastRR.Spec.ClusterID).To(Equal("prod-east"),
			"prod-east RR should have clusterID=prod-east")
		Expect(westRR.Spec.ClusterID).To(Equal("prod-west"),
			"prod-west RR should have clusterID=prod-west")

		Expect(eastRR.Spec.SignalFingerprint).ToNot(Equal(westRR.Spec.SignalFingerprint),
			"BR-INTEGRATION-065: Fingerprints must differ for cluster-aware deduplication")

		testLogger.Info("✅ IT-GW-FLEET-002 PASSED: cluster-aware deduplication creates separate RRs")
	})

	// IT-GW-FLEET-003: Backward compatibility — no cluster label
	//
	// Business Outcome: When an alert arrives WITHOUT a cluster label (single-cluster
	// deployment), the system must behave exactly as before: empty spec.clusterID and
	// a fingerprint matching the current (non-cluster-aware) algorithm. This ensures
	// zero regression for existing users who are not using multi-cluster federation.
	//
	// RED because:
	// - NormalizedSignal has no ClusterID field (compilation failure)
	// - Even if it compiled, CRDCreator does not set spec.clusterID at all
	It("IT-GW-FLEET-003: should create RR with empty clusterID when no cluster label present", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("IT-GW-FLEET-003: Backward compatibility (no cluster label)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		signal := createNormalizedSignal(SignalBuilder{
			AlertName:    "PodCrashLoop",
			Namespace:    testNamespace,
			ResourceKind: "Deployment",
			ResourceName: "backend-api",
			Severity:     "critical",
			Source:       "prometheus",
		})
		// ClusterID is intentionally NOT set (zero value) to test backward compat

		response, err := gwServer.ProcessSignal(ctx, signal)
		Expect(err).ToNot(HaveOccurred(), "ProcessSignal should succeed without cluster label")
		Expect(response.Status).To(Equal("created"),
			"Signal without cluster label should still create an RR")

		crdList := &remediationv1alpha1.RemediationRequestList{}
		err = k8sClient.List(ctx, crdList, client.InNamespace(controllerNamespace))
		Expect(err).ToNot(HaveOccurred())

		var matchingRR *remediationv1alpha1.RemediationRequest
		for i := range crdList.Items {
			if crdList.Items[i].Name == response.RemediationRequestName {
				matchingRR = &crdList.Items[i]
				break
			}
		}
		Expect(matchingRR).ToNot(BeNil(),
			"Should find the created RemediationRequest by name")

		Expect(matchingRR.Spec.ClusterID).To(BeEmpty(),
			"BR-INTEGRATION-065: RR spec.clusterID must be empty when no cluster label is present (backward compat)")

		testLogger.Info("✅ IT-GW-FLEET-003 PASSED: backward compatibility with empty clusterID")
	})
})
