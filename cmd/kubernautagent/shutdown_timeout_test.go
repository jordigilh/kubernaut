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

package main

import (
	"testing"
	"time"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
)

// UT-KA-1329-004: shutdownTimeout returns configured value (CM-3)
func TestShutdownTimeout_UsesConfig(t *testing.T) {
	cfg := kaconfig.DefaultConfig()
	cfg.Runtime.Shutdown.DrainSeconds = 3

	timeout := shutdownTimeout(cfg)
	if timeout != 3*time.Second {
		t.Errorf("UT-KA-1329-004: CM-3: expected 3s from config, got %v", timeout)
	}
}

// UT-KA-1329-005: shutdownTimeout returns 30s default on zero (CM-3)
func TestShutdownTimeout_DefaultsOnZero(t *testing.T) {
	cfg := kaconfig.DefaultConfig()
	cfg.Runtime.Shutdown.DrainSeconds = 0

	timeout := shutdownTimeout(cfg)
	if timeout != 30*time.Second {
		t.Errorf("UT-KA-1329-005: CM-3: expected 30s default, got %v", timeout)
	}
}
