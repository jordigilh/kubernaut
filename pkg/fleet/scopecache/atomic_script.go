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

package scopecache

import (
	"context"
	"crypto/sha1" // #nosec G401,G505 -- SHA-1 is the Valkey SCRIPT LOAD/EVALSHA identifier, not a security hash.
	"embed"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

//go:embed scripts/fmc_v2_atomic.lua
var atomicScriptFS embed.FS

const atomicScriptPath = "scripts/fmc_v2_atomic.lua"

// ScriptManager loads the one approved mutation script and executes all cache
// operations through its fixed EVALSHA digest.
type ScriptManager struct {
	client *redis.Client
	script string
	digest string

	mu     sync.Mutex
	loaded bool
}

// atomicCommand prevents go-redis from replaying an EVALSHA whose reply was
// lost. The server may already have mutated the partition. Only eval's explicit
// definitive-NOSCRIPT recovery may issue the command a second time (#2593).
type atomicCommand struct{ *redis.Cmd }

func (*atomicCommand) NoRetry() bool { return true }

func (m *ScriptManager) execute(ctx context.Context, keys []string, args ...interface{}) ([]interface{}, error) {
	commandArgs := make([]interface{}, 0, 3+len(keys)+len(args))
	commandArgs = append(commandArgs, "evalsha", m.digest, len(keys))
	for _, key := range keys {
		commandArgs = append(commandArgs, key)
	}
	commandArgs = append(commandArgs, args...)
	command := &atomicCommand{Cmd: redis.NewCmd(ctx, commandArgs...)}
	if err := m.client.Process(ctx, command); err != nil {
		var serverError redis.Error
		if errors.As(err, &serverError) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: atomic script reply unavailable: %w", ErrAmbiguousWrite, err)
	}
	items, err := asScriptArray(command.Val())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAmbiguousWrite, err)
	}
	return items, nil
}

// NewScriptManager creates a Valkey script manager. Load must succeed before
// the service publishes readiness.
func NewScriptManager(client *redis.Client) (*ScriptManager, error) {
	if client == nil {
		return nil, errors.New("valkey client is required")
	}
	script, err := atomicScriptFS.ReadFile(atomicScriptPath)
	if err != nil {
		return nil, fmt.Errorf("read atomic cache script: %w", err)
	}
	digestBytes := sha1.Sum(script) //nolint:gosec // Redis EVALSHA uses SHA-1 as a protocol digest, not a security primitive
	return &ScriptManager{
		client: client,
		script: string(script),
		digest: fmt.Sprintf("%x", digestBytes[:]),
	}, nil
}

// Digest returns the embedded SHA-1 identifier used by EVALSHA.
func (m *ScriptManager) Digest() string { return m.digest }

func (m *ScriptManager) loadedState() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loaded
}

// Load bootstraps the script and verifies that Valkey returned the embedded
// digest. This is the only operation that uses SCRIPT LOAD.
func (m *ScriptManager) Load(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadLocked(ctx)
}

func (m *ScriptManager) loadLocked(ctx context.Context) error {
	digest, err := m.client.ScriptLoad(ctx, m.script).Result()
	if err != nil {
		m.loaded = false
		return fmt.Errorf("%w: script load failed: %w", ErrScriptUnavailable, err)
	}
	if digest != m.digest {
		m.loaded = false
		return fmt.Errorf("%w: script digest mismatch: got %s want %s", ErrScriptUnavailable, digest, m.digest)
	}
	m.loaded = true
	return nil
}

func (m *ScriptManager) eval(ctx context.Context, keys []string, args ...interface{}) ([]interface{}, error) {
	m.mu.Lock()
	loaded := m.loaded
	m.mu.Unlock()
	if !loaded {
		if err := m.Load(ctx); err != nil {
			return nil, err
		}
	}
	result, err := m.execute(ctx, keys, args...)
	if err == nil || !isNoScript(err) {
		return result, err
	}
	m.mu.Lock()
	loadErr := m.loadLocked(ctx)
	m.mu.Unlock()
	if loadErr != nil {
		return nil, loadErr
	}
	result, err = m.execute(ctx, keys, args...)
	if isNoScript(err) {
		return nil, fmt.Errorf("%w: NOSCRIPT after recovery", ErrScriptUnavailable)
	}
	return result, err
}

func asScriptArray(value interface{}) ([]interface{}, error) {
	items, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected atomic script result %T", value)
	}
	return items, nil
}

func isNoScript(err error) bool {
	var serverError redis.Error
	if !errors.As(err, &serverError) {
		return false
	}
	message := strings.TrimSpace(serverError.Error())
	return message == "NOSCRIPT" || strings.HasPrefix(message, "NOSCRIPT ")
}
