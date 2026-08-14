package auth

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/redis/go-redis/v9"
)

// ValkeyReplayCache is a distributed jti replay cache backed by Valkey/Redis.
// It closes the HA gap of the in-memory ReplayCache (GAP-08, kubernaut#1505):
// in a multi-replica APIFrontend deployment, all replicas share the same
// backing store, so a token replayed against a different replica than the one
// that first observed it is still detected.
type ValkeyReplayCache struct {
	client *redis.Client
	ttl    time.Duration
	logger logr.Logger
}

// NewValkeyReplayCache creates a distributed jti replay cache. ttl should
// match or exceed the maximum token lifetime, mirroring NewReplayCache. The
// caller owns the redis.Client lifecycle (created once in main.go and reused
// across the process).
func NewValkeyReplayCache(client *redis.Client, ttl time.Duration, logger logr.Logger) *ValkeyReplayCache {
	return &ValkeyReplayCache{client: client, ttl: ttl, logger: logger}
}

// MissingJTI mirrors ReplayCache.MissingJTI: true indicates the token lacks
// a jti claim needed for replay protection.
func (c *ValkeyReplayCache) MissingJTI(jti string) bool {
	return jti == ""
}

// Seen atomically records (jti, sourceKey) as observed and reports whether
// jti was already present from a *different* sourceKey — a replay attempt
// (#1999, BR-SECURITY-1505). A jti re-presented from the same sourceKey is
// legitimate Bearer-token reuse, not a replay, and returns false.
//
// Uses SET-NX-with-TTL, storing sourceKey as the value instead of a plain
// sentinel, so the first presentation is still a single round-trip. On a
// SetNX miss (jti already recorded), a follow-up GET compares the stored
// sourceKey — safe without a Lua script or extra locking because the stored
// value is write-once: nothing overwrites it until TTL eviction, so there is
// no race window between the miss and the GET.
//
// Fail-open on Valkey errors: replay detection is defense-in-depth layered on
// top of signature/expiry/audience/issuer validation (all of which already
// passed by the time this is called), not the sole authentication control.
// Treating a transient infrastructure outage as "not seen" avoids turning a
// Valkey blip into a full authentication outage. Every failure is logged so
// the degradation is observable.
func (c *ValkeyReplayCache) Seen(jti, sourceKey string) bool {
	if jti == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	key := replayCacheKey(jti)
	stored, err := c.client.SetNX(ctx, key, sourceKey, c.ttl).Result()
	if err != nil {
		c.logger.Error(err, "valkey replay cache unavailable; failing open (replay detection degraded for this request)")
		return false
	}
	if stored {
		return false
	}
	recordedSource, err := c.client.Get(ctx, key).Result()
	if err != nil {
		c.logger.Error(err, "valkey replay cache unavailable on source-key lookup; failing open (replay detection degraded for this request)")
		return false
	}
	return recordedSource != sourceKey
}

// Stop is a no-op: the redis.Client lifecycle is owned by the caller, not by
// this cache, so there is nothing for Stop to release here.
func (c *ValkeyReplayCache) Stop() {}

func replayCacheKey(jti string) string {
	return "apifrontend:replay:jti:" + jti
}
