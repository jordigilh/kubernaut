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

// Package capacityretry contains a dedicated, lightweight integration suite
// proving BR-AI-009 / DD-AA-KA-001 amendment: when KA reports an AgentSession
// as Failed with Status.Reason=AgentSessionReasonCapacityExceeded (a
// transient, self-resolving dispatch-capacity rejection, not a genuine
// investigation failure), AA's InvestigatingHandler must delete-and-retry
// (bounded, with backoff) rather than permanently failing the AIAnalysis.
//
// Deliberately isolated from test/integration/aianalysis's heavy shared
// suite (PostgreSQL, Redis, DataStorage, Mock LLM, real KA container) --
// following the same convention established by the sibling
// test/integration/aianalysis/schemarejection suite (#2030 Part A). None of
// that infrastructure is needed to prove this fix: the real KA subprocess
// that heavy suite runs would race with this suite's synchronous, directly
// seeded AgentSession fixtures (KA's dispatcher watches AgentSession too),
// making the outcome nondeterministic. This suite starts its own minimal
// per-process envtest (AIAnalysis + AgentSession CRDs only, no KA/manager
// loop watching them) and drives the real AIAnalysisReconciler and the real
// creator.AgentSessionCreator (the same production AgentSessionGetOrCreator
// wired in cmd/aianalysis/main.go) directly against a genuine Kubernetes API
// server -- proving the full delete/recreate/non-terminal wiring without any
// other actor able to race the test's synchronous Reconcile() calls.
package capacityretry

import (
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	agentsessionv1alpha1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

// Plain (non-Synchronized) BeforeSuite/AfterSuite: Ginkgo runs these once per
// parallel PROCESS (not once globally), so each process gets its own fully
// independent envtest instance -- no cross-process coordination needed since
// nothing here is shared external infrastructure.
var (
	testEnv *envtest.Environment
	cfg     *rest.Config
)

func TestCapacityExceededRetryIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AIAnalysis Capacity-Exceeded Retry Integration Suite (BR-AI-009, DD-AA-KA-001 amendment)")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	Expect(aianalysisv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(agentsessionv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	// KUBEBUILDER_ASSETS is set by Makefile via setup-envtest dependency.
	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	if testEnv != nil {
		Expect(testEnv.Stop()).To(Succeed())
	}
})
