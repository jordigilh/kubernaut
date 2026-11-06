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
// A disabled or unconfigured Fleet block returns a nil reader. ACM Console
// inventory is unavailable until Search visibility and completeness are qualified
// (DD-AF-018); this does not affect the independent ACM scope checker.
func NewInventoryReader(cfg FleetConfig) (inventory.Reader, error) {
	if !cfg.Enabled || cfg.EffectiveEndpoint() == "" {
		return nil, nil //nolint:nilnil // disabled Fleet is a valid no-reader state
	}

	backendName := cfg.effectiveBackend()
	if backendName == BackendACM {
		return nil, nil //nolint:nilnil // ACM Console inventory deliberately fails closed pending qualification
	}
	transport := http.DefaultTransport
	caTransport, err := caReloaderTransportOrNil(backendName, cfg.TLSCAFile)
	if err != nil {
		return nil, err
	}
	if caTransport != nil {
		transport = caTransport
	}

	switch backendName {
	case BackendFMC:
		tokenSource := auth.NewDefaultTokenSource()
		if cfg.TokenPath != "" {
			tokenSource = auth.NewTokenSource(cfg.TokenPath)
		}
		transport = auth.NewAuthTransport(tokenSource, transport)
		return fmc.NewHTTPClient(cfg.EffectiveEndpoint(), fmc.WithHTTPClient(&http.Client{
			Timeout:   5 * time.Second,
			Transport: transport,
		})), nil
	default:
		return nil, fmt.Errorf("fleet: unsupported inventory backend %q", backendName)
	}
}
