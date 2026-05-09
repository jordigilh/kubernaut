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

package datastorage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// ========================================
// Issue #1070: Concurrent Handler Safety
// ========================================
// Authority: BR-STORAGE-014, Issue #1070
// Purpose: Pre-parallelization baseline proving HandleCreateWorkflow is
// safe to call from multiple goroutines. Intended to be run with -race.
//
// Pattern: Fire N concurrent requests at a handler wired with mock validators.
// Assert every response returns a valid HTTP status (no panics, no races).
// ========================================

var _ = Describe("Issue #1070: Concurrent HandleCreateWorkflow Safety", Label("unit", "issue-1070", "concurrent"), func() {

	const concurrency = 10

	It("UT-WF-1070-CONC-001: concurrent requests produce valid responses without data races", func() {
		schemaYAML := func() string {
			crd := testutil.NewTestWorkflowCRD("concurrent-test-wf", "RestartPod", "job")
			crd.Spec.Description = sharedtypes.StructuredDescription{
				What:      "Concurrent safety test workflow",
				WhenToUse: "When testing #1070 race conditions",
			}
			crd.Spec.Execution.Bundle = "quay.io/kubernaut/concurrent-test:v1.0.0@sha256:f313b9632f3a8d0ffd41150b12715a43a41c6c8e7871bb830fd82c09b5988cc4"
			crd.Spec.Dependencies = &models.WorkflowDependencies{
				Secrets: []models.ResourceDependency{{Name: "test-secret"}},
			}
			return testutil.MarshalWorkflowCRD(crd)
		}()

		mockPuller := oci.NewMockImagePuller(schemaYAML)
		extractor := oci.NewSchemaExtractor(mockPuller, schema.NewParser())

		acceptingValidator := &mockActionTypeValidator{
			existsFn: func(_ context.Context, _ string) (bool, error) {
				return true, nil
			},
		}

		depValidator := &mockDependencyValidator{
			err: fmt.Errorf("secret test-secret not found"),
		}

		handler := server.NewHandler(nil,
			server.WithActionTypeValidator(acceptingValidator),
			server.WithSchemaExtractor(extractor),
			server.WithDependencyValidator(depValidator, "test-ns"),
		)

		var wg sync.WaitGroup
		results := make([]int, concurrency)

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				defer GinkgoRecover()

				body := map[string]string{"content": schemaYAML}
				jsonBody, err := json.Marshal(body)
				Expect(err).ToNot(HaveOccurred())

				req := httptest.NewRequest(http.MethodPost, "/api/v1/workflows", bytes.NewReader(jsonBody))
				rr := httptest.NewRecorder()

				handler.HandleCreateWorkflow(rr, req)
				results[idx] = rr.Code
			}(i)
		}

		wg.Wait()

		for i, code := range results {
			Expect(code).To(BeNumerically(">=", 200),
				fmt.Sprintf("request %d: expected valid HTTP status, got %d", i, code))
			Expect(code).To(BeNumerically("<", 600),
				fmt.Sprintf("request %d: HTTP status %d out of valid range", i, code))
		}
	})
})
