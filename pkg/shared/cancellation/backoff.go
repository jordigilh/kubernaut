package cancellation

import (
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
)

const (
	// MaxCancellationRetries bounds persisted cancellation attempts across
	// child controllers so a backend outage cannot create an endless poll loop.
	MaxCancellationRetries  int32 = 5
	cancellationBackoffBase       = 1 * time.Second
	cancellationBackoffMax        = 30 * time.Second
)

// RetryBackoff returns a bounded exponential delay for cancellation polling.
// Jitter prevents many child controllers from retrying the same backend at the
// same instant after a parent cancellation fan-out.
func RetryBackoff(attempt int32) time.Duration {
	return backoff.Config{
		BasePeriod:    cancellationBackoffBase,
		MaxPeriod:     cancellationBackoffMax,
		Multiplier:    2,
		JitterPercent: 10,
	}.Calculate(attempt)
}
