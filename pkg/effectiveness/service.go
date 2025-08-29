package effectiveness

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Service manages the background effectiveness assessment process
type Service struct {
	assessor *Assessor
	interval time.Duration
	log      *logrus.Logger

	// Service lifecycle
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.RWMutex
}

// NewService creates a new effectiveness assessment service
func NewService(assessor *Assessor, interval time.Duration, log *logrus.Logger) *Service {
	return &Service{
		assessor: assessor,
		interval: interval,
		log:      log,
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

	if err := s.assessor.ProcessPendingAssessments(s.ctx); err != nil {
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
