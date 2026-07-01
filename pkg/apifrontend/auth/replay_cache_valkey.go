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

// Seen atomically records jti as observed and reports whether it was already
// present, using SET-NX-with-TTL semantics so a single round-trip both checks
// and marks the token — required to avoid a race between concurrent requests
// replaying the same token against different replicas at nearly the same time.
//
// Fail-open on Valkey errors: replay detection is defense-in-depth layered on
// top of signature/expiry/audience/issuer validation (all of which already
// passed by the time this is called), not the sole authentication control.
// Treating a transient infrastructure outage as "not seen" avoids turning a
// Valkey blip into a full authentication outage. Every failure is logged so
// the degradation is observable.
func (c *ValkeyReplayCache) Seen(jti string) bool {
	if jti == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stored, err := c.client.SetNX(ctx, replayCacheKey(jti), "1", c.ttl).Result()
	if err != nil {
		c.logger.Error(err, "valkey replay cache unavailable; failing open (replay detection degraded for this request)")
		return false
	}
	return !stored
}

// Stop is a no-op: the redis.Client lifecycle is owned by the caller, not by
// this cache, so there is nothing for Stop to release here.
func (c *ValkeyReplayCache) Stop() {}

func replayCacheKey(jti string) string {
	return "apifrontend:replay:jti:" + jti
}
