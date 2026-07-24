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

// Package workflowcatalog_test proves KubernautAgent's informer-backed
// workflow/action-type cache (DD-WORKFLOW-019, Issue #1677 Phase 2a) against
// a real envtest API server. No Postgres/Redis/DataStorage container is
// needed here -- unlike the heavier MCP IT suite, this cache has zero
// runtime dependency on DataStorage (KA now owns discovery directly).
package workflowcatalog_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/workflowcatalog"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

var (
	testEnv   *envtest.Environment
	k8sConfig *rest.Config
	k8sClient client.Client
	logger    = kubelog.NewLogger(kubelog.DevelopmentOptions())
	ctx       context.Context
	cancelCtx context.CancelFunc
)

func TestWorkflowCatalogIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "KubernautAgent Workflow Catalog Integration Suite")
}

var _ = BeforeSuite(func() {
	ctx, cancelCtx = context.WithCancel(context.Background())

	assetsDir := os.Getenv("KUBEBUILDER_ASSETS")
	if assetsDir == "" {
		out, err := exec.Command("setup-envtest", "use", "-p", "path").CombinedOutput()
		if err == nil {
			assetsDir = strings.TrimSpace(string(out))
		}
	}
	testEnv = &envtest.Environment{
		BinaryAssetsDirectory: assetsDir,
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred(), "envtest should start")
	k8sConfig = cfg

	scheme, err := workflowcatalog.NewScheme()
	Expect(err).ToNot(HaveOccurred(), "workflowcatalog scheme should register RemediationWorkflow/ActionType")

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred(), "controller-runtime client should build")
})

var _ = AfterSuite(func() {
	if cancelCtx != nil {
		cancelCtx()
	}
	if testEnv != nil {
		Expect(testEnv.Stop()).To(Succeed())
	}
})
