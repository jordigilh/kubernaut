package config

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
)

const debounceDuration = 200 * time.Millisecond

// ReloadCallback is called when ConfigMap content changes.
// Return error to reject the new configuration (keeps previous).
type ReloadCallback func(newContent []byte) error

// FileWatcher watches a mounted ConfigMap file and triggers callbacks on change.
// Adapted from kubernaut DD-INFRA-001 pattern (pkg/shared/hotreload).
type FileWatcher struct {
	path     string
	callback ReloadCallback

	mu          sync.RWMutex
	lastContent []byte
	lastHash    string
	lastReload  time.Time

	watcher *fsnotify.Watcher
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewFileWatcher creates a new file-based hot-reloader.
func NewFileWatcher(path string, callback ReloadCallback) (*FileWatcher, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if callback == nil {
		return nil, fmt.Errorf("callback is required")
	}
	return &FileWatcher{
		path:     path,
		callback: callback,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}, nil
}

// Start begins watching the file. Loads initial content, then watches for changes.
func (w *FileWatcher) Start(ctx context.Context) error {
	if err := w.loadInitial(); err != nil {
		return fmt.Errorf("initial load: %w", err)
	}

	var err error
	w.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create fsnotify watcher: %w", err)
	}

	dir := filepath.Dir(w.path)
	if err := w.watcher.Add(dir); err != nil {
		_ = w.watcher.Close()
		return fmt.Errorf("watch directory %s: %w", dir, err)
	}

	go w.watchLoop(ctx)
	return nil
}

// Stop gracefully stops the file watcher.
func (w *FileWatcher) Stop() {
	close(w.stopCh)
	<-w.doneCh
	if w.watcher != nil {
		_ = w.watcher.Close()
	}
}

// GetLastContent returns the currently active configuration content.
func (w *FileWatcher) GetLastContent() []byte {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastContent
}

// GetLastHash returns the SHA256 hash of the current content.
func (w *FileWatcher) GetLastHash() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastHash
}

func (w *FileWatcher) loadInitial() error {
	content, err := os.ReadFile(w.path)
	if err != nil {
		return fmt.Errorf("read file %s: %w", w.path, err)
	}

	if err := w.callback(content); err != nil {
		return fmt.Errorf("callback rejected initial content: %w", err)
	}

	w.mu.Lock()
	w.lastContent = content
	w.lastHash = computeHash(content)
	w.lastReload = time.Now()
	w.mu.Unlock()
	return nil
}

func (w *FileWatcher) watchLoop(ctx context.Context) {
	defer close(w.doneCh)

	filename := filepath.Base(w.path)
	var debounceTimer *time.Timer
	var debounceCh <-chan time.Time

	defer func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if filepath.Base(event.Name) == filename ||
				(event.Has(fsnotify.Create) && filepath.Base(event.Name) == "..data") {
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.NewTimer(debounceDuration)
				debounceCh = debounceTimer.C
			}
		case <-debounceCh:
			debounceCh = nil
			w.handleFileChange()
		case _, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (w *FileWatcher) handleFileChange() {
	content, err := os.ReadFile(w.path)
	if err != nil {
		return
	}

	newHash := computeHash(content)

	w.mu.RLock()
	currentHash := w.lastHash
	w.mu.RUnlock()

	if newHash == currentHash {
		return
	}

	if err := w.callback(content); err != nil {
		return
	}

	w.mu.Lock()
	w.lastContent = content
	w.lastHash = newHash
	w.lastReload = time.Now()
	w.mu.Unlock()
}

func computeHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}
