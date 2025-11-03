package datastorage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"go.uber.org/zap"
)

var (
	db         *sql.DB
	srv        *server.Server
	baseURL    string
	testLogger *zap.Logger
)

var _ = BeforeSuite(func() {
	var err error
	testLogger, err = zap.NewDevelopment()
	Expect(err).ToNot(HaveOccurred())

	// Connect to PostgreSQL (assuming datastorage-postgres container is running)
	connStr := "host=localhost port=5432 user=db_user password=test dbname=action_history sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	Expect(err).ToNot(HaveOccurred())

	// Verify connection
	err = db.Ping()
	Expect(err).ToNot(HaveOccurred(), "PostgreSQL must be running: podman run -d --name datastorage-postgres -p 5432:5432 -e POSTGRESQL_USER=db_user -e POSTGRESQL_PASSWORD=test -e POSTGRESQL_DATABASE=action_history registry.redhat.io/rhel9/postgresql-16:latest")

	// Create server with real database
	srv, err = server.NewServer(connStr, testLogger, &server.Config{
		Port:         18080, // Use different port for integration tests
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	})
	Expect(err).ToNot(HaveOccurred())

	// Start server in background
	go func() {
		_ = srv.Start() // Will error when stopped, ignore
	}()

	// Wait for server to be ready
	time.Sleep(1 * time.Second)
	baseURL = "http://localhost:18080"

	// Verify server is ready
	Eventually(func() int {
		resp, err := http.Get(baseURL + "/health/ready")
		if err != nil {
			return 0
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}, "10s", "500ms").Should(Equal(http.StatusOK), "Server should be ready")

	// BR-STORAGE-027: Populate test data for performance tests (10,000+ records)
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM resource_action_traces WHERE alert_name LIKE 'test-perf-%'").Scan(&count)
	Expect(err).ToNot(HaveOccurred())

	if count < 10000 {
		GinkgoWriter.Printf("Populating performance test data (%d existing, need 10000)...\n", count)

		// Insert in batches for performance
		batchSize := 1000
		recordsNeeded := 10000 - count

		for batch := 0; batch < (recordsNeeded/batchSize)+1; batch++ {
			recordsInBatch := batchSize
			if batch == recordsNeeded/batchSize {
				recordsInBatch = recordsNeeded % batchSize
			}
			if recordsInBatch == 0 {
				break
			}

			// Build batch insert
			for i := 0; i < recordsInBatch; i++ {
				recordNum := count + (batch * batchSize) + i
				// Keep timestamps within current month to avoid partition issues
				// Spread over last 12 hours only
				hoursAgo := recordNum % 12
				_, err := db.Exec(`
					INSERT INTO resource_action_traces
					(action_history_id, action_id, alert_name, alert_severity, action_type, action_timestamp, model_used, model_confidence, execution_status)
					VALUES (1, gen_random_uuid()::text, $1, $2, $3, NOW() - INTERVAL '1 hour' * $4, 'test-model', 0.9, 'completed')
				`,
					fmt.Sprintf("test-perf-%d", recordNum),
					[]string{"critical", "high", "medium", "low"}[recordNum%4],
					[]string{"scale", "restart", "check", "alert"}[recordNum%4],
					hoursAgo,
				)
				Expect(err).ToNot(HaveOccurred())
			}

			if (batch+1)%5 == 0 {
				GinkgoWriter.Printf("  ... inserted %d/%d records\n", count+(batch+1)*batchSize, 10000)
			}
		}

		// Verify final count
		err = db.QueryRow("SELECT COUNT(*) FROM resource_action_traces WHERE alert_name LIKE 'test-perf-%'").Scan(&count)
		Expect(err).ToNot(HaveOccurred())
		GinkgoWriter.Printf("Performance test data ready: %d records\n", count)
		Expect(count).To(BeNumerically(">=", 10000), "Should have at least 10000 test records")
	} else {
		GinkgoWriter.Printf("Performance test data already exists: %d records\n", count)
	}
})

var _ = AfterSuite(func() {
	// Cleanup performance test data
	if db != nil {
		GinkgoWriter.Println("Cleaning up performance test data...")
		_, err := db.Exec("DELETE FROM resource_action_traces WHERE alert_name LIKE 'test-perf-%'")
		if err != nil {
			GinkgoWriter.Printf("Warning: Failed to cleanup performance test data: %v\n", err)
		} else {
			var remaining int
			_ = db.QueryRow("SELECT COUNT(*) FROM resource_action_traces WHERE alert_name LIKE 'test-perf-%'").Scan(&remaining)
			GinkgoWriter.Printf("Performance test data cleaned up (%d records remaining)\n", remaining)
		}
	}

	if srv != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = srv.Shutdown(shutdownCtx)
	}
	if db != nil {
		_ = db.Close()
	}
	if testLogger != nil {
		_ = testLogger.Sync()
	}
})

