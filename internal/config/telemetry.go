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

import "fmt"

// TelemetryConfig holds OpenTelemetry distributed tracing settings shared
// across services (GAP-14 / Issue #1519).
//
// ADR-030: YAML-based configuration with camelCase field names.
// Bring-your-own-collector: an empty Endpoint disables tracing entirely
// (OTel's default no-op TracerProvider stays active) -- this is a valid,
// zero-overhead production configuration, not an error state. Set Endpoint
// to "stdout" for local debugging without any collector infrastructure.
type TelemetryConfig struct {
	// Endpoint is the OTLP/HTTP collector endpoint (host:port, no scheme).
	// Empty disables OTLP export. Independent of LogSink -- both may be
	// enabled at once, either alone, or neither (fully disabled).
	Endpoint string `yaml:"endpoint,omitempty"`

	// LogSink, when true, emits one compact structured log line per
	// completed span through the service's existing logr.Logger instead of
	// (or alongside) exporting to Endpoint. No collector infrastructure
	// required -- spans land in the same log stream already collected by
	// must-gather and CI log capture. Recommended default for CI/E2E test
	// harnesses; leave false in production unless explicitly requested.
	LogSink bool `yaml:"logSink,omitempty"`

	// TLS configures trust and optional client authentication for the OTLP/HTTP
	// export connection to Endpoint. TLS is mandatory for network endpoints.
	TLS TelemetryTLSConfig `yaml:"tls,omitempty"`
}

// TelemetryTLSConfig configures TLS for the OTLP/HTTP exporter connection.
// Mirrors the shape of pkg/datastorage/config.RedisTLSConfig for consistency:
// CAFile is for a self-signed/private collector certificate (optional --
// omit to trust the system CA pool); CertFile/KeyFile are for mTLS, if the
// collector requires a client certificate.
type TelemetryTLSConfig struct {
	// CAFile is the path to a PEM CA certificate used to verify a
	// self-signed or privately-issued collector certificate. Optional --
	// leave empty to trust the system CA pool (e.g. a publicly-trusted
	// vendor collector).
	CAFile string `yaml:"caFile,omitempty"`

	// CertFile and KeyFile are the client certificate/key pair for mTLS,
	// if the collector requires client authentication. Both are optional
	// and must be set together.
	CertFile string `yaml:"certFile,omitempty"`
	KeyFile  string `yaml:"keyFile,omitempty"`
}

// DefaultTelemetryConfig returns tracing fully disabled by default
// (BYO-collector: operators opt in per-field; neither Endpoint nor LogSink
// costs anything until set).
func DefaultTelemetryConfig() TelemetryConfig {
	return TelemetryConfig{Endpoint: "", LogSink: false}
}

// Validate validates telemetry configuration that applies to network export.
// The stdout endpoint is a local log sink and does not require TLS material.
func (c TelemetryConfig) Validate() error {
	if (c.TLS.CertFile == "") != (c.TLS.KeyFile == "") {
		return fmt.Errorf("telemetry TLS certFile and keyFile must be set together")
	}
	return nil
}
