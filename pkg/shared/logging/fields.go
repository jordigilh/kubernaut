<<<<<<< HEAD
=======
/*
Copyright 2025 Jordi Gil.

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

>>>>>>> crd_implementation
package logging

import (
	"time"

	"github.com/sirupsen/logrus"
)

// StandardFields provides common structured logging fields
type StandardFields map[string]interface{}

// NewFields creates a new StandardFields instance
func NewFields() StandardFields {
	return make(StandardFields)
}

// Component adds component information to log fields
func (f StandardFields) Component(name string) StandardFields {
	f["component"] = name
	return f
}

// Operation adds operation information to log fields
func (f StandardFields) Operation(op string) StandardFields {
	f["operation"] = op
	return f
}

// Resource adds resource information to log fields
func (f StandardFields) Resource(resourceType, resourceName string) StandardFields {
	f["resource_type"] = resourceType
	if resourceName != "" {
		f["resource_name"] = resourceName
	}
	return f
}

// Duration adds duration information to log fields
func (f StandardFields) Duration(d time.Duration) StandardFields {
	f["duration_ms"] = d.Milliseconds()
	return f
}

// Error adds error information to log fields
func (f StandardFields) Error(err error) StandardFields {
	if err != nil {
		f["error"] = err.Error()
	}
	return f
}

// UserID adds user identification to log fields
func (f StandardFields) UserID(id string) StandardFields {
	if id != "" {
		f["user_id"] = id
	}
	return f
}

// RequestID adds request identification to log fields
func (f StandardFields) RequestID(id string) StandardFields {
	if id != "" {
		f["request_id"] = id
	}
	return f
}

// TraceID adds trace identification to log fields
func (f StandardFields) TraceID(id string) StandardFields {
	if id != "" {
		f["trace_id"] = id
	}
	return f
}

// StatusCode adds HTTP status code to log fields
func (f StandardFields) StatusCode(code int) StandardFields {
	f["status_code"] = code
	return f
}

// Method adds HTTP method to log fields
func (f StandardFields) Method(method string) StandardFields {
	if method != "" {
		f["method"] = method
	}
	return f
}

// URL adds URL information to log fields
func (f StandardFields) URL(url string) StandardFields {
	if url != "" {
		f["url"] = url
	}
	return f
}

// Count adds count/quantity information to log fields
func (f StandardFields) Count(count int) StandardFields {
	f["count"] = count
	return f
}

// Size adds size information to log fields
func (f StandardFields) Size(size int64) StandardFields {
	f["size_bytes"] = size
	return f
}

// Version adds version information to log fields
func (f StandardFields) Version(version string) StandardFields {
	if version != "" {
		f["version"] = version
	}
	return f
}

// Custom adds custom key-value pair to log fields
func (f StandardFields) Custom(key string, value interface{}) StandardFields {
	f[key] = value
	return f
}

// ToLogrus converts StandardFields to logrus.Fields
func (f StandardFields) ToLogrus() logrus.Fields {
	fields := make(logrus.Fields, len(f))
	for k, v := range f {
		fields[k] = v
	}
	return fields
}

// Common preset field combinations for frequently used scenarios

// DatabaseFields creates fields for database operations
func DatabaseFields(operation, table string) StandardFields {
	return NewFields().
		Component("database").
		Operation(operation).
		Resource("table", table)
}

// HTTPFields creates fields for HTTP operations
func HTTPFields(method, url string, statusCode int) StandardFields {
	return NewFields().
		Component("http").
		Method(method).
		URL(url).
		StatusCode(statusCode)
}

// WorkflowFields creates fields for workflow operations
func WorkflowFields(operation, workflowID string) StandardFields {
	return NewFields().
		Component("workflow").
		Operation(operation).
		Resource("workflow", workflowID)
}

// KubernetesFields creates fields for Kubernetes operations
func KubernetesFields(operation, resourceType, resourceName, namespace string) StandardFields {
	fields := NewFields().
		Component("kubernetes").
		Operation(operation).
		Resource(resourceType, resourceName)

	if namespace != "" {
		fields["namespace"] = namespace
	}

	return fields
}

// AIFields creates fields for AI/ML operations
func AIFields(operation, model string) StandardFields {
	return NewFields().
		Component("ai").
		Operation(operation).
		Custom("model", model)
}

// MetricsFields creates fields for metrics operations
func MetricsFields(operation, metricName string, value float64) StandardFields {
	return NewFields().
		Component("metrics").
		Operation(operation).
		Custom("metric_name", metricName).
		Custom("value", value)
}

// SecurityFields creates fields for security-related operations
func SecurityFields(operation, subject string) StandardFields {
	return NewFields().
		Component("security").
		Operation(operation).
		Custom("subject", subject)
}

// PerformanceFields creates fields for performance monitoring
func PerformanceFields(operation string, duration time.Duration, success bool) StandardFields {
	return NewFields().
		Component("performance").
		Operation(operation).
		Duration(duration).
		Custom("success", success)
}
