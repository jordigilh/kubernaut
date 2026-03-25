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

package dlq

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ========================================
// DLQ PROMETHEUS METRICS (Gap 3.3 REFACTOR)
// 📋 Authority: DD-009 (DLQ Fallback), ADR-038 (Async Buffered Audit)
// ========================================
//
// Prometheus metrics for DLQ capacity monitoring and alerting.
//
// REFACTOR enhancements:
// - Prometheus metric export for capacity monitoring
// - Real-time capacity ratio tracking
// - Alert threshold metrics for monitoring systems
// - Per-stream granularity (notifications, events)
// ========================================

var (
	// DLQ threshold alerts (0 or 1 for alert state)
	dlqWarning = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "datastorage_dlq_warning",
		Help: "DLQ at 80% capacity (1 = warning active)",
	}, []string{"stream"})

	dlqCritical = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "datastorage_dlq_critical",
		Help: "DLQ at 90% capacity (1 = critical alert active)",
	}, []string{"stream"})
)
