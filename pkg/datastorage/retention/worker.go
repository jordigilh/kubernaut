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

package retention

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
)

// Worker periodically purges expired audit events from PostgreSQL.
// BR-AUDIT-009: Retention policies for audit data.
// Lifecycle matches DLQRetryWorker (Start/Stop).
type Worker struct {
	db     *sql.DB
	config Config
	logger logr.Logger
	clock  Clock

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewWorker creates a retention worker. Does not start automatically.
func NewWorker(db *sql.DB, config Config, logger logr.Logger) *Worker {
	return &Worker{
		db:     db,
		config: config,
		logger: logger.WithName("retention-worker"),
		clock:  UTCClock{},
	}
}

// Start begins the periodic purge loop. No-op if retention is disabled.
func (w *Worker) Start(ctx context.Context) {
	if !w.config.Enabled {
		w.logger.Info("Retention worker disabled (config.retention.enabled=false)")
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	workerCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel

	w.wg.Add(1)
	go w.run(workerCtx)

	w.logger.Info("Retention worker started",
		"interval", w.config.GetInterval(),
		"batch_size", w.config.GetBatchSize(),
	)
}

// Stop cancels the worker and waits for completion. Safe if Start was skipped or disabled.
func (w *Worker) Stop() {
	if w.cancel == nil {
		return
	}
	w.cancel()
	w.wg.Wait()
	w.cancel = nil
	w.logger.Info("Retention worker stopped")
}

func (w *Worker) run(ctx context.Context) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.config.GetInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.executePurge(ctx)
		}
	}
}

func (w *Worker) executePurge(ctx context.Context) {
	runID := uuid.New()
	start := time.Now()

	w.logger.Info("Starting retention purge run",
		"run_id", runID,
		"batch_size", w.config.GetBatchSize(),
	)

	now := w.clock.Now()
	totalDeleted := int64(0)

	for {
		result, err := w.db.ExecContext(ctx,
			PurgeSQLBatched,
			now,
			w.config.GetBatchSize(),
		)
		if err != nil {
			w.logger.Error(err, "Retention purge batch failed",
				"run_id", runID,
				"total_deleted_so_far", totalDeleted,
			)
			w.recordOperation(ctx, runID, totalDeleted, start, "failed", err.Error())
			return
		}

		rowsAffected, _ := result.RowsAffected()
		totalDeleted += rowsAffected

		if rowsAffected < int64(w.config.GetBatchSize()) {
			break
		}
	}

	duration := time.Since(start)
	w.logger.Info("Retention purge run completed",
		"run_id", runID,
		"rows_deleted", totalDeleted,
		"duration", duration,
	)

	w.recordOperation(ctx, runID, totalDeleted, start, "completed", "")
}

func (w *Worker) recordOperation(ctx context.Context, runID uuid.UUID, rowsDeleted int64, start time.Time, status, errMsg string) {
	duration := time.Since(start)
	_, err := w.db.ExecContext(ctx,
		`INSERT INTO retention_operations (run_id, rows_deleted, status, error_message, operation_start, operation_end, operation_duration_ms)
		 VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7)`,
		runID, rowsDeleted, status, errMsg, start, time.Now().UTC(), duration.Milliseconds(),
	)
	if err != nil {
		w.logger.Error(err, "Failed to record retention operation",
			"run_id", runID,
		)
	}
}
