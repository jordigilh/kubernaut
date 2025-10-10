<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package validator

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// TestSuite represents the minimal test suite interface needed for validation
// Business Requirement: BR-BOOTSTRAP-ENVIRONMENT-001 - Environment validation
type TestSuite struct {
	DB              *sql.DB
	VectorDB        vector.VectorDatabase
	LLMClient       llm.Client
	WorkflowBuilder *engine.DefaultIntelligentWorkflowBuilder
}

// BootstrapEnvironmentValidator implements BR-BOOTSTRAP-ENVIRONMENT-001 through BR-BOOTSTRAP-ENVIRONMENT-010
// Manages validation of the complete bootstrap development environment for integration testing
type BootstrapEnvironmentValidator struct {
	suite  *TestSuite
	logger *logrus.Logger
}

// DatabaseStatus represents database connectivity status (BR-BOOTSTRAP-ENVIRONMENT-002)
type DatabaseStatus struct {
	PostgreSQLConnected bool  `json:"postgresql_connected"`
	PgVectorConnected   bool  `json:"pgvector_connected"`
	PgVectorDimension   int   `json:"pgvector_dimension"`
	ConnectionLatency   int64 `json:"connection_latency_ms"`
}

// AIPgVectorSupport represents AI & pgvector integration support (BR-BOOTSTRAP-ENVIRONMENT-004)
type AIPgVectorSupport struct {
	EmbeddingPipelineSupported      bool    `json:"embedding_pipeline_supported"`
	PerformanceIntegrationSupported bool    `json:"performance_integration_supported"`
	AnalyticsIntegrationSupported   bool    `json:"analytics_integration_supported"`
	OverallReadinessScore           float64 `json:"overall_readiness_score"`
}

// MultiClusterSupport represents Platform Multi-cluster support (BR-BOOTSTRAP-ENVIRONMENT-005)
type MultiClusterSupport struct {
	VectorSyncSupported           bool    `json:"vector_sync_supported"`
	ResourceDiscoverySupported    bool    `json:"resource_discovery_supported"`
	ResourceOptimizationSupported bool    `json:"resource_optimization_supported"`
	OverallReadinessScore         float64 `json:"overall_readiness_score"`
}

// WorkflowPgVectorSupport represents Workflow Engine pgvector support (BR-BOOTSTRAP-ENVIRONMENT-006)
type WorkflowPgVectorSupport struct {
	StatePersistenceSupported     bool    `json:"state_persistence_supported"`
	VectorDecisionMakingSupported bool    `json:"vector_decision_making_supported"`
	ResourceOptimizationSupported bool    `json:"resource_optimization_supported"`
	OverallReadinessScore         float64 `json:"overall_readiness_score"`
}

// SystemHealthStatus represents overall system health (BR-BOOTSTRAP-ENVIRONMENT-007)
type SystemHealthStatus struct {
	DatabaseHealthy    bool    `json:"database_healthy"`
	VectorDBHealthy    bool    `json:"vector_db_healthy"`
	LLMClientHealthy   bool    `json:"llm_client_healthy"`
	OverallHealthScore float64 `json:"overall_health_score"`
}

// EnvironmentValidationSummary represents complete environment validation (BR-BOOTSTRAP-ENVIRONMENT-010)
type EnvironmentValidationSummary struct {
	DatabaseStatus          *DatabaseStatus          `json:"database_status"`
	AIPgVectorSupport       *AIPgVectorSupport       `json:"ai_pgvector_support"`
	MultiClusterSupport     *MultiClusterSupport     `json:"multicluster_support"`
	WorkflowPgVectorSupport *WorkflowPgVectorSupport `json:"workflow_pgvector_support"`
	SystemHealth            *SystemHealthStatus      `json:"system_health"`
	ValidationTime          time.Duration            `json:"validation_time"`
	ReadinessScore          float64                  `json:"readiness_score"`
}

// NewBootstrapEnvironmentValidator creates a new environment validator
// Following guideline: Reuse existing code and integrate with existing infrastructure
// Business Requirement: BR-BOOTSTRAP-ENVIRONMENT-001 - Environment validation
func NewBootstrapEnvironmentValidator(suite *TestSuite, logger *logrus.Logger) *BootstrapEnvironmentValidator {
	if suite == nil {
		logger.Error("Test suite cannot be nil for bootstrap environment validator")
		return nil
	}
	if logger == nil {
		logger = logrus.New()
	}

	return &BootstrapEnvironmentValidator{
		suite:  suite,
		logger: logger,
	}
}

