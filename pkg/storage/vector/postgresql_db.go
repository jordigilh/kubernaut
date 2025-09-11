package vector

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// PostgreSQLVectorDatabase implements VectorDatabase interface using PostgreSQL with pgvector extension
type PostgreSQLVectorDatabase struct {
	db               *sql.DB
	embeddingService EmbeddingGenerator
	log              *logrus.Logger
}

// NewPostgreSQLVectorDatabase creates a new PostgreSQL vector database instance
func NewPostgreSQLVectorDatabase(db *sql.DB, embeddingService EmbeddingGenerator, log *logrus.Logger) *PostgreSQLVectorDatabase {
	return &PostgreSQLVectorDatabase{
		db:               db,
		embeddingService: embeddingService,
		log:              log,
	}
}

// StoreActionPattern stores an action pattern as a vector in PostgreSQL
func (db *PostgreSQLVectorDatabase) StoreActionPattern(ctx context.Context, pattern *ActionPattern) error {
	if pattern.ID == "" {
		return fmt.Errorf("pattern ID cannot be empty")
	}

	// Generate embedding if not provided
	if len(pattern.Embedding) == 0 && db.embeddingService != nil {
		embedding, err := db.generatePatternEmbedding(ctx, pattern)
		if err != nil {
			db.log.WithError(err).Warn("Failed to generate embedding for pattern")
			return fmt.Errorf("failed to generate embedding: %w", err)
		}
		pattern.Embedding = embedding
	}

	if len(pattern.Embedding) == 0 {
		return fmt.Errorf("pattern embedding cannot be empty")
	}

	// Serialize complex fields to JSON
	actionParamsJSON, err := json.Marshal(pattern.ActionParameters)
	if err != nil {
		return fmt.Errorf("failed to marshal action parameters: %w", err)
	}

	contextLabelsJSON, err := json.Marshal(pattern.ContextLabels)
	if err != nil {
		return fmt.Errorf("failed to marshal context labels: %w", err)
	}

	effectivenessJSON, err := json.Marshal(pattern.EffectivenessData)
	if err != nil {
		return fmt.Errorf("failed to marshal effectiveness data: %w", err)
	}

	metadataJSON, err := json.Marshal(pattern.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Convert embedding to PostgreSQL vector format
	embeddingStr := db.embeddingToString(pattern.Embedding)

	// Upsert pattern (insert or update if exists)
	query := `
		INSERT INTO action_patterns (
			id, action_type, alert_name, alert_severity, namespace, resource_type, resource_name,
			action_parameters, context_labels, pre_conditions, post_conditions, effectiveness_data,
			embedding, metadata, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::vector, $14, $15, $16)
		ON CONFLICT (id) DO UPDATE SET
			action_type = EXCLUDED.action_type,
			alert_name = EXCLUDED.alert_name,
			alert_severity = EXCLUDED.alert_severity,
			namespace = EXCLUDED.namespace,
			resource_type = EXCLUDED.resource_type,
			resource_name = EXCLUDED.resource_name,
			action_parameters = EXCLUDED.action_parameters,
			context_labels = EXCLUDED.context_labels,
			pre_conditions = EXCLUDED.pre_conditions,
			post_conditions = EXCLUDED.post_conditions,
			effectiveness_data = EXCLUDED.effectiveness_data,
			embedding = EXCLUDED.embedding,
			metadata = EXCLUDED.metadata,
			updated_at = EXCLUDED.updated_at
	`

	preConditionsJSON, _ := json.Marshal(pattern.PreConditions)
	postConditionsJSON, _ := json.Marshal(pattern.PostConditions)

	now := time.Now()
	createdAt := pattern.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	_, err = db.db.ExecContext(ctx, query,
		pattern.ID, pattern.ActionType, pattern.AlertName, pattern.AlertSeverity,
		pattern.Namespace, pattern.ResourceType, pattern.ResourceName,
		actionParamsJSON, contextLabelsJSON, preConditionsJSON, postConditionsJSON,
		effectivenessJSON, embeddingStr, metadataJSON, createdAt, now)

	if err != nil {
		db.log.WithError(err).WithField("pattern_id", pattern.ID).Error("Failed to store action pattern")
		return fmt.Errorf("failed to store pattern: %w", err)
	}

	db.log.WithFields(logrus.Fields{
		"pattern_id":    pattern.ID,
		"action_type":   pattern.ActionType,
		"alert_name":    pattern.AlertName,
		"embedding_dim": len(pattern.Embedding),
	}).Debug("Stored action pattern in PostgreSQL vector database")

	return nil
}

// FindSimilarPatterns finds patterns similar to the given one using vector similarity
func (db *PostgreSQLVectorDatabase) FindSimilarPatterns(ctx context.Context, pattern *ActionPattern, limit int, threshold float64) ([]*SimilarPattern, error) {
	if len(pattern.Embedding) == 0 && db.embeddingService != nil {
		embedding, err := db.generatePatternEmbedding(ctx, pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to generate embedding for search pattern: %w", err)
		}
		pattern.Embedding = embedding
	}

	if len(pattern.Embedding) == 0 {
		return nil, fmt.Errorf("pattern embedding cannot be empty for similarity search")
	}

	embeddingStr := db.embeddingToString(pattern.Embedding)

	// Use L2 distance for similarity search with pgvector
	query := `
		SELECT id, action_type, alert_name, alert_severity, namespace, resource_type, resource_name,
			   action_parameters, context_labels, pre_conditions, post_conditions, effectiveness_data,
			   embedding, metadata, created_at, updated_at,
			   embedding <-> $1::vector as distance
		FROM action_patterns
		WHERE embedding <-> $1::vector <= $2
		ORDER BY embedding <-> $1::vector
		LIMIT $3
	`

	rows, err := db.db.QueryContext(ctx, query, embeddingStr, threshold, limit)
	if err != nil {
		db.log.WithError(err).Error("Failed to execute similarity search")
		return nil, fmt.Errorf("failed to execute similarity search: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			db.log.WithError(err).Error("Failed to close database rows")
		}
	}()

	var similarPatterns []*SimilarPattern
	rank := 1

	for rows.Next() {
		var p ActionPattern
		var actionParamsJSON, contextLabelsJSON, preConditionsJSON, postConditionsJSON []byte
		var effectivenessJSON, metadataJSON []byte
		var embeddingStr string
		var distance float64

		err := rows.Scan(
			&p.ID, &p.ActionType, &p.AlertName, &p.AlertSeverity,
			&p.Namespace, &p.ResourceType, &p.ResourceName,
			&actionParamsJSON, &contextLabelsJSON, &preConditionsJSON, &postConditionsJSON,
			&effectivenessJSON, &embeddingStr, &metadataJSON,
			&p.CreatedAt, &p.UpdatedAt, &distance,
		)
		if err != nil {
			db.log.WithError(err).Warn("Failed to scan similarity search result")
			continue
		}

		// Deserialize JSON fields
		if err := json.Unmarshal(actionParamsJSON, &p.ActionParameters); err != nil {
			db.log.WithError(err).Warn("Failed to unmarshal action parameters")
		}
		if err := json.Unmarshal(contextLabelsJSON, &p.ContextLabels); err != nil {
			db.log.WithError(err).Warn("Failed to unmarshal context labels")
		}
		if err := json.Unmarshal(preConditionsJSON, &p.PreConditions); err != nil {
			db.log.WithError(err).Warn("Failed to unmarshal pre conditions")
		}
		if err := json.Unmarshal(postConditionsJSON, &p.PostConditions); err != nil {
			db.log.WithError(err).Warn("Failed to unmarshal post conditions")
		}
		if err := json.Unmarshal(effectivenessJSON, &p.EffectivenessData); err != nil {
			db.log.WithError(err).Warn("Failed to unmarshal effectiveness data")
		}
		if err := json.Unmarshal(metadataJSON, &p.Metadata); err != nil {
			db.log.WithError(err).Warn("Failed to unmarshal metadata")
		}

		// Convert embedding string back to float64 slice
		p.Embedding = db.stringToEmbedding(embeddingStr)

		// Convert distance to similarity (0 = identical, higher values = less similar)
		similarity := 1.0 / (1.0 + distance)

		similarPatterns = append(similarPatterns, &SimilarPattern{
			Pattern:    &p,
			Similarity: similarity,
			Rank:       rank,
		})
		rank++
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating similarity search results: %w", err)
	}

	db.log.WithFields(logrus.Fields{
		"found_patterns": len(similarPatterns),
		"limit":          limit,
		"threshold":      threshold,
	}).Debug("Found similar patterns")

	return similarPatterns, nil
}

// UpdatePatternEffectiveness updates the effectiveness score of a stored pattern
func (db *PostgreSQLVectorDatabase) UpdatePatternEffectiveness(ctx context.Context, patternID string, effectiveness float64) error {
	// First, get the current effectiveness data
	var effectivenessJSON []byte
	query := `SELECT effectiveness_data FROM action_patterns WHERE id = $1`

	err := db.db.QueryRowContext(ctx, query, patternID).Scan(&effectivenessJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("pattern with ID %s not found", patternID)
		}
		return fmt.Errorf("failed to get current effectiveness data: %w", err)
	}

	// Deserialize current effectiveness data
	var effectivenessData EffectivenessData
	if len(effectivenessJSON) > 0 {
		if err := json.Unmarshal(effectivenessJSON, &effectivenessData); err != nil {
			db.log.WithError(err).Warn("Failed to unmarshal existing effectiveness data, creating new")
		}
	}

	// Update the effectiveness score
	effectivenessData.Score = effectiveness
	effectivenessData.LastAssessed = time.Now()

	// Serialize updated effectiveness data
	updatedJSON, err := json.Marshal(effectivenessData)
	if err != nil {
		return fmt.Errorf("failed to marshal updated effectiveness data: %w", err)
	}

	// Update the pattern
	updateQuery := `UPDATE action_patterns SET effectiveness_data = $1, updated_at = $2 WHERE id = $3`

	_, err = db.db.ExecContext(ctx, updateQuery, updatedJSON, time.Now(), patternID)
	if err != nil {
		return fmt.Errorf("failed to update pattern effectiveness: %w", err)
	}

	db.log.WithFields(logrus.Fields{
		"pattern_id":    patternID,
		"effectiveness": effectiveness,
	}).Debug("Updated pattern effectiveness")

	return nil
}

