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

// Package shared holds the Fleet Metadata Cache (FMC) E2E scenario bodies
// that are common to both gateway lanes (Kuadrant --
// test/e2e/fleetmetadatacache, and Envoy AI Gateway --
// test/e2e/fleetmetadatacache/eaigw). Only the actual gateway differences
// (dynamic-registration CRD shape, RBAC surface, resource-name/scenario-ID
// prefixes) are abstracted behind the Variant interface (variant.go); every
// assertion here runs unmodified against whichever gateway lane wires it up.
//
// Each Describe/It tree is exported as a function (e.g. SyncJourney) that a
// variant package registers at package-init time via a one-line
// `var _ = shared.SyncJourney(harness, someVariant{})` in its own
// *_test.go file -- the harness pointer's fields are populated later, in
// that package's SynchronizedBeforeSuite, exactly as they would be for
// package-level vars in a non-shared suite (Ginkgo only needs the Describe
// tree registered before RunSpecs; it does not need the referenced data to
// exist yet).
//
// Authority: Issue #54, ADR-068, BR-INTEGRATION-065. See
// docs/testing/BR-INTEGRATION-054/TEST_PLAN.md for the full scenario
// inventory (both lanes) and docs/architecture/DESIGN_DECISIONS.md for the
// DD covering this shared-scenario refactor.
package shared

import (
	"context"
	"net/http"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Timing constants shared by every scenario. Values match what both lanes
// used independently before this refactor (see git history of
// sync_journey_test.go for the fmcSyncTimeout rationale: FMC's
// sync.interval=10s / keyTtl=30s and syncAll() iterates 3 clusters x 6
// resource kinds sequentially per cycle, so worst-case staleness exceeds the
// nominal 10s interval -- 60s gives ample margin).
const (
	Timeout     = 60 * time.Second
	Interval    = 2 * time.Second
	SyncTimeout = 60 * time.Second
)

// Harness carries the per-lane runtime state (populated by each variant
// package's own SynchronizedBeforeSuite) that every shared scenario needs:
// a live Kubernetes client, FMC's own HTTP API, and the kubeconfig path for
// shelling out to kubectl (least-privilege RBAC checks, resilience's Valkey
// scale commands).
//
// A *Harness is constructed once per variant package (as a package-level
// var) and passed by pointer to every shared.XxxJourney(...) call; its
// fields are filled in by SynchronizedBeforeSuite after the Describe tree
// has already been registered, which is safe because Ginkgo's It() closures
// only read harness fields when they execute (at RunSpecs time), not when
// they are registered (at package-init time).
type Harness struct {
	Ctx context.Context

	K8sClient      client.Client
	KubeconfigPath string
	Namespace      string

	FMCHTTPClient *http.Client
	FMCAPIBaseURL string

	// RemoteK8sClient/RemoteKubeconfigPath target the second, independent
	// Kind cluster backing the "prod-east" registration (DD-TEST-013, Spike
	// S19). Used by the cross-cluster isolation scenario to create/verify
	// resources directly against the remote control plane, proving genuine
	// isolation from loopback-cluster/prod-west (which share the primary
	// cluster's API server).
	RemoteK8sClient      client.Client
	RemoteKubeconfigPath string
}