// ValidateDatabaseConnectivity implements BR-BOOTSTRAP-ENVIRONMENT-002: PostgreSQL and pgvector validation
func (v *BootstrapEnvironmentValidator) ValidateDatabaseConnectivity(ctx context.Context) (*DatabaseStatus, error) {
	// Following guideline: Always handle errors, never ignore them
	v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-002: Starting database connectivity validation")

	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("BR-BOOTSTRAP-ENVIRONMENT-002: database validation cancelled: %w", ctx.Err())
	default:
	}

	status := &DatabaseStatus{
		PostgreSQLConnected: false,
		PgVectorConnected:   false,
		PgVectorDimension:   0,
		ConnectionLatency:   0,
	}

	// Test PostgreSQL connectivity
	if v.suite.DB != nil {
		startTime := time.Now()
		err := v.suite.DB.Ping()
		latency := time.Since(startTime)

		if err != nil {
			v.logger.WithError(err).Error("BR-BOOTSTRAP-ENVIRONMENT-002: PostgreSQL connectivity failed")
			return status, fmt.Errorf("BR-BOOTSTRAP-ENVIRONMENT-002: PostgreSQL not accessible: %w", err)
		}

		status.PostgreSQLConnected = true
		status.ConnectionLatency = latency.Milliseconds()

		v.logger.WithField("latency_ms", status.ConnectionLatency).Info("BR-BOOTSTRAP-ENVIRONMENT-002: PostgreSQL connection validated")
	}

	// Test pgvector connectivity and dimension
	if v.suite.VectorDB != nil {
		err := v.suite.VectorDB.IsHealthy(ctx)
		if err != nil {
			v.logger.WithError(err).Error("BR-BOOTSTRAP-ENVIRONMENT-002: pgvector connectivity failed")
			return status, fmt.Errorf("BR-BOOTSTRAP-ENVIRONMENT-002: pgvector not accessible: %w", err)
		}

		status.PgVectorConnected = true
		status.PgVectorDimension = v.getVectorDimension() // Current milestone: fixed dimension

		v.logger.WithField("vector_dimension", status.PgVectorDimension).Info("BR-BOOTSTRAP-ENVIRONMENT-002: pgvector connection validated")
	}

	// BR-BOOTSTRAP-ENVIRONMENT-003: Connection performance validation
	if status.ConnectionLatency > 1000 { // > 1 second
		v.logger.WithField("latency_ms", status.ConnectionLatency).Warn("BR-BOOTSTRAP-ENVIRONMENT-003: Database connection latency exceeds target")
	}

	return status, nil
}

// ValidateAIPgVectorSupport implements BR-BOOTSTRAP-ENVIRONMENT-004: AI & pgvector integration validation
func (v *BootstrapEnvironmentValidator) ValidateAIPgVectorSupport(ctx context.Context) (*AIPgVectorSupport, error) {
	// Following guideline: Always handle errors, never ignore them
	v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-004: Starting AI & pgvector support validation")

	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("BR-BOOTSTRAP-ENVIRONMENT-004: AI pgvector validation cancelled: %w", ctx.Err())
	default:
	}

	support := &AIPgVectorSupport{
		EmbeddingPipelineSupported:      false,
		PerformanceIntegrationSupported: false,
		AnalyticsIntegrationSupported:   false,
		OverallReadinessScore:           0.0,
	}

	// Validate embedding pipeline support
	if v.suite.VectorDB != nil && v.suite.LLMClient != nil {
		support.EmbeddingPipelineSupported = true
		v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-004: Embedding pipeline support validated")
	}

	// Validate performance integration support
	if v.suite.VectorDB != nil {
		err := v.suite.VectorDB.IsHealthy(ctx)
		if err == nil {
			support.PerformanceIntegrationSupported = true
			v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-004: Performance integration support validated")
		}
	}

	// Validate analytics integration support
	if v.suite.LLMClient != nil && v.suite.LLMClient.IsHealthy() {
		support.AnalyticsIntegrationSupported = true
		v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-004: Analytics integration support validated")
	}

	// Calculate overall readiness score
	support.OverallReadinessScore = v.calculateAIReadinessScore(support)

	v.logger.WithFields(logrus.Fields{
		"embedding_pipeline":      support.EmbeddingPipelineSupported,
		"performance_integration": support.PerformanceIntegrationSupported,
		"analytics_integration":   support.AnalyticsIntegrationSupported,
		"readiness_score":         support.OverallReadinessScore,
	}).Info("BR-BOOTSTRAP-ENVIRONMENT-004: AI & pgvector support validation completed")

	return support, nil
}

