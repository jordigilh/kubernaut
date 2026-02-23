package credentials

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
)

// Resolver resolves named credential references to their secret values
// by reading files from a configured credentials directory.
// BR-NOT-104-001: Named Credential Resolution
// BR-NOT-104-002: Credential Hot-Reload via fsnotify
type Resolver struct {
	credentialsDir string
	mu             sync.RWMutex
	cache          map[string]string
	logger         logr.Logger

	watcher    *fsnotify.Watcher
	watchDoneCh chan struct{}
}

// NewResolver creates a new credential resolver that reads credential files
// from the given directory. Each file's name is the credential name and its
// content (trimmed) is the secret value.
func NewResolver(dir string, logger logr.Logger) (*Resolver, error) {
	if dir == "" {
		return nil, fmt.Errorf("credentials directory path is required")
	}
	r := &Resolver{
		credentialsDir: dir,
		cache:          make(map[string]string),
		logger:         logger.WithName("credential-resolver"),
	}
	if err := r.Reload(); err != nil {
		return nil, fmt.Errorf("initial credential load failed: %w", err)
	}
	return r, nil
}

// Resolve returns the credential value for the given name.
// Returns an error if the name is empty or the credential is not found in the cache.
func (r *Resolver) Resolve(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("credential name is required")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	value, ok := r.cache[name]
	if !ok {
		return "", fmt.Errorf("credential %q not found in %s", name, r.credentialsDir)
	}
	return value, nil
}

// Count returns the number of credentials currently cached.
func (r *Resolver) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.cache)
}

// Reload re-reads all credential files from the directory, replacing the cache.
func (r *Resolver) Reload() error {
	entries, err := os.ReadDir(r.credentialsDir)
	if err != nil {
		return fmt.Errorf("failed to read credentials directory %s: %w", r.credentialsDir, err)
	}

	newCache := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip hidden files (e.g., Kubernetes symlink markers like ..data)
		if strings.HasPrefix(name, ".") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(r.credentialsDir, name))
		if err != nil {
			r.logger.Error(err, "Failed to read credential file", "name", name)
			continue
		}
		newCache[name] = strings.TrimSpace(string(content))
	}

	r.mu.Lock()
	r.cache = newCache
	r.mu.Unlock()

	r.logger.V(1).Info("Credentials reloaded", "count", len(newCache))
	return nil
}

// ValidateRefs checks that all credential references can be resolved.
// Returns an error listing ALL unresolvable references.
func (r *Resolver) ValidateRefs(refs []string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var missing []string
	for _, ref := range refs {
		if _, ok := r.cache[ref]; !ok {
			missing = append(missing, ref)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%d unresolvable credential ref(s): %s", len(missing), strings.Join(missing, ", "))
	}
	return nil
}

// StartWatching begins watching the credentials directory for changes using fsnotify.
// When files change, the cache is automatically reloaded.
// BR-NOT-104-002: Credential hot-reload via fsnotify.
// Uses the Kubernetes projected volume pattern: watches the directory for any file events.
func (r *Resolver) StartWatching(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	if err := watcher.Add(r.credentialsDir); err != nil {
		_ = watcher.Close()
		return fmt.Errorf("failed to watch credentials directory %s: %w", r.credentialsDir, err)
	}

	r.watcher = watcher
	r.watchDoneCh = make(chan struct{})

	go r.watchLoop(ctx)

	r.logger.Info("Credential directory watcher started", "dir", r.credentialsDir)
	return nil
}

func (r *Resolver) watchLoop(ctx context.Context) {
	defer close(r.watchDoneCh)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-r.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || event.Has(fsnotify.Remove) {
				r.logger.V(1).Info("Credential file change detected", "event", event.Op.String(), "name", event.Name)
				if err := r.Reload(); err != nil {
					r.logger.Error(err, "Failed to reload credentials after file change")
				}
			}
		case err, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
			r.logger.Error(err, "Credential watcher error")
		}
	}
}

// Close stops the resolver and releases resources.
func (r *Resolver) Close() error {
	if r.watcher != nil {
		if err := r.watcher.Close(); err != nil {
			return fmt.Errorf("failed to close credential watcher: %w", err)
		}
		if r.watchDoneCh != nil {
			<-r.watchDoneCh
		}
	}
	return nil
}
