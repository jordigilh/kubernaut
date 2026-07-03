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

package config

import (
	"fmt"
	"strings"
	"time"
)

// Getter methods deriving effective runtime values (with defaults/clamping)
// from the raw YAML-configured string fields on ServerConfig and
// DatabaseConfig. Split from config.go (GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 3, pure code motion, no behavior change).

// GetReadTimeout returns the read timeout as a time.Duration
func (c *ServerConfig) GetReadTimeout() time.Duration {
	if c.ReadTimeout == "" {
		return 30 * time.Second // default
	}
	duration, err := time.ParseDuration(c.ReadTimeout)
	if err != nil {
		return 30 * time.Second // fallback to default
	}
	return duration
}

// GetWriteTimeout returns the write timeout as a time.Duration
func (c *ServerConfig) GetWriteTimeout() time.Duration {
	if c.WriteTimeout == "" {
		return 30 * time.Second // default
	}
	duration, err := time.ParseDuration(c.WriteTimeout)
	if err != nil {
		return 30 * time.Second // fallback to default
	}
	return duration
}

// GetShutdownTimeout returns the graceful shutdown budget as a time.Duration.
// DD-007: Defaults to 60s. Clamped to [30s, 120s] for safety — a value below 30s
// risks data loss (DLQ drain alone needs 10s), a value above 120s wastes K8s resources.
func (c *ServerConfig) GetShutdownTimeout() time.Duration {
	const (
		defaultTimeout = 60 * time.Second
		minTimeout     = 30 * time.Second
		maxTimeout     = 120 * time.Second
	)
	if c.ShutdownTimeout == "" {
		return defaultTimeout
	}
	duration, err := time.ParseDuration(c.ShutdownTimeout)
	if err != nil {
		return defaultTimeout
	}
	if duration < minTimeout {
		return minTimeout
	}
	if duration > maxTimeout {
		return maxTimeout
	}
	return duration
}

// GetEndpointPropagationDelay returns the endpoint propagation delay as time.Duration.
// #1088 Phase 7 / SRE-L1: Defaults to 5s. Clamped to [1s, 30s] — below 1s risks
// receiving traffic before K8s propagation; above 30s wastes shutdown budget.
func (c *ServerConfig) GetEndpointPropagationDelay() time.Duration {
	const (
		defaultDelay = 5 * time.Second
		minDelay     = 1 * time.Second
		maxDelay     = 30 * time.Second
	)
	if c.EndpointPropagationDelay == "" {
		return defaultDelay
	}
	duration, err := time.ParseDuration(c.EndpointPropagationDelay)
	if err != nil {
		return defaultDelay
	}
	if duration < minDelay {
		return minDelay
	}
	if duration > maxDelay {
		return maxDelay
	}
	return duration
}

// GetMaxBodySize returns the maximum request body size in bytes.
// #1048 Phase 4 / SC-5: Defaults to 5 MiB. Clamped to [1 MiB, 50 MiB].
// 5 MiB accommodates batch audit event requests (500 events × ~2-5 KB each).
func (c *ServerConfig) GetMaxBodySize() int64 {
	const (
		mib         = 1 << 20
		defaultSize = 5 * mib
		minSize     = 1 * mib
		maxSize     = 50 * mib
	)
	if c.MaxBodySize == "" {
		return int64(defaultSize)
	}
	var size int64
	if n, err := fmt.Sscanf(c.MaxBodySize, "%d", &size); err == nil && n == 1 {
		// Plain integer in bytes
	} else {
		return int64(defaultSize)
	}
	if size < int64(minSize) {
		return int64(minSize)
	}
	if size > int64(maxSize) {
		return int64(maxSize)
	}
	return size
}

// GetCORSAllowedOrigins returns the configured CORS origins.
// SEC-H3/AC-4: Defaults to empty (reject all cross-origin requests).
// Operators must configure explicit origins for browser-based access.
func (c *ServerConfig) GetCORSAllowedOrigins() []string {
	return c.CORSAllowedOrigins
}

// GetSignerCertDir returns the directory holding tls.crt/tls.key for audit export signing.
// #1048 Phase 5 / AU-9: Defaults to /etc/certs when unset.
func (c *ServerConfig) GetSignerCertDir() string {
	if c.SignerCertDir == "" {
		return "/etc/certs"
	}
	return c.SignerCertDir
}

// GetConnectionString returns the PostgreSQL connection string with PG-level timeouts.
// Issue #667/M1: statement_timeout and lock_timeout are set as DSN options so PostgreSQL
// itself enforces limits, independent of Go-side context cancellation.
func (c *DatabaseConfig) GetConnectionString() string {
	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		c.Host, c.Port, c.Name, c.User, c.Password, c.SSLMode)

	var pgOpts []string
	if c.StatementTimeout != "" {
		if d, err := time.ParseDuration(c.StatementTimeout); err == nil {
			pgOpts = append(pgOpts, fmt.Sprintf("-c statement_timeout=%d", d.Milliseconds()))
		}
	}
	if c.LockTimeout != "" {
		if d, err := time.ParseDuration(c.LockTimeout); err == nil {
			pgOpts = append(pgOpts, fmt.Sprintf("-c lock_timeout=%d", d.Milliseconds()))
		}
	}
	if len(pgOpts) > 0 {
		dsn += fmt.Sprintf(" options='%s'", strings.Join(pgOpts, " "))
	}
	return dsn
}
