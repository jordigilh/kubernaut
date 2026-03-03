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

package remediationorchestrator

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	config "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
)

// ========================================
// Issue #253: Propagation Delay Compounding Tests
// ========================================
//
// Business Requirements:
// - BR-RO-103.5: Compounding logic for GitOps + operator propagation delays
//
// Design Document:
// - DD-EM-004 v2.0: Compounding table (Section "RO: Propagation Delay Computation")

var _ = Describe("Propagation Delay Compounding (#253, BR-RO-103.5)", Label("propagation-delay"), func() {

	var cfg config.AsyncPropagationConfig

	BeforeEach(func() {
		cfg = config.AsyncPropagationConfig{
			GitOpsSyncDelay:        3 * time.Minute,
			OperatorReconcileDelay: 1 * time.Minute,
		}
	})

	// ========================================
	// UT-RO-253-004: GitOps-only compounding
	// ========================================
	It("UT-RO-253-004: GitOps-only target gets gitOpsSyncDelay only", Label("UT-RO-253-004"), func() {
		delay := cfg.ComputePropagationDelay(true, false)
		Expect(delay).To(Equal(3*time.Minute),
			"GitOps-managed, built-in API group: only gitOpsSyncDelay applies")
	})

	// ========================================
	// UT-RO-253-005: Operator-only compounding
	// ========================================
	It("UT-RO-253-005: CRD-only target gets operatorReconcileDelay only", Label("UT-RO-253-005"), func() {
		delay := cfg.ComputePropagationDelay(false, true)
		Expect(delay).To(Equal(1*time.Minute),
			"CRD target, not GitOps-managed: only operatorReconcileDelay applies")
	})

	// ========================================
	// UT-RO-253-006: GitOps + operator compounding
	// ========================================
	It("UT-RO-253-006: GitOps + CRD target compounds both delays", Label("UT-RO-253-006"), func() {
		delay := cfg.ComputePropagationDelay(true, true)
		Expect(delay).To(Equal(4*time.Minute),
			"GitOps-managed CRD: gitOpsSyncDelay(3m) + operatorReconcileDelay(1m) = 4m")
	})

	// ========================================
	// UT-RO-253-007: Sync target — no delay
	// ========================================
	It("UT-RO-253-007: sync target (neither GitOps nor CRD) returns zero", Label("UT-RO-253-007"), func() {
		delay := cfg.ComputePropagationDelay(false, false)
		Expect(delay).To(Equal(time.Duration(0)),
			"built-in API group, no GitOps: no propagation delay")
	})

	// ========================================
	// UT-RO-253-003 (compounding sub-case): zero delay + detection true
	// ========================================
	It("UT-RO-253-003: zero gitOpsSyncDelay with isGitOps=true degrades gracefully", Label("UT-RO-253-003"), func() {
		cfg.GitOpsSyncDelay = 0
		delay := cfg.ComputePropagationDelay(true, true)
		Expect(delay).To(Equal(1*time.Minute),
			"0 + operatorReconcileDelay(1m) = 1m; zero disables that stage")
	})
})
