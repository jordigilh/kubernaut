<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package workflow

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-WORKFLOW-API-001: Unified workflow API client for reuse between business logic and tests
// BR-WORKFLOW-API-002: Eliminate code duplication in HTTP client patterns
// BR-WORKFLOW-API-003: Integration with existing webhook response patterns

func TestWorkflowClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow API Client Unit Tests Suite")
}
<<<<<<< HEAD

=======
>>>>>>> crd_implementation
