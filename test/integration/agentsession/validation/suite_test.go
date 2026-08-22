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

// Package validation is a dedicated, lightweight integration suite for
// issue #2190: proves the AgentSession CRD's OpenAPI schema rejects
// investigation-identity fields the retired agentclient.IncidentRequest
// HTTP schema previously enforced server-side (minLength:1 on
// remediation_id per DD-WORKFLOW-002 v2.2, equivalent enforcement for
// incident_id) -- so removing pkg/agentclient does not silently drop that
// input-validation guarantee (SI-10).
//
// Deliberately isolated from test/integration/aianalysis's heavy shared
// suite, mirroring test/integration/aianalysis/schemarejection's precedent:
// none of that infrastructure (PostgreSQL, Redis, DataStorage, Mock LLM,
// real KA container) is needed to prove a CRD OpenAPI schema constraint --
// this only needs a real API server enforcing the schema, which a fake
// client does not do (client-go's fake/interceptor clients don't run
// server-side OpenAPI validation).
package validation

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

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
)

// Plain (non-Synchronized) BeforeSuite/AfterSuite: Ginkgo runs these once per
// parallel PROCESS, each getting its own independent envtest instance -- no
// cross-process coordination needed since nothing here is shared external
// infrastructure.
var (
	testEnv *envtest.Environment
	cfg     *rest.Config
)

func TestAgentSessionSchemaValidationIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AgentSession Schema Validation Integration Suite (#2190)")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	Expect(agentsessionv1.AddToScheme(scheme.Scheme)).To(Succeed())

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
