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

// E2E-FMC-1683-016. See shared.TLSPortSplit for the scenario body -- 100%
// portable, shared verbatim with the EAIGW lane (this journey exercises
// FMC's own server TLS/port-split config, never the MCP Gateway edge).
var _ = shared.TLSPortSplit(harness, kuadrantVariant{})
