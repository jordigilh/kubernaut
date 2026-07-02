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

package eaigw

import "github.com/jordigilh/kubernaut/test/e2e/fleetmetadatacache/shared"

// E2E-FMC-EAIGW-054-011. See shared.LeastPrivilege for the scenario body
// (shared verbatim with the Kuadrant lane) and variant.go for what's
// EAIGW-specific (RBACChecks against the gateway.envoyproxy.io CRD group).
var _ = shared.LeastPrivilege(harness, eaigwVariant{})