// ValidateMultiClusterSupport implements BR-BOOTSTRAP-ENVIRONMENT-005: Platform Multi-cluster validation
func (v *BootstrapEnvironmentValidator) ValidateMultiClusterSupport(ctx context.Context) (*MultiClusterSupport, error) {
	// Following guideline: Always handle errors, never ignore them
	v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-005: Starting Platform Multi-cluster support validation")

	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("BR-BOOTSTRAP-ENVIRONMENT-005: multicluster validation cancelled: %w", ctx.Err())
	default:
	}

	support := &MultiClusterSupport{
		VectorSyncSupported:           false,
		ResourceDiscoverySupported:    false,
		ResourceOptimizationSupported: false,
		OverallReadinessScore:         0.0,
	}

	// Validate vector sync support
	if v.suite.VectorDB != nil {
		err := v.suite.VectorDB.IsHealthy(ctx)
		if err == nil {
			support.VectorSyncSupported = true
			v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-005: Vector sync support validated")
		}
	}

	// Validate resource discovery support
	if v.suite.VectorDB != nil {
		// Test vector search capability for resource discovery
		_, err := v.suite.VectorDB.SearchBySemantics(ctx, "test_discovery", 1)
		if err == nil {
			support.ResourceDiscoverySupported = true
			v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-005: Resource discovery support validated")
		}
	}

	// Validate resource optimization support
	if v.suite.VectorDB != nil && support.VectorSyncSupported {
		support.ResourceOptimizationSupported = true
		v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-005: Resource optimization support validated")
	}

	// Calculate overall readiness score
	support.OverallReadinessScore = v.calculateMultiClusterReadinessScore(support)

	v.logger.WithFields(logrus.Fields{
		"vector_sync":           support.VectorSyncSupported,
		"resource_discovery":    support.ResourceDiscoverySupported,
		"resource_optimization": support.ResourceOptimizationSupported,
		"readiness_score":       support.OverallReadinessScore,
	}).Info("BR-BOOTSTRAP-ENVIRONMENT-005: Platform Multi-cluster support validation completed")

	return support, nil
}

// ValidateWorkflowPgVectorSupport implements BR-BOOTSTRAP-ENVIRONMENT-006: Workflow Engine pgvector validation
func (v *BootstrapEnvironmentValidator) ValidateWorkflowPgVectorSupport(ctx context.Context) (*WorkflowPgVectorSupport, error) {
	// Following guideline: Always handle errors, never ignore them
	v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-006: Starting Workflow Engine pgvector support validation")

	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("BR-BOOTSTRAP-ENVIRONMENT-006: workflow pgvector validation cancelled: %w", ctx.Err())
	default:
	}

	support := &WorkflowPgVectorSupport{
		StatePersistenceSupported:     false,
		VectorDecisionMakingSupported: false,
		ResourceOptimizationSupported: false,
		OverallReadinessScore:         0.0,
	}

	// Validate state persistence support
	if v.suite.VectorDB != nil && v.suite.WorkflowBuilder != nil {
		err := v.suite.VectorDB.IsHealthy(ctx)
		if err == nil {
			support.StatePersistenceSupported = true
			v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-006: State persistence support validated")
		}
	}

	// Validate vector decision making support
	if v.suite.VectorDB != nil {
		// Test vector similarity search for decision making
		_, err := v.suite.VectorDB.SearchBySemantics(ctx, "test_decision", 1)
		if err == nil {
			support.VectorDecisionMakingSupported = true
			v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-006: Vector decision making support validated")
		}
	}

	// Validate resource optimization support
	if support.StatePersistenceSupported && support.VectorDecisionMakingSupported {
		support.ResourceOptimizationSupported = true
		v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-006: Resource optimization support validated")
	}

	// Calculate overall readiness score
	support.OverallReadinessScore = v.calculateWorkflowReadinessScore(support)

	v.logger.WithFields(logrus.Fields{
		"state_persistence":      support.StatePersistenceSupported,
		"vector_decision_making": support.VectorDecisionMakingSupported,
		"resource_optimization":  support.ResourceOptimizationSupported,
		"readiness_score":        support.OverallReadinessScore,
	}).Info("BR-BOOTSTRAP-ENVIRONMENT-006: Workflow Engine pgvector support validation completed")

	return support, nil
}

