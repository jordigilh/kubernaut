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
	"sync"
	"time"
)

// DefaultAnomalyDetectorTTL bounds how long a per-investigation AnomalyDetector
// entry may sit idle before StartAnomalyDetectorCleanupLoop reclaims it.
// Investigations normally complete in minutes; 2h gives ample margin for
// slow interactive sessions while still bounding worst-case memory growth
// when a correlationID's investigation ends without a clean exit path
// (crash, cancellation, client disconnect).
const DefaultAnomalyDetectorTTL = 2 * time.Hour

// anomalyDetectorEntry tracks a per-investigation AnomalyDetector alongside
// the last time it was accessed, so the TTL sweep can identify abandoned
// entries without depending on any investigation exit path firing (#1892).
type anomalyDetectorEntry struct {
	detector   *AnomalyDetector
	lastAccess time.Time
}

// anomalyScope holds one isolated AnomalyDetector per investigation
// (keyed by correlation ID), cloned from a shared config template. This
// replaces the pre-#1892 design where every concurrent investigation on a
// KA pod shared a single AnomalyDetector, so one investigation's Reset()
// (fired at RCA->workflow-discovery phase transitions) could silently zero
// another in-flight investigation's tool-call budget.
type anomalyScope struct {
	mu       sync.Mutex
	template *AnomalyDetector
	entries  map[string]*anomalyDetectorEntry
}

// anomalyDetectorFor returns the AnomalyDetector scoped to correlationID,
// creating one (cloned from the shared config template) on first access.
// Safe for concurrent use. An empty correlationID falls back to the shared
// template detector directly: this should not happen on any production call
// path (correlationID is always signal.RemediationID or an explicit
// parameter), but failing open to a single detector is safer than panicking
// or silently creating an unbounded number of ""-keyed entries.
func (inv *Investigator) anomalyDetectorFor(correlationID string) *AnomalyDetector {
	if correlationID == "" {
		return inv.anomalyScope.template
	}

	inv.anomalyScope.mu.Lock()
	defer inv.anomalyScope.mu.Unlock()

	entry, ok := inv.anomalyScope.entries[correlationID]
	if !ok {
		entry = &anomalyDetectorEntry{detector: inv.anomalyScope.template.Clone()}
		inv.anomalyScope.entries[correlationID] = entry
	}
	entry.lastAccess = time.Now()
	return entry.detector
}

// pruneAnomalyDetectors removes entries whose lastAccess predates
// time.Now()-maxAge, and returns the number of entries removed.
func (inv *Investigator) pruneAnomalyDetectors(maxAge time.Duration) int {
	cutoff := time.Now().Add(-maxAge)

	inv.anomalyScope.mu.Lock()
	defer inv.anomalyScope.mu.Unlock()

	removed := 0
	for id, entry := range inv.anomalyScope.entries {
		if entry.lastAccess.Before(cutoff) {
			delete(inv.anomalyScope.entries, id)
			removed++
		}
	}
	return removed
}

// StartAnomalyDetectorCleanupLoop periodically prunes per-investigation
// AnomalyDetector entries idle longer than maxAge, until ctx is cancelled.
// Mirrors session.Store.StartCleanupLoop's background-sweep pattern (#1892).
func (inv *Investigator) StartAnomalyDetectorCleanupLoop(ctx context.Context, interval, maxAge time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				removed := inv.pruneAnomalyDetectors(maxAge)
				if removed > 0 {
					inv.logger.V(1).Info("pruned idle per-investigation anomaly detectors",
						"removed", removed, "max_age", maxAge)
				}
			}
		}
	}()
}
