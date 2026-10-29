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

package investigator

import (
	"context"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/sanitization"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// structuredContextRuntime owns only sanitization and the fixed Phase 3
// structured-context budget. Raw PhaseRCA tool results are never retained.
type structuredContextRuntime struct {
	budget              int
	sanitizer           *sanitization.Pipeline
	additionalSanitizer *sanitization.Pipeline
}

// newStructuredContextRuntime creates the sanitizer and code-owned Phase 3
// context budget without retaining tool observations.
func newStructuredContextRuntime(p Pipeline) *structuredContextRuntime {
	budget := p.StructuredPhase3ContextBudget
	if budget <= 0 {
		budget = katypes.DefaultStructuredPhase3ContextBudget
	}
	sanitizer := sanitization.NewPipeline(sanitization.NewSecretSanitizer(),
		sanitization.NewCredentialSanitizer(), sanitization.NewInjectionSanitizer(nil))
	return &structuredContextRuntime{budget: budget, sanitizer: sanitizer, additionalSanitizer: p.Sanitizer}
}

// sanitizeStructuredContextText sanitizes structured handoff leaves using the
// mandatory baseline and any configured additional policy.
func (inv *Investigator) sanitizeStructuredContextText(ctx context.Context, text string) (string, error) {
	clean, err := inv.structuredContext.sanitizer.Run(ctx, text)
	if err != nil {
		return "", err
	}
	if inv.structuredContext.additionalSanitizer != nil {
		return inv.structuredContext.additionalSanitizer.Run(ctx, clean)
	}
	return clean, nil
}
