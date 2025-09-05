package logging

import (
	"errors"
	"testing"
	"time"
)

func TestNewFields(t *testing.T) {
	fields := NewFields()
	if fields == nil {
		t.Fatal("NewFields() returned nil")
	}
	if len(fields) != 0 {
		t.Errorf("NewFields() should be empty, got %d fields", len(fields))
	}
}

func TestStandardFields_Component(t *testing.T) {
	fields := NewFields().Component("test-component")

	if fields["component"] != "test-component" {
		t.Errorf("Component() = %v, want %v", fields["component"], "test-component")
	}
}

func TestStandardFields_Operation(t *testing.T) {
	fields := NewFields().Operation("create")

	if fields["operation"] != "create" {
		t.Errorf("Operation() = %v, want %v", fields["operation"], "create")
	}
}

func TestStandardFields_Resource(t *testing.T) {
	fields := NewFields().Resource("pod", "my-pod")

	if fields["resource_type"] != "pod" {
		t.Errorf("Resource() resource_type = %v, want %v", fields["resource_type"], "pod")
	}
	if fields["resource_name"] != "my-pod" {
		t.Errorf("Resource() resource_name = %v, want %v", fields["resource_name"], "my-pod")
	}
}

func TestStandardFields_ResourceWithoutName(t *testing.T) {
	fields := NewFields().Resource("pod", "")

	if fields["resource_type"] != "pod" {
		t.Errorf("Resource() resource_type = %v, want %v", fields["resource_type"], "pod")
	}
	if _, exists := fields["resource_name"]; exists {
		t.Error("Resource() should not set resource_name when empty")
	}
}

func TestStandardFields_Duration(t *testing.T) {
	duration := 150 * time.Millisecond
	fields := NewFields().Duration(duration)

	if fields["duration_ms"] != int64(150) {
		t.Errorf("Duration() = %v, want %v", fields["duration_ms"], int64(150))
	}
}

func TestStandardFields_Error(t *testing.T) {
	err := errors.New("test error")
	fields := NewFields().Error(err)

	if fields["error"] != "test error" {
		t.Errorf("Error() = %v, want %v", fields["error"], "test error")
	}
}

func TestStandardFields_ErrorNil(t *testing.T) {
	fields := NewFields().Error(nil)

	if _, exists := fields["error"]; exists {
		t.Error("Error(nil) should not set error field")
	}
}

func TestStandardFields_UserID(t *testing.T) {
	fields := NewFields().UserID("user-123")

	if fields["user_id"] != "user-123" {
		t.Errorf("UserID() = %v, want %v", fields["user_id"], "user-123")
	}
}

func TestStandardFields_UserIDEmpty(t *testing.T) {
	fields := NewFields().UserID("")

	if _, exists := fields["user_id"]; exists {
		t.Error("UserID(\"\") should not set user_id field")
	}
}

func TestStandardFields_RequestID(t *testing.T) {
	fields := NewFields().RequestID("req-123")

	if fields["request_id"] != "req-123" {
		t.Errorf("RequestID() = %v, want %v", fields["request_id"], "req-123")
	}
}

func TestStandardFields_TraceID(t *testing.T) {
	fields := NewFields().TraceID("trace-123")

	if fields["trace_id"] != "trace-123" {
		t.Errorf("TraceID() = %v, want %v", fields["trace_id"], "trace-123")
	}
}

func TestStandardFields_StatusCode(t *testing.T) {
	fields := NewFields().StatusCode(404)

	if fields["status_code"] != 404 {
		t.Errorf("StatusCode() = %v, want %v", fields["status_code"], 404)
	}
}

func TestStandardFields_Method(t *testing.T) {
	fields := NewFields().Method("GET")

	if fields["method"] != "GET" {
		t.Errorf("Method() = %v, want %v", fields["method"], "GET")
	}
}

func TestStandardFields_URL(t *testing.T) {
	fields := NewFields().URL("https://api.example.com")

	if fields["url"] != "https://api.example.com" {
		t.Errorf("URL() = %v, want %v", fields["url"], "https://api.example.com")
	}
}

func TestStandardFields_Count(t *testing.T) {
	fields := NewFields().Count(42)

	if fields["count"] != 42 {
		t.Errorf("Count() = %v, want %v", fields["count"], 42)
	}
}

func TestStandardFields_Size(t *testing.T) {
	fields := NewFields().Size(1024)

	if fields["size_bytes"] != int64(1024) {
		t.Errorf("Size() = %v, want %v", fields["size_bytes"], int64(1024))
	}
}

func TestStandardFields_Version(t *testing.T) {
	fields := NewFields().Version("v1.2.3")

	if fields["version"] != "v1.2.3" {
		t.Errorf("Version() = %v, want %v", fields["version"], "v1.2.3")
	}
}

func TestStandardFields_Custom(t *testing.T) {
	fields := NewFields().Custom("custom_key", "custom_value")

	if fields["custom_key"] != "custom_value" {
		t.Errorf("Custom() = %v, want %v", fields["custom_key"], "custom_value")
	}
}

