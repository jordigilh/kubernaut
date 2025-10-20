package contextapi

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	"github.com/jordigilh/kubernaut/pkg/contextapi/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/query" // For Vector type
)

// InsertTestIncident inserts a test incident into the database
// BR-CONTEXT-001: Historical context query test data setup
func InsertTestIncident(db *sqlx.DB, incident *models.IncidentEvent) error {
	sqlQuery := `
		INSERT INTO remediation_audit (
			id, name, alert_fingerprint, remediation_request_id,
			namespace, cluster_name, environment, target_resource,
			phase, status, severity, action_type,
			start_time, end_time, duration,
			error_message, metadata, embedding,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8,
			$9, $10, $11, $12,
			$13, $14, $15,
			$16, $17, $18,
			$19, $20
		)
	`

	// Convert embedding to query.Vector for pgvector compatibility
	var vectorEmb query.Vector
	if incident.Embedding != nil {
		vectorEmb = query.Vector(incident.Embedding)
	}

	_, err := db.Exec(sqlQuery,
		incident.ID, incident.Name, incident.AlertFingerprint, incident.RemediationRequestID,
		incident.Namespace, incident.ClusterName, incident.Environment, incident.TargetResource,
		incident.Phase, incident.Status, incident.Severity, incident.ActionType,
		incident.StartTime, incident.EndTime, incident.Duration,
		incident.ErrorMessage, incident.Metadata, vectorEmb,
		incident.CreatedAt, incident.UpdatedAt,
	)

	return err
}

// CreateTestEmbedding generates a test embedding vector
// BR-CONTEXT-003: Semantic similarity search test data
func CreateTestEmbedding(dimensions int) []float32 {
	if dimensions <= 0 {
		dimensions = 1536 // Default OpenAI embedding size
	}

	embedding := make([]float32, dimensions)
	for i := range embedding {
		// Generate a random float between -1 and 1
		var b [4]byte
		rand.Read(b[:])
		embedding[i] = (float32(b[0])/255.0)*2.0 - 1.0
	}
	return embedding
}

// CreateSimilarEmbedding creates an embedding similar to a reference
// For testing semantic search similarity thresholds
func CreateSimilarEmbedding(reference []float32, similarity float32) []float32 {
	if similarity < 0 || similarity > 1 {
		similarity = 0.9 // Default high similarity
	}

	similar := make([]float32, len(reference))
	noise := 1.0 - similarity

	for i := range reference {
		// Add small noise while maintaining similarity
		var b [4]byte
		rand.Read(b[:])
		randomNoise := (float32(b[0])/255.0 - 0.5) * noise
		similar[i] = reference[i] + randomNoise
	}

	return similar
}

// WaitForCachePopulation waits for a cache key to be populated
// Anti-flaky pattern for async cache operations
func WaitForCachePopulation(ctx context.Context, cacheManager cache.CacheManager, key string, timeout time.Duration) {
	EventuallyWithOffset(1, func() bool {
		_, err := cacheManager.Get(ctx, key)
		return err == nil
	}, timeout, 100*time.Millisecond).Should(BeTrue(), fmt.Sprintf("Cache key %s should be populated", key))
}

// ClearTestCache clears all test cache keys
// Ensures clean state between tests
func ClearTestCache(ctx context.Context, cacheManager cache.CacheManager, prefix string) {
	// Note: Actual implementation would need cache.Del or similar
	// For now, this is a placeholder for cache cleanup
	// Real implementation will depend on CacheManager interface
}

