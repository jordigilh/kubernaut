package ka

import (
	"context"
	"io"
)

// ParseSSEStreamForTest exports parseSSEStream for unit testing.
func ParseSSEStreamForTest(ctx context.Context, body io.Reader, ch chan<- InvestigationEvent) {
	parseSSEStream(ctx, body, ch)
}