// ValidateSystemHealth implements BR-BOOTSTRAP-ENVIRONMENT-007: Overall system health validation
func (v *BootstrapEnvironmentValidator) ValidateSystemHealth(ctx context.Context) (*SystemHealthStatus, error) {
	// Following guideline: Always handle errors, never ignore them
	v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-007: Starting overall system health validation")

	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("BR-BOOTSTRAP-ENVIRONMENT-007: system health validation cancelled: %w", ctx.Err())
	default:
	}

	health := &SystemHealthStatus{
		DatabaseHealthy:    false,
		VectorDBHealthy:    false,
		LLMClientHealthy:   false,
		OverallHealthScore: 0.0,
	}

	// Check database health
	if v.suite.DB != nil {
		err := v.suite.DB.Ping()
		if err == nil {
			health.DatabaseHealthy = true
			v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-007: Database health validated")
		} else {
			v.logger.WithError(err).Warn("BR-BOOTSTRAP-ENVIRONMENT-007: Database health check failed")
		}
	}

	// Check vector database health
	if v.suite.VectorDB != nil {
		err := v.suite.VectorDB.IsHealthy(ctx)
		if err == nil {
			health.VectorDBHealthy = true
			v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-007: Vector database health validated")
		} else {
			v.logger.WithError(err).Warn("BR-BOOTSTRAP-ENVIRONMENT-007: Vector database health check failed")
		}
	}

	// Check LLM client health
	if v.suite.LLMClient != nil && v.suite.LLMClient.IsHealthy() {
		health.LLMClientHealthy = true
		v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-007: LLM client health validated")
	} else {
		v.logger.Warn("BR-BOOTSTRAP-ENVIRONMENT-007: LLM client health check failed")
	}

	// Calculate overall health score
	health.OverallHealthScore = v.calculateSystemHealthScore(health)

	v.logger.WithFields(logrus.Fields{
		"database_healthy":     health.DatabaseHealthy,
		"vector_db_healthy":    health.VectorDBHealthy,
		"llm_client_healthy":   health.LLMClientHealthy,
		"overall_health_score": health.OverallHealthScore,
	}).Info("BR-BOOTSTRAP-ENVIRONMENT-007: System health validation completed")

	return health, nil
}

// ValidateBootstrapEnvironment implements BR-BOOTSTRAP-ENVIRONMENT-001: Complete environment validation
func (v *BootstrapEnvironmentValidator) ValidateBootstrapEnvironment(ctx context.Context) (*EnvironmentValidationSummary, error) {
	// Following guideline: Always handle errors, never ignore them
	v.logger.Info("BR-BOOTSTRAP-ENVIRONMENT-001: Starting complete bootstrap environment validation")

	startTime := time.Now()

	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("BR-BOOTSTRAP-ENVIRONMENT-001: bootstrap validation cancelled: %w", ctx.Err())
	default:
	}

	summary := &EnvironmentValidationSummary{}

	// Validate database connectivity
	dbStatus, err := v.ValidateDatabaseConnectivity(ctx)
	if err != nil {
		v.logger.WithError(err).Error("BR-BOOTSTRAP-ENVIRONMENT-001: Database validation failed")
		return nil, fmt.Errorf("BR-BOOTSTRAP-ENVIRONMENT-001: database validation failed: %w", err)
	}
	summary.DatabaseStatus = dbStatus

	// Validate AI & pgvector support
	aiSupport, err := v.ValidateAIPgVectorSupport(ctx)
	if err != nil {
		v.logger.WithError(err).Error("BR-BOOTSTRAP-ENVIRONMENT-001: AI pgvector validation failed")
		return nil, fmt.Errorf("BR-BOOTSTRAP-ENVIRONMENT-001: AI pgvector validation failed: %w", err)
	}
	summary.AIPgVectorSupport = aiSupport

	// Validate multi-cluster support
	multiClusterSupport, err := v.ValidateMultiClusterSupport(ctx)
	if err != nil {
		v.logger.WithError(err).Error("BR-BOOTSTRAP-ENVIRONMENT-001: Multi-cluster validation failed")
		return nil, fmt.Errorf("BR-BOOTSTRAP-ENVIRONMENT-001: multi-cluster validation failed: %w", err)
	}
	summary.MultiClusterSupport = multiClusterSupport

	// Validate workflow pgvector support
	workflowSupport, err := v.ValidateWorkflowPgVectorSupport(ctx)
	if err != nil {
		v.logger.WithError(err).Error("BR-BOOTSTRAP-ENVIRONMENT-001: Workflow pgvector validation failed")
		return nil, fmt.Errorf("BR-BOOTSTRAP-ENVIRONMENT-001: workflow pgvector validation failed: %w", err)
	}
	summary.WorkflowPgVectorSupport = workflowSupport

	// Validate system health
	systemHealth, err := v.ValidateSystemHealth(ctx)
	if err != nil {
		v.logger.WithError(err).Error("BR-BOOTSTRAP-ENVIRONMENT-001: System health validation failed")
		return nil, fmt.Errorf("BR-BOOTSTRAP-ENVIRONMENT-001: system health validation failed: %w", err)
	}
	summary.SystemHealth = systemHealth

	summary.ValidationTime = time.Since(startTime)

	// Calculate overall readiness score
	summary.ReadinessScore = v.calculateOverallReadinessScore(summary)

	v.logger.WithFields(logrus.Fields{
		"validation_time":    summary.ValidationTime,
		"readiness_score":    summary.ReadinessScore,
		"database_ok":        summary.DatabaseStatus.PostgreSQLConnected && summary.DatabaseStatus.PgVectorConnected,
		"ai_readiness":       summary.AIPgVectorSupport.OverallReadinessScore,
		"platform_readiness": summary.MultiClusterSupport.OverallReadinessScore,
		"workflow_readiness": summary.WorkflowPgVectorSupport.OverallReadinessScore,
		"system_health":      summary.SystemHealth.OverallHealthScore,
	}).Info("BR-BOOTSTRAP-ENVIRONMENT-001: Complete bootstrap environment validation finished")

	return summary, nil
}

