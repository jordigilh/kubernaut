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

package fleetmetadatacache

import "github.com/jordigilh/kubernaut/test/e2e/fleetmetadatacache/shared"

// E2E-FMC-054-013. See shared.DynamicRegistration for the scenario body
// (shared verbatim with the EAIGW lane) and variant.go for what's
// Kuadrant-specific (the MCPServerRegistration CRD factory).
var _ = shared.DynamicRegistration(harness, kuadrantVariant{})
