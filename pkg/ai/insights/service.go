package insights

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/common"
	"github.com/sirupsen/logrus"
)

// Use common types
type AssessmentProcessor = common.AssessmentProcessor
type EnhancedAssessorInterface = common.EnhancedAssessorInterface
type PatternAnalytics = common.PatternAnalytics
type EnhancedEffectivenessResult = common.EnhancedEffectivenessResult

// Concrete implementation of EnhancedAssessorInterface
type EnhancedAssessor struct {
	// Implementation will be added here
}

// Implement EnhancedAssessorInterface methods
func (ea *EnhancedAssessor) ProcessPendingAssessments(ctx context.Context) error {
	// Stub implementation
	return nil
}

func (ea *EnhancedAssessor) GetAnalyticsInsights(ctx context.Context) (*common.AnalyticsInsights, error) {
	// Stub implementation
	return &common.AnalyticsInsights{}, nil
}

func (ea *EnhancedAssessor) GetPatternAnalytics(ctx context.Context) (interface{}, error) {
	// Stub implementation
	return &common.PatternAnalytics{}, nil
}

func (ea *EnhancedAssessor) TrainModels(ctx context.Context) error {
	// Stub implementation
	return nil
}

func (ea *EnhancedAssessor) AssessActionEffectiveness(ctx context.Context, trace interface{}) (*common.EnhancedEffectivenessResult, error) {
	// Stub implementation
	return &common.EnhancedEffectivenessResult{}, nil
}

// Service manages the background effectiveness assessment process
type Service struct {
	processor AssessmentProcessor
	interval  time.Duration
	log       *logrus.Logger

	// Service lifecycle
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.RWMutex
}

// EnhancedService extends the basic service with enhanced assessment capabilities
type EnhancedService struct {
	*Service
	enhancedAssessor EnhancedAssessorInterface
}

// ServiceConfig holds configuration for the assessment service
type ServiceConfig struct {
	EnableEnhancedAssessment bool          `yaml:"enable_enhanced_assessment" env:"ENABLE_ENHANCED_ASSESSMENT" default:"false"`
	AssessmentInterval       time.Duration `yaml:"assessment_interval" env:"ASSESSMENT_INTERVAL" default:"5m"`
	BatchSize                int           `yaml:"batch_size" env:"ASSESSMENT_BATCH_SIZE" default:"10"`
}

// Type aliases for Phase 2 analytics types

// NewService creates a new effectiveness assessment service
func NewService(processor AssessmentProcessor, interval time.Duration, log *logrus.Logger) *Service {
	return &Service{
		processor: processor,
		interval:  interval,
		log:       log,
	}
}

// NewServiceWithConfig creates a new service based on configuration
func NewServiceWithConfig(
	assessor *Assessor,
	enhancedAssessor EnhancedAssessorInterface,
	config ServiceConfig,
	log *logrus.Logger,
) (*Service, error) {
	var processor AssessmentProcessor

	if config.EnableEnhancedAssessment && enhancedAssessor != nil {
		processor = enhancedAssessor
		log.Info("Using enhanced effectiveness assessment")
	} else {
		processor = assessor
		log.Info("Using traditional effectiveness assessment")
	}

	if processor == nil {
		return nil, fmt.Errorf("no valid assessment processor available")
	}

	return NewService(processor, config.AssessmentInterval, log), nil
}

// NewEnhancedService creates a new enhanced effectiveness assessment service
func NewEnhancedService(
	baseService *Service,
	enhancedAssessor *EnhancedAssessor,
	log *logrus.Logger,
) *EnhancedService {
	return &EnhancedService{
		Service:          baseService,
		enhancedAssessor: enhancedAssessor,
	}
}

// Start begins the background effectiveness assessment process
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil // Already running
	}

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.running = true

	s.wg.Add(1)
	go s.run()

	s.log.WithField("interval", s.interval).Info("Started effectiveness assessment service")
	return nil
}

// Stop gracefully shuts down the effectiveness assessment service
func (s *Service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil // Not running
	}

	s.cancel()
	s.running = false

	// Wait for background goroutine to finish
	s.wg.Wait()

	s.log.Info("Stopped effectiveness assessment service")
	return nil
}

