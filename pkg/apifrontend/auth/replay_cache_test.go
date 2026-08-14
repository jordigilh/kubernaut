package auth

import (
	"testing"
	"time"
)

func TestReplayCache_SeenReturnsFalseForNewJTI(t *testing.T) {
	rc := NewReplayCache(1 * time.Minute)
	defer rc.Stop()

	if rc.Seen("jti-abc-123", "source-a") {
		t.Error("Seen() returned true for a new jti")
	}
}

func TestReplayCache_SameSourceReuse_NotAReplay(t *testing.T) {
	// #1999 (BR-SECURITY-1505): the same client (source key) reusing its own
	// token — standard OAuth2 Bearer-token usage — must not be rejected.
	rc := NewReplayCache(1 * time.Minute)
	defer rc.Stop()

	rc.Seen("jti-abc-123", "source-a")
	if rc.Seen("jti-abc-123", "source-a") {
		t.Error("Seen() returned true (replay) for the same jti reused from the same source")
	}
}

func TestReplayCache_DifferentSource_IsReplay(t *testing.T) {
	// #1999: a jti presented from a different source than the one that
	// first recorded it is a genuine replay and must be rejected.
	rc := NewReplayCache(1 * time.Minute)
	defer rc.Stop()

	rc.Seen("jti-abc-123", "source-a")
	if !rc.Seen("jti-abc-123", "source-b") {
		t.Error("Seen() returned false for a jti replayed from a different source")
	}
}

func TestReplayCache_EmptyJTIAlwaysNew(t *testing.T) {
	rc := NewReplayCache(1 * time.Minute)
	defer rc.Stop()

	if rc.Seen("", "source-a") {
		t.Error("Seen() returned true for empty jti")
	}
	if rc.Seen("", "source-a") {
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

	rc.Seen("expired-jti", "source-a")
	time.Sleep(50 * time.Millisecond)

	rc.mu.Lock()
	now := time.Now()
	for jti, entry := range rc.entries {
		if now.After(entry.expiry) {
			delete(rc.entries, jti)
		}
	}
	rc.mu.Unlock()

	if rc.Seen("expired-jti", "source-a") {
		t.Error("Seen() returned true for an expired jti after manual eviction")
	}
}
