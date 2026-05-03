/*
Copyright 2026 Jordi Gil.

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

package session

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-logr/logr"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// ErrCapacityExhausted is returned when the maximum number of concurrent
// investigations has been reached.
var ErrCapacityExhausted = errors.New("investigation capacity exhausted")

// InvestigateFunc is the function signature for running an investigation.
type InvestigateFunc func(ctx context.Context) (*katypes.InvestigationResult, error)

// Manager orchestrates investigation sessions, running each in a
// background goroutine and tracking progress via the Store.
type Manager struct {
	store       *Store
	logger      logr.Logger
	sem         chan struct{}
	wg          sync.WaitGroup
	shutdownCtx context.Context
	shutdownFn  context.CancelFunc
}

// NewManager creates a session manager backed by the given store.
// maxConcurrent limits the number of simultaneous investigations.
func NewManager(store *Store, logger logr.Logger, maxConcurrent ...int) *Manager {
	cap := 10
	if len(maxConcurrent) > 0 && maxConcurrent[0] > 0 {
		cap = maxConcurrent[0]
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		store:       store,
		logger:      logger,
		sem:         make(chan struct{}, cap),
		shutdownCtx: ctx,
		shutdownFn:  cancel,
	}
}

// StartInvestigation creates a new session and launches the investigation
// function in a background goroutine. Returns the session ID immediately.
// metadata is stored on the session for later retrieval (e.g., incident_id).
//
// Returns ErrCapacityExhausted if the maximum concurrent investigations cap
// has been reached. The goroutine uses context.Background() to ensure the
// investigation outlives the originating HTTP request.
func (m *Manager) StartInvestigation(_ context.Context, fn InvestigateFunc, metadata map[string]string) (string, error) {
	select {
	case m.sem <- struct{}{}:
	default:
		return "", ErrCapacityExhausted
	}

	id, err := m.store.Create()
	if err != nil {
		<-m.sem
		return "", err
	}
	if metadata != nil {
		m.store.SetMetadata(id, metadata)
	}
	m.updateSession(id, StatusRunning, nil, nil)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() { <-m.sem }()
		result, fnErr := fn(m.shutdownCtx)
		if fnErr != nil {
			m.logger.Error(fnErr, "investigation failed", "session_id", id)
			m.updateSession(id, StatusFailed, nil, fnErr)
			return
		}
		m.updateSession(id, StatusCompleted, result, nil)
	}()

	return id, nil
}

// GetSession retrieves the current state of an investigation session.
func (m *Manager) GetSession(id string) (*Session, error) {
	return m.store.Get(id)
}

// DrainAndWait signals all in-flight investigations to cancel and waits
// until they complete or the timeout expires.
func (m *Manager) DrainAndWait(timeout time.Duration) {
	m.shutdownFn()
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		m.logger.Info("all investigations drained")
	case <-time.After(timeout):
		m.logger.Error(nil, "drain timeout expired, some investigations may still be running",
			"timeout", timeout)
	}
}

func (m *Manager) updateSession(id string, status Status, result *katypes.InvestigationResult, err error) {
	if updateErr := m.store.Update(id, status, result, err); updateErr != nil {
		m.logger.Error(updateErr, "failed to update session",
			"session_id", id, "target_status", string(status))
	}
}