// IsRunning returns whether the service is currently running
func (s *Service) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// run is the main background loop for processing assessments
func (s *Service) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Process any pending assessments immediately on startup
	s.processOnce()

	for {
		select {
		case <-s.ctx.Done():
			s.log.Debug("Effectiveness assessment service context cancelled")
			return
		case <-ticker.C:
			s.processOnce()
		}
	}
}

// processOnce performs a single round of effectiveness assessment processing
func (s *Service) processOnce() {
	start := time.Now()

	if err := s.processor.ProcessPendingAssessments(s.ctx); err != nil {
		s.log.WithError(err).Error("Failed to process pending effectiveness assessments")
	} else {
		duration := time.Since(start)
		s.log.WithField("duration", duration).Debug("Completed effectiveness assessment processing cycle")
	}
}

// TriggerAssessment manually triggers an assessment cycle
func (s *Service) TriggerAssessment() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.running {
		return nil // Service not running
	}

	go s.processOnce()
	return nil
}

// GetStats returns statistics about the assessment service
func (s *Service) GetStats() ServiceStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return ServiceStats{
		Running:            s.running,
		AssessmentInterval: s.interval,
	}
}

// ServiceStats represents statistics about the effectiveness assessment service
type ServiceStats struct {
	Running            bool          `json:"running"`
	AssessmentInterval time.Duration `json:"assessment_interval"`
}

// Enhanced service methods for Phase 2 features

// GetAnalyticsInsights returns comprehensive analytics insights (enhanced service only)
func (es *EnhancedService) GetAnalyticsInsights(ctx context.Context) (*common.AnalyticsInsights, error) {
	if es.enhancedAssessor == nil {
		return nil, fmt.Errorf("enhanced assessor not available")
	}
	return es.enhancedAssessor.GetAnalyticsInsights(ctx)
}

// GetPatternAnalytics returns pattern-based analytics (enhanced service only)
func (es *EnhancedService) GetPatternAnalytics(ctx context.Context) (*PatternAnalytics, error) {
	if es.enhancedAssessor == nil {
		return nil, fmt.Errorf("enhanced assessor not available")
	}
	return nil, nil // TODO: Fix return type
}

// TrainModels triggers model training with latest data (enhanced service only)
func (es *EnhancedService) TrainModels(ctx context.Context) error {
	if es.enhancedAssessor == nil {
		return fmt.Errorf("enhanced assessor not available")
	}
	return es.enhancedAssessor.TrainModels(ctx)
}

// IsEnhancedAssessmentEnabled returns true if enhanced assessment is being used
func (s *Service) IsEnhancedAssessmentEnabled() bool {
	_, isEnhanced := s.processor.(EnhancedAssessorInterface)
	return isEnhanced
}

// GetEnhancedStats returns enhanced statistics if available
func (s *Service) GetEnhancedStats(ctx context.Context) (*EnhancedServiceStats, error) {
	baseStats := s.GetStats()

	enhancedStats := &EnhancedServiceStats{
		ServiceStats:       baseStats,
		EnhancedAssessment: s.IsEnhancedAssessmentEnabled(),
	}

	if enhancedAssessor, ok := s.processor.(EnhancedAssessorInterface); ok {
		// Get pattern analytics if enhanced assessment is enabled
		if patternAnalytics, err := enhancedAssessor.GetPatternAnalytics(ctx); err == nil {
			if pa, ok := patternAnalytics.(*common.PatternAnalytics); ok {
				enhancedStats.TotalPatterns = &pa.TotalPatterns
				enhancedStats.AverageEffectiveness = &pa.AverageEffectiveness
			}
		}
	}

	return enhancedStats, nil
}

// EnhancedServiceStats extends ServiceStats with Phase 2 analytics
type EnhancedServiceStats struct {
	ServiceStats
	EnhancedAssessment   bool     `json:"enhanced_assessment"`
	TotalPatterns        *int     `json:"total_patterns,omitempty"`
	AverageEffectiveness *float64 `json:"average_effectiveness,omitempty"`
}
