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

package server

import (
	"context"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// InvestigationRunner abstracts the investigation entry point so that
// decorators (e.g. alignment.InvestigatorWrapper) can wrap it transparently.
//
// #2190: previously defined in the now-deleted ogen HTTP handler.go. The
// retired HTTP handler was one caller of this abstraction, not its owner --
// the AgentSession dispatcher (internal/kubernautagent/agentsession) is now
// the sole production caller, so the interface moves here as a standalone
// package-level contract, independent of any transport.
type InvestigationRunner interface {
	Investigate(ctx context.Context, signal katypes.SignalContext) (*katypes.InvestigationResult, error)
}
