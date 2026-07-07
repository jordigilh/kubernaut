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

package main

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	auth "github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// buildMCPHandler's three required-dependency guard clauses — characterization
// tests (Issue #1520 Phase 2: pins behavior before the mcpHandlerParams struct
// extraction, per AGENTS.md's TDD mandate for zero-coverage refactor
// targets). Each case returns before touching the network (ctrlclient.New),
// so no live cluster is needed.
var _ = Describe("buildMCPHandler — required-dependency guards", func() {
	var (
		validInfra  *k8sInfra
		validAuthMw *auth.Middleware
		validInv    *investigator.Investigator
	)

	BeforeEach(func() {
		validInfra = &k8sInfra{kubeConfig: &rest.Config{}}
		validAuthMw = &auth.Middleware{}
		validInv = &investigator.Investigator{}
	})

	DescribeTable("returns a nil handler and nil drainer when a required dependency is missing",
		func(paramsFn func() mcpHandlerParams) {
			handler, drainer := buildMCPHandler(context.Background(), paramsFn())
			Expect(handler).To(BeNil())
			Expect(drainer).To(BeNil())
		},
		Entry("nil infra", func() mcpHandlerParams {
			return mcpHandlerParams{infra: nil, authMw: validAuthMw, inv: validInv, logger: logr.Discard()}
		}),
		Entry("infra with nil kubeConfig", func() mcpHandlerParams {
			return mcpHandlerParams{infra: &k8sInfra{}, authMw: validAuthMw, inv: validInv, logger: logr.Discard()}
		}),
		Entry("nil auth middleware (DD-AUTH-MCP-001)", func() mcpHandlerParams {
			return mcpHandlerParams{infra: validInfra, authMw: nil, inv: validInv, logger: logr.Discard()}
		}),
		Entry("nil investigator (SEC-05)", func() mcpHandlerParams {
			return mcpHandlerParams{infra: validInfra, authMw: validAuthMw, inv: nil, logger: logr.Discard()}
		}),
	)
})
