package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/go-logr/logr"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
)

const debounceDuration = 200 * time.Millisecond

// maxConfigSize is the maximum allowed config file size (1 MiB).
// Prevents OOM from rogue ConfigMap mounts.
const maxConfigSize = 1 << 20

// ReloadCallback is called when ConfigMap content changes.
// Return error to reject the new configuration (keeps previous).
type ReloadCallback func(newContent []byte) error

// FileWatcher watches a mounted ConfigMap file and triggers callbacks on change.
// Adapted from kubernaut DD-INFRA-001 pattern (pkg/shared/hotreload).
//
// CM-3(5) limitation (accepted): The current implementation is VALIDATE-ONLY.
// On config change the callback parses and validates the new YAML, but the
// running process keeps the original startup config. Full hot-apply would
// require coordinated updates to the HTTP listener, auth pipeline, and rate
// limiter — deferred to v1.6 (issue #1450). The audit trail still records
// every accepted or rejected reload for FedRAMP CM-3.
//
// Kubernetes ConfigMap volume mounts use a "..data" symlink that is atomically
// swapped on update. The watcher monitors the parent directory and detects
// both direct file writes and symlink rotation (CREATE on "..data").
type FileWatcher struct {
	path     string
	callback ReloadCallback
	logger   logr.Logger
	auditor  audit.Emitter

	mu          sync.RWMutex
	lastContent []byte
	lastHash    string
	lastReload  time.Time

	started  atomic.Bool
	stopOnce sync.Once
	watcher  *fsnotify.Watcher
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewFileWatcher creates a new file-based hot-reloader.
func NewFileWatcher(path string, callback ReloadCallback, opts ...FileWatcherOption) (*FileWatcher, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if callback == nil {
		return nil, fmt.Errorf("callback is required")
	}
	fw := &FileWatcher{
		path:     path,
		callback: callback,
		logger:   logr.Discard(),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
	for _, o := range opts {
		o(fw)
	}
	return fw, nil
}

// FileWatcherOption configures a FileWatcher.
type FileWatcherOption func(*FileWatcher)

// WithLogger sets the logger for the FileWatcher.
func WithLogger(l logr.Logger) FileWatcherOption {
	return func(fw *FileWatcher) {
		if l.GetSink() != nil {
			fw.logger = l
		}
	}
}

// WithAuditor sets the audit emitter for FedRAMP AU-2 compliance.
func WithAuditor(e audit.Emitter) FileWatcherOption {
	return func(fw *FileWatcher) {
		fw.auditor = e
	}
}

// Start begins watching the file. Loads initial content, then watches for changes.
// Not safe to call after Stop — returns an error in that case.
func (w *FileWatcher) Start(ctx context.Context) error {
	select {
	case <-w.stopCh:
		return fmt.Errorf("file watcher already stopped — cannot restart")
	default:
	}
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

	w.started.Store(true)
	w.logger.Info("config hot-reload is VALIDATE-ONLY — config changes are validated but not applied at runtime (issue #1450, deferred to v1.6)")
	go w.watchLoop(ctx)
	return nil
}

// Stop gracefully stops the file watcher.
// Safe to call multiple times or before Start.
func (w *FileWatcher) Stop() {
	w.stopOnce.Do(func() { close(w.stopCh) })
	if w.started.Load() {
		<-w.doneCh
	}
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

func (w *FileWatcher) readFileLimited() ([]byte, error) {
	f, err := os.Open(w.path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	content, err := io.ReadAll(io.LimitReader(f, maxConfigSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maxConfigSize {
		return nil, fmt.Errorf("config file exceeds maximum size (%d bytes)", maxConfigSize)
	}
	return content, nil
}

func (w *FileWatcher) loadInitial() error {
	content, err := w.readFileLimited()
	if err != nil {
		return fmt.Errorf("read file %s: %w", w.path, err)
	}

	if err := w.callback(content); err != nil {
		return fmt.Errorf("callback rejected initial content: %w", err)
	}

	hash := computeHash(content)
	w.mu.Lock()
	w.lastContent = content
	w.lastHash = hash
	w.lastReload = time.Now()
	w.mu.Unlock()

	if w.auditor != nil {
		w.auditor.Emit(context.Background(), &audit.Event{
			Type: audit.EventConfigReloaded,
			Detail: map[string]string{
				"path":           w.path,
				"config_version": hash,
				"trigger":        "initial_load",
			},
		})
	}
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
			w.handleFileChange(ctx)
		case watchErr, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Info("WARNING: fsnotify error", "path", w.path, "error", watchErr)
		}
	}
}

func (w *FileWatcher) handleFileChange(ctx context.Context) {
	content, err := w.readFileLimited()
	if err != nil {
		w.logger.Info("WARNING: config reload: read file failed", "path", w.path, "error", err)
		if w.auditor != nil {
			w.auditor.Emit(ctx, &audit.Event{
				Type: audit.EventConfigRejected,
				Detail: map[string]string{
					"path":             w.path,
					"rejection_reason": security.RedactError(fmt.Errorf("read failed: %w", err)),
				},
			})
		}
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
		w.logger.Info("WARNING: config reload: callback rejected new content", "path", w.path, "error", security.RedactError(err))
		if w.auditor != nil {
			w.auditor.Emit(ctx, &audit.Event{
				Type: audit.EventConfigRejected,
				Detail: map[string]string{
					"path":             w.path,
					"config_version":   newHash,
					"rejection_reason": security.RedactError(err),
				},
			})
		}
		return
	}

	w.logger.Info("config validated but NOT applied (validate-only mode, issue #1450)", "path", w.path, "hash", newHash)
	if w.auditor != nil {
		w.auditor.Emit(ctx, &audit.Event{
			Type: audit.EventConfigReloaded,
			Detail: map[string]string{
				"path":           w.path,
				"config_version": newHash,
			},
		})
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
