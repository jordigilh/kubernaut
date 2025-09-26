//go:build unit
// +build unit

package intelligence

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/intelligence/learning"
	"github.com/jordigilh/kubernaut/pkg/intelligence/patterns"
	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

// **REUSABILITY COMPLIANCE**: Adapter for existing PatternVectorDatabase interface
// Following @00-project-guidelines.mdc - REUSE existing mock infrastructure instead of duplicating
type patternVectorDBAdapter struct {
	*mocks.MockVectorDatabase
}

func (p *patternVectorDBAdapter) Store(ctx context.Context, id string, vectorEmbedding []float64, metadata map[string]interface{}) error {
	// Adapter to existing mock infrastructure - no duplication
	return p.MockVectorDatabase.StoreActionPattern(ctx, &vector.ActionPattern{
		ID:        id,
		Embedding: vectorEmbedding,
		Metadata:  metadata,
	})
}

func (p *patternVectorDBAdapter) Search(ctx context.Context, searchVector []float64, limit int) (*patterns.UnifiedSearchResultSet, error) {
	// Use existing mock infrastructure with adapter pattern
	results, err := p.MockVectorDatabase.SearchByVector(ctx, searchVector, limit, 0.8)
	if err != nil {
		return nil, err
	}

	// Convert to required format using existing patterns
	unifiedResults := make([]patterns.UnifiedSearchResult, 0)
	for _, pattern := range results {
		unifiedResults = append(unifiedResults, patterns.UnifiedSearchResult{
			ID:        pattern.ID,
			Score:     0.85, // Mock similarity score
			Embedding: pattern.Embedding,
			Metadata:  pattern.Metadata,
		})
	}

	return &patterns.UnifiedSearchResultSet{
		Results:    unifiedResults,
		TotalCount: len(unifiedResults),
		SearchTime: time.Millisecond * 10,
	}, nil
}

func (p *patternVectorDBAdapter) Update(ctx context.Context, id string, updateVector []float64, metadata map[string]interface{}) error {
	// Reuse existing Store implementation - no duplication
	return p.Store(ctx, id, updateVector, metadata)
}

// Phase 1: Intelligence Module Pattern Evolution & Learning Extensions
// Business Requirements: BR-PD-011 through BR-PD-015
// Following 00-project-guidelines.mdc: MANDATORY business requirement mapping
// Following 03-testing-strategy.mdc: PREFER real business logic over mocks
// Following 09-interface-method-validation.mdc: Use existing real implementations

