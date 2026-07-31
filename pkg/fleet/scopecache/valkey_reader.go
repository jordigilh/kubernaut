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

package scopecache

import (
	"context"
	"crypto/tls"

	"github.com/redis/go-redis/v9"
)

// ValkeyCacheReader implements CacheReader using a Valkey (Redis-compatible) client.
type ValkeyCacheReader struct {
	client *redis.Client
}

// ValkeyOption configures optional behavior on the redis.Client constructed
// by NewValkeyCacheReader. Variadic so existing plaintext callers (tests,
// BYO Valkey without TLS) keep compiling unchanged.
type ValkeyOption func(*redis.Options)

// WithTLSConfig enables TLS on the Valkey connection using a pre-built
// *tls.Config (DD-PLATFORM-006 DA9 follow-up). A nil tlsConfig is a no-op,
// leaving the connection in plaintext.
func WithTLSConfig(tlsConfig *tls.Config) ValkeyOption {
	return func(o *redis.Options) {
		o.TLSConfig = tlsConfig
	}
}

// NewValkeyCacheReader creates a CacheReader backed by Valkey.
func NewValkeyCacheReader(addr string, opts ...ValkeyOption) *ValkeyCacheReader {
	redisOpts := &redis.Options{Addr: addr}
	for _, opt := range opts {
		opt(redisOpts)
	}
	return &ValkeyCacheReader{
		client: redis.NewClient(redisOpts),
	}
}

// Exists checks if the given key exists in Valkey.
func (v *ValkeyCacheReader) Exists(ctx context.Context, key string) (bool, error) {
	result, err := v.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// Ping checks connectivity to the Valkey backend.
func (v *ValkeyCacheReader) Ping(ctx context.Context) error {
	return v.client.Ping(ctx).Err()
}

// Close terminates the underlying Redis connection.
func (v *ValkeyCacheReader) Close() error {
	return v.client.Close()
}
