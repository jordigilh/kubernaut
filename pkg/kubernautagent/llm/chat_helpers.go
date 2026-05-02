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
)

// ChatWithParams wraps a Client.Chat call, injecting runtime parameters
// (temperature, timeout) from the hot-reloadable LLM config. The context
// timeout is cancelled immediately after Chat returns to avoid resource leaks.
func ChatWithParams(ctx context.Context, client Client, req ChatRequest, params RuntimeParams) (ChatResponse, error) {
	temp := params.Temperature
	req.Options.Temperature = &temp

	chatCtx := ctx
	var chatCancel context.CancelFunc
	if params.TimeoutSeconds > 0 {
		chatCtx, chatCancel = context.WithTimeout(ctx, time.Duration(params.TimeoutSeconds)*time.Second)
	}

	resp, err := client.Chat(chatCtx, req)

	if chatCancel != nil {
		chatCancel()
	}

	return resp, err
}
