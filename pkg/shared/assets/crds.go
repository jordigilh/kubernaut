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

package assets

import "embed"

// CRDsFS embeds the generated CRD YAML files from the main kubernaut repo.
// The kubernaut-operator imports this to install/update CRDs during reconciliation.
// DD-4 (Issue #578): Shared assets via Go embed for the operator.
//
//go:embed crds/*.yaml
var CRDsFS embed.FS