var _ = Describe("BR-DS-001: List Incidents with Filters", func() {
	BeforeEach(func() {
		// Clear existing test data
		_, err := db.Exec("DELETE FROM resource_action_traces WHERE alert_name LIKE 'test-integration-%'")
		Expect(err).ToNot(HaveOccurred())

		// Insert test data
		testData := []struct {
			alert_name     string
			alert_severity string
			action_type    string
		}{
			{"test-integration-prod-cpu-high", "critical", "scale"},
			{"test-integration-prod-memory-high", "high", "restart"},
			{"test-integration-staging-cpu-high", "critical", "scale"},
			{"test-integration-dev-disk-space", "low", "check"},
		}

		for _, td := range testData {
			_, err := db.Exec(`
					INSERT INTO resource_action_traces
					(action_history_id, action_id, alert_name, alert_severity, action_type, action_timestamp, model_used, model_confidence, execution_status)
					VALUES (1, gen_random_uuid()::text, $1, $2, $3, NOW(), 'test-model', 0.9, 'completed')
				`, td.alert_name, td.alert_severity, td.action_type)
			Expect(err).ToNot(HaveOccurred())
		}
	})

	It("should return filtered incidents by alert_name pattern", func() {
		resp, err := http.Get(baseURL + "/api/v1/incidents?alert_name=test-integration-prod-cpu-high")
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		Expect(err).ToNot(HaveOccurred())

		data, ok := response["data"].([]interface{})
		Expect(ok).To(BeTrue(), "Response should have 'data' field with array")
		Expect(data).To(HaveLen(1))

		incident := data[0].(map[string]interface{})
		Expect(incident["alert_name"]).To(Equal("test-integration-prod-cpu-high"))
	})

	It("should return filtered incidents by severity", func() {
		// Note: Also filter by alert_name to isolate from performance test data
		resp, err := http.Get(baseURL + "/api/v1/incidents?alert_name=test-integration-prod-cpu-high&severity=critical")
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		Expect(err).ToNot(HaveOccurred())

		data, ok := response["data"].([]interface{})
		Expect(ok).To(BeTrue(), "Response should have 'data' field with array")
		Expect(data).To(HaveLen(1))

		for _, item := range data {
			incident := item.(map[string]interface{})
			Expect(incident["alert_severity"]).To(Equal("critical"))
			Expect(incident["alert_name"]).To(Equal("test-integration-prod-cpu-high"))
		}
	})

	It("should return filtered incidents by action_type", func() {
		// Note: Also filter by alert_name pattern to isolate from performance test data
		resp, err := http.Get(baseURL + "/api/v1/incidents?alert_name=test-integration-prod-cpu-high&action_type=scale")
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		Expect(err).ToNot(HaveOccurred())

		data, ok := response["data"].([]interface{})
		Expect(ok).To(BeTrue(), "Response should have 'data' field with array")
		Expect(data).To(HaveLen(1))

		for _, item := range data {
			incident := item.(map[string]interface{})
			Expect(incident["action_type"]).To(Equal("scale"))
			Expect(incident["alert_name"]).To(Equal("test-integration-prod-cpu-high"))
		}
	})

	It("should return empty array for nonexistent alert_name", func() {
		resp, err := http.Get(baseURL + "/api/v1/incidents?alert_name=nonexistent-alert")
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		Expect(err).ToNot(HaveOccurred())

		data, ok := response["data"].([]interface{})
		Expect(ok).To(BeTrue(), "Response should have 'data' field with array")
		Expect(data).To(HaveLen(0)) // Empty array, not null
	})
})

var _ = Describe("BR-DS-002: Get Incident by ID", func() {
	var testID int

	BeforeEach(func() {
		// Clear existing test data
		_, err := db.Exec("DELETE FROM resource_action_traces WHERE alert_name = 'test-integration-getbyid'")
		Expect(err).ToNot(HaveOccurred())

		// Insert test incident
		err = db.QueryRow(`
			INSERT INTO resource_action_traces
			(action_history_id, action_id, alert_name, alert_severity, action_type, action_timestamp, model_used, model_confidence, execution_status)
			VALUES (1, gen_random_uuid()::text, 'test-integration-getbyid', 'critical', 'scale', NOW(), 'test-model', 0.9, 'completed')
			RETURNING id
		`).Scan(&testID)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return incident by ID", func() {
		resp, err := http.Get(fmt.Sprintf("%s/api/v1/incidents/%d", baseURL, testID))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		Expect(err).ToNot(HaveOccurred())

		// Verify ID matches (could be float64 or int depending on JSON unmarshaling)
		Expect(result["id"]).To(Or(
			Equal(float64(testID)), // JSON numbers unmarshal as float64
			Equal(testID),
		))
		Expect(result["alert_name"]).To(Equal("test-integration-getbyid"))
		Expect(result["alert_severity"]).To(Equal("critical"))
		Expect(result["action_type"]).To(Equal("scale"))
	})

	It("should return 404 for nonexistent ID", func() {
		resp, err := http.Get(fmt.Sprintf("%s/api/v1/incidents/999999", baseURL))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		Expect(err).ToNot(HaveOccurred())

		// Verify RFC 7807 error response
		Expect(result["type"]).ToNot(BeEmpty())
		Expect(result["title"]).To(Equal("Incident Not Found"))
		Expect(result["status"]).To(Equal(float64(404)))
	})
})

