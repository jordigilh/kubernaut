package launcher

import (
	"context"
)

// EnrichRRDetailForTest exports enrichRRDetail for unit testing.
func EnrichRRDetailForTest(ctx context.Context, detail map[string]string) {
	enrichRRDetail(ctx, detail)
}
