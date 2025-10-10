//go:build integration

<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package ai

import (
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Integration scenario management and utilities

type IntegrationScenarioManager struct {
	logger          *logrus.Logger
	scenarioResults map[string]*ScenarioResult
	mutex           sync.RWMutex
}

type ScenarioResult struct {
	Name      string
	Success   bool
	Duration  time.Duration
	Timestamp time.Time
	Metadata  map[string]interface{}
}

func NewIntegrationScenarioManager(logger *logrus.Logger) *IntegrationScenarioManager {
	return &IntegrationScenarioManager{
		logger:          logger,
		scenarioResults: make(map[string]*ScenarioResult),
	}
}

func (ism *IntegrationScenarioManager) RecordScenario(name string, success bool, duration time.Duration) {
	ism.mutex.Lock()
	defer ism.mutex.Unlock()

	ism.scenarioResults[name] = &ScenarioResult{
		Name:      name,
		Success:   success,
		Duration:  duration,
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
}

func (ism *IntegrationScenarioManager) GenerateReport() map[string]interface{} {
	ism.mutex.RLock()
	defer ism.mutex.RUnlock()

	totalScenarios := len(ism.scenarioResults)
	successfulScenarios := 0
	totalDuration := time.Duration(0)

	for _, result := range ism.scenarioResults {
		if result.Success {
			successfulScenarios++
		}
		totalDuration += result.Duration
	}

	var avgDuration time.Duration
	if totalScenarios > 0 {
		avgDuration = totalDuration / time.Duration(totalScenarios)
	}

	return map[string]interface{}{
		"total_scenarios":      totalScenarios,
		"successful_scenarios": successfulScenarios,
		"success_rate":         fmt.Sprintf("%.2f%%", float64(successfulScenarios)/float64(totalScenarios)*100),
		"average_duration":     avgDuration,
		"scenario_details":     ism.scenarioResults,
	}
}
