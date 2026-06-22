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

	"github.com/redis/go-redis/v9"
)

// ValkeyCacheReader implements CacheReader using a Valkey (Redis-compatible) client.
type ValkeyCacheReader struct {
	client *redis.Client
}

// NewValkeyCacheReader creates a CacheReader backed by Valkey.
func NewValkeyCacheReader(addr string) *ValkeyCacheReader {
	return &ValkeyCacheReader{
		client: redis.NewClient(&redis.Options{Addr: addr}),
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
