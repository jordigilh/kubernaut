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
package tracker

import (
	"net/http"
	"strings"
	"sync"
)

// HeaderRecorder captures configured HTTP headers from incoming requests.
type HeaderRecorder struct {
	mu            sync.RWMutex
	headerNames   []string
	recorded      map[string]string
}

// NewHeaderRecorder creates a recorder for the comma-separated header names.
func NewHeaderRecorder(headerList string) *HeaderRecorder {
	var names []string
	for _, h := range strings.Split(headerList, ",") {
		h = strings.TrimSpace(h)
		if h != "" {
			names = append(names, http.CanonicalHeaderKey(h))
		}
	}
	return &HeaderRecorder{
		headerNames: names,
		recorded:    make(map[string]string),
	}
}

// RecordFrom captures the configured headers from the HTTP request.
func (hr *HeaderRecorder) RecordFrom(r *http.Request) {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	for _, name := range hr.headerNames {
		if v := r.Header.Get(name); v != "" {
			hr.recorded[name] = v
		}
	}
}

// GetRecordedHeaders returns a copy of the captured headers.
func (hr *HeaderRecorder) GetRecordedHeaders() map[string]string {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	result := make(map[string]string, len(hr.recorded))
	for k, v := range hr.recorded {
		result[k] = v
	}
	return result
}

// Reset clears all recorded headers.
func (hr *HeaderRecorder) Reset() {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	hr.recorded = make(map[string]string)
}
