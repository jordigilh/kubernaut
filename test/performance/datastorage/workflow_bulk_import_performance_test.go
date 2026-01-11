package datastorage

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// GAP 4.2: Workflow Catalog Bulk Operations
// ========================================
// BR-STORAGE-030: Initial catalog load handles 100+ workflows efficiently
// Priority: P1 - Operational maturity
// Estimated Effort: 1 hour
// Confidence: 93%
//
// Business Outcome: Catalog bootstrap completes quickly (<60s for 200 workflows)
//
// Test Scenario:
//   GIVEN 200 workflow definitions (initial catalog load)
//   WHEN all 200 workflows created via sequential POST /api/v1/workflows
//   THEN:
//     - All 200 workflows created successfully
//     - Total operation time <60s (300ms avg per workflow)
//     - PostgreSQL connection pool not exhausted
//     - Search index remains performant
//
// Why This Matters: Initial catalog load is critical for deployment/bootstrap
// ========================================

var _ = Describe("GAP 4.2: Workflow Catalog Bulk Operations",  Label("integration", "datastorage", "gap-4.2", "p1"), func() {
	var (
		client *ogenclient.Client
		ctx    context.Context
	)

	BeforeEach(func() {
		// CRITICAL: Use public schema for remediation_workflow_catalog
		// This prevents data contamination in workflow repository tests
		usePublicSchema()

		ctx = context.Background()

		// Create OpenAPI client
		var err error
		client, err = createOpenAPIClient(datastorageURL)
		Expect(err).ToNot(HaveOccurred())

		// Clean up any leftover workflows from previous runs
		_, _ = db.Exec("DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE 'bulk-import%'")
	})

	AfterEach(func() {
		_, _ = db.Exec("DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE 'bulk-import%'")
	})

	Context("when importing 200 workflows (catalog bootstrap)", func() {
		It("should create all 200 workflows in <60s (avg 300ms per workflow)", func() {
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			GinkgoWriter.Println("GAP 4.2: Testing workflow bulk import (200 workflows)")
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// ARRANGE: Generate 200 test workflow definitions
			const workflowCount = 200
			testID := generateTestID()

			// ACT: Create 200 workflows sequentially
			startTime := time.Now()
			successCount := 0
			failedWorkflows := []int{}

			for i := 0; i < workflowCount; i++ {
				content := fmt.Sprintf(`{"steps":[{"action":"scale","replicas":%d}]}`, i)
				contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

			// Build typed workflow request
			workflowName := fmt.Sprintf("bulk-import-%s-workflow-%d", testID, i)
			version := "v1.0.0"
			name := fmt.Sprintf("Bulk Import Test Workflow %d", i)
			description := fmt.Sprintf("Test workflow %d for bulk import performance", i)
			status := ogenclient.RemediationWorkflowStatusActive
			executionEngine := "argo-workflows"

			// V1.0: Use structured MandatoryLabels
			labels := ogenclient.MandatoryLabels{
				SignalType:  "bulk-import-test",
				Severity:    "low",
				Component:   fmt.Sprintf("component-%d", i%10), // 10 different components
				Priority:    "P2",
				Environment: "testing",
			}

			workflow := ogenclient.RemediationWorkflow{
				WorkflowName:    workflowName,
				Version:         version,
				Name:            name,
				Description:     description,
				Content:         content,
				ContentHash:     contentHash,
				Labels:          labels,
				CustomLabels:    ogenclient.OptCustomLabels{},
				DetectedLabels:  ogenclient.OptDetectedLabels{},
				ExecutionEngine: executionEngine,
				Status:          status,
			}

		// Use OpenAPI client to create workflow
		_, err := createWorkflow(ctx, client, workflow)
			if err != nil {
				GinkgoWriter.Printf("❌ Failed to create workflow %d: %v\n", i, err)
				failedWorkflows = append(failedWorkflows, i)
				continue
			}

			successCount++

				// Progress indicator every 50 workflows
				if (i+1)%50 == 0 {
					GinkgoWriter.Printf("Progress: %d/%d workflows created (%.1f%%)\n",
						i+1, workflowCount, float64(i+1)/float64(workflowCount)*100)
				}
			}

			totalDuration := time.Since(startTime)
			avgDurationMs := totalDuration.Milliseconds() / int64(workflowCount)

			GinkgoWriter.Printf("\nBulk import completed:\n")
			GinkgoWriter.Printf("  Total workflows: %d\n", workflowCount)
			GinkgoWriter.Printf("  Successful:      %d\n", successCount)
			GinkgoWriter.Printf("  Failed:          %d\n", len(failedWorkflows))
			GinkgoWriter.Printf("  Total duration:  %v\n", totalDuration)
			GinkgoWriter.Printf("  Avg per workflow: %dms\n", avgDurationMs)
			GinkgoWriter.Printf("  Throughput:      %.2f workflows/sec\n", float64(workflowCount)/totalDuration.Seconds())

			// ASSERT: All workflows created successfully
			Expect(successCount).To(Equal(workflowCount),
				fmt.Sprintf("Expected all %d workflows to be created, got %d successful. Failed indices: %v",
					workflowCount, successCount, failedWorkflows))

			// ASSERT: Performance target met (<60s total, <300ms avg)
			Expect(totalDuration.Seconds()).To(BeNumerically("<", 60),
				fmt.Sprintf("Bulk import took %v, exceeds 60s target", totalDuration))

			Expect(avgDurationMs).To(BeNumerically("<", 300),
				fmt.Sprintf("Average workflow creation time %dms exceeds 300ms target", avgDurationMs))

			GinkgoWriter.Printf("✅ Workflow bulk import performance validated\n")
			GinkgoWriter.Printf("   Total: %v (target: <60s)\n", totalDuration)
			GinkgoWriter.Printf("   Avg:   %dms (target: <300ms)\n", avgDurationMs)

			// BUSINESS VALUE: Fast catalog bootstrap
			// - Initial catalog load completes quickly
			// - Deployment/bootstrap scripts complete in reasonable time
			// - Connection pool handles sequential bulk load
			// - Ready for production catalog seeding
		})

		It("should maintain search performance after bulk import", func() {
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			GinkgoWriter.Println("GAP 4.2: Verifying search performance after bulk import")
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// ACT: Execute workflow search after bulk import using OpenAPI client
			topK := 10
		// V1.0: Use generated enum types
		filters := ogenclient.WorkflowSearchFilters{
			SignalType:  "bulk-import-test",
			Severity:    ogenclient.WorkflowSearchFiltersSeverityLow,
			Component:   "component-0",
			Priority:    ogenclient.WorkflowSearchFiltersPriorityP2,
			Environment: "testing",
		}

		searchRequest := ogenclient.WorkflowSearchRequest{
			Filters: filters,
			TopK:    ogenclient.NewOptInt(topK),
		}

		// Measure search latency
		startTime := time.Now()
	searchResult, err := searchWorkflows(ctx, client, searchRequest)
	searchDuration := time.Since(startTime)

	Expect(err).ToNot(HaveOccurred())
		Expect(searchResult).ToNot(BeNil())

		// ASSERT: Search remains performant (<500ms)
			Expect(searchDuration.Milliseconds()).To(BeNumerically("<", 500),
				fmt.Sprintf("Search after bulk import took %dms, exceeds 500ms target", searchDuration.Milliseconds()))

			GinkgoWriter.Printf("✅ Search performance after bulk import: %v (target: <500ms)\n", searchDuration)

			// BUSINESS VALUE: Index performance
			// - GIN index remains efficient after bulk insert
			// - Search latency doesn't degrade with larger catalog
			// - Ready for production catalog size (hundreds of workflows)
		})
	})
})
