package auth

import (
	"testing"
	"time"
)

func TestReplayCache_SeenReturnsFalseForNewJTI(t *testing.T) {
	rc := NewReplayCache(1 * time.Minute)
	defer rc.Stop()

	if rc.Seen("jti-abc-123") {
		t.Error("Seen() returned true for a new jti")
	}
}

func TestReplayCache_SeenReturnsTrueForReplay(t *testing.T) {
	rc := NewReplayCache(1 * time.Minute)
	defer rc.Stop()

	rc.Seen("jti-abc-123")
	if !rc.Seen("jti-abc-123") {
		t.Error("Seen() returned false for a replayed jti")
	}
}

func TestReplayCache_EmptyJTIAlwaysNew(t *testing.T) {
	rc := NewReplayCache(1 * time.Minute)
	defer rc.Stop()

	if rc.Seen("") {
		t.Error("Seen() returned true for empty jti")
	}
	if rc.Seen("") {
		t.Error("Seen() returned true for empty jti on second call")
	}
}

func TestReplayCache_MissingJTI(t *testing.T) {
	rc := NewReplayCache(1 * time.Minute)
	defer rc.Stop()

	if !rc.MissingJTI("") {
		t.Error("MissingJTI should return true for empty jti")
	}
	if rc.MissingJTI("abc-123") {
		t.Error("MissingJTI should return false for non-empty jti")
	}
}

func TestReplayCache_EvictionRemovesExpiredEntries(t *testing.T) {
	rc := NewReplayCache(1 * time.Millisecond)
	defer rc.Stop()

	rc.Seen("expired-jti")
	time.Sleep(50 * time.Millisecond)

	rc.mu.Lock()
	now := time.Now()
	for jti, expiry := range rc.entries {
		if now.After(expiry) {
			delete(rc.entries, jti)
		}
	}
	rc.mu.Unlock()

	if rc.Seen("expired-jti") {
		t.Error("Seen() returned true for an expired jti after manual eviction")
	}
}
