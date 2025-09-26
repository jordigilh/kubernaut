//go:build integration
// +build integration

package multicluster

import (
	"context"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/platform/multicluster"
	testshared "github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("BR-PLATFORM-MULTICLUSTER-001: Multi-Cluster pgvector Sync", Ordered, func() {
	var (
		hooks                   *testshared.TestLifecycleHooks
		ctx                     context.Context
		suite                   *testshared.StandardTestSuite
		multiClusterSyncManager *multicluster.MultiClusterPgVectorSyncManager
		logger                  *logrus.Logger
	)

	BeforeAll(func() {
		// Following guideline: Reuse existing test infrastructure
		hooks = testshared.SetupAIIntegrationTest("Multi-Cluster pgvector Sync",
			testshared.WithRealVectorDB(), // Current milestone: pgvector only
			testshared.WithDatabaseIsolation(testshared.TransactionIsolation),
		)
		hooks.Setup()

		suite = hooks.GetSuite()
		logger = suite.Logger
	})

	AfterAll(func() {
		if hooks != nil {
			hooks.Cleanup()
		}
	})

	BeforeEach(func() {
		ctx = context.Background()

		// Validate test environment is healthy before each test
		Expect(suite.VectorDB).ToNot(BeNil(), "Vector database should be available")

		// Create multi-cluster sync manager for integration testing
		multiClusterSyncManager = multicluster.NewMultiClusterPgVectorSyncManager(suite.VectorDB, logger)
		Expect(multiClusterSyncManager).ToNot(BeNil(), "Multi-cluster sync manager should be created successfully")
	})

	Context("when managing multi-cluster operations with shared pgvector", func() {
		It("should synchronize vector data across clusters with integrity validation", func() {
			By("setting up multi-cluster configuration")
			clusterConfigs := []*multicluster.ClusterConfig{
				{
					ID:             "cluster-primary",
					Name:           "primary-cluster",
					Role:           "primary",
					VectorEndpoint: "localhost:5434", // pgvector DB from bootstrap
					Priority:       1,
				},
				{
					ID:             "cluster-secondary",
					Name:           "secondary-cluster",
					Role:           "secondary",
					VectorEndpoint: "localhost:5434", // Shared pgvector for current milestone
					Priority:       2,
				},
			}

			err := multiClusterSyncManager.ConfigureClusters(ctx, clusterConfigs)
			Expect(err).ToNot(HaveOccurred(), "Cluster configuration should succeed")

			// BR-PLATFORM-MULTICLUSTER-001: Cluster configuration validation
			activeClusterCount := multiClusterSyncManager.GetActiveClusterCount()
			Expect(activeClusterCount).To(Equal(2), "BR-PLATFORM-MULTICLUSTER-001: Should have 2 active clusters")

			By("storing vector data in primary cluster")
			vectorData := &multicluster.VectorDataEntry{
				ID:      "test-vector-multicluster-001",
				Content: "Critical pod restart recommendation for multi-cluster environment",
				Vector:  generateTestVector(384), // MiniLM dimension
				Metadata: map[string]interface{}{
					"cluster_origin": "primary",
					"alert_type":     "critical_cpu",
					"timestamp":      time.Now().Unix(),
					"sync_required":  true,
				},
			}

			storageStartTime := time.Now()
			err = multiClusterSyncManager.StoreVectorData(ctx, "cluster-primary", vectorData)
			storageTime := time.Since(storageStartTime)

			Expect(err).ToNot(HaveOccurred(), "Vector data storage should succeed")

			// BR-PLATFORM-MULTICLUSTER-002: Storage performance validation
			Expect(storageTime).To(BeNumerically("<", 2*time.Second), "BR-PLATFORM-MULTICLUSTER-002: Storage should be efficient")

			By("synchronizing vector data to secondary cluster")
			syncStartTime := time.Now()
			syncResult, err := multiClusterSyncManager.SynchronizeVectorData(ctx, "cluster-primary", "cluster-secondary")
			syncTime := time.Since(syncStartTime)

			Expect(err).ToNot(HaveOccurred(), "Vector synchronization should succeed")
			Expect(syncResult.EntriesSynced).To(BeNumerically(">=", 1), "Should sync at least the stored entry")

			// BR-PLATFORM-MULTICLUSTER-003: Sync performance validation
			Expect(syncTime).To(BeNumerically("<", 5*time.Second), "BR-PLATFORM-MULTICLUSTER-003: Sync should complete within SLA")

			// BR-PLATFORM-MULTICLUSTER-004: Data integrity validation
			Expect(syncResult.IntegrityValidated).To(BeTrue(), "BR-PLATFORM-MULTICLUSTER-004: Data integrity should be validated")
			Expect(syncResult.ConsistencyScore).To(BeNumerically(">=", 0.95), "BR-PLATFORM-MULTICLUSTER-004: Should maintain high consistency")

			By("validating cross-cluster vector data consistency")
			primaryData, err := multiClusterSyncManager.RetrieveVectorData(ctx, "cluster-primary", vectorData.ID)
			Expect(err).ToNot(HaveOccurred(), "Primary cluster data retrieval should succeed")

			secondaryData, err := multiClusterSyncManager.RetrieveVectorData(ctx, "cluster-secondary", vectorData.ID)
			Expect(err).ToNot(HaveOccurred(), "Secondary cluster data retrieval should succeed")

			// Validate data consistency across clusters
			Expect(primaryData.Content).To(Equal(secondaryData.Content), "Content should be consistent across clusters")
			Expect(primaryData.ID).To(Equal(secondaryData.ID), "IDs should be consistent across clusters")

			// Validate vector similarity (should be very high for identical data)
			similarity := calculateVectorSimilarity(primaryData.Vector, secondaryData.Vector)
			Expect(similarity).To(BeNumerically(">=", 0.99), "Vector data should be nearly identical across clusters")
		})

		It("should handle cluster failover with vector data integrity", func() {
			By("setting up multi-cluster configuration for failover testing")
			clusterConfigs := []*multicluster.ClusterConfig{
				{
					ID:             "cluster-primary",
					Name:           "primary-cluster",
					Role:           "primary",
					VectorEndpoint: "localhost:5434", // pgvector DB from bootstrap
					Priority:       1,
				},
				{
					ID:             "cluster-secondary",
					Name:           "secondary-cluster",
					Role:           "secondary",
					VectorEndpoint: "localhost:5434", // Shared pgvector for current milestone
					Priority:       2,
				},
			}

			err := multiClusterSyncManager.ConfigureClusters(ctx, clusterConfigs)
			Expect(err).ToNot(HaveOccurred(), "Cluster configuration should succeed")

			By("creating test scenario with cluster failure")
			// Set up initial data in primary cluster
			testVectorData := &multicluster.VectorDataEntry{
				ID:      "failover-test-vector-001",
				Content: "High memory usage alert requiring cluster failover testing",
				Vector:  generateTestVector(384),
				Metadata: map[string]interface{}{
					"critical":      true,
					"failover_test": true,
				},
			}

			err = multiClusterSyncManager.StoreVectorData(ctx, "cluster-primary", testVectorData)
			Expect(err).ToNot(HaveOccurred(), "Initial data storage should succeed")

			By("simulating primary cluster failure")
			failureStartTime := time.Now()
			err = multiClusterSyncManager.SimulateClusterFailure(ctx, "cluster-primary")
			Expect(err).ToNot(HaveOccurred(), "Cluster failure simulation should be configured")

			By("executing automatic failover to secondary cluster")
			failoverResult, err := multiClusterSyncManager.ExecuteFailover(ctx, "cluster-primary", "cluster-secondary")
			failoverTime := time.Since(failureStartTime)

			Expect(err).ToNot(HaveOccurred(), "Failover should execute successfully")

			// BR-PLATFORM-MULTICLUSTER-005: Failover time validation
			Expect(failoverTime).To(BeNumerically("<", 30*time.Second), "BR-PLATFORM-MULTICLUSTER-005: Failover should complete within 30s")

			// BR-PLATFORM-MULTICLUSTER-006: Failover success validation
			Expect(failoverResult.Success).To(BeTrue(), "BR-PLATFORM-MULTICLUSTER-006: Failover should succeed")
			Expect(failoverResult.DataIntegrityMaintained).To(BeTrue(), "BR-PLATFORM-MULTICLUSTER-006: Should maintain data integrity during failover")

			By("validating vector data accessibility from secondary cluster after failover")
			retrievedData, err := multiClusterSyncManager.RetrieveVectorData(ctx, "cluster-secondary", testVectorData.ID)
			Expect(err).ToNot(HaveOccurred(), "Data should be accessible from secondary cluster")
			Expect(retrievedData.Content).To(Equal(testVectorData.Content), "Data content should be preserved")

			// Validate that operations continue to work on the secondary cluster
			newVectorData := &multicluster.VectorDataEntry{
				ID:      "post-failover-vector-001",
				Content: "New alert processed after failover",
				Vector:  generateTestVector(384),
			}

			err = multiClusterSyncManager.StoreVectorData(ctx, "cluster-secondary", newVectorData)
			Expect(err).ToNot(HaveOccurred(), "New data storage should work on secondary cluster")
		})
	})

	Context("when managing Kubernetes resource discovery with vector correlation", func() {
		It("should discover and correlate Kubernetes resources using vector analysis", func() {
			By("creating test Kubernetes resources for discovery")
			testResources := []*multicluster.KubernetesResource{
				{
					ID:        "resource-discovery-pod-001",
					Kind:      "Pod",
					Name:      "critical-app-pod",
					Namespace: "production",
					Status:    "Running",
					Metadata: map[string]interface{}{
						"cpu_usage":    "85%",
						"memory_usage": "70%",
						"alerts":       []string{"high_cpu", "memory_warning"},
					},
				},
				{
					ID:        "resource-discovery-deployment-001",
					Kind:      "Deployment",
					Name:      "critical-app",
					Namespace: "production",
					Status:    "Available",
					Metadata: map[string]interface{}{
						"replicas":       3,
						"ready_replicas": 2,
						"strategy":       "RollingUpdate",
					},
				},
			}

			By("executing resource discovery with vector correlation")
			discoveryStartTime := time.Now()
			discoveryResult, err := multiClusterSyncManager.DiscoverResourcesWithVectorCorrelation(ctx, testResources)
			discoveryTime := time.Since(discoveryStartTime)

			Expect(err).ToNot(HaveOccurred(), "Resource discovery should succeed")

			// BR-PLATFORM-MULTICLUSTER-007: Discovery performance validation
			Expect(discoveryTime).To(BeNumerically("<", 10*time.Second), "BR-PLATFORM-MULTICLUSTER-007: Discovery should complete efficiently")

			// BR-PLATFORM-MULTICLUSTER-008: Discovery accuracy validation
			Expect(len(discoveryResult.DiscoveredResources)).To(BeNumerically(">=", 2), "BR-PLATFORM-MULTICLUSTER-008: Should discover all provided resources")
			Expect(len(discoveryResult.VectorCorrelations)).To(BeNumerically(">=", 1), "BR-PLATFORM-MULTICLUSTER-008: Should find vector correlations")

			By("validating similarity-based resource correlation")
			for _, correlation := range discoveryResult.VectorCorrelations {
				// Each correlation should have meaningful similarity
				Expect(correlation.SimilarityScore).To(BeNumerically(">=", 0.7), "Resource correlations should have meaningful similarity")

				// Correlations should provide actionable insights
				Expect(correlation.Insights).ToNot(BeEmpty(), "Correlations should provide insights")

				// Cost optimization focus - correlations should suggest efficiency improvements
				Expect(correlation.EfficiencyScore).To(BeNumerically(">=", 0.7), "BR-PLATFORM-MULTICLUSTER-008: Resource correlation efficiency should meet threshold for cost optimization")
			}
		})

		It("should optimize cross-cluster resource allocation using vector insights", func() {
			By("analyzing resource allocation patterns across clusters")
			allocationRequest := &multicluster.ResourceAllocationRequest{
				ResourceType: "Pod",
				Requirements: multicluster.ResourceRequirements{
					CPU:     "500m",
					Memory:  "1Gi",
					Storage: "10Gi",
				},
				Constraints: map[string]interface{}{
					"cost_optimization":  true,
					"availability_zones": []string{"us-east-1a", "us-east-1b"},
					"max_cost_per_hour":  0.05,
				},
			}

			allocationStartTime := time.Now()
			allocationResult, err := multiClusterSyncManager.OptimizeResourceAllocation(ctx, allocationRequest)
			allocationTime := time.Since(allocationStartTime)

			Expect(err).ToNot(HaveOccurred(), "Resource allocation optimization should succeed")

			// BR-PLATFORM-MULTICLUSTER-009: Allocation optimization validation
			Expect(allocationTime).To(BeNumerically("<", 15*time.Second), "BR-PLATFORM-MULTICLUSTER-009: Allocation should be efficient")
			Expect(allocationResult.RecommendedCluster).ToNot(BeEmpty(), "BR-PLATFORM-MULTICLUSTER-009: Should recommend optimal cluster")
			Expect(allocationResult.CostOptimized).To(BeTrue(), "BR-PLATFORM-MULTICLUSTER-009: Should prioritize cost optimization")

			// Validate cost constraints are respected
			Expect(allocationResult.EstimatedCostPerHour).To(BeNumerically("<=", 0.05), "Should respect cost constraints")

			// Validate allocation uses vector insights for decision making
			Expect(allocationResult.VectorInsightsUsed).To(BeTrue(), "Should use vector insights for allocation decisions")
			Expect(allocationResult.ConfidenceScore).To(BeNumerically(">=", 0.8), "Should have high confidence in allocation decision")
		})
	})
})

// Helper functions for test scenarios

func generateTestVector(dimension int) []float32 {
	vector := make([]float32, dimension)
	for i := range vector {
		vector[i] = float32(i) * 0.1 // Simple test pattern
	}
	return vector
}

func calculateVectorSimilarity(vec1, vec2 []float32) float64 {
	if len(vec1) != len(vec2) {
		return 0.0
	}

	// Correct cosine similarity calculation
	var dotProduct, norm1, norm2 float64
	for i := range vec1 {
		dotProduct += float64(vec1[i] * vec2[i])
		norm1 += float64(vec1[i] * vec1[i])
		norm2 += float64(vec2[i] * vec2[i])
	}

	if norm1 == 0 || norm2 == 0 {
		return 0.0
	}

	// Cosine similarity: dotProduct / (||vec1|| * ||vec2||)
	return dotProduct / (math.Sqrt(norm1) * math.Sqrt(norm2))
}
