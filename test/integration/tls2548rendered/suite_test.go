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

// Package tls2548rendered gates the CI-rendered operator deployment
// contract for BR-SECURITY-2548 (issue #2548). Like the live E2E packages,
// it is deliberately not reachable by any regular make target: it is
// invoked only by the tls-rendered-contract CI job, which downloads the
// operator-qualification artifact (operator-bootstrap-rendered.yaml and
// operator-dependencies-rendered.yaml) rendered from the production
// operator chart, and points TLS_2548_RENDERED_DIR at it. The package
// fails closed when that variable is unset -- it never skips silently.
package tls2548rendered

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTLS2548RenderedContract(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TLS Rendered Deployment Contract Suite (BR-SECURITY-2548)")
}
