/*
Copyright 2026 Jordi Gil.

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

package fmc

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/redis/go-redis/v9"
)

// ValkeyWriter implements CacheWriter using a Valkey (Redis-compatible) client.
type ValkeyWriter struct {
	client *redis.Client
}

// ValkeyOption configures optional behavior on the redis.Client constructed
// by NewValkeyWriter. Variadic so existing plaintext callers (tests, BYO
// Valkey without TLS) keep compiling unchanged.
type ValkeyOption func(*redis.Options)

// WithTLSConfig enables TLS on the Valkey connection using a pre-built
// *tls.Config (DD-PLATFORM-006 DA9 follow-up). A nil tlsConfig is a no-op,
// leaving the connection in plaintext.
func WithTLSConfig(tlsConfig *tls.Config) ValkeyOption {
	return func(o *redis.Options) {
		o.TLSConfig = tlsConfig
	}
}

// NewValkeyWriter creates a CacheWriter backed by Valkey at the given address.
func NewValkeyWriter(addr string, opts ...ValkeyOption) *ValkeyWriter {
	redisOpts := &redis.Options{Addr: addr}
	for _, opt := range opts {
		opt(redisOpts)
	}
	return &ValkeyWriter{
		client: redis.NewClient(redisOpts),
	}
}

// Set writes a key with the given TTL. The value is "1" (existence-only semantics).
func (v *ValkeyWriter) Set(ctx context.Context, key string, ttl time.Duration) error {
	return v.client.Set(ctx, key, "1", ttl).Err()
}

// Close terminates the underlying Redis connection.
func (v *ValkeyWriter) Close() error {
	return v.client.Close()
}
