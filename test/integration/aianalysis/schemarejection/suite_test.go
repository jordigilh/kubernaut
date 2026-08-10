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

// Package schemarejection contains a dedicated, lightweight integration
// suite for GitHub issue #2030 (main-tracking clone of #2029) Part A:
// reconcileInvestigating and reconcileAnalyzing must retry (bounded, with
// backoff) instead of fail-closing forever when a live cluster's installed
// AIAnalysis CRD rejects a Status().Update() call with apierrors.IsInvalid
// (e.g. CRD version skew during a rolling upgrade).
//
// Deliberately isolated from test/integration/aianalysis's heavy shared
// suite (PostgreSQL, Redis, DataStorage, Mock LLM, real KA container):
// none of that infrastructure is needed to prove this fix, and reusing the
// existing SynchronizedBeforeSuite/manager would require intercepting a
// client already wired into 15+ unrelated test files. This suite starts
// its own minimal per-process envtest (AIAnalysis CRD only) and talks to
// the real Kubernetes API server through a real client.WithWatch wrapped
// with an interceptor — proving that handleSchemaRejectedStatusUpdate's
// "plain Update() survives even while Status().Update() is rejected"
// assumption holds against genuine apiserver status-subresource semantics,
// not just the fake client's approximation of them (see UT-AA-2030-001/002/
// 003/005 in internal/controller/aianalysis for the fake-client-based unit
// coverage of the same logic).
package schemarejection

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

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

// Plain (non-Synchronized) BeforeSuite/AfterSuite: Ginkgo runs these once per
// parallel PROCESS (not once globally), so each process gets its own fully
// independent envtest instance — no cross-process coordination needed since
// nothing here is shared external infrastructure.
var (
	testEnv *envtest.Environment
	cfg     *rest.Config
)

func TestSchemaRejectionRetryIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AIAnalysis Schema-Rejection Retry Integration Suite (#2030 Part A)")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	Expect(aianalysisv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

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
