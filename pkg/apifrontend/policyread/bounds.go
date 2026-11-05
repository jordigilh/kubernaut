package policyread

import (
	"time"

	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
)

// These values are provisional implementation safety defaults for the local
// Console projection. They are deliberately not a product item cap and have
// not been calibrated as production RSS limits. Change them only with memory,
// latency, and response-size evidence.
const (
	// DefaultInventoryWorkingSetBytes bounds retained canonical projections for
	// one local inventory operation.
	DefaultInventoryWorkingSetBytes int64 = 32 << 20

	// defaultInventoryBackendPageLimit is the positive Kubernetes API-server
	// item-count hint for one metadata traversal request. It is an internal
	// traversal chunk, not the Console page limit or an inventory total cap.
	defaultInventoryBackendPageLimit int64 = inventory.MaxLimit

	// DefaultInventoryBackendResponseBytes limits one decrypted Kubernetes API
	// response before client-go decodes it. It is a provisional containment
	// safeguard, not a Console item cap or a calibrated RSS guarantee.
	DefaultInventoryBackendResponseBytes int64 = 16 << 20

	// DefaultInventoryResponseBytes is enforced at the existing Console handler
	// JSON serialization seam. Oversized successful responses become a
	// state-only unavailable response; no partial response is written.
	DefaultInventoryResponseBytes int64 = 2 << 20

	// DefaultInventoryOperationTimeout bounds the whole local List/Get
	// operation, including time waiting for the local inventory semaphore.
	DefaultInventoryOperationTimeout = 15 * time.Second

	// DefaultInventoryMaxConcurrentReads is process-local and therefore applies
	// independently to each API Frontend replica.
	DefaultInventoryMaxConcurrentReads = 2
)
