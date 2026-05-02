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

package llm

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
)

var defaultRetryBackoff = backoff.Config{
	BasePeriod:    500 * time.Millisecond,
	MaxPeriod:     5 * time.Second,
	Multiplier:    2.0,
	JitterPercent: 20,
}

// ChatWithParams wraps a Client.Chat call, injecting runtime parameters
// (temperature, timeout, retries) from the hot-reloadable LLM config.
// Each attempt gets a fresh context timeout; the timeout is cancelled
// immediately after Chat returns to avoid resource leaks.
// Retries use exponential backoff and respect parent context cancellation.
func ChatWithParams(ctx context.Context, client Client, req ChatRequest, params RuntimeParams) (ChatResponse, error) {
	temp := params.Temperature
	req.Options.Temperature = &temp

	bo := defaultRetryBackoff
	if params.RetryBackoff != nil {
		bo = *params.RetryBackoff
	}

	maxAttempts := 1 + params.MaxRetries
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		chatCtx := ctx
		var chatCancel context.CancelFunc
		if params.TimeoutSeconds > 0 {
			chatCtx, chatCancel = context.WithTimeout(ctx, time.Duration(params.TimeoutSeconds)*time.Second)
		}

		resp, err := client.Chat(chatCtx, req)

		if chatCancel != nil {
			chatCancel()
		}

		if err == nil {
			return resp, nil
		}

		lastErr = err

		if attempt < maxAttempts-1 {
			delay := bo.Calculate(int32(attempt + 1))
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ChatResponse{}, ctx.Err()
			case <-timer.C:
			}
		}
	}

	return ChatResponse{}, lastErr
}