// SearchBySemantics performs semantic search for patterns
func (db *PostgreSQLVectorDatabase) SearchBySemantics(ctx context.Context, query string, limit int) ([]*ActionPattern, error) {
	if db.embeddingService == nil {
		return nil, fmt.Errorf("embedding service not available for semantic search")
	}

	// Generate embedding for the search query
	queryEmbedding, err := db.embeddingService.GenerateTextEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding for search query: %w", err)
	}

	embeddingStr := db.embeddingToString(queryEmbedding)

	// Semantic search using vector similarity
	sqlQuery := `
		SELECT id, action_type, alert_name, alert_severity, namespace, resource_type, resource_name,
			   action_parameters, context_labels, pre_conditions, post_conditions, effectiveness_data,
			   embedding, metadata, created_at, updated_at
		FROM action_patterns
		ORDER BY embedding <-> $1::vector
		LIMIT $2
	`

	rows, err := db.db.QueryContext(ctx, sqlQuery, embeddingStr, limit)
	if err != nil {
		db.log.WithError(err).Error("Failed to execute semantic search")
		return nil, fmt.Errorf("failed to execute semantic search: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			db.log.WithError(err).Error("Failed to close database rows")
		}
	}()

	var patterns []*ActionPattern

	for rows.Next() {
		var p ActionPattern
		var actionParamsJSON, contextLabelsJSON, preConditionsJSON, postConditionsJSON []byte
		var effectivenessJSON, metadataJSON []byte
		var embeddingStr string

		err := rows.Scan(
			&p.ID, &p.ActionType, &p.AlertName, &p.AlertSeverity,
			&p.Namespace, &p.ResourceType, &p.ResourceName,
			&actionParamsJSON, &contextLabelsJSON, &preConditionsJSON, &postConditionsJSON,
			&effectivenessJSON, &embeddingStr, &metadataJSON,
			&p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			db.log.WithError(err).Warn("Failed to scan semantic search result")
			continue
		}

		// Deserialize JSON fields
		if err := json.Unmarshal(actionParamsJSON, &p.ActionParameters); err != nil {
			db.log.WithError(err).Warn("Failed to unmarshal action parameters")
		}
		if err := json.Unmarshal(contextLabelsJSON, &p.ContextLabels); err != nil {
			db.log.WithError(err).Warn("Failed to unmarshal context labels")
		}
		if err := json.Unmarshal(preConditionsJSON, &p.PreConditions); err != nil {
			db.log.WithError(err).Warn("Failed to unmarshal pre conditions")
		}
		if err := json.Unmarshal(postConditionsJSON, &p.PostConditions); err != nil {
			db.log.WithError(err).Warn("Failed to unmarshal post conditions")
		}
		if err := json.Unmarshal(effectivenessJSON, &p.EffectivenessData); err != nil {
			db.log.WithError(err).Warn("Failed to unmarshal effectiveness data")
		}
		if err := json.Unmarshal(metadataJSON, &p.Metadata); err != nil {
			db.log.WithError(err).Warn("Failed to unmarshal metadata")
		}

		// Convert embedding string back to float64 slice
		p.Embedding = db.stringToEmbedding(embeddingStr)

		patterns = append(patterns, &p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating semantic search results: %w", err)
	}

	db.log.WithFields(logrus.Fields{
		"query":          query,
		"found_patterns": len(patterns),
		"limit":          limit,
	}).Debug("Performed semantic search")

	return patterns, nil
}

