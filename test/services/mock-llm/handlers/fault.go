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
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/fault"
)

type faultHandler struct {
	injector *fault.Injector
}

func (fh *faultHandler) handleConfigureFault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var cfg fault.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	fh.injector.Configure(cfg)
	writeJSON(w, http.StatusOK, map[string]string{"status": "configured"})
}

func (fh *faultHandler) handleGetFault(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, fh.injector.GetConfig())
}

func (fh *faultHandler) handleResetFault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	fh.injector.Reset()
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}
