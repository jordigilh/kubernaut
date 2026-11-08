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

package override

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

func TestOverride(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Override Suite")
}

var _ = Describe("BR-AUDIT-006: override parameter evidence", func() {
	It("UT-ORCH-2575-022: rejects workflow switching before lookup", func() {
		selected := &aianalysisv1.SelectedWorkflow{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{WorkflowID: "original"}}
		_, _, err := ResolveWorkflow(context.Background(), nil, &remediationv1.WorkflowOverride{WorkflowName: "different"}, selected, "trusted")
		Expect(err).To(HaveOccurred())
		Expect(selected.WorkflowID).To(Equal("original"))
	})
	It("UT-ORCH-2575-021: retains submitted numeric lexemes and rejects inconsistent evidence", func() {
		original, originalEvidence, err := sharedtypes.DecodeTypedParameterMap([]byte(`{"A":9007199254740993,"B":1.0}`))
		Expect(err).NotTo(HaveOccurred())
		changes, changesEvidence, err := sharedtypes.DecodeTypedParameterMap([]byte(`{"B":1e0}`))
		Expect(err).NotTo(HaveOccurred())
		evidence, err := mergeEvidence(original, originalEvidence, changes, changesEvidence)
		Expect(err).NotTo(HaveOccurred())
		Expect(evidence).To(Equal(`{"A":9007199254740993,"B":1e0}`))
		_, err = mergeEvidence(original, originalEvidence, changes, `{"B":2e0}`)
		Expect(err).To(HaveOccurred())
	})
})
