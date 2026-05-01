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

package mcp_test

import (
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	sharedTestEnv  *envtest.Environment
	sharedK8sConfig *rest.Config
	sharedK8sClient client.Client
	suiteLogger    *slog.Logger
)

func TestMCPIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kubernaut Agent MCP Integration Suite")
}

var _ = SynchronizedBeforeSuite(
	func() []byte {
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("MCP IT - Phase 0: envtest bootstrap")
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		By("Starting envtest (Leases are core K8s — no CRDs needed)")
		assetsDir := os.Getenv("KUBEBUILDER_ASSETS")
		if assetsDir == "" {
			out, err := exec.Command("setup-envtest", "use", "-p", "path").CombinedOutput()
			if err == nil {
				assetsDir = strings.TrimSpace(string(out))
			}
		}
		sharedTestEnv = &envtest.Environment{
			BinaryAssetsDirectory: assetsDir,
		}
		cfg, err := sharedTestEnv.Start()
		Expect(err).ToNot(HaveOccurred(), "envtest should start")
		GinkgoWriter.Printf("envtest API server: %s\n", cfg.Host)

		scheme := runtime.NewScheme()
		Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).ToNot(HaveOccurred(), "controller-runtime client should build")

		sharedK8sConfig = cfg
		sharedK8sClient = k8sClient

		GinkgoWriter.Println("envtest ready — shared K8s client available")
		return nil
	},
	func(_ []byte) {
		suiteLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

		if sharedK8sClient == nil {
			scheme := runtime.NewScheme()
			Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())

			var err error
			sharedK8sClient, err = client.New(sharedK8sConfig, client.Options{Scheme: scheme})
			Expect(err).ToNot(HaveOccurred())
		}
	},
)

var _ = SynchronizedAfterSuite(
	func() {},
	func() {
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("MCP IT - Stopping envtest")
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		if sharedTestEnv != nil {
			Expect(sharedTestEnv.Stop()).To(Succeed())
		}
		GinkgoWriter.Println("Suite complete")
	},
)
