package datastorage

import (
	"context"
	"database/sql"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/database/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
)

var _ = Describe("Integration Test 3: Embedding Pipeline Integration", func() {
	var (
		testCtx     context.Context
		testDB      *sql.DB
		testSchema  string
		initializer *schema.Initializer
	)

	BeforeEach(func() {
		testCtx = context.Background()

		// Create isolated test schema
		testSchema = "test_embedding_" + time.Now().Format("20060102_150405")
		_, err := db.ExecContext(testCtx, "CREATE SCHEMA "+testSchema)
		Expect(err).ToNot(HaveOccurred())

		// Connect to test schema
		connStr := "host=localhost port=5432 user=postgres password=postgres dbname=postgres sslmode=disable search_path=" + testSchema
		testDB, err = sql.Open("postgres", connStr)
		Expect(err).ToNot(HaveOccurred())

		// Initialize schema (includes pgvector extension)
		initializer = schema.NewInitializer(testDB, logger)
		err = initializer.Initialize(testCtx)
		Expect(err).ToNot(HaveOccurred())

		GinkgoWriter.Printf("✅ Test schema %s initialized for embedding tests\n", testSchema)
	})

	AfterEach(func() {
		if testDB != nil {
			_ = testDB.Close()
		}
		_, _ = db.ExecContext(testCtx, "DROP SCHEMA IF EXISTS "+testSchema+" CASCADE")
		GinkgoWriter.Printf("✅ Test schema %s cleaned up\n", testSchema)
	})

	Context("BR-STORAGE-011: Embedding generation and storage", func() {
		It("should store vector embeddings in PostgreSQL", func() {
			// Create mock embedding (real AI embedding API tested in unit tests)
			embedding := make([]float32, 384) // 384-dimensional vector
			for i := range embedding {
				embedding[i] = float32(i) / 384.0
			}

		// Insert audit with embedding
		sqlQuery := `
			INSERT INTO remediation_audit (
				name, namespace, phase, action_type, status, start_time,
				remediation_request_id, alert_fingerprint, severity,
				environment, cluster_name, target_resource, metadata, embedding
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
			) RETURNING id
		`

	// Convert to query.Vector for pgvector compatibility
	vectorEmb := query.Vector(embedding)

	var id int64
	err := testDB.QueryRowContext(testCtx, sqlQuery,
			"embedding-test", "default", "processing", "restart_pod",
			"pending", time.Now(), "req-emb-001", "alert-emb",
			"high", "production", "prod-cluster", "pod/app", "{}",
			vectorEmb,
		).Scan(&id)

			Expect(err).ToNot(HaveOccurred())
			Expect(id).To(BeNumerically(">", 0))

		// Read embedding back using query.Vector for scanning
		var storedVector query.Vector
		err = testDB.QueryRowContext(testCtx, "SELECT embedding FROM remediation_audit WHERE id = $1", id).Scan(&storedVector)
		Expect(err).ToNot(HaveOccurred())
		Expect(storedVector).To(HaveLen(384))
		Expect(storedVector[0]).To(BeNumerically("~", embedding[0], 0.0001))
		Expect(storedVector[383]).To(BeNumerically("~", embedding[383], 0.0001))

			GinkgoWriter.Printf("✅ Vector embedding stored and retrieved (384 dimensions)\n")
		})

		It("should support NULL embeddings (for optional vector search)", func() {
			// Insert audit without embedding
			query := `
				INSERT INTO remediation_audit (
					name, namespace, phase, action_type, status, start_time,
					remediation_request_id, alert_fingerprint, severity,
					environment, cluster_name, target_resource, metadata
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
				) RETURNING id
			`

			var id int64
			err := testDB.QueryRowContext(testCtx, query,
				"no-embedding-test", "default", "processing", "restart_pod",
				"pending", time.Now(), "req-no-emb-001", "alert-no-emb",
				"high", "production", "prod-cluster", "pod/app", "{}",
			).Scan(&id)

			Expect(err).ToNot(HaveOccurred())

			// Verify embedding is NULL
			var embedding sql.NullString
			err = testDB.QueryRowContext(testCtx, "SELECT embedding FROM remediation_audit WHERE id = $1", id).Scan(&embedding)
			Expect(err).ToNot(HaveOccurred())
			Expect(embedding.Valid).To(BeFalse(), "Embedding should be NULL")

			GinkgoWriter.Println("✅ NULL embeddings supported")
		})

	It("should enforce vector dimension (384)", func() {
		// Attempt to insert embedding with wrong dimension
		wrongSizeEmbedding := []float32{0.1, 0.2, 0.3} // Only 3 dimensions instead of 384
		wrongVector := query.Vector(wrongSizeEmbedding)

		sqlQuery := `
			INSERT INTO remediation_audit (
				name, namespace, phase, action_type, status, start_time,
				remediation_request_id, alert_fingerprint, severity,
				environment, cluster_name, target_resource, metadata, embedding
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
			)
		`

		_, err := testDB.ExecContext(testCtx, sqlQuery,
				"wrong-dim-test", "default", "processing", "restart_pod",
				"pending", time.Now(), "req-wrong-dim-001", "alert-wrong-dim",
				"high", "production", "prod-cluster", "pod/app", "{}",
				wrongVector,
			)

			// Should fail with dimension mismatch error
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected 384 dimensions"))

			GinkgoWriter.Println("✅ Vector dimension constraint enforced (384)")
		})

	It("should verify HNSW index exists for vector search", func() {
		// Check if HNSW index exists on the embedding column
		// Use current_schema() to query the active test schema
		indexQuery := `
			SELECT i.relname AS indexname
			FROM pg_class t
			JOIN pg_index ix ON t.oid = ix.indrelid
			JOIN pg_class i ON i.oid = ix.indexrelid
			WHERE t.relname = 'remediation_audit'
			  AND t.relnamespace = (SELECT oid FROM pg_namespace WHERE nspname = current_schema())
			  AND i.relname LIKE '%embedding%'
		`

		rows, err := testDB.QueryContext(testCtx, indexQuery)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = rows.Close() }()

		hnswIndexes := []string{}
		for rows.Next() {
			var indexName string
			err := rows.Scan(&indexName)
			Expect(err).ToNot(HaveOccurred())
			hnswIndexes = append(hnswIndexes, indexName)
			GinkgoWriter.Printf("   Embedding Index: %s\n", indexName)
		}

		Expect(hnswIndexes).ToNot(BeEmpty(), "At least one embedding index should exist")

		GinkgoWriter.Println("✅ Embedding index exists for efficient vector search")
	})
	})

	Context("BR-STORAGE-009: Embedding cache integration", func() {
		It("should demonstrate cache mechanism (mock for integration test)", func() {
			// Note: Real Redis cache tested in unit tests
			// Here we verify the embedding interface contract

			// Create mock cache
			cache := &MockCache{Data: make(map[string][]float32)}

			// Store embedding
			testEmbedding := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
			err := cache.Set(testCtx, "test-key", testEmbedding, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred())

			// Retrieve embedding
			cachedEmbedding, err := cache.Get(testCtx, "test-key")
			Expect(err).ToNot(HaveOccurred())
			Expect(cachedEmbedding).To(Equal(testEmbedding))

			// Cache miss
			missingEmbedding, err := cache.Get(testCtx, "non-existent-key")
			Expect(err).ToNot(HaveOccurred())
			Expect(missingEmbedding).To(BeNil())

			GinkgoWriter.Println("✅ Embedding cache contract validated")
		})
	})
})

// MockCache implements embedding.Cache for integration tests
type MockCache struct {
	Data map[string][]float32
}

func (m *MockCache) Get(ctx context.Context, key string) ([]float32, error) {
	if embedding, exists := m.Data[key]; exists {
		return embedding, nil
	}
	return nil, nil // Cache miss
}

func (m *MockCache) Set(ctx context.Context, key string, embedding []float32, ttl time.Duration) error {
	m.Data[key] = embedding
	return nil
}
