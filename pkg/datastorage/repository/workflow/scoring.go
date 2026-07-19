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

package workflow

// sanitizeEnumValue validates that value is one of the allowedValues.
// Returns the value if valid, empty string otherwise.
//
// Issue #1661 Phase C: this file used to also hold the SQL-builder half of
// DD-WORKFLOW-004's label-only scoring (buildDetectedLabelsBoostSQL/
// buildDetectedLabelsPenaltySQL/buildCustomLabelsBoostSQL and their
// appendBoolBoostCase/appendWildcardStringBoostCase/sanitizeJSONBKey/
// sanitizeJSONBValue helpers), used only by discovery.go's now-deleted
// Postgres SQL fallback (selectScoredWorkflows). sanitizeEnumValue and
// detectedLabelWeights below are the two symbols the Go-native cache-backed
// equivalent (cache_filter.go) still depends on; everything else was deleted
// with zero remaining callers.
func sanitizeEnumValue(value string, allowedValues []string) string {
	for _, allowed := range allowedValues {
		if value == allowed {
			return value
		}
	}
	return ""
}

// detectedLabelWeights are the DD-WORKFLOW-004 v1.5 label-only scoring
// weights, shared by cache_filter.go's Go-native boost/penalty computation.
var detectedLabelWeights = map[string]float64{
	"git_ops_managed":  0.10,
	"git_ops_tool":     0.10,
	"pdb_protected":    0.05,
	"service_mesh":     0.05,
	"network_isolated": 0.03,
	"helm_managed":     0.02,
	"stateful":         0.02,
	"hpa_enabled":      0.02,
	"virtual_machine":  0.08,
	"live_migratable":  0.04,
	"cdi_managed":      0.03,
	"storage_backend":  0.05,
}
