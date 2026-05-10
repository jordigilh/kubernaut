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

package alignment

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	alignmentVerdictTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kubernaut",
		Subsystem: "alignment",
		Name:      "verdict_total",
		Help:      "Total alignment verdicts by result (aligned, suspicious) and mode (enforce, monitor).",
	}, []string{"result", "mode"})

	alignmentStepTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kubernaut",
		Subsystem: "alignment",
		Name:      "step_total",
		Help:      "Total alignment steps evaluated by outcome (aligned, suspicious, panic).",
	}, []string{"outcome"})

	alignmentCanaryTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kubernaut",
		Subsystem: "alignment",
		Name:      "canary_total",
		Help:      "Total canary checks by result (pass, fail).",
	}, []string{"result"})

	alignmentVerdictDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "kubernaut",
		Subsystem: "alignment",
		Name:      "verdict_duration_seconds",
		Help:      "Time from canary start to verdict completion in seconds.",
		Buckets:   prometheus.DefBuckets,
	})

	alignmentShadowAuditTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kubernaut",
		Subsystem: "alignment",
		Name:      "shadow_audit_total",
		Help:      "Total shadow LLM audit events emitted by event_type (request, response).",
	}, []string{"event_type"})

	alignmentCircuitBreakerTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kubernaut",
		Subsystem: "alignment",
		Name:      "circuit_breaker_total",
		Help:      "Total circuit breaker activations by mode (enforce).",
	}, []string{"mode"})
)
