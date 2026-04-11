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

package evaluator

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/evaluator"
)

// BR-SP-051: Hot-reload policy evaluation
var _ = Describe("UT-SP-668-004: Evaluator Hot-Reload and Lifecycle", func() {
	var (
		eval    *evaluator.Evaluator
		ctx     context.Context
		cancel  context.CancelFunc
		tempDir string
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		var err error
		tempDir, err = os.MkdirTemp("", "sp-hotreload-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cancel()
		os.RemoveAll(tempDir)
	})

	Describe("StartHotReload", func() {
		It("BR-SP-051: should load initial policy from file and start watching", func() {
			policyPath := filepath.Join(tempDir, "policy.rego")
			Expect(os.WriteFile(policyPath, []byte(testPolicy), 0644)).To(Succeed())

			logger := zap.New(zap.UseDevMode(true))
			eval = evaluator.New(policyPath, logger)

			err := eval.StartHotReload(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer eval.Stop()

			Expect(eval.GetPolicyHash()).NotTo(BeEmpty())
		})

		It("BR-SP-051: should return error when policy file does not exist", func() {
			policyPath := filepath.Join(tempDir, "nonexistent.rego")

			logger := zap.New(zap.UseDevMode(true))
			eval = evaluator.New(policyPath, logger)

			err := eval.StartHotReload(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to load initial content"))
		})

		It("BR-SP-051: should return error when policy content is invalid Rego", func() {
			policyPath := filepath.Join(tempDir, "bad.rego")
			Expect(os.WriteFile(policyPath, []byte("this is not valid rego"), 0644)).To(Succeed())

			logger := zap.New(zap.UseDevMode(true))
			eval = evaluator.New(policyPath, logger)

			err := eval.StartHotReload(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("callback failed"))
		})
	})

	Describe("Stop", func() {
		It("BR-SP-051: should stop gracefully after StartHotReload", func() {
			policyPath := filepath.Join(tempDir, "policy.rego")
			Expect(os.WriteFile(policyPath, []byte(testPolicy), 0644)).To(Succeed())

			logger := zap.New(zap.UseDevMode(true))
			eval = evaluator.New(policyPath, logger)

			Expect(eval.StartHotReload(ctx)).To(Succeed())

			eval.Stop()
			Expect(eval.GetPolicyHash()).NotTo(BeEmpty())
		})

		It("BR-SP-051: should be safe to call on evaluator that was never started", func() {
			logger := zap.New(zap.UseDevMode(true))
			eval = evaluator.New("/tmp/unused.rego", logger)

			Expect(func() { eval.Stop() }).NotTo(Panic())
		})
	})
})