func TestStandardFields_ChainedCalls(t *testing.T) {
	fields := NewFields().
		Component("test").
		Operation("create").
		Resource("pod", "test-pod").
		Duration(100 * time.Millisecond).
		Count(5)

	expected := map[string]interface{}{
		"component":     "test",
		"operation":     "create",
		"resource_type": "pod",
		"resource_name": "test-pod",
		"duration_ms":   int64(100),
		"count":         5,
	}

	for key, expectedValue := range expected {
		if fields[key] != expectedValue {
			t.Errorf("Chained calls: %s = %v, want %v", key, fields[key], expectedValue)
		}
	}
}

func TestStandardFields_ToLogrus(t *testing.T) {
	fields := NewFields().
		Component("test").
		Operation("create")

	logrusFields := fields.ToLogrus()

	if logrusFields == nil {
		t.Fatal("ToLogrus() should not return nil")
	}

	if logrusFields["component"] != "test" {
		t.Errorf("ToLogrus() component = %v, want %v", logrusFields["component"], "test")
	}
	if logrusFields["operation"] != "create" {
		t.Errorf("ToLogrus() operation = %v, want %v", logrusFields["operation"], "create")
	}
}

func TestDatabaseFields(t *testing.T) {
	fields := DatabaseFields("insert", "users")

	expected := map[string]interface{}{
		"component":     "database",
		"operation":     "insert",
		"resource_type": "table",
		"resource_name": "users",
	}

	for key, expectedValue := range expected {
		if fields[key] != expectedValue {
			t.Errorf("DatabaseFields() %s = %v, want %v", key, fields[key], expectedValue)
		}
	}
}

func TestHTTPFields(t *testing.T) {
	fields := HTTPFields("POST", "/api/users", 201)

	expected := map[string]interface{}{
		"component":   "http",
		"method":      "POST",
		"url":         "/api/users",
		"status_code": 201,
	}

	for key, expectedValue := range expected {
		if fields[key] != expectedValue {
			t.Errorf("HTTPFields() %s = %v, want %v", key, fields[key], expectedValue)
		}
	}
}

func TestWorkflowFields(t *testing.T) {
	fields := WorkflowFields("execute", "workflow-123")

	expected := map[string]interface{}{
		"component":     "workflow",
		"operation":     "execute",
		"resource_type": "workflow",
		"resource_name": "workflow-123",
	}

	for key, expectedValue := range expected {
		if fields[key] != expectedValue {
			t.Errorf("WorkflowFields() %s = %v, want %v", key, fields[key], expectedValue)
		}
	}
}

func TestKubernetesFields(t *testing.T) {
	fields := KubernetesFields("create", "pod", "test-pod", "default")

	expected := map[string]interface{}{
		"component":     "kubernetes",
		"operation":     "create",
		"resource_type": "pod",
		"resource_name": "test-pod",
		"namespace":     "default",
	}

	for key, expectedValue := range expected {
		if fields[key] != expectedValue {
			t.Errorf("KubernetesFields() %s = %v, want %v", key, fields[key], expectedValue)
		}
	}
}

func TestKubernetesFieldsWithoutNamespace(t *testing.T) {
	fields := KubernetesFields("create", "pod", "test-pod", "")

	if _, exists := fields["namespace"]; exists {
		t.Error("KubernetesFields() should not set namespace when empty")
	}
}

func TestAIFields(t *testing.T) {
	fields := AIFields("inference", "gpt-3.5")

	expected := map[string]interface{}{
		"component": "ai",
		"operation": "inference",
		"model":     "gpt-3.5",
	}

	for key, expectedValue := range expected {
		if fields[key] != expectedValue {
			t.Errorf("AIFields() %s = %v, want %v", key, fields[key], expectedValue)
		}
	}
}

func TestMetricsFields(t *testing.T) {
	fields := MetricsFields("record", "cpu_usage", 85.5)

	expected := map[string]interface{}{
		"component":   "metrics",
		"operation":   "record",
		"metric_name": "cpu_usage",
		"value":       85.5,
	}

	for key, expectedValue := range expected {
		if fields[key] != expectedValue {
			t.Errorf("MetricsFields() %s = %v, want %v", key, fields[key], expectedValue)
		}
	}
}

func TestSecurityFields(t *testing.T) {
	fields := SecurityFields("authenticate", "user-123")

	expected := map[string]interface{}{
		"component": "security",
		"operation": "authenticate",
		"subject":   "user-123",
	}

	for key, expectedValue := range expected {
		if fields[key] != expectedValue {
			t.Errorf("SecurityFields() %s = %v, want %v", key, fields[key], expectedValue)
		}
	}
}

func TestPerformanceFields(t *testing.T) {
	duration := 250 * time.Millisecond
	fields := PerformanceFields("query_database", duration, true)

	expected := map[string]interface{}{
		"component":   "performance",
		"operation":   "query_database",
		"duration_ms": int64(250),
		"success":     true,
	}

	for key, expectedValue := range expected {
		if fields[key] != expectedValue {
			t.Errorf("PerformanceFields() %s = %v, want %v", key, fields[key], expectedValue)
		}
	}
}