// DeletePattern removes a pattern from the vector database
func (db *PostgreSQLVectorDatabase) DeletePattern(ctx context.Context, patternID string) error {
	query := `DELETE FROM action_patterns WHERE id = $1`

	result, err := db.db.ExecContext(ctx, query, patternID)
	if err != nil {
		return fmt.Errorf("failed to delete pattern: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("pattern with ID %s not found", patternID)
	}

	db.log.WithField("pattern_id", patternID).Debug("Deleted pattern from PostgreSQL vector database")

	return nil
}

// GetPatternAnalytics returns analytics about stored patterns
func (db *PostgreSQLVectorDatabase) GetPatternAnalytics(ctx context.Context) (*PatternAnalytics, error) {
	analytics := &PatternAnalytics{
		PatternsByActionType:      make(map[string]int),
		PatternsBySeverity:        make(map[string]int),
		EffectivenessDistribution: make(map[string]int),
		GeneratedAt:               time.Now(),
	}

	// Get total patterns count
	err := db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM action_patterns`).Scan(&analytics.TotalPatterns)
	if err != nil {
		return nil, fmt.Errorf("failed to get total patterns count: %w", err)
	}

	// Get patterns by action type
	actionTypeQuery := `SELECT action_type, COUNT(*) FROM action_patterns GROUP BY action_type`
	rows, err := db.db.QueryContext(ctx, actionTypeQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get patterns by action type: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			db.log.WithError(err).Error("Failed to close database rows")
		}
	}()

	for rows.Next() {
		var actionType string
		var count int
		if err := rows.Scan(&actionType, &count); err == nil {
			analytics.PatternsByActionType[actionType] = count
		}
	}

	// Get patterns by severity
	severityQuery := `SELECT alert_severity, COUNT(*) FROM action_patterns GROUP BY alert_severity`
	rows, err = db.db.QueryContext(ctx, severityQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get patterns by severity: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			db.log.WithError(err).Error("Failed to close database rows")
		}
	}()

	for rows.Next() {
		var severity string
		var count int
		if err := rows.Scan(&severity, &count); err == nil {
			analytics.PatternsBySeverity[severity] = count
		}
	}

	// Get average effectiveness
	var avgEffectiveness sql.NullFloat64
	effectivenessQuery := `
		SELECT AVG((effectiveness_data->>'score')::float)
		FROM action_patterns
		WHERE effectiveness_data->>'score' IS NOT NULL
	`
	err = db.db.QueryRowContext(ctx, effectivenessQuery).Scan(&avgEffectiveness)
	if err == nil && avgEffectiveness.Valid {
		analytics.AverageEffectiveness = avgEffectiveness.Float64
	}

	// Get top performing patterns (limit to 10)
	topPatternsQuery := `
		SELECT id, action_type, alert_name, alert_severity, effectiveness_data
		FROM action_patterns
		WHERE effectiveness_data->>'score' IS NOT NULL
		ORDER BY (effectiveness_data->>'score')::float DESC
		LIMIT 10
	`
	rows, err = db.db.QueryContext(ctx, topPatternsQuery)
	if err == nil {
		defer func() {
			if err := rows.Close(); err != nil {
				db.log.WithError(err).Error("Failed to close database rows")
			}
		}()
		for rows.Next() {
			var p ActionPattern
			var effectivenessJSON []byte
			err := rows.Scan(&p.ID, &p.ActionType, &p.AlertName, &p.AlertSeverity, &effectivenessJSON)
			if err == nil {
				if err := json.Unmarshal(effectivenessJSON, &p.EffectivenessData); err == nil {
					analytics.TopPerformingPatterns = append(analytics.TopPerformingPatterns, &p)
				}
			}
		}
	}

	// Get recent patterns (limit to 10)
	recentPatternsQuery := `
		SELECT id, action_type, alert_name, alert_severity, created_at
		FROM action_patterns
		ORDER BY created_at DESC
		LIMIT 10
	`
	rows, err = db.db.QueryContext(ctx, recentPatternsQuery)
	if err == nil {
		defer func() {
			if err := rows.Close(); err != nil {
				db.log.WithError(err).Error("Failed to close database rows")
			}
		}()
		for rows.Next() {
			var p ActionPattern
			err := rows.Scan(&p.ID, &p.ActionType, &p.AlertName, &p.AlertSeverity, &p.CreatedAt)
			if err == nil {
				analytics.RecentPatterns = append(analytics.RecentPatterns, &p)
			}
		}
	}

	db.log.WithFields(logrus.Fields{
		"total_patterns":        analytics.TotalPatterns,
		"average_effectiveness": analytics.AverageEffectiveness,
		"action_types":          len(analytics.PatternsByActionType),
	}).Debug("Generated pattern analytics")

	return analytics, nil
}

// IsHealthy performs a health check on the database
func (db *PostgreSQLVectorDatabase) IsHealthy(ctx context.Context) error {
	// Check database connection
	if err := db.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}

	// Check if pgvector extension is available
	var extensionExists bool
	query := `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'vector')`
	err := db.db.QueryRowContext(ctx, query).Scan(&extensionExists)
	if err != nil {
		return fmt.Errorf("failed to check pgvector extension: %w", err)
	}

	if !extensionExists {
		return fmt.Errorf("pgvector extension is not installed")
	}

	// Check if action_patterns table exists
	var tableExists bool
	tableQuery := `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'action_patterns')`
	err = db.db.QueryRowContext(ctx, tableQuery).Scan(&tableExists)
	if err != nil {
		return fmt.Errorf("failed to check action_patterns table: %w", err)
	}

	if !tableExists {
		return fmt.Errorf("action_patterns table does not exist")
	}

	return nil
}

// Helper methods

func (db *PostgreSQLVectorDatabase) generatePatternEmbedding(ctx context.Context, pattern *ActionPattern) ([]float64, error) {
	// Create a text representation of the pattern for embedding generation
	text := fmt.Sprintf("%s %s %s %s %s",
		pattern.ActionType,
		pattern.AlertName,
		pattern.AlertSeverity,
		pattern.ResourceType,
		pattern.Namespace,
	)

	return db.embeddingService.GenerateTextEmbedding(ctx, text)
}

func (db *PostgreSQLVectorDatabase) embeddingToString(embedding []float64) string {
	strParts := make([]string, len(embedding))
	for i, val := range embedding {
		strParts[i] = fmt.Sprintf("%f", val)
	}
	return "[" + strings.Join(strParts, ",") + "]"
}

func (db *PostgreSQLVectorDatabase) stringToEmbedding(s string) []float64 {
	// Remove brackets and split by comma
	s = strings.Trim(s, "[]")
	if s == "" {
		return nil
	}

	parts := strings.Split(s, ",")
	embedding := make([]float64, 0, len(parts))

	for _, part := range parts {
		var val float64
		if _, err := fmt.Sscanf(strings.TrimSpace(part), "%f", &val); err == nil {
			embedding = append(embedding, val)
		}
	}

	return embedding
}