var _ = Describe("BR-DS-007: Pagination", func() {
	BeforeEach(func() {
		// Clear existing test data
		_, err := db.Exec("DELETE FROM resource_action_traces WHERE alert_name = 'test-integration-pagination'")
		Expect(err).ToNot(HaveOccurred())

		// Insert 25 test incidents
		for i := 0; i < 25; i++ {
			_, err := db.Exec(`
				INSERT INTO resource_action_traces
				(action_history_id, action_id, alert_name, alert_severity, action_type, action_timestamp, model_used, model_confidence, execution_status)
				VALUES (1, gen_random_uuid()::text, 'test-integration-pagination', 'high', 'scale', NOW(), 'test-model', 0.9, 'completed')
			`)
			Expect(err).ToNot(HaveOccurred())
		}
	})

	It("should respect limit parameter", func() {
		resp, err := http.Get(baseURL + "/api/v1/incidents?alert_name=test-integration-pagination&limit=10")
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		Expect(err).ToNot(HaveOccurred())

		data, ok := response["data"].([]interface{})
		Expect(ok).To(BeTrue(), "Response should have 'data' field with array")
		Expect(data).To(HaveLen(10))
	})

	It("should respect offset parameter", func() {
		// Get first page
		resp1, err := http.Get(baseURL + "/api/v1/incidents?alert_name=test-integration-pagination&limit=10&offset=0")
		Expect(err).ToNot(HaveOccurred())
		defer resp1.Body.Close()

		var response1 map[string]interface{}
		err = json.NewDecoder(resp1.Body).Decode(&response1)
		Expect(err).ToNot(HaveOccurred())

		page1, ok := response1["data"].([]interface{})
		Expect(ok).To(BeTrue(), "Response should have 'data' field with array")
		Expect(page1).To(HaveLen(10))

		// Get second page
		resp2, err := http.Get(baseURL + "/api/v1/incidents?alert_name=test-integration-pagination&limit=10&offset=10")
		Expect(err).ToNot(HaveOccurred())
		defer resp2.Body.Close()

		var response2 map[string]interface{}
		err = json.NewDecoder(resp2.Body).Decode(&response2)
		Expect(err).ToNot(HaveOccurred())

		page2, ok := response2["data"].([]interface{})
		Expect(ok).To(BeTrue(), "Response should have 'data' field with array")
		Expect(page2).To(HaveLen(10))

		// Verify pages don't overlap
		firstID := page1[0].(map[string]interface{})["id"]
		secondID := page2[0].(map[string]interface{})["id"]
		Expect(firstID).ToNot(Equal(secondID))
	})

	It("should apply default limit of 100", func() {
		// Insert 150 more records (total 175)
		for i := 0; i < 150; i++ {
			_, err := db.Exec(`
				INSERT INTO resource_action_traces
				(action_history_id, action_id, alert_name, alert_severity, action_type, action_timestamp, model_used, model_confidence, execution_status)
				VALUES (1, gen_random_uuid()::text, 'test-integration-pagination', 'high', 'scale', NOW(), 'test-model', 0.9, 'completed')
			`)
			Expect(err).ToNot(HaveOccurred())
		}

		resp, err := http.Get(baseURL + "/api/v1/incidents?alert_name=test-integration-pagination")
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		Expect(err).ToNot(HaveOccurred())

		data, ok := response["data"].([]interface{})
		Expect(ok).To(BeTrue(), "Response should have 'data' field with array")
		Expect(data).To(HaveLen(100)) // Default limit
	})

	// 🚨 CRITICAL TEST - This would have caught the pagination bug (handler.go:178)
	It("should return accurate total count in pagination metadata", func() {
		// Known dataset: 25 records from BeforeEach
		resp, err := http.Get(baseURL + "/api/v1/incidents?alert_name=test-integration-pagination&limit=10")
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		Expect(err).ToNot(HaveOccurred())

		// Verify pagination metadata exists
		pagination, ok := response["pagination"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "Response should have pagination metadata")

		// ⭐⭐ CRITICAL ASSERTION - This catches the len(array) bug
		Expect(pagination["total"]).To(Equal(float64(25)),
			"pagination.total MUST equal database count (25), not page size (10)")

		// Also verify page size is correct (existing behavior)
		data, ok := response["data"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(data).To(HaveLen(10), "page size should be 10 (limit parameter)")
	})
})

var _ = Describe("Health Endpoints", func() {
	It("should return 200 for /health", func() {
		resp, err := http.Get(baseURL + "/health")
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("should return 200 for /health/ready when not shutting down", func() {
		resp, err := http.Get(baseURL + "/health/ready")
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("should return 200 for /health/live", func() {
		resp, err := http.Get(baseURL + "/health/live")
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})
})
