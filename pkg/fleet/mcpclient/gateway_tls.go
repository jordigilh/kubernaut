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

package mcpclient

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// WithMCPGatewayTLSCAFile configures a scoped, reloadable CA transport for
// MCP Gateway protocol traffic. The option must be applied before the OAuth2
// option so OAuth2 wraps this transport rather than replacing it.
// DD-FLEET-010 defines the independent trust-lane and lifecycle contract.
//
// Construction errors are retained on clientConfig and surfaced by New before
// any MCP handshake. This prevents an explicitly configured CA from silently
// degrading to the process default transport.
func WithMCPGatewayTLSCAFile(caFile string, logger logr.Logger) Option {
	return func(cfg *clientConfig) {
		if caFile == "" {
			return
		}

		reloader, err := sharedtls.NewCAReloaderFromFile(caFile)
		if err != nil {
			cfg.transportErr = fmt.Errorf("mcp gateway tls ca: %w", err)
			return
		}

		watcher, err := hotreload.NewFileWatcher(
			caFile,
			reloader.ReloadCallback,
			logger.WithName("mcp-gateway-ca"),
		)
		if err != nil {
			cfg.transportErr = fmt.Errorf("mcp gateway tls ca watcher: %w", err)
			return
		}

		if cfg.httpClient == nil {
			cfg.httpClient = &http.Client{Transport: reloader}
		} else {
			cfg.httpClient.Transport = reloader
		}
		cfg.gatewayCAWatcher = watcher
	}
}

func startGatewayCAWatcher(ctx context.Context, watcher *hotreload.FileWatcher) error {
	if watcher == nil {
		return nil
	}
	if err := watcher.Start(context.WithoutCancel(ctx)); err != nil {
		watcher.Stop()
		return fmt.Errorf("start mcp gateway tls ca watcher: %w", err)
	}
	return nil
}
