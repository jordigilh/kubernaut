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

package hotreload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
)

// ReloadCallback is called when ConfigMap content changes.
// Return error to reject the new configuration (keeps previous).
// Per DD-INFRA-001: Graceful degradation on callback errors.
type ReloadCallback func(newContent string) error

// FileWatcher watches a mounted ConfigMap file and triggers
// callbacks when content changes. Provides graceful degradation
// on callback errors.
//
// Per DD-INFRA-001: ConfigMap Hot-Reload Pattern
// See: docs/architecture/decisions/DD-INFRA-001-configmap-hotreload-pattern.md
type FileWatcher struct {
	path     string         // Path to the mounted ConfigMap file
	callback ReloadCallback // Called when content changes
	logger   logr.Logger

	mu          sync.RWMutex
	lastContent string
	lastHash    string
	lastReload  time.Time
	reloadCount int64
	errorCount  int64

	watcher *fsnotify.Watcher
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewFileWatcher creates a new file-based hot-reloader for a specific ConfigMap key.
//
// Parameters:
//   - path: Path to the mounted ConfigMap file (e.g., "/etc/kubernaut/policies/priority.rego")
//   - callback: Function called when content changes; return error to reject new content
//   - logger: Logger for debugging and audit (DD-005 compliant)
//
// Per DD-INFRA-001: File-based approach using fsnotify.
func NewFileWatcher(path string, callback ReloadCallback, logger logr.Logger) (*FileWatcher, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if callback == nil {
		return nil, fmt.Errorf("callback is required")
	}

	return &FileWatcher{
		path:     path,
		callback: callback,
		logger:   logger.WithName("file-watcher").WithValues("path", path),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}, nil
}

// Start begins watching the file. This method:
//  1. Loads the initial file content
//  2. Starts the fsnotify watcher
//  3. Blocks until context is cancelled or Stop() is called
//
// Returns error if initial file load fails or watcher cannot be created.
func (w *FileWatcher) Start(ctx context.Context) error {
	// Load initial content
	if err := w.loadInitial(ctx); err != nil {
		return fmt.Errorf("failed to load initial content: %w", err)
	}

	// Create fsnotify watcher
	var err error
	w.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	// Watch the directory (ConfigMap mounts use symlinks)
	// Per DD-INFRA-001: Watch directory, not file directly
	dir := filepath.Dir(w.path)
	if err := w.watcher.Add(dir); err != nil {
		if closeErr := w.watcher.Close(); closeErr != nil {
			w.logger.Error(closeErr, "Failed to close watcher after Add error")
		}
		return fmt.Errorf("failed to watch directory %s: %w", dir, err)
	}

	w.logger.Info("File watcher started",
		"hash", w.lastHash,
		"directory", dir)

	// Start watch loop in background
	go w.watchLoop(ctx)

	return nil
}

// Stop gracefully stops the file watcher.
func (w *FileWatcher) Stop() {
	close(w.stopCh)
	<-w.doneCh // Wait for watchLoop to finish

	if w.watcher != nil {
		if err := w.watcher.Close(); err != nil {
			w.logger.Error(err, "Failed to close watcher during Stop")
		}
	}

	w.logger.Info("File watcher stopped",
		"totalReloads", w.reloadCount,
		"totalErrors", w.errorCount)
}

// GetLastHash returns the hash of the currently active configuration.
func (w *FileWatcher) GetLastHash() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastHash
}

// GetLastReloadTime returns when configuration was last successfully reloaded.
func (w *FileWatcher) GetLastReloadTime() time.Time {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastReload
}

// GetReloadCount returns total successful reloads since start.
func (w *FileWatcher) GetReloadCount() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.reloadCount
}

// GetErrorCount returns total failed reload attempts since start.
func (w *FileWatcher) GetErrorCount() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.errorCount
}

// GetLastContent returns the currently active configuration content.
func (w *FileWatcher) GetLastContent() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastContent
}

// loadInitial loads the file content at startup.
func (w *FileWatcher) loadInitial(ctx context.Context) error {
	content, err := os.ReadFile(w.path)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", w.path, err)
	}

	// Execute callback with initial content
	if err := w.callback(string(content)); err != nil {
		return fmt.Errorf("callback failed for initial content: %w", err)
	}

	// Store initial state
	w.mu.Lock()
	w.lastContent = string(content)
	w.lastHash = computeHash(content)
	w.lastReload = time.Now()
	w.reloadCount = 1
	w.mu.Unlock()

	w.logger.Info("Initial content loaded",
		"hash", w.lastHash,
		"size", len(content))

	return nil
}

// watchLoop runs the fsnotify event loop.
func (w *FileWatcher) watchLoop(ctx context.Context) {
	defer close(w.doneCh)

	filename := filepath.Base(w.path)

	for {
		select {
		case <-ctx.Done():
			w.logger.V(1).Info("Context cancelled, stopping watch loop")
			return

		case <-w.stopCh:
			w.logger.V(1).Info("Stop requested, stopping watch loop")
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				w.logger.V(1).Info("Watcher events channel closed")
				return
			}

			// Only process events for our target file
			// ConfigMap mounts use symlinks, so we watch for CREATE events on the symlink
			if filepath.Base(event.Name) == filename ||
				event.Has(fsnotify.Create) && filepath.Base(event.Name) == "..data" {
				w.logger.V(1).Info("File change detected",
					"event", event.Op.String(),
					"name", event.Name)
				w.handleFileChange()
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				w.logger.V(1).Info("Watcher errors channel closed")
				return
			}
			w.logger.Error(err, "Watcher error")
		}
	}
}

// handleFileChange processes a file change event.
func (w *FileWatcher) handleFileChange() {
	// Small delay to allow file write to complete
	// ConfigMap updates involve symlink changes
	time.Sleep(100 * time.Millisecond)

	content, err := os.ReadFile(w.path)
	if err != nil {
		w.mu.Lock()
		w.errorCount++
		w.mu.Unlock()
		w.logger.Error(err, "Failed to read file after change event")
		return
	}

	newHash := computeHash(content)

	w.mu.RLock()
	currentHash := w.lastHash
	w.mu.RUnlock()

	// Skip if content hasn't actually changed
	if newHash == currentHash {
		w.logger.V(1).Info("Hash unchanged, skipping reload")
		return
	}

	// Execute callback with new content
	if err := w.callback(string(content)); err != nil {
		w.mu.Lock()
		w.errorCount++
		w.mu.Unlock()
		w.logger.Error(err, "Callback rejected new content, keeping previous",
			"newHash", newHash)
		return
	}

	// Update state on success
	w.mu.Lock()
	w.lastContent = string(content)
	w.lastHash = newHash
	w.lastReload = time.Now()
	w.reloadCount++
	w.mu.Unlock()

	w.logger.Info("Configuration reloaded successfully",
		"hash", newHash,
		"size", len(content),
		"totalReloads", w.reloadCount)
}

// computeHash calculates SHA256 hash of content.
// Returns full 64-character hash for audit trail and policy version tracking (BR-SP-072).
func computeHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:]) // Full SHA256 hash for audit compliance
}
