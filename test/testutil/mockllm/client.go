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
package mockllm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	. "github.com/onsi/gomega"
)

// Client wraps the Mock LLM verification API for test assertions.
type Client struct {
	baseURL string
}

// NewClient creates a new Mock LLM verification client.
func NewClient(baseURL string) *Client {
	return &Client{baseURL: baseURL}
}

// AssertToolCalled asserts that the named tool was called at least once.
func (c *Client) AssertToolCalled(toolName string) {
	calls := c.getToolCalls()
	found := false
	for _, tc := range calls {
		if tc.Name == toolName {
			found = true
			break
		}
	}
	ExpectWithOffset(1, found).To(BeTrue(), "expected tool %q to have been called", toolName)
}

// AssertToolSequence asserts the exact sequence of tool calls made.
func (c *Client) AssertToolSequence(expected ...string) {
	calls := c.getToolCalls()
	actual := make([]string, len(calls))
	for i, tc := range calls {
		actual[i] = tc.Name
	}
	ExpectWithOffset(1, actual).To(Equal(expected), "tool call sequence mismatch")
}

// AssertScenarioMatched asserts the detected scenario name.
func (c *Client) AssertScenarioMatched(expected string) {
	resp := c.getJSON("/api/test/scenario")
	ExpectWithOffset(1, resp["scenario"]).To(Equal(expected), "scenario mismatch")
}

// AssertDAGPath asserts the DAG traversal path.
func (c *Client) AssertDAGPath(expected ...string) {
	resp := c.getJSON("/api/test/dag-path")
	pathRaw := resp["path"].([]interface{})
	path := make([]string, len(pathRaw))
	for i, v := range pathRaw {
		path[i] = v.(string)
	}
	ExpectWithOffset(1, path).To(Equal(expected), "DAG path mismatch")
}

// AssertHeaderReceived asserts that the named header was recorded with the expected value.
func (c *Client) AssertHeaderReceived(name, expectedValue string) {
	resp := c.getJSON("/api/test/headers")
	headers := resp["headers"].(map[string]interface{})
	val, ok := headers[http.CanonicalHeaderKey(name)]
	ExpectWithOffset(1, ok).To(BeTrue(), "header %q not recorded", name)
	ExpectWithOffset(1, val).To(Equal(expectedValue), "header %q value mismatch", name)
}

// AssertNoHeaderReceived asserts that the named header was NOT recorded.
func (c *Client) AssertNoHeaderReceived(name string) {
	resp := c.getJSON("/api/test/headers")
	headers := resp["headers"].(map[string]interface{})
	_, ok := headers[http.CanonicalHeaderKey(name)]
	ExpectWithOffset(1, ok).To(BeFalse(), "header %q should not have been recorded", name)
}

// AssertRequestCount asserts the total number of requests made.
func (c *Client) AssertRequestCount(expected int) {
	resp := c.getJSON("/api/test/request-count")
	count := int(resp["count"].(float64))
	ExpectWithOffset(1, count).To(Equal(expected), "request count mismatch")
}

// ConfigureFault sets fault injection via the API.
func (c *Client) ConfigureFault(enabled bool, statusCode int, message string) {
	cfg := map[string]interface{}{
		"enabled":     enabled,
		"status_code": statusCode,
		"message":     message,
	}
	data, _ := json.Marshal(cfg)
	resp, err := http.Post(c.baseURL+"/api/test/fault", "application/json", bytes.NewReader(data))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	ExpectWithOffset(1, resp.StatusCode).To(Equal(200))
}

// ResetFault disables fault injection.
func (c *Client) ResetFault() {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/test/fault/reset", nil)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	resp, err := http.DefaultClient.Do(req)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	ExpectWithOffset(1, resp.StatusCode).To(Equal(200))
}

// Reset clears all verification state.
func (c *Client) Reset() {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/test/reset", nil)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	resp, err := http.DefaultClient.Do(req)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	ExpectWithOffset(1, resp.StatusCode).To(Equal(200))
}

type toolCallEntry struct {
	Name string `json:"name"`
}

func (c *Client) getToolCalls() []toolCallEntry {
	resp := c.getJSON("/api/test/tool-calls")
	raw, ok := resp["tool_calls"]
	if !ok || raw == nil {
		return nil
	}
	arr := raw.([]interface{})
	result := make([]toolCallEntry, len(arr))
	for i, v := range arr {
		m := v.(map[string]interface{})
		result[i] = toolCallEntry{Name: m["name"].(string)}
	}
	return result
}

func (c *Client) getJSON(path string) map[string]interface{} {
	resp, err := http.Get(c.baseURL + path)
	ExpectWithOffset(2, err).NotTo(HaveOccurred(), fmt.Sprintf("GET %s failed", path))
	defer resp.Body.Close()
	ExpectWithOffset(2, resp.StatusCode).To(Equal(200))

	var result map[string]interface{}
	ExpectWithOffset(2, json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
	return result
}