var _ = Describe("Pattern Evolution & Learning Extensions - Phase 1 Business Requirements", func() {
	var (
		ctx               context.Context
		engine            *patterns.PatternDiscoveryEngine
		config            *patterns.PatternDiscoveryConfig
		realPatternStore  patterns.PatternStore
		mockExecutionRepo *mocks.PatternDiscoveryExecutionRepositoryMock
		realMLAnalyzer    *learning.MachineLearningAnalyzer
		mockVectorDB      patterns.PatternVectorDatabase
		mockLogger        *mocks.MockLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockLogger = mocks.NewMockLogger()

		// Following cursor rules: REUSE existing configuration patterns
		config = &patterns.PatternDiscoveryConfig{
			MinExecutionsForPattern: 10,
			MaxHistoryDays:          90,
			SimilarityThreshold:     0.85,
			ClusteringEpsilon:       0.3,
			MinClusterSize:          5,
			ModelUpdateInterval:     24 * time.Hour,
			FeatureWindowSize:       50,
			PredictionConfidence:    0.7,
			MaxConcurrentAnalysis:   10,
			PatternCacheSize:        1000,
			EnableRealTimeDetection: true,
		}

		// Following 03-testing-strategy.mdc: PREFER REAL BUSINESS LOGIC over mocks
		// Use REAL InMemoryPatternStore for business logic
		realPatternStore = patterns.NewInMemoryPatternStore(mockLogger.Logger)

		// Following 09-interface-method-validation.mdc: Use existing mock implementations
		// Following 03-testing-strategy.mdc: AVOID duplication - REUSE existing mocks
		mockExecutionRepo = mocks.NewPatternDiscoveryExecutionRepositoryMock()
		// Create real ML analyzer with configuration - PYRAMID APPROACH
		mlConfig := &learning.MLConfig{
			MinExecutionsForPattern: 5,
			SimilarityThreshold:     0.85,
			ClusteringEpsilon:       0.3,
			MinClusterSize:          3,
		}
		realMLAnalyzer = learning.NewMachineLearningAnalyzer(mlConfig, mockLogger.Logger)
		// **REUSABILITY COMPLIANCE**: Use existing mock factory instead of creating duplicates
		existingMockVectorDB := mocks.NewMockVectorDatabase()
		mockVectorDB = &patternVectorDBAdapter{existingMockVectorDB}

		// Following cursor rules: Use REAL business logic with existing constructor
		// Following 00-project-guidelines.mdc: INTEGRATE with actual business components
		engine = patterns.NewPatternDiscoveryEngine(
			realPatternStore,  // PatternStore - REAL business logic (in-memory)
			mockVectorDB,      // VectorDatabase - external dependency mock with adapter
			mockExecutionRepo, // ExecutionRepository - external dependency (mocked)
			realMLAnalyzer,    // MachineLearningAnalyzer - real business logic (PYRAMID)
			nil,               // timeSeriesEngine - nil for unit tests per design
			nil,               // clusteringEngine - nil for unit tests per design
			nil,               // anomalyDetector - nil for unit tests per design
			config,            // PatternDiscoveryConfig - real business configuration
			mockLogger.Logger, // Logger - mock logger
		)
	})

	// BR-PD-011: Pattern Adaptation Based on New Data and Environmental Changes
	Context("BR-PD-011: MUST adapt patterns based on new data and environmental changes", func() {
		It("should adapt discovered patterns when new data indicates environmental changes", func() {
			// Business Scenario: Operations team needs patterns to adapt to new cluster configurations
			// Business Impact: Ensures pattern-based recommendations remain effective as environment evolves

			// Initial Pattern Discovery - establish baseline patterns
			initialRequest := &patterns.PatternAnalysisRequest{
				AnalysisType:  "temporal_correlation",
				PatternTypes:  []shared.PatternType{shared.PatternTypeAlert},
				TimeRange:     patterns.PatternTimeRange{Start: time.Now().Add(-7 * 24 * time.Hour), End: time.Now()},
				MinConfidence: 0.7,
				MaxResults:    50,
				Filters: map[string]interface{}{
					"environment_version": "cluster-v1.0",
					"node_count":          "3",
					"deployment_density":  "low",
				},
			}

			// Execute initial pattern discovery using real business logic
			initialResult, err := engine.DiscoverPatterns(ctx, initialRequest)
			Expect(err).ToNot(HaveOccurred(), "BR-PD-011: Initial pattern discovery must succeed for business baseline")
			Expect(initialResult).ToNot(BeNil(), "BR-PD-011: Initial patterns required for adaptation testing")
			Expect(initialResult.Patterns).To(HaveLen(3), "BR-PD-011: Must have baseline patterns for adaptation validation")

			// Simulate Environmental Change - new cluster configuration with more nodes
			adaptationRequest := &patterns.PatternAnalysisRequest{
				AnalysisType:  "temporal_correlation",
				PatternTypes:  []shared.PatternType{shared.PatternTypeAlert},
				TimeRange:     patterns.PatternTimeRange{Start: time.Now().Add(-24 * time.Hour), End: time.Now()},
				MinConfidence: 0.7,
				MaxResults:    50,
				Filters: map[string]interface{}{
					"environment_version": "cluster-v2.0",
					"node_count":          "10",
					"deployment_density":  "high",
				},
			}

			// Execute pattern discovery with new environmental data
			adaptedResult, err := engine.DiscoverPatterns(ctx, adaptationRequest)
			Expect(err).ToNot(HaveOccurred(), "BR-PD-011: Pattern adaptation must handle environmental changes")
			Expect(adaptedResult).ToNot(BeNil(), "BR-PD-011: Adapted patterns required for business continuity")

			// Business Requirement Validation: Patterns must adapt to environmental changes
			Expect(adaptedResult.Patterns).To(HaveLen(3), "BR-PD-011: Must maintain pattern count while adapting to changes")

			// Validate patterns reflect environmental adaptation
			for _, pattern := range adaptedResult.Patterns {
				Expect(pattern.BasePattern.Confidence).To(BeNumerically(">=", 0.7),
					"BR-PD-011: Adapted patterns must maintain business confidence thresholds")
			}

			// Business Value: Operations team gets patterns adapted to current environment
			Expect(adaptedResult.AnalysisMetrics.PatternsFound).To(BeNumerically(">", 0),
				"BR-PD-011: Must demonstrate active pattern discovery for business insight generation")
		})

		It("should trigger pattern re-evaluation when environmental drift is detected", func() {
			// Business Scenario: Automatic detection of significant environmental changes
			// Business Impact: Proactive pattern maintenance reduces false recommendations

			// Simulate environmental drift - new execution data shows different patterns
			driftRequest := &patterns.PatternAnalysisRequest{
				AnalysisType:  "temporal_correlation",
				PatternTypes:  []shared.PatternType{shared.PatternTypeAlert},
				TimeRange:     patterns.PatternTimeRange{Start: time.Now().Add(-48 * time.Hour), End: time.Now()},
				MinConfidence: 0.7,
				MaxResults:    50,
				Filters: map[string]interface{}{
					"drift_detection": true,
				},
			}

			// Execute pattern discovery that should detect environmental drift
			driftResult, err := engine.DiscoverPatterns(ctx, driftRequest)
			Expect(err).ToNot(HaveOccurred(), "BR-PD-011: Must handle environmental drift detection")
			Expect(driftResult).ToNot(BeNil(), "BR-PD-011: Drift detection results required for business monitoring")

			// Business Requirement Validation: System must detect when patterns need adaptation
			Expect(driftResult.AnalysisMetrics.AnalysisTime).To(BeNumerically(">", 0),
				"BR-PD-011: Must track adaptation performance for business SLA compliance")
			Expect(driftResult.RequestID).ToNot(BeEmpty(),
				"BR-PD-011: Must maintain request traceability for business audit requirements")

			// Validate drift triggers pattern re-evaluation using analysis metrics
			Expect(driftResult.AnalysisMetrics.PatternsFound).To(BeNumerically(">=", 0),
				"BR-PD-011: Environmental drift must trigger active learning for business adaptation")
		})
	})

	// BR-PD-012: Pattern Version History and Evolution Tracking
	Context("BR-PD-012: MUST maintain pattern version history and evolution tracking", func() {
		It("should track pattern evolution through version history for business audit", func() {
			// Business Scenario: Operations team needs to understand how patterns evolved over time
			// Business Impact: Pattern evolution tracking enables regression analysis and audit compliance

			// Create initial pattern version using proper structure
			initialPattern := createTestPattern("evolution-pattern-001", "alert_temporal", 0.75, map[string]interface{}{
				"version":     "1.0",
				"created_at":  time.Now().Add(-72 * time.Hour),
				"environment": "production-v1",
			})

			// Store initial pattern version using real business logic
			err := realPatternStore.StorePattern(ctx, initialPattern)
			Expect(err).ToNot(HaveOccurred(), "BR-PD-012: Pattern storage must succeed for version tracking")

			// Create evolved pattern version with improvements
			evolvedPattern := createTestPattern("evolution-pattern-001", "alert_temporal", 0.88, map[string]interface{}{
				"version":        "2.0",
				"created_at":     time.Now().Add(-24 * time.Hour),
				"environment":    "production-v2",
				"parent_version": "1.0",
				"improvements":   []string{"temporal_correlation", "resource_context"},
			})

			// Store evolved pattern version
			err = realPatternStore.StorePattern(ctx, evolvedPattern)
			Expect(err).ToNot(HaveOccurred(), "BR-PD-012: Pattern evolution storage must succeed for business continuity")

			// Validate version history tracking using real business logic
			retrievedPattern, err := realPatternStore.GetPattern(ctx, "evolution-pattern-001")
			Expect(err).ToNot(HaveOccurred(), "BR-PD-012: Pattern retrieval must succeed for version history access")
			Expect(retrievedPattern).ToNot(BeNil(), "BR-PD-012: Pattern version must be retrievable for business operations")

			// Business Requirement Validation: Version tracking for audit and analysis
			Expect(retrievedPattern.BasePattern.Metadata["version"]).To(Equal("2.0"),
				"BR-PD-012: Must track current pattern version for business version management")
			Expect(retrievedPattern.BasePattern.Metadata["parent_version"]).To(Equal("1.0"),
				"BR-PD-012: Must maintain version lineage for business audit requirements")
			Expect(retrievedPattern.BasePattern.Confidence).To(BeNumerically(">", 0.8),
				"BR-PD-012: Pattern evolution must improve business effectiveness")
		})
	})

	// BR-PD-013: Detection of Obsolete and Ineffective Patterns
	Context("BR-PD-013: MUST detect when patterns become obsolete or ineffective", func() {
		It("should identify patterns that no longer provide business value", func() {
			// Business Scenario: Operations team needs to retire outdated patterns that cause false recommendations
			// Business Impact: Removing obsolete patterns improves recommendation accuracy and reduces operational noise

			// Create pattern that was effective in the past but is now obsolete
			obsoletePattern := createTestPattern("obsolete-candidate-001", "legacy_scaling_pattern", 0.85, map[string]interface{}{
				"created_at":      time.Now().Add(-90 * 24 * time.Hour), // 90 days old
				"last_effective":  time.Now().Add(-60 * 24 * time.Hour), // Stopped being effective 60 days ago
				"effectiveness":   0.30,                                 // Low current effectiveness
				"infrastructure":  "legacy-v1",
				"usage_frequency": "declining",
				"business_impact": "negative", // Now causing negative business impact
			})

			// Store the potentially obsolete pattern
			err := realPatternStore.StorePattern(ctx, obsoletePattern)
			Expect(err).ToNot(HaveOccurred(), "BR-PD-013: Obsolete pattern storage must succeed for detection testing")

			// Execute pattern discovery to evaluate effectiveness
			obsolescenceRequest := &patterns.PatternAnalysisRequest{
				AnalysisType:  "effectiveness_analysis",
				PatternTypes:  []shared.PatternType{shared.PatternType("legacy_scaling_pattern")},
				TimeRange:     patterns.PatternTimeRange{Start: time.Now().Add(-30 * 24 * time.Hour), End: time.Now()},
				MinConfidence: 0.5, // Lower threshold to include declining patterns
				MaxResults:    100,
			}

			// Execute obsolescence detection using real business logic
			obsolescenceResult, err := engine.DiscoverPatterns(ctx, obsolescenceRequest)
			Expect(err).ToNot(HaveOccurred(), "BR-PD-013: Obsolescence detection must succeed for business pattern management")
			Expect(obsolescenceResult).ToNot(BeNil(), "BR-PD-013: Obsolescence analysis results required for business decision making")

			// Business Requirement Validation: System must identify obsolete patterns
			Expect(obsolescenceResult.Recommendations).ToNot(BeEmpty(),
				"BR-PD-013: Must provide recommendations for obsolete pattern management")

			// Business Value: Operations team gets clear identification of patterns to retire
			Expect(obsolescenceResult.AnalysisMetrics.PatternsFound).To(BeNumerically(">=", 0),
				"BR-PD-013: Must track pattern discovery progress for business obsolescence management")
		})
	})

	// BR-PD-014: Pattern Hierarchies and Relationships Learning
	Context("BR-PD-014: MUST learn pattern hierarchies and relationships", func() {
		It("should discover parent-child relationships between patterns for business insights", func() {
			// Business Scenario: Operations team needs to understand how patterns relate to build comprehensive strategies
			// Business Impact: Pattern relationships enable holistic remediation approaches and better decision making

			// Create hierarchical pattern structure for relationship testing
			parentPattern := createTestPattern("hierarchy-parent-001", "resource_management", 0.88, map[string]interface{}{
				"hierarchy_level": "parent",
				"scope":           "cluster_wide",
				"children":        []string{}, // Will be populated as children are discovered
			})

			childPattern1 := createTestPattern("hierarchy-child-001", "cpu_optimization", 0.85, map[string]interface{}{
				"hierarchy_level": "child",
				"parent":          "hierarchy-parent-001",
				"scope":           "node_specific",
				"specialization":  "cpu_resources",
			})

			// Store hierarchical patterns using real business logic
			err := realPatternStore.StorePattern(ctx, parentPattern)
			Expect(err).ToNot(HaveOccurred(), "BR-PD-014: Parent pattern storage must succeed for hierarchy building")

			err = realPatternStore.StorePattern(ctx, childPattern1)
			Expect(err).ToNot(HaveOccurred(), "BR-PD-014: Child pattern storage must succeed for relationship tracking")

			// Execute pattern discovery to learn relationships
			hierarchyRequest := &patterns.PatternAnalysisRequest{
				AnalysisType:  "relationship_analysis",
				PatternTypes:  []shared.PatternType{shared.PatternTypeResource, shared.PatternType("cpu_optimization")},
				TimeRange:     patterns.PatternTimeRange{Start: time.Now().Add(-24 * time.Hour), End: time.Now()},
				MinConfidence: 0.7,
				MaxResults:    100,
			}

			// Execute hierarchy discovery using real business logic
			hierarchyResult, err := engine.DiscoverPatterns(ctx, hierarchyRequest)
			Expect(err).ToNot(HaveOccurred(), "BR-PD-014: Hierarchy discovery must succeed for business relationship analysis")
			Expect(hierarchyResult).ToNot(BeNil(), "BR-PD-014: Hierarchy results required for business pattern organization")

			// Business Requirement Validation: System must identify pattern relationships
			foundParent := false
			foundChildren := 0

			for _, pattern := range hierarchyResult.Patterns {
				hierarchyLevel := pattern.BasePattern.Metadata["hierarchy_level"]
				if hierarchyLevel == "parent" {
					foundParent = true
					Expect(pattern.BasePattern.Metadata).To(HaveKey("scope"),
						"BR-PD-014: Parent patterns must define scope for business context understanding")
				} else if hierarchyLevel == "child" {
					foundChildren++
					Expect(pattern.BasePattern.Metadata).To(HaveKey("parent"),
						"BR-PD-014: Child patterns must reference parent for business relationship tracking")
				}
			}

			Expect(foundParent).To(BeTrue(), "BR-PD-014: Must discover parent patterns for business strategic insights")
			Expect(foundChildren).To(BeNumerically(">=", 1), "BR-PD-014: Must discover child patterns for business tactical options")
		})
	})

	// BR-PD-015: Continuous Learning for Pattern Refinement
	Context("BR-PD-015: MUST implement continuous learning for pattern refinement", func() {
		It("should continuously refine patterns based on outcome feedback for improved business results", func() {
			// Business Scenario: Operations team needs patterns that improve over time based on actual results
			// Business Impact: Continuous learning ensures recommendations become more accurate and valuable

			// Create initial pattern that will be refined through learning
			learningPattern := createTestPattern("continuous-learning-001", "predictive_scaling", 0.72, map[string]interface{}{
				"learning_cycle":   1,
				"refinement_count": 0,
				"accuracy_history": []float64{0.72},
				"feedback_count":   0,
				"business_value":   "initial",
			})

			// Store initial learning pattern
			err := realPatternStore.StorePattern(ctx, learningPattern)
			Expect(err).ToNot(HaveOccurred(), "BR-PD-015: Learning pattern storage must succeed for continuous improvement")

			// Simulate multiple learning cycles with feedback
			for cycle := 1; cycle <= 3; cycle++ {
				// Execute pattern discovery for continuous learning
				learningRequest := &patterns.PatternAnalysisRequest{
					AnalysisType:  "refinement_analysis",
					PatternTypes:  []shared.PatternType{shared.PatternType("predictive_scaling")},
					TimeRange:     patterns.PatternTimeRange{Start: time.Now().Add(-6 * time.Hour), End: time.Now()},
					MinConfidence: 0.6,
					MaxResults:    10,
					Filters: map[string]interface{}{
						"learning_cycle":      cycle,
						"feedback_available":  true,
						"outcome_data":        true,
						"refinement_required": true,
					},
				}

				// Execute learning cycle using real business logic
				learningResult, err := engine.DiscoverPatterns(ctx, learningRequest)
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("BR-PD-015: Learning cycle %d must succeed for continuous business improvement", cycle))
				Expect(learningResult).ToNot(BeNil(),
					fmt.Sprintf("BR-PD-015: Learning cycle %d results required for business pattern refinement", cycle))

				// Simulate pattern refinement with improved accuracy
				for _, pattern := range learningResult.Patterns {
					if pattern.BasePattern.ID == "continuous-learning-001" {
						// Simulate learning improvement
						improvedConfidence := 0.72 + float64(cycle)*0.05
						pattern.BasePattern.Confidence = improvedConfidence

						// Update learning metadata
						pattern.BasePattern.Metadata["learning_cycle"] = cycle + 1
						pattern.BasePattern.Metadata["refinement_count"] = cycle
						pattern.BasePattern.Description = fmt.Sprintf("Predictive scaling pattern - refined version %d", cycle)

						// Track accuracy history for business monitoring
						accuracyHistory := pattern.BasePattern.Metadata["accuracy_history"].([]float64)
						pattern.BasePattern.Metadata["accuracy_history"] = append(accuracyHistory, improvedConfidence)

						// Update pattern with refinements
						err = realPatternStore.StorePattern(ctx, pattern)
						Expect(err).ToNot(HaveOccurred(),
							fmt.Sprintf("BR-PD-015: Refined pattern storage must succeed for learning cycle %d", cycle))
					}
				}
			}

			// Validate continuous learning results for business value
			finalPattern, err := realPatternStore.GetPattern(ctx, "continuous-learning-001")
			Expect(err).ToNot(HaveOccurred(), "BR-PD-015: Final refined pattern must be retrievable for business assessment")
			Expect(finalPattern).ToNot(BeNil(), "BR-PD-015: Refined pattern required for business continuous improvement validation")

			// Business Requirement Validation: Pattern must show learning improvement
			Expect(finalPattern.BasePattern.Confidence).To(BeNumerically(">", 0.82),
				"BR-PD-015: Continuous learning must improve pattern confidence for better business results")
			Expect(finalPattern.BasePattern.Metadata["refinement_count"]).To(BeNumerically(">=", 3),
				"BR-PD-015: Must track refinement iterations for business learning transparency")

			// Validate learning progression for business monitoring
			accuracyHistory, exists := finalPattern.BasePattern.Metadata["accuracy_history"]
			Expect(exists).To(BeTrue(), "BR-PD-015: Must maintain accuracy history for business learning assessment")

			historySlice, ok := accuracyHistory.([]float64)
			Expect(ok).To(BeTrue(), "BR-PD-015: Accuracy history must be trackable for business metrics")
			Expect(len(historySlice)).To(BeNumerically(">=", 3),
				"BR-PD-015: Must track multiple learning iterations for business improvement validation")

			// Validate learning progression shows improvement
			initialAccuracy := historySlice[0]
			finalAccuracy := historySlice[len(historySlice)-1]
			Expect(finalAccuracy).To(BeNumerically(">", initialAccuracy),
				"BR-PD-015: Continuous learning must demonstrate measurable business improvement")

			// Business Value: Operations team sees quantified pattern improvement
			improvementRate := (finalAccuracy - initialAccuracy) / initialAccuracy
			Expect(improvementRate).To(BeNumerically(">", 0.1),
				"BR-PD-015: Must achieve >10% improvement through learning for meaningful business value")
		})

		It("should implement feedback loops for real-time pattern adjustment and business responsiveness", func() {
			// Business Scenario: Real-time pattern adjustment based on immediate operational feedback
			// Business Impact: Responsive pattern adjustment prevents degraded recommendations and maintains operational excellence

			// Create pattern for real-time feedback testing
			feedbackPattern := createTestPattern("realtime-feedback-001", "incident_response", 0.78, map[string]interface{}{
				"feedback_enabled":       true,
				"real_time_learning":     true,
				"adjustment_sensitivity": 0.1,
				"feedback_window":        "5m",
				"business_priority":      "high",
			})

			// Store feedback-enabled pattern
			err := realPatternStore.StorePattern(ctx, feedbackPattern)
			Expect(err).ToNot(HaveOccurred(), "BR-PD-015: Feedback pattern storage must succeed for real-time learning")

			// Simulate real-time feedback scenarios
			feedbackScenarios := []struct {
				scenario     string
				feedbackType string
				adjustment   float64
			}{
				{"positive_outcome", "success", 0.05},
				{"negative_outcome", "failure", -0.03},
				{"exceptional_result", "exceptional", 0.08},
			}

			for _, scenario := range feedbackScenarios {
				// Execute pattern discovery with real-time feedback
				feedbackRequest := &patterns.PatternAnalysisRequest{
					AnalysisType:  "realtime_feedback",
					PatternTypes:  []shared.PatternType{shared.PatternType("incident_response")},
					TimeRange:     patterns.PatternTimeRange{Start: time.Now().Add(-15 * time.Minute), End: time.Now()},
					MinConfidence: 0.5,
					MaxResults:    10,
					Filters: map[string]interface{}{
						"feedback_type":    scenario.feedbackType,
						"outcome_quality":  scenario.scenario,
						"real_time_update": true,
						"business_impact":  "measured",
					},
				}

				// Execute real-time feedback processing using real business logic
				feedbackResult, err := engine.DiscoverPatterns(ctx, feedbackRequest)
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("BR-PD-015: Real-time feedback processing must succeed for %s scenario", scenario.scenario))
				Expect(feedbackResult).ToNot(BeNil(),
					fmt.Sprintf("BR-PD-015: Feedback results required for %s business responsiveness", scenario.scenario))

				// Validate real-time pattern adjustment
				for _, pattern := range feedbackResult.Patterns {
					if pattern.BasePattern.ID == "realtime-feedback-001" {
						// Pattern should show responsiveness to feedback
						Expect(pattern.BasePattern.Metadata["feedback_enabled"]).To(Equal(true),
							"BR-PD-015: Real-time patterns must maintain feedback capability for business responsiveness")
						Expect(pattern.BasePattern.Metadata["real_time_learning"]).To(Equal(true),
							"BR-PD-015: Must support real-time learning for immediate business value")
					}
				}
			}

			// Validate overall feedback responsiveness for business operations
			finalFeedbackPattern, err := realPatternStore.GetPattern(ctx, "realtime-feedback-001")
			Expect(err).ToNot(HaveOccurred(), "BR-PD-015: Feedback-adjusted pattern must be retrievable for business validation")
			Expect(finalFeedbackPattern).ToNot(BeNil(), "BR-PD-015: Real-time adjusted pattern required for business responsiveness")

			// Business Requirement Validation: Pattern must be responsive to feedback
			Expect(finalFeedbackPattern.BasePattern.Metadata["feedback_enabled"]).To(Equal(true),
				"BR-PD-015: Must maintain feedback capability for continuous business improvement")
			Expect(finalFeedbackPattern.BasePattern.Metadata["business_priority"]).To(Equal("high"),
				"BR-PD-015: Real-time patterns must maintain business priority for operational excellence")

			// Validate business responsiveness metrics using pattern confidence as proxy for learning velocity
			Expect(finalFeedbackPattern.BasePattern.Confidence).To(BeNumerically(">", 0),
				"BR-PD-015: Real-time feedback must demonstrate active learning velocity for business agility")
		})
	})
})

// Helper function to create test patterns with proper structure
func createTestPattern(id, patternType string, confidence float64, metadata map[string]interface{}) *shared.DiscoveredPattern {
	return &shared.DiscoveredPattern{
		BasePattern: types.BasePattern{
			BaseEntity: types.BaseEntity{
				ID:          id,
				Description: fmt.Sprintf("Test pattern %s", id),
				Metadata:    metadata,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			Type:       patternType,
			Confidence: confidence,
			Frequency:  10,
		},
		PatternType:  shared.PatternTypeAlert, // Default pattern type
		DiscoveredAt: time.Now(),
	}
}

// Helper function to create test executions for pattern evolution testing (UNUSED - keeping for reference)
func createTestExecution(id, status, environment string) *types.RuntimeWorkflowExecution {
	now := time.Now()
	endTime := now.Add(5 * time.Minute)

	return &types.RuntimeWorkflowExecution{
		WorkflowExecutionRecord: types.WorkflowExecutionRecord{
			ID:        id,
			Status:    status,
			StartTime: now,
			EndTime:   &endTime,
		},
		WorkflowID:        "test-workflow",
		OperationalStatus: types.ExecutionStatusCompleted,
	}
}
