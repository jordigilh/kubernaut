package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
)

// GAP-08 (kubernaut#1505): buildReplayCache selects the distributed Valkey
// backend when configured, and must fall back to the in-memory cache rather
// than disabling replay protection outright when the backend is unreachable.

func TestBuildReplayCache_NilConfigLegacyDisabled_ReturnsNil(t *testing.T) {
	rc := buildReplayCache(nil, false, logr.Discard())
	if rc != nil {
		t.Errorf("expected nil replay cache when config is nil and legacy flag disabled, got %T", rc)
	}
}

func TestBuildReplayCache_NilConfigLegacyEnabled_ReturnsInMemory(t *testing.T) {
	rc := buildReplayCache(nil, true, logr.Discard())
	defer rc.Stop()
	if _, ok := rc.(*auth.ReplayCache); !ok {
		t.Errorf("expected legacy enableReplayProtection=true to construct an in-memory ReplayCache, got %T", rc)
	}
}

func TestBuildReplayCache_MemoryBackend_ReturnsInMemory(t *testing.T) {
	rc := buildReplayCache(&config.ReplayCacheConfig{Backend: "memory"}, false, logr.Discard())
	defer rc.Stop()
	if _, ok := rc.(*auth.ReplayCache); !ok {
		t.Errorf("expected backend=memory to construct an in-memory ReplayCache, got %T", rc)
	}
}

func TestBuildReplayCache_RedisBackend_ReturnsValkeyReplayCache(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rc := buildReplayCache(&config.ReplayCacheConfig{
		Backend:   "redis",
		RedisAddr: mr.Addr(),
	}, false, logr.Discard())
	defer rc.Stop()

	if _, ok := rc.(*auth.ValkeyReplayCache); !ok {
		t.Errorf("expected backend=redis to construct a ValkeyReplayCache, got %T", rc)
	}
	// Sanity check the constructed cache is actually wired to the running instance.
	if rc.Seen("jti-wiring-check") {
		t.Error("expected a fresh jti to not be seen yet")
	}
	if !rc.Seen("jti-wiring-check") {
		t.Error("expected the same jti to be detected as seen on the second call")
	}
}

func TestBuildReplayCache_RedisBackendUnreachable_FallsBackToInMemory(t *testing.T) {
	rc := buildReplayCache(&config.ReplayCacheConfig{
		Backend:   "redis",
		RedisAddr: "127.0.0.1:1", // nothing listens here — connection refused
	}, false, logr.Discard())
	defer rc.Stop()

	if _, ok := rc.(*auth.ReplayCache); !ok {
		t.Errorf("expected an unreachable redis backend to fall back to in-memory ReplayCache, got %T", rc)
	}
}

func TestLoadReplayCachePassword_EmptyPath_ReturnsEmptyNoError(t *testing.T) {
	password, err := loadReplayCachePassword("")
	if err != nil {
		t.Fatalf("unexpected error for empty path: %v", err)
	}
	if password != "" {
		t.Errorf("expected empty password for empty path, got %q", password)
	}
}

func TestLoadReplayCachePassword_ValidFile_ReturnsPassword(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "valkey-secrets.yaml")
	if err := os.WriteFile(path, []byte("password: s3cr3t\n"), 0o600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	password, err := loadReplayCachePassword(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if password != "s3cr3t" {
		t.Errorf("expected password %q, got %q", "s3cr3t", password)
	}
}

func TestLoadReplayCachePassword_MissingPasswordKey_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "valkey-secrets.yaml")
	if err := os.WriteFile(path, []byte("username: admin\n"), 0o600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	if _, err := loadReplayCachePassword(path); err == nil {
		t.Error("expected an error when the password key is missing")
	}
}

func TestLoadReplayCachePassword_MissingFile_ReturnsError(t *testing.T) {
	if _, err := loadReplayCachePassword(filepath.Join(t.TempDir(), "does-not-exist.yaml")); err == nil {
		t.Error("expected an error for a missing credentials file")
	}
}

func TestBuildReplayCache_UnauthenticatedValkeyStillWorks(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rc := buildReplayCache(&config.ReplayCacheConfig{
		Backend:   "redis",
		RedisAddr: mr.Addr(),
		RedisDB:   0,
	}, false, logr.Discard())
	defer rc.Stop()

	if rc.MissingJTI("") != true {
		t.Error("expected MissingJTI(\"\") to be true")
	}
}
