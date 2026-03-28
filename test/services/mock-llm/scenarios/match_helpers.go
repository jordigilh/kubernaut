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
package scenarios

import (
	"regexp"
	"strings"
)

var reSignalName = regexp.MustCompile(`(?i)signal name:\s*(\S+)`)

func mockKeywordScenario(name, keyword string, cfg MockScenarioConfig) *configScenario {
	return mockKeywordScenarioMulti(name, []string{keyword, strings.ReplaceAll(keyword, "_", " ")}, cfg)
}

func mockKeywordScenarioMulti(name string, keywords []string, cfg MockScenarioConfig) *configScenario {
	cfg.ScenarioName = name
	return &configScenario{
		config: cfg,
		matchFunc: func(ctx *DetectionContext) (bool, float64) {
			combined := strings.ToLower(ctx.Content + " " + ctx.AllText)
			for _, kw := range keywords {
				if strings.Contains(combined, kw) {
					return true, 1.0
				}
			}
			return false, 0
		},
	}
}

func signalScenario(name string, patterns []string, cfg MockScenarioConfig) *configScenario {
	cfg.ScenarioName = name
	return &configScenario{
		config: cfg,
		matchFunc: func(ctx *DetectionContext) (bool, float64) {
			signal := extractSignal(ctx)
			if signal == "" {
				return false, 0
			}
			for _, p := range patterns {
				if strings.Contains(signal, p) {
					return true, 0.8
				}
			}
			return false, 0
		},
	}
}

func isProactive(ctx *DetectionContext) bool {
	if ctx.IsProactive {
		return true
	}
	lower := strings.ToLower(ctx.Content)
	return (strings.Contains(lower, "proactive mode") || strings.Contains(lower, "proactive signal")) ||
		(strings.Contains(lower, "predicted") && strings.Contains(lower, "not yet occurred"))
}

func extractSignal(ctx *DetectionContext) string {
	if ctx.SignalName != "" {
		return strings.ToLower(ctx.SignalName)
	}
	m := reSignalName.FindStringSubmatch(ctx.Content)
	if len(m) > 1 {
		return strings.ToLower(strings.TrimSpace(m[1]))
	}
	return ""
}