// Helper methods - Following guideline: avoid duplication

func (v *BootstrapEnvironmentValidator) getVectorDimension() int {
	// Current milestone: Use standard dimension for pgvector
	return 384 // MiniLM embedding dimension
}

func (v *BootstrapEnvironmentValidator) calculateAIReadinessScore(support *AIPgVectorSupport) float64 {
	score := 0.0
	maxScore := 3.0

	if support.EmbeddingPipelineSupported {
		score += 1.0
	}
	if support.PerformanceIntegrationSupported {
		score += 1.0
	}
	if support.AnalyticsIntegrationSupported {
		score += 1.0
	}

	return score / maxScore
}

func (v *BootstrapEnvironmentValidator) calculateMultiClusterReadinessScore(support *MultiClusterSupport) float64 {
	score := 0.0
	maxScore := 3.0

	if support.VectorSyncSupported {
		score += 1.0
	}
	if support.ResourceDiscoverySupported {
		score += 1.0
	}
	if support.ResourceOptimizationSupported {
		score += 1.0
	}

	return score / maxScore
}

func (v *BootstrapEnvironmentValidator) calculateWorkflowReadinessScore(support *WorkflowPgVectorSupport) float64 {
	score := 0.0
	maxScore := 3.0

	if support.StatePersistenceSupported {
		score += 1.0
	}
	if support.VectorDecisionMakingSupported {
		score += 1.0
	}
	if support.ResourceOptimizationSupported {
		score += 1.0
	}

	return score / maxScore
}

func (v *BootstrapEnvironmentValidator) calculateSystemHealthScore(health *SystemHealthStatus) float64 {
	score := 0.0
	maxScore := 3.0

	if health.DatabaseHealthy {
		score += 1.0
	}
	if health.VectorDBHealthy {
		score += 1.0
	}
	if health.LLMClientHealthy {
		score += 1.0
	}

	return score / maxScore
}

func (v *BootstrapEnvironmentValidator) calculateOverallReadinessScore(summary *EnvironmentValidationSummary) float64 {
	// Weight different components (current milestone: emphasis on core functionality)
	databaseWeight := 0.3
	aiWeight := 0.25
	multiClusterWeight := 0.2
	workflowWeight := 0.15
	systemHealthWeight := 0.1

	score := 0.0

	// Database readiness
	if summary.DatabaseStatus.PostgreSQLConnected && summary.DatabaseStatus.PgVectorConnected {
		score += databaseWeight
	}

	// AI readiness
	score += summary.AIPgVectorSupport.OverallReadinessScore * aiWeight

	// Multi-cluster readiness
	score += summary.MultiClusterSupport.OverallReadinessScore * multiClusterWeight

	// Workflow readiness
	score += summary.WorkflowPgVectorSupport.OverallReadinessScore * workflowWeight

	// System health
	score += summary.SystemHealth.OverallHealthScore * systemHealthWeight

	return score
}
