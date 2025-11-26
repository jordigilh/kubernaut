/*
Copyright 2025 Jordi Gil.

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

package redis

import (
	"time"

	"github.com/redis/go-redis/v9"
)

// Options contains Redis connection configuration.
//
// This struct is extracted from Gateway's config.RedisOptions to provide
// a shared, reusable configuration pattern across all services that use Redis.
//
// Design Decision: DD-CACHE-001 (Shared Redis Library)
// Provides consistent Redis configuration across Gateway, Data Storage, and future services.
//
// Configuration fields:
//   - Addr: Redis server address (host:port, e.g., "localhost:6379")
//   - DB: Redis database number (0-15, default: 0)
//   - Password: Redis password (optional, empty string if no auth)
//   - DialTimeout: Timeout for establishing connection (default: 5s)
//   - ReadTimeout: Timeout for socket reads (default: 3s)
//   - WriteTimeout: Timeout for socket writes (default: 3s)
//   - PoolSize: Maximum number of socket connections (default: 10)
//   - MinIdleConns: Minimum number of idle connections (default: 5)
//
// Example YAML configuration:
//
//	redis:
//	  addr: "localhost:6379"
//	  db: 0
//	  password: ""
//	  dial_timeout: 5s
//	  read_timeout: 3s
//	  write_timeout: 3s
//	  pool_size: 10
//	  min_idle_conns: 5
type Options struct {
	Addr         string        `yaml:"addr"`
	DB           int           `yaml:"db"`
	Password     string        `yaml:"password,omitempty"`
	DialTimeout  time.Duration `yaml:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	PoolSize     int           `yaml:"pool_size"`
	MinIdleConns int           `yaml:"min_idle_conns"`
}

// ToGoRedisOptions converts our Options to go-redis Options.
//
// This method provides a clean conversion between our configuration struct
// (designed for YAML unmarshaling) and the go-redis library's Options struct
// (designed for programmatic use).
//
// Returns:
//   - *redis.Options: go-redis options ready for use with redis.NewClient()
//
// Example:
//
//	opts := &redis.Options{
//	    Addr:         "localhost:6379",
//	    DB:           0,
//	    DialTimeout:  5 * time.Second,
//	    ReadTimeout:  3 * time.Second,
//	    WriteTimeout: 3 * time.Second,
//	    PoolSize:     10,
//	    MinIdleConns: 5,
//	}
//	redisOpts := opts.ToGoRedisOptions()
//	client := redis.NewClient(redisOpts)
func (o *Options) ToGoRedisOptions() *redis.Options {
	return &redis.Options{
		Addr:         o.Addr,
		DB:           o.DB,
		Password:     o.Password,
		DialTimeout:  o.DialTimeout,
		ReadTimeout:  o.ReadTimeout,
		WriteTimeout: o.WriteTimeout,
		PoolSize:     o.PoolSize,
		MinIdleConns: o.MinIdleConns,
	}
}

// DefaultOptions returns Redis options with sensible defaults.
//
// Default values:
//   - Addr: "localhost:6379" (standard Redis port)
//   - DB: 0 (default Redis database)
//   - Password: "" (no authentication)
//   - DialTimeout: 5s (connection establishment)
//   - ReadTimeout: 3s (socket reads)
//   - WriteTimeout: 3s (socket writes)
//   - PoolSize: 10 (max connections)
//   - MinIdleConns: 5 (idle connection pool)
//
// These defaults are suitable for:
//   - Development environments
//   - Low-traffic production services
//   - Single-instance Redis deployments
//
// For high-traffic production, consider:
//   - Increasing PoolSize (20-50)
//   - Increasing MinIdleConns (10-20)
//   - Adjusting timeouts based on network latency
//
// Returns:
//   - *Options: Redis options with default values
//
// Example:
//
//	opts := redis.DefaultOptions()
//	opts.Addr = "redis.prod.svc.cluster.local:6379"
//	opts.PoolSize = 20
//	client := redis.NewClient(opts.ToGoRedisOptions(), logger)
func DefaultOptions() *Options {
	return &Options{
		Addr:         "localhost:6379",
		DB:           0,
		Password:     "",
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 5,
	}
}



