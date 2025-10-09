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

package execution

import (
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ProgressReporter provides detailed progress tracking for long-running analyses
type ProgressReporter struct {
	log      *logrus.Logger
	sessions map[string]*ProgressSession
	mu       sync.RWMutex
}

// ProgressSession tracks progress for a specific analysis session
type ProgressSession struct {
	ID                string                 `json:"id"`
	Title             string                 `json:"title"`
	StartTime         time.Time              `json:"start_time"`
	EndTime           *time.Time             `json:"end_time,omitempty"`
	Status            ProgressStatus         `json:"status"`
	CurrentStage      *ProgressStage         `json:"current_stage"`
	Stages            []*ProgressStage       `json:"stages"`
	OverallProgress   float64                `json:"overall_progress"`
	EstimatedDuration *time.Duration         `json:"estimated_duration,omitempty"`
	Metadata          map[string]interface{} `json:"metadata"`
	Callbacks         []ProgressCallback     `json:"-"`
	mu                sync.RWMutex           `json:"-"`
}

// ProgressStage represents a stage in the analysis process
type ProgressStage struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	StartTime      *time.Time     `json:"start_time,omitempty"`
	EndTime        *time.Time     `json:"end_time,omitempty"`
	Status         ProgressStatus `json:"status"`
	Progress       float64        `json:"progress"`
	Substeps       []*Substep     `json:"substeps"`
	CurrentSubstep *Substep       `json:"current_substep,omitempty"`
	EstimatedTime  time.Duration  `json:"estimated_time"`
	ActualTime     time.Duration  `json:"actual_time"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	Metrics        StageMetrics   `json:"metrics"`
}

// Substep represents a substep within a stage
type Substep struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Status      ProgressStatus    `json:"status"`
	Progress    float64           `json:"progress"`
	StartTime   *time.Time        `json:"start_time,omitempty"`
	EndTime     *time.Time        `json:"end_time,omitempty"`
	Duration    time.Duration     `json:"duration"`
	Details     map[string]string `json:"details"`
}

// ProgressStatus defines the status of progress items
type ProgressStatus string

const (
	ProgressStatusPending    ProgressStatus = "pending"
	ProgressStatusInProgress ProgressStatus = "in_progress"
	ProgressStatusCompleted  ProgressStatus = "completed"
	ProgressStatusFailed     ProgressStatus = "failed"
	ProgressStatusSkipped    ProgressStatus = "skipped"
	ProgressStatusCancelled  ProgressStatus = "cancelled"
)

// StageMetrics contains metrics for a progress stage
type StageMetrics struct {
	ItemsProcessed int       `json:"items_processed"`
	ItemsTotal     int       `json:"items_total"`
	ProcessingRate float64   `json:"processing_rate"` // items per second
	ThroughputMBps float64   `json:"throughput_mbps,omitempty"`
	ErrorCount     int       `json:"error_count"`
	WarningCount   int       `json:"warning_count"`
	LastUpdateTime time.Time `json:"last_update_time"`
}

// ProgressCallback is called when progress updates occur
type ProgressCallback func(*ProgressSession)

// ProgressSnapshot provides a point-in-time view of progress
type ProgressSnapshot struct {
	SessionID          string                     `json:"session_id"`
	Title              string                     `json:"title"`
	Status             ProgressStatus             `json:"status"`
	OverallProgress    float64                    `json:"overall_progress"`
	CurrentStage       string                     `json:"current_stage"`
	ElapsedTime        time.Duration              `json:"elapsed_time"`
	EstimatedRemaining *time.Duration             `json:"estimated_remaining,omitempty"`
	RecentActivity     []string                   `json:"recent_activity"`
	Performance        ProgressPerformanceMetrics `json:"performance"`
}

// ProgressPerformanceMetrics provides performance information
type ProgressPerformanceMetrics struct {
	ItemsPerSecond   float64       `json:"items_per_second"`
	AverageStageTime time.Duration `json:"average_stage_time"`
	PeakMemoryUsage  int64         `json:"peak_memory_usage"`
	CPUUtilization   float64       `json:"cpu_utilization"`
}

// NewProgressReporter creates a new progress reporter
func NewProgressReporter(log *logrus.Logger) *ProgressReporter {
	return &ProgressReporter{
		log:      log,
		sessions: make(map[string]*ProgressSession),
	}
}

// StartSession starts a new progress tracking session
func (pr *ProgressReporter) StartSession(title string, stages []string) *ProgressSession {
	sessionID := fmt.Sprintf("session_%d", time.Now().UnixNano())

	session := &ProgressSession{
		ID:              sessionID,
		Title:           title,
		StartTime:       time.Now(),
		Status:          ProgressStatusInProgress,
		Stages:          make([]*ProgressStage, len(stages)),
		OverallProgress: 0.0,
		Metadata:        make(map[string]interface{}),
		Callbacks:       make([]ProgressCallback, 0),
	}

	// Initialize stages
	for i, stageName := range stages {
		session.Stages[i] = &ProgressStage{
			ID:          fmt.Sprintf("stage_%d", i),
			Name:        stageName,
			Description: fmt.Sprintf("Processing %s", stageName),
			Status:      ProgressStatusPending,
			Progress:    0.0,
			Substeps:    make([]*Substep, 0),
			Metrics:     StageMetrics{LastUpdateTime: time.Now()},
		}
	}

	if len(session.Stages) > 0 {
		session.CurrentStage = session.Stages[0]
	}

	pr.mu.Lock()
	pr.sessions[sessionID] = session
	pr.mu.Unlock()

	pr.log.WithFields(logrus.Fields{
		"session_id": sessionID,
		"title":      title,
		"stages":     len(stages),
	}).Info("Progress session started")

	return session
}

// StartStage starts a specific stage in the session
func (ps *ProgressSession) StartStage(stageID string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stage := ps.findStage(stageID)
	if stage == nil {
		return fmt.Errorf("stage %s not found", stageID)
	}

	now := time.Now()
	stage.StartTime = &now
	stage.Status = ProgressStatusInProgress
	ps.CurrentStage = stage

	ps.notifyCallbacks()
	return nil
}

// UpdateStageProgress updates progress for a specific stage
func (ps *ProgressSession) UpdateStageProgress(stageID string, progress float64, details map[string]string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stage := ps.findStage(stageID)
	if stage == nil {
		return fmt.Errorf("stage %s not found", stageID)
	}

	stage.Progress = progress
	stage.Metrics.LastUpdateTime = time.Now()

	if details != nil {
		if stage.CurrentSubstep != nil {
			for k, v := range details {
				stage.CurrentSubstep.Details[k] = v
			}
		}
	}

	ps.calculateOverallProgress()
	ps.notifyCallbacks()
	return nil
}

// AddSubstep adds a substep to a stage
func (ps *ProgressSession) AddSubstep(stageID, substepName, description string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stage := ps.findStage(stageID)
	if stage == nil {
		return fmt.Errorf("stage %s not found", stageID)
	}

	substep := &Substep{
		ID:          fmt.Sprintf("substep_%d", len(stage.Substeps)),
		Name:        substepName,
		Description: description,
		Status:      ProgressStatusPending,
		Progress:    0.0,
		Details:     make(map[string]string),
	}

	stage.Substeps = append(stage.Substeps, substep)
	stage.CurrentSubstep = substep

	ps.notifyCallbacks()
	return nil
}

// UpdateSubstepProgress updates progress for a specific substep
func (ps *ProgressSession) UpdateSubstepProgress(stageID, substepID string, progress float64) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stage := ps.findStage(stageID)
	if stage == nil {
		return fmt.Errorf("stage %s not found", stageID)
	}

	substep := ps.findSubstep(stage, substepID)
	if substep == nil {
		return fmt.Errorf("substep %s not found in stage %s", substepID, stageID)
	}

	substep.Progress = progress
	if progress >= 1.0 {
		substep.Status = ProgressStatusCompleted
		now := time.Now()
		substep.EndTime = &now
		if substep.StartTime != nil {
			substep.Duration = now.Sub(*substep.StartTime)
		}
	} else {
		substep.Status = ProgressStatusInProgress
		if substep.StartTime == nil {
			now := time.Now()
			substep.StartTime = &now
		}
	}

	ps.calculateOverallProgress()
	ps.notifyCallbacks()
	return nil
}

// CompleteStage marks a stage as completed
func (ps *ProgressSession) CompleteStage(stageID string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stage := ps.findStage(stageID)
	if stage == nil {
		return fmt.Errorf("stage %s not found", stageID)
	}

	now := time.Now()
	stage.EndTime = &now
	stage.Status = ProgressStatusCompleted
	stage.Progress = 1.0

	if stage.StartTime != nil {
		stage.ActualTime = now.Sub(*stage.StartTime)
	}

	// Find next pending stage
	ps.CurrentStage = nil
	for _, nextStage := range ps.Stages {
		if nextStage.Status == ProgressStatusPending {
			ps.CurrentStage = nextStage
			break
		}
	}

	ps.calculateOverallProgress()
	ps.notifyCallbacks()
	return nil
}

// FailStage marks a stage as failed
func (ps *ProgressSession) FailStage(stageID string, errorMsg string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stage := ps.findStage(stageID)
	if stage == nil {
		return fmt.Errorf("stage %s not found", stageID)
	}

	now := time.Now()
	stage.EndTime = &now
	stage.Status = ProgressStatusFailed
	stage.ErrorMessage = errorMsg

	if stage.StartTime != nil {
		stage.ActualTime = now.Sub(*stage.StartTime)
	}

	ps.Status = ProgressStatusFailed
	ps.notifyCallbacks()
	return nil
}

// CompleteSession marks the entire session as completed
func (ps *ProgressSession) CompleteSession() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now()
	ps.EndTime = &now
	ps.Status = ProgressStatusCompleted
	ps.OverallProgress = 1.0

	ps.notifyCallbacks()
}

// UpdateMetrics updates metrics for a specific stage
func (ps *ProgressSession) UpdateMetrics(stageID string, processed, total int, errors, warnings int) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stage := ps.findStage(stageID)
	if stage == nil {
		return fmt.Errorf("stage %s not found", stageID)
	}

	stage.Metrics.ItemsProcessed = processed
	stage.Metrics.ItemsTotal = total
	stage.Metrics.ErrorCount = errors
	stage.Metrics.WarningCount = warnings
	stage.Metrics.LastUpdateTime = time.Now()

	// Calculate processing rate
	if stage.StartTime != nil {
		elapsed := time.Since(*stage.StartTime)
		if elapsed.Seconds() > 0 {
			stage.Metrics.ProcessingRate = float64(processed) / elapsed.Seconds()
		}
	}

	ps.notifyCallbacks()
	return nil
}

// AddCallback adds a progress callback
func (ps *ProgressSession) AddCallback(callback ProgressCallback) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.Callbacks = append(ps.Callbacks, callback)
}

// GetSnapshot returns a snapshot of current progress
func (ps *ProgressSession) GetSnapshot() *ProgressSnapshot {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	snapshot := &ProgressSnapshot{
		SessionID:       ps.ID,
		Title:           ps.Title,
		Status:          ps.Status,
		OverallProgress: ps.OverallProgress,
		ElapsedTime:     time.Since(ps.StartTime),
		RecentActivity:  make([]string, 0),
	}

	if ps.CurrentStage != nil {
		snapshot.CurrentStage = ps.CurrentStage.Name
	}

	// Calculate estimated remaining time
	if ps.OverallProgress > 0 && ps.OverallProgress < 1.0 {
		elapsedTime := time.Since(ps.StartTime)
		totalEstimated := time.Duration(float64(elapsedTime) / ps.OverallProgress)
		remaining := totalEstimated - elapsedTime
		snapshot.EstimatedRemaining = &remaining
	}

	// Collect recent activity
	for _, stage := range ps.Stages {
		if stage.Status == ProgressStatusInProgress || stage.Status == ProgressStatusCompleted {
			activity := fmt.Sprintf("%s: %.1f%%", stage.Name, stage.Progress*100)
			snapshot.RecentActivity = append(snapshot.RecentActivity, activity)
		}
	}

	// Calculate performance metrics
	snapshot.Performance = ps.calculatePerformanceMetrics()

	return snapshot
}

// GetSession retrieves a session by ID
func (pr *ProgressReporter) GetSession(sessionID string) *ProgressSession {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	return pr.sessions[sessionID]
}

// GetAllSessions returns all active sessions
func (pr *ProgressReporter) GetAllSessions() []*ProgressSession {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	sessions := make([]*ProgressSession, 0, len(pr.sessions))
	for _, session := range pr.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// CleanupSession removes a completed session
func (pr *ProgressReporter) CleanupSession(sessionID string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	delete(pr.sessions, sessionID)
}

// CleanupOldSessions removes sessions older than the specified duration
func (pr *ProgressReporter) CleanupOldSessions(maxAge time.Duration) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for id, session := range pr.sessions {
		if session.EndTime != nil && session.EndTime.Before(cutoff) {
			delete(pr.sessions, id)
		}
	}
}

// Private helper methods

func (ps *ProgressSession) findStage(stageID string) *ProgressStage {
	for _, stage := range ps.Stages {
		if stage.ID == stageID {
			return stage
		}
	}
	return nil
}

func (ps *ProgressSession) findSubstep(stage *ProgressStage, substepID string) *Substep {
	for _, substep := range stage.Substeps {
		if substep.ID == substepID {
			return substep
		}
	}
	return nil
}

func (ps *ProgressSession) calculateOverallProgress() {
	if len(ps.Stages) == 0 {
		return
	}

	totalProgress := 0.0
	for _, stage := range ps.Stages {
		totalProgress += stage.Progress
	}
	ps.OverallProgress = totalProgress / float64(len(ps.Stages))
}

func (ps *ProgressSession) notifyCallbacks() {
	for _, callback := range ps.Callbacks {
		go callback(ps) // Non-blocking callback execution
	}
}

func (ps *ProgressSession) calculatePerformanceMetrics() ProgressPerformanceMetrics {
	metrics := ProgressPerformanceMetrics{}

	totalItems := 0
	totalProcessed := 0
	stageTimes := make([]time.Duration, 0)

	for _, stage := range ps.Stages {
		totalItems += stage.Metrics.ItemsTotal
		totalProcessed += stage.Metrics.ItemsProcessed

		if stage.ActualTime > 0 {
			stageTimes = append(stageTimes, stage.ActualTime)
		}
	}

	// Calculate items per second
	elapsed := time.Since(ps.StartTime)
	if elapsed.Seconds() > 0 {
		metrics.ItemsPerSecond = float64(totalProcessed) / elapsed.Seconds()
	}

	// Calculate average stage time
	if len(stageTimes) > 0 {
		var totalTime time.Duration
		for _, t := range stageTimes {
			totalTime += t
		}
		metrics.AverageStageTime = totalTime / time.Duration(len(stageTimes))
	}

	// Placeholder values for system metrics (would be collected from actual system)
	metrics.PeakMemoryUsage = 50 * 1024 * 1024 // 50MB placeholder
	metrics.CPUUtilization = 25.0              // 25% placeholder

	return metrics
}

// ProgressLoggerCallback creates a callback that logs progress updates
func ProgressLoggerCallback(log *logrus.Logger) ProgressCallback {
	return func(session *ProgressSession) {
		snapshot := session.GetSnapshot()

		log.WithFields(logrus.Fields{
			"session_id":       snapshot.SessionID,
			"overall_progress": fmt.Sprintf("%.1f%%", snapshot.OverallProgress*100),
			"current_stage":    snapshot.CurrentStage,
			"elapsed_time":     snapshot.ElapsedTime.String(),
			"items_per_second": snapshot.Performance.ItemsPerSecond,
		}).Info("Progress update")
	}
}

// ProgressMetricsCallback creates a callback that exports progress metrics
func ProgressMetricsCallback(metricsExporter func(string, float64)) ProgressCallback {
	return func(session *ProgressSession) {
		snapshot := session.GetSnapshot()

		metricsExporter(fmt.Sprintf("progress.%s.overall", session.ID), snapshot.OverallProgress)
		metricsExporter(fmt.Sprintf("progress.%s.items_per_second", session.ID), snapshot.Performance.ItemsPerSecond)

		for _, stage := range session.Stages {
			metricsExporter(fmt.Sprintf("progress.%s.stage.%s", session.ID, stage.ID), stage.Progress)
		}
	}
}
