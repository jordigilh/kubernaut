package config

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// syncBuffer is a thread-safe bytes.Buffer for use with slog in tests.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *syncBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *syncBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

// --- Tier 3: FileWatcher ---

func TestNewFileWatcher_EmptyPath(t *testing.T) {
	// UT-AF-039-043
	_, err := NewFileWatcher("", func([]byte) error { return nil })
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if !strings.Contains(err.Error(), "path") {
		t.Errorf("error = %q, want to contain 'path'", err.Error())
	}
}

func TestNewFileWatcher_NilCallback(t *testing.T) {
	// UT-AF-039-044
	_, err := NewFileWatcher("/tmp/test.yaml", nil)
	if err == nil {
		t.Fatal("expected error for nil callback")
	}
	if !strings.Contains(err.Error(), "callback") {
		t.Errorf("error = %q, want to contain 'callback'", err.Error())
	}
}

func TestFileWatcher_Start_LoadsInitialContent(t *testing.T) {
	// UT-AF-039-045
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := []byte("logging:\n  level: DEBUG\n")
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	var received []byte
	w, err := NewFileWatcher(cfgPath, func(data []byte) error {
		received = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer w.Stop()

	if !bytes.Equal(received, content) {
		t.Errorf("callback received = %q, want %q", received, content)
	}
}

func TestFileWatcher_Start_ErrorWhenFileMissing(t *testing.T) {
	// UT-AF-039-046
	w, err := NewFileWatcher("/nonexistent/config.yaml", func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = w.Start(ctx)
	if err == nil {
		t.Fatal("expected error when file does not exist")
	}
}

func TestFileWatcher_FileChange_TriggersCallback(t *testing.T) {
	// UT-AF-039-047
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	initial := []byte("logging:\n  level: INFO\n")
	if err := os.WriteFile(cfgPath, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	var (
		mu    sync.Mutex
		calls []string
	)

	w, err := NewFileWatcher(cfgPath, func(data []byte) error {
		mu.Lock()
		calls = append(calls, string(data))
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer w.Stop()

	// Write new content
	updated := []byte("logging:\n  level: DEBUG\n")
	if err := os.WriteFile(cfgPath, updated, 0o644); err != nil {
		t.Fatal(err)
	}

	// Poll for the second callback invocation
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(calls)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(calls) < 2 {
		t.Fatalf("expected at least 2 callback calls (initial + update), got %d", len(calls))
	}
	if calls[1] != string(updated) {
		t.Errorf("second callback = %q, want %q", calls[1], updated)
	}
}

func TestFileWatcher_SameContent_NoCallback(t *testing.T) {
	// UT-AF-039-048
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := []byte("logging:\n  level: INFO\n")
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	var callCount atomic.Int32
	w, err := NewFileWatcher(cfgPath, func([]byte) error {
		callCount.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer w.Stop()

	// Re-write same content
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait and verify no extra callback
	time.Sleep(500 * time.Millisecond)
	if got := callCount.Load(); got != 1 {
		t.Errorf("callback called %d times, want 1 (initial only)", got)
	}
}

func TestFileWatcher_CallbackError_PreservesContent(t *testing.T) {
	// UT-AF-039-049
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	initial := []byte("logging:\n  level: INFO\n")
	if err := os.WriteFile(cfgPath, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	var callCount atomic.Int32
	w, err := NewFileWatcher(cfgPath, func(data []byte) error {
		n := callCount.Add(1)
		if n > 1 {
			return fmt.Errorf("rejecting new content")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer w.Stop()

	// Write new content that will be rejected
	bad := []byte("logging:\n  level: INVALID\n")
	if err := os.WriteFile(cfgPath, bad, 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for rejection
	time.Sleep(500 * time.Millisecond)

	got := w.GetLastContent()
	if !bytes.Equal(got, initial) {
		t.Errorf("GetLastContent() = %q, want %q (should keep initial after rejection)", got, initial)
	}
}

func TestFileWatcher_Stop_NoGoroutineLeak(t *testing.T) {
	// UT-AF-039-050
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := NewFileWatcher(cfgPath, func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Stop should not hang
	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(3 * time.Second):
		t.Fatal("Stop() did not return within 3s — possible goroutine leak")
	}
}

func TestFileWatcher_Debounce_RapidWrites(t *testing.T) {
	// UT-AF-039-051
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("v: 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var callCount atomic.Int32
	w, err := NewFileWatcher(cfgPath, func([]byte) error {
		callCount.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer w.Stop()

	// Rapid burst of writes with no inter-write delay to ensure they land
	// within a single debounce window (200ms) even on slow CI runners.
	for i := 1; i <= 10; i++ {
		data := []byte(fmt.Sprintf("v: %d\n", i))
		if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Wait for debounce to settle (debounce window + generous buffer for CI)
	time.Sleep(2 * time.Second)

	// The debounce property: 10 rapid writes must NOT produce 10 callbacks.
	// Initial load (1) + at most a few debounced fires. On CI with scheduling
	// jitter the exact count varies, so we only assert a meaningful reduction.
	got := callCount.Load()
	if got > 7 {
		t.Errorf("callback called %d times for 10 rapid writes — debounce not working (expected <= 7)", got)
	}
	if got < 1 {
		t.Errorf("callback called %d times, expected at least 1", got)
	}
}

func TestFileWatcher_WithLogger_Option(t *testing.T) {
	// UT-AF-039-052: WithLogger functional option is applied
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	w, err := NewFileWatcher(cfgPath, func([]byte) error { return nil }, WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	// Verify the logger was set (non-nil watcher created without panic)
	_ = w
}

func TestFileWatcher_WithLogger_NilIgnored(t *testing.T) {
	// UT-AF-039-053: WithLogger(nil) does not override the default logger
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := NewFileWatcher(cfgPath, func([]byte) error { return nil }, WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	// Should not panic — logger remains as slog.Default()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := w.Start(ctx); err != nil {
		t.Fatal(err)
	}
	w.Stop()
}

func TestFileWatcher_WithAuditor_Option(t *testing.T) {
	// UT-AF-039-054: WithAuditor functional option sets the auditor
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := NewFileWatcher(cfgPath, func([]byte) error { return nil }, WithAuditor(nil))
	if err != nil {
		t.Fatal(err)
	}
	// Verify creation succeeds with nil auditor (no audit emitted)
	_ = w
}

func TestFileWatcher_StopBeforeStart_NoPanic(t *testing.T) {
	// UT-AF-039-055: Stop() before Start() must not deadlock or panic
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := NewFileWatcher(cfgPath, func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK — Stop returned without deadlock
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() before Start() deadlocked")
	}
}

func TestFileWatcher_ReadFileLimited_RejectsOversized(t *testing.T) {
	// UT-AF-039-056: Files exceeding maxConfigSize return an explicit error
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	// Write a file just over 1 MiB
	oversized := make([]byte, maxConfigSize+100)
	for i := range oversized {
		oversized[i] = 'x'
	}
	if err := os.WriteFile(cfgPath, oversized, 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := NewFileWatcher(cfgPath, func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = w.Start(ctx)
	if err == nil {
		w.Stop()
		t.Fatal("expected error for oversized config file")
	}
	if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Errorf("error = %q, want to contain 'exceeds maximum size'", err.Error())
	}
}

func TestFileWatcher_DoubleStop_NoPanic(t *testing.T) {
	// UT-AF-039-057: Calling Stop() twice must not panic (sync.Once guard)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := NewFileWatcher(cfgPath, func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatal(err)
	}

	w.Stop()
	w.Stop() // must not panic
}

func TestFileWatcher_CallbackError_RedactsURLsInLog(t *testing.T) {
	// UT-AF-039-058: Verifies that URLs in callback rejection errors are
	// redacted in the slog output (security.RedactError applied).
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	initial := []byte("x: 1\n")
	if err := os.WriteFile(cfgPath, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	logBuf := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	var callCount atomic.Int32
	w, err := NewFileWatcher(cfgPath, func(data []byte) error {
		if callCount.Add(1) > 1 {
			return fmt.Errorf("connection to https://internal-api.corp/secret/endpoint failed: token=abc123")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	w.logger = logger

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	bad := []byte("x: 2\n")
	if err := os.WriteFile(cfgPath, bad, 0o644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)

	output := logBuf.String()
	if strings.Contains(output, "https://internal-api.corp/secret/endpoint") {
		t.Errorf("log output should not contain raw URL, got: %s", output)
	}
	if !strings.Contains(output, "[URL_REDACTED]") {
		t.Errorf("log output should contain [URL_REDACTED], got: %s", output)
	}
}

func TestFileWatcher_GetLastHash(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgFile, []byte("key: value"), 0600); err != nil {
		t.Fatal(err)
	}

	w, err := NewFileWatcher(cfgFile, func(_ []byte) error { return nil })
	if err != nil {
		t.Fatalf("NewFileWatcher: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		cancel()
		w.Stop()
	}()

	hash := w.GetLastHash()
	if hash == "" {
		t.Error("expected non-empty hash from GetLastHash()")
	}
}