// SetupTestData creates bulk test data for performance testing
// BR-CONTEXT-001: Historical context query with multiple incidents
func SetupTestData(db *sqlx.DB, count int) ([]*models.IncidentEvent, error) {
	incidents := make([]*models.IncidentEvent, count)

	namespaces := []string{"default", "kube-system", "monitoring", "app"}
	severities := []string{"critical", "high", "medium", "low"}
	actionTypes := []string{"restart", "scale", "patch", "rollback"}
	phases := []string{"detection", "analysis", "remediation", "verification"}

	now := time.Now()
	for i := 0; i < count; i++ {
		startTime := now.Add(-time.Duration(i) * time.Hour)
		status := "completed"
		var endTime *time.Time
		var duration *int64

		// 66% success rate
		if i%3 == 0 {
			status = "failed"
		} else {
			// Set end time for completed incidents
			et := startTime.Add(10 * time.Minute)
			endTime = &et
			dur := int64(10 * 60 * 1000) // 10 minutes in milliseconds
			duration = &dur
		}

		incident := &models.IncidentEvent{
			ID:                   int64(i + 1),
			Name:                 fmt.Sprintf("test-alert-%d", i),
			AlertFingerprint:     fmt.Sprintf("fp-%d", i),
			RemediationRequestID: fmt.Sprintf("rr-%d", i),
			Namespace:            namespaces[i%len(namespaces)],
			ClusterName:          fmt.Sprintf("cluster-%d", (i%3)+1),
			Environment:          "test",
			TargetResource:       fmt.Sprintf("pod/test-%d", i),
			Phase:                phases[i%len(phases)],
			Status:               status,
			Severity:             severities[i%len(severities)],
			ActionType:           actionTypes[i%len(actionTypes)],
			StartTime:            &startTime,
			EndTime:              endTime,
			Duration:             duration,
			ErrorMessage:         nil,
			Metadata:             fmt.Sprintf(`{"test_id": %d}`, i),
			Embedding:            CreateTestEmbedding(384), // 384 dimensions per validation
			CreatedAt:            startTime,
			UpdatedAt:            startTime,
		}

		if err := InsertTestIncident(db, incident); err != nil {
			return nil, fmt.Errorf("failed to insert test incident %d: %w", i, err)
		}

		incidents[i] = incident
	}

	return incidents, nil
}

// AssertLatency asserts that a duration is below a maximum threshold
// For performance testing
func AssertLatency(duration time.Duration, max time.Duration, operation string) {
	ExpectWithOffset(1, duration).To(BeNumerically("<", max),
		fmt.Sprintf("%s should complete in less than %v (actual: %v)", operation, max, duration))
}

// GenerateCacheKey generates a deterministic cache key for query parameters
// Must match the logic in pkg/contextapi/query/executor.go
func GenerateCacheKey(params *models.ListIncidentsParams) string {
	namespace := "all"
	if params.Namespace != nil {
		namespace = *params.Namespace
	}

	clusterName := "all"
	if params.ClusterName != nil {
		clusterName = *params.ClusterName
	}

	return fmt.Sprintf("contextapi:incidents:%s:%s:%d:%d",
		namespace, clusterName, params.Limit, params.Offset)
}

// WaitForConditionWithDeadline waits for a condition with a context deadline
// Returns true if condition met, false if timeout
func WaitForConditionWithDeadline(ctx context.Context, condition func() bool, interval time.Duration) bool {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if condition() {
			return true
		}

		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			continue
		}
	}
}

// CreateIncidentWithEmbedding creates a complete test incident with embedding
func CreateIncidentWithEmbedding(id int64, namespace string) *models.IncidentEvent {
	now := time.Now()
	return &models.IncidentEvent{
		ID:                   id,
		Name:                 fmt.Sprintf("test-alert-%d", id),
		AlertFingerprint:     fmt.Sprintf("fp-%d", id),
		RemediationRequestID: fmt.Sprintf("rr-%d", id),
		Namespace:            namespace,
		ClusterName:          "test-cluster",
		Environment:          "test",
		TargetResource:       fmt.Sprintf("pod/test-%d", id),
		Phase:                "completed",
		Status:               "completed",
		Severity:             "high",
		ActionType:           "restart",
		StartTime:            &now,
		EndTime:              nil,
		Duration:             nil,
		ErrorMessage:         nil,
		Metadata:             fmt.Sprintf(`{"test_id": %d}`, id),
		Embedding:            CreateTestEmbedding(384), // 384 dimensions per validation
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

// WaitForAsyncOperation waits for an async operation to complete
// Generic helper for various async scenarios
func WaitForAsyncOperation(operation func() error, timeout time.Duration, description string) {
	EventuallyWithOffset(1, operation, timeout, 100*time.Millisecond).
		Should(Succeed(), fmt.Sprintf("%s should complete successfully", description))
}
