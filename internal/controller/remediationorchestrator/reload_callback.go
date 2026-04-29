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

package controller

import (
	"fmt"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	roconfig "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
)

// NewReloadCallback creates a hotreload.ReloadCallback that applies validated
// configuration changes to the reconciler's hot-reloadable fields.
//
// Per DD-INFRA-001: Graceful degradation — returns error on invalid config
// (watcher keeps previous), logs before/after for audit trail.
//
// Hot-reloadable fields (#835):
//   - dryRun, dryRunHoldPeriod (#712, #736)
//   - retentionPeriod (#265)
//   - asyncPropagation (#253, DD-EM-004)
func NewReloadCallback(reconciler *Reconciler, logger logr.Logger) hotreload.ReloadCallback {
	return func(newContent string) error {
		cfg := roconfig.DefaultConfig()

		if err := yaml.Unmarshal([]byte(newContent), cfg); err != nil {
			return fmt.Errorf("failed to parse config YAML: %w", err)
		}

		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("invalid configuration: %w", err)
		}

		prevDryRun := reconciler.IsDryRunExported()
		prevHoldPeriod := reconciler.GetDryRunHoldPeriodExported()
		prevRetention := reconciler.GetRetentionPeriodExported()
		prevAsync := reconciler.GetAsyncPropagationExported()

		reconciler.SetDryRun(cfg.DryRun, cfg.DryRunHoldPeriod)
		reconciler.SetRetentionPeriod(cfg.Retention.Period)
		reconciler.SetAsyncPropagation(cfg.AsyncPropagation)

		logger.Info("Configuration hot-reloaded successfully",
			"dryRun", fmt.Sprintf("%v → %v", prevDryRun, cfg.DryRun),
			"dryRunHoldPeriod", fmt.Sprintf("%v → %v", prevHoldPeriod, cfg.DryRunHoldPeriod),
			"retentionPeriod", fmt.Sprintf("%v → %v", prevRetention, cfg.Retention.Period),
			"asyncPropagation.gitOpsSyncDelay", fmt.Sprintf("%v → %v", prevAsync.GitOpsSyncDelay, cfg.AsyncPropagation.GitOpsSyncDelay),
			"asyncPropagation.operatorReconcileDelay", fmt.Sprintf("%v → %v", prevAsync.OperatorReconcileDelay, cfg.AsyncPropagation.OperatorReconcileDelay),
			"asyncPropagation.proactiveAlertDelay", fmt.Sprintf("%v → %v", prevAsync.ProactiveAlertDelay, cfg.AsyncPropagation.ProactiveAlertDelay),
		)

		return nil
	}
}
