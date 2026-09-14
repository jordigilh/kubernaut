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

import "strings"

// ScenarioSelector is the canonical declarative matcher for static scenarios.
// See DD-TEST-017 for the migration and deprecation policy.
// CustomMatch is reserved for scenarios whose matching depends on state or
// context that cannot be represented by keywords, signals, and scope.
type ScenarioSelector struct {
	Scope             ScenarioScope
	Keywords          []string
	SignalPatterns    []string
	MatchLastUserOnly bool
	RequireProactive  bool
	Confidence        float64
	CustomMatch       func(ctx *DetectionContext) (bool, float64)
}

// Match evaluates the selector against a request context. Keyword and signal
// values are treated case-insensitively and match if any configured value is
// present. An empty selector does not match.
func (s ScenarioSelector) Match(ctx *DetectionContext) (bool, float64) {
	if ctx == nil {
		return false, 0
	}
	if s.RequireProactive && !isProactive(ctx) {
		return false, 0
	}
	if s.CustomMatch != nil {
		return s.CustomMatch(ctx)
	}

	target := strings.ToLower(ctx.Content + " " + ctx.AllText)
	if s.MatchLastUserOnly {
		if ctx.LastUserContent == "" {
			return false, 0
		}
		target = strings.ToLower(ctx.LastUserContent)
	}
	for _, keyword := range s.Keywords {
		if strings.Contains(target, strings.ToLower(keyword)) {
			return true, s.confidence(1.0)
		}
	}

	signal := extractSignal(ctx)
	for _, pattern := range s.SignalPatterns {
		if strings.Contains(signal, strings.ToLower(pattern)) {
			return true, s.confidence(0.8)
		}
	}
	return false, 0
}

func (s ScenarioSelector) confidence(defaultConfidence float64) float64 {
	if s.Confidence != 0 {
		return s.Confidence
	}
	return defaultConfidence
}

func newSelectorScenario(name string, selector ScenarioSelector, cfg MockScenarioConfig) *configScenario {
	cfg.ScenarioName = name
	selector.Keywords = append([]string(nil), selector.Keywords...)
	selector.SignalPatterns = append([]string(nil), selector.SignalPatterns...)
	return &configScenario{config: cfg, selector: selector}
}

func newKeywordScenario(name, keyword string, cfg MockScenarioConfig) *configScenario {
	return newSelectorScenario(name, ScenarioSelector{
		Keywords:   []string{keyword, strings.ReplaceAll(keyword, "_", " ")},
		Confidence: 1.0,
	}, cfg)
}

func newKeywordScenarioMulti(name string, keywords []string, cfg MockScenarioConfig) *configScenario {
	return newSelectorScenario(name, ScenarioSelector{
		Keywords:   keywords,
		Confidence: 1.0,
	}, cfg)
}

func newSignalScenario(name string, patterns []string, cfg MockScenarioConfig) *configScenario {
	return newSelectorScenario(name, ScenarioSelector{
		SignalPatterns: patterns,
		Confidence:     0.8,
	}, cfg)
}
