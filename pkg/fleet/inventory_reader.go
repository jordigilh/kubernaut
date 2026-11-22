package fleet

import (
	"fmt"
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// NewInventoryReader creates the authenticated control-plane projection used
// by Console remote reads. It never creates a remote Kubernetes or MCP reader.
// The reader is built only from cfg.FMC.Endpoint (fleet.fmc.endpoint,
// DD-FLEET-017 D2.1a): a disabled Fleet block or an absent FMC inventory
// endpoint returns a nil reader, and the inventory path never inherits the
// scope adapter's fleet.endpoint, the auto-derived FMC service URL, or the
// ACM Search URL. The former DD-AF-018 BackendACM gate is removed: with
// fleet.fmc.endpoint configured the same FMC reader is wired under any valid
// backend, because FMC is the sole Console inventory contact.
func NewInventoryReader(cfg FleetConfig) (inventory.Reader, error) {
	if !cfg.Enabled || cfg.FMC.Endpoint == "" {
		return nil, nil //nolint:nilnil // disabled Fleet or absent FMC inventory endpoint is a valid no-reader state
	}
	backendName := cfg.effectiveBackend()
	if !supportedBackends[backendName] {
		return nil, fmt.Errorf("fleet: unsupported inventory backend %q", backendName)
	}
	transport := http.DefaultTransport
	caTransport, err := caReloaderTransportOrNil(backendName, cfg.TLSCAFile)
	if err != nil {
		return nil, err
	}
	if caTransport != nil {
		transport = caTransport
	}
	tokenSource := auth.NewDefaultTokenSource()
	if cfg.TokenPath != "" {
		tokenSource = auth.NewTokenSource(cfg.TokenPath)
	}
	transport = auth.NewAuthTransport(tokenSource, transport)
	return fmc.NewHTTPClient(cfg.FMC.Endpoint, fmc.WithHTTPClient(&http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	})), nil
}
