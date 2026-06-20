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
	"time"

	"github.com/redis/go-redis/v9"
)

// ValkeyWriter implements CacheWriter using a Valkey (Redis-compatible) client.
type ValkeyWriter struct {
	client *redis.Client
}

// NewValkeyWriter creates a CacheWriter backed by Valkey at the given address.
func NewValkeyWriter(addr string) *ValkeyWriter {
	return &ValkeyWriter{
		client: redis.NewClient(&redis.Options{Addr: addr}),
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
