// Package pii provides PII (Personally Identifiable Information) redaction utilities
// for SOC2 privacy compliance and data minimization.
package pii

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Redactor handles PII redaction with configurable patterns
type Redactor struct {
	emailRegex *regexp.Regexp
	ipv4Regex  *regexp.Regexp
	phoneRegex *regexp.Regexp
}

// NewRedactor creates a new PII redactor with default patterns
func NewRedactor() *Redactor {
	return &Redactor{
		// Email: matches standard email format (RFC 5322 simplified)
		emailRegex: regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),

		// IPv4: matches standard IPv4 addresses (0.0.0.0 - 255.255.255.255)
		ipv4Regex: regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`),

		// Phone: matches various phone number formats
		// +1-555-1234, (555) 555-1234, 555-555-1234, 555.555.1234, +1 555 555 1234
		phoneRegex: regexp.MustCompile(`\+?[0-9]{1,3}[-\s.]?\(?[0-9]{3}\)?[-\s.]?[0-9]{3}[-\s.]?[0-9]{4}`),
	}
}

// RedactEmail redacts email addresses
// Example: user@domain.com → u***@d***.com
func (r *Redactor) RedactEmail(email string) string {
	if email == "" {
		return ""
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		// Not a valid email, return as-is or redact entirely
		return "***@***.***"
	}

	localPart := parts[0]
	domainPart := parts[1]

	// Redact local part: show first character only
	redactedLocal := string(localPart[0]) + "***"

	// Redact domain: show first character of each part
	domainParts := strings.Split(domainPart, ".")
	redactedDomain := make([]string, len(domainParts))
	for i, part := range domainParts {
		if len(part) > 0 {
			redactedDomain[i] = string(part[0]) + "***"
		} else {
			redactedDomain[i] = "***"
		}
	}

	return redactedLocal + "@" + strings.Join(redactedDomain, ".")
}

// RedactIPv4 redacts IPv4 addresses
// Example: 192.168.1.1 → 192.***.*.***
func (r *Redactor) RedactIPv4(ip string) string {
	if ip == "" {
		return ""
	}

	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		// Not a valid IPv4, return redacted
		return "***.***.***"
	}

	// Keep first octet, redact the rest
	return parts[0] + ".***.***.***"
}

// RedactPhone redacts phone numbers
// Example: +1-555-1234 → +1-***-****
func (r *Redactor) RedactPhone(phone string) string {
	if phone == "" {
		return ""
	}

	// Extract country code if present (+1, +44, etc.)
	countryCode := ""
	if strings.HasPrefix(phone, "+") {
		// Find the country code (up to 3 digits after +)
		for i, char := range phone {
			if i == 0 {
				countryCode += string(char) // Keep the '+'
				continue
			}
			if char >= '0' && char <= '9' && len(countryCode) < 4 {
				countryCode += string(char)
			} else if len(countryCode) > 1 {
				break
			}
		}
	}

	if countryCode != "" {
		return countryCode + "-***-****"
	}

	return "***-***-****"
}

// RedactString applies all PII redaction patterns to a string
func (r *Redactor) RedactString(input string) string {
	if input == "" {
		return ""
	}

	// Apply redactions in order: email, IP, phone
	result := r.emailRegex.ReplaceAllStringFunc(input, r.RedactEmail)
	result = r.ipv4Regex.ReplaceAllStringFunc(result, r.RedactIPv4)
	result = r.phoneRegex.ReplaceAllStringFunc(result, r.RedactPhone)

	return result
}

// RedactJSON recursively redacts PII from JSON-serializable data structures
// It redacts string values that match PII patterns
func (r *Redactor) RedactJSON(data interface{}) interface{} {
	switch v := data.(type) {
	case string:
		// Redact string values
		return r.RedactString(v)

	case map[string]interface{}:
		// Recursively redact map values
		result := make(map[string]interface{}, len(v))
		for key, value := range v {
			result[key] = r.RedactJSON(value)
		}
		return result

	case []interface{}:
		// Recursively redact array elements
		result := make([]interface{}, len(v))
		for i, value := range v {
			result[i] = r.RedactJSON(value)
		}
		return result

	default:
		// Non-string, non-composite types: return as-is
		return v
	}
}

// RedactJSONBytes unmarshals JSON, redacts PII, and re-marshals
// This is a convenience function for working with JSON byte slices
func (r *Redactor) RedactJSONBytes(jsonBytes []byte) ([]byte, error) {
	var data interface{}
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return nil, err
	}

	redacted := r.RedactJSON(data)

	return json.Marshal(redacted)
}

// PIIFields lists common field names that typically contain PII
// Used for targeted redaction in structured data
var PIIFields = []string{
	"email",
	"user_email",
	"userEmail",
	"sender_email",
	"recipient_email",
	"phone",
	"phone_number",
	"phoneNumber",
	"mobile",
	"ip",
	"ip_address",
	"ipAddress",
	"source_ip",
	"sourceIP",
	"client_ip",
	"clientIP",
	"remote_addr",
	"remoteAddr",
	"ssn",
	"social_security_number",
	"credit_card",
	"creditCard",
	"cc_number",
}

// RedactMapByFieldNames applies redaction only to specific field names
// This is more efficient than RedactJSON for structured data with known PII fields
func (r *Redactor) RedactMapByFieldNames(data map[string]interface{}, fieldNames []string) map[string]interface{} {
	result := make(map[string]interface{}, len(data))

	// Create a set of field names to redact
	fieldsToRedact := make(map[string]bool, len(fieldNames))
	for _, field := range fieldNames {
		fieldsToRedact[field] = true
	}

	for key, value := range data {
		if fieldsToRedact[key] {
			// Redact this field
			if str, ok := value.(string); ok {
				result[key] = r.RedactString(str)
			} else {
				result[key] = r.RedactJSON(value)
			}
		} else {
			// Keep as-is (but recursively handle nested maps/arrays)
			switch v := value.(type) {
			case map[string]interface{}:
				result[key] = r.RedactMapByFieldNames(v, fieldNames)
			case []interface{}:
				array := make([]interface{}, len(v))
				for i, elem := range v {
					if elemMap, ok := elem.(map[string]interface{}); ok {
						array[i] = r.RedactMapByFieldNames(elemMap, fieldNames)
					} else {
						array[i] = elem
					}
				}
				result[key] = array
			default:
				result[key] = value
			}
		}
	}

	return result
}


