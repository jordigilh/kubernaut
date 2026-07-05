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

package notification_test

import (
	"testing"

	"github.com/jordigilh/kubernaut/pkg/notification/routing"
)

// FuzzRoutingParseConfig exercises routing.ParseConfig (BR-NOT-066) with
// adversarial YAML input to surface panics in the parsing, validation, and
// matchRe regex-compilation paths. Config source is typically an
// operator-edited ConfigMap/CRD field rather than a per-request external
// input, but its YAML parsing and dynamic regexp.Compile of "matchRe"
// patterns (Issue #416) is the most structurally complex parser in the
// notification service and a good target for panics or pathological regex.
// Pure function, no dependencies.
//
// NOTE: native Go fuzzing (func FuzzXxx(f *testing.F)) is the sole,
// documented exception to AGENTS.md's Ginkgo/Gomega mandate -- see
// "Exception: Go Native Fuzz Tests" in AGENTS.md. This file intentionally
// contains no business-outcome assertions.
//
// Run locally with: go test ./pkg/notification/ -run=^$ -fuzz=FuzzRoutingParseConfig
func FuzzRoutingParseConfig(f *testing.F) {
	seeds := []string{
		`
route:
  receiver: default-receiver
receivers:
  - name: default-receiver
    slackConfigs:
      - channel: '#alerts'
`,
		`
route:
  receiver: default-receiver
  routes:
    - matchRe:
        alertname: "^High.*"
      receiver: default-receiver
receivers:
  - name: default-receiver
    slackConfigs:
      - channel: '#alerts'
`,
		``,
		`not: [valid`,
		`route: {}`,
		`{}`,
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = routing.ParseConfig(data)
	})
}
