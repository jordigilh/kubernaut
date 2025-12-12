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

// Package hotreload provides generic ConfigMap hot-reloading functionality.
//
// Per DD-INFRA-001: ConfigMap Hot-Reload Pattern
// See: docs/architecture/decisions/DD-INFRA-001-configmap-hotreload-pattern.md
//
// This package enables any service to dynamically reload configuration from
// ConfigMaps without pod restarts. It uses fsnotify to watch mounted ConfigMap
// files and triggers callbacks when content changes.
//
// Key Features:
//   - Standard Kubernetes pattern (ConfigMap volume mounts)
//   - fsnotify-based file watching (~60s update latency)
//   - Graceful degradation (keeps old config on callback errors)
//   - Thread-safe configuration access
//   - SHA256 hash tracking for audit/debugging
//
// Usage Example:
//
//	watcher, err := hotreload.NewFileWatcher(
//	    "/etc/kubernaut/policies/priority.rego",
//	    func(content string) error {
//	        // Compile and validate new content
//	        return nil
//	    },
//	    logger,
//	)
//	if err != nil {
//	    return err
//	}
//
//	if err := watcher.Start(ctx); err != nil {
//	    return err
//	}
//	defer watcher.Stop()
//
// The FileWatcher is used by:
//   - Signal Processing: Priority Engine (Rego), Environment Classifier (ConfigMap overrides)
//   - AI Analysis: Safety policies, context filtering rules
//   - Workflow Execution: Playbook selection policies, execution parameters
//   - Gateway: Rate limiting rules, adapter configurations
package hotreload






