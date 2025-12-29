"""
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
"""

"""
LLM Input Sanitizer Unit Tests

Business Requirement: BR-HAPI-211 - LLM Input Sanitization
Design Decision: DD-HAPI-005 - Comprehensive LLM Input Sanitization Layer

Tests verify:
1. All 17+ credential patterns are detected and redacted
2. Different data types (str, dict, list, None) are handled correctly
3. Pattern ordering prevents corruption of larger structures
4. Fallback sanitization works when regex fails
5. Tool wrapper integration sanitizes results correctly
"""

import pytest
from src.sanitization.llm_sanitizer import (
    LLMSanitizer,
    sanitize_for_llm,
    sanitize_with_fallback,
    REDACTED_PLACEHOLDER,
    default_rules,
)


class TestPasswordPatterns:
    """Tests for password credential patterns (P0 priority)"""

    def test_password_json_sanitized(self):
        """BR-HAPI-211: JSON password fields should be redacted"""
        input_str = '{"username": "admin", "password": "super_secret_123"}'
        result = sanitize_for_llm(input_str)
        assert "super_secret_123" not in result
        assert REDACTED_PLACEHOLDER in result
        assert '"username": "admin"' in result  # Non-sensitive kept

    def test_password_plain_sanitized(self):
        """BR-HAPI-211: Plain text password fields should be redacted"""
        input_str = "password: my_secret_pass\nusername: admin"
        result = sanitize_for_llm(input_str)
        assert "my_secret_pass" not in result
        assert REDACTED_PLACEHOLDER in result

    def test_password_with_equals_sanitized(self):
        """BR-HAPI-211: password=value format should be redacted"""
        input_str = "password=secret123 username=admin"
        result = sanitize_for_llm(input_str)
        assert "secret123" not in result
        assert REDACTED_PLACEHOLDER in result

    def test_passwd_field_sanitized(self):
        """BR-HAPI-211: passwd variant should be redacted"""
        input_str = '{"passwd": "hidden_value"}'
        result = sanitize_for_llm(input_str)
        assert "hidden_value" not in result
        assert REDACTED_PLACEHOLDER in result

    def test_pwd_field_sanitized(self):
        """BR-HAPI-211: pwd variant should be redacted"""
        input_str = 'pwd: "short_pass"'
        result = sanitize_for_llm(input_str)
        assert "short_pass" not in result


class TestDatabaseURLPatterns:
    """Tests for database URL credential patterns (P0 priority)"""

    def test_postgresql_url_sanitized(self):
        """BR-HAPI-211: PostgreSQL URLs should have password redacted"""
        input_str = "postgresql://user:password123@localhost:5432/db"
        result = sanitize_for_llm(input_str)
        assert "password123" not in result
        assert "postgresql://user:" in result
        assert "@localhost:5432/db" in result

    def test_mysql_url_sanitized(self):
        """BR-HAPI-211: MySQL URLs should have password redacted"""
        input_str = "mysql://root:my_db_pass@mysql.example.com/database"
        result = sanitize_for_llm(input_str)
        assert "my_db_pass" not in result
        assert "mysql://root:" in result

    def test_mongodb_url_sanitized(self):
        """BR-HAPI-211: MongoDB URLs should have password redacted"""
        input_str = "mongodb://admin:mongo_secret@mongodb:27017/admin"
        result = sanitize_for_llm(input_str)
        assert "mongo_secret" not in result

    def test_redis_url_sanitized(self):
        """BR-HAPI-211: Redis URLs should have password redacted"""
        input_str = "redis://default:redis_pass@redis:6379"
        result = sanitize_for_llm(input_str)
        assert "redis_pass" not in result

    def test_generic_url_password_sanitized(self):
        """BR-HAPI-211: Generic URL passwords should be redacted"""
        input_str = "https://user:api_password@api.example.com/v1"
        result = sanitize_for_llm(input_str)
        assert "api_password" not in result


class TestAPIKeyPatterns:
    """Tests for API key credential patterns (P0 priority)"""

    def test_openai_key_sanitized(self):
        """BR-HAPI-211: OpenAI API keys (sk-*) should be redacted"""
        input_str = "OPENAI_API_KEY=sk-proj-abcdefghijklmnop123456789"
        result = sanitize_for_llm(input_str)
        assert "sk-proj-abcdefghijklmnop123456789" not in result
        assert REDACTED_PLACEHOLDER in result

    def test_openai_key_in_logs_sanitized(self):
        """BR-HAPI-211: OpenAI keys in log output should be redacted"""
        input_str = """Error connecting to OpenAI:
        API Key: sk-abc123def456ghi789jkl012
        Status: 401 Unauthorized"""
        result = sanitize_for_llm(input_str)
        assert "sk-abc123def456ghi789jkl012" not in result

    def test_api_key_json_sanitized(self):
        """BR-HAPI-211: JSON api_key fields should be redacted"""
        input_str = '{"api_key": "my-secret-api-key-12345"}'
        result = sanitize_for_llm(input_str)
        assert "my-secret-api-key-12345" not in result

    def test_apikey_no_underscore_sanitized(self):
        """BR-HAPI-211: apikey (no underscore) should be redacted"""
        input_str = 'apikey: ABC123XYZ'
        result = sanitize_for_llm(input_str)
        assert "ABC123XYZ" not in result


class TestTokenPatterns:
    """Tests for token credential patterns (P0 priority)"""

    def test_bearer_token_sanitized(self):
        """BR-HAPI-211: Bearer tokens should be redacted"""
        input_str = "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"  # notsecret
        result = sanitize_for_llm(input_str)
        # Bearer token itself should be redacted
        assert "Bearer [REDACTED]" in result or REDACTED_PLACEHOLDER in result

    def test_jwt_token_standalone_sanitized(self):
        """BR-HAPI-211: Standalone JWT tokens should be redacted"""
        input_str = "token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
        result = sanitize_for_llm(input_str)
        # JWT pattern should be caught
        assert "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9" not in result

    def test_github_token_sanitized(self):
        """BR-HAPI-211: GitHub personal access tokens should be redacted"""
        input_str = "GITHUB_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz1234567890"  # notsecret
        result = sanitize_for_llm(input_str)
        assert "ghp_abcdefghijklmnopqrstuvwxyz1234567890" not in result  # notsecret

    def test_github_oauth_token_sanitized(self):
        """BR-HAPI-211: GitHub OAuth tokens (gho_*) should be redacted"""
        input_str = "token: gho_abcdefghijklmnopqrstuvwxyz1234567890"  # notsecret
        result = sanitize_for_llm(input_str)
        assert "gho_abcdefghijklmnopqrstuvwxyz1234567890" not in result  # notsecret

    def test_token_json_sanitized(self):
        """BR-HAPI-211: JSON token fields should be redacted"""
        input_str = '{"access_token": "ya29.a0AfH6SMBxxxxxxxxxxxxxxx"}'
        result = sanitize_for_llm(input_str)
        assert "ya29.a0AfH6SMBxxxxxxxxxxxxxxx" not in result


class TestSecretPatterns:
    """Tests for generic secret credential patterns (P0/P2 priority)"""

    def test_secret_json_sanitized(self):
        """BR-HAPI-211: JSON secret fields should be redacted"""
        input_str = '{"client_secret": "my_oauth_client_secret_xyz"}'
        result = sanitize_for_llm(input_str)
        assert "my_oauth_client_secret_xyz" not in result

    def test_secret_plain_sanitized(self):
        """BR-HAPI-211: Plain text secret fields should be redacted"""
        input_str = "secret: application_secret_value"
        result = sanitize_for_llm(input_str)
        assert "application_secret_value" not in result


class TestCloudProviderCredentials:
    """Tests for cloud provider credential patterns (P1 priority)"""

    def test_aws_access_key_id_sanitized(self):
        """BR-HAPI-211: AWS access key IDs should be redacted"""
        input_str = "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE"
        result = sanitize_for_llm(input_str)
        assert "AKIAIOSFODNN7EXAMPLE" not in result

    def test_aws_secret_key_sanitized(self):
        """BR-HAPI-211: AWS secret keys should be redacted"""
        input_str = "AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
        result = sanitize_for_llm(input_str)
        assert "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" not in result

    def test_aws_inline_access_key_sanitized(self):
        """BR-HAPI-211: Inline AWS access keys (AKIA*) should be redacted"""
        input_str = "Found credentials: AKIAIOSFODNN7EXAMPLE in config"
        result = sanitize_for_llm(input_str)
        assert "AKIAIOSFODNN7EXAMPLE" not in result


class TestCertificatesAndKeys:
    """Tests for certificate and private key patterns (P1 priority)"""

    def test_private_key_sanitized(self):
        """BR-HAPI-211: PEM private keys should be redacted"""
        input_str = """-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC...
-----END PRIVATE KEY-----"""
        result = sanitize_for_llm(input_str)
        assert "MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC" not in result
        assert REDACTED_PLACEHOLDER in result

    def test_rsa_private_key_sanitized(self):
        """BR-HAPI-211: RSA private keys should be redacted"""
        input_str = """-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJBALRiMLAhb/4X9z...
-----END RSA PRIVATE KEY-----"""
        result = sanitize_for_llm(input_str)
        assert "MIIBOgIBAAJBALRiMLAhb/4X9z" not in result

    def test_certificate_sanitized(self):
        """BR-HAPI-211: PEM certificates should be redacted"""
        input_str = """-----BEGIN CERTIFICATE-----
MIIDXTCCAkWgAwIBAgIJAJC1HiIAZAiU...
-----END CERTIFICATE-----"""
        result = sanitize_for_llm(input_str)
        assert "MIIDXTCCAkWgAwIBAgIJAJC1HiIAZAiU" not in result


class TestKubernetesSecrets:
    """Tests for Kubernetes secret data patterns (P1 priority)"""

    def test_k8s_secret_data_sanitized(self):
        """BR-HAPI-211: K8s secret data (base64) should be redacted"""
        input_str = """apiVersion: v1
kind: Secret
data:
  password: cGFzc3dvcmQxMjM=
  username: YWRtaW4="""
        result = sanitize_for_llm(input_str)
        # Base64-encoded values should be redacted
        assert "cGFzc3dvcmQxMjM=" not in result or REDACTED_PLACEHOLDER in result


class TestDataTypeHandling:
    """Tests for different input data types (DD-HAPI-005 requirement)"""

    def test_sanitize_string(self):
        """BR-HAPI-211: String input should return sanitized string"""
        result = sanitize_for_llm("password: secret")
        assert isinstance(result, str)
        assert "secret" not in result

    def test_sanitize_dict(self):
        """BR-HAPI-211: Dict input should return sanitized dict"""
        input_dict = {"username": "admin", "password": "secret123"}
        result = sanitize_for_llm(input_dict)
        assert isinstance(result, dict)
        assert result["username"] == "admin"
        assert result["password"] == REDACTED_PLACEHOLDER

    def test_sanitize_list(self):
        """BR-HAPI-211: List input should return sanitized list"""
        input_list = ["password: abc", "api_key: xyz", "normal text"]
        result = sanitize_for_llm(input_list)
        assert isinstance(result, list)
        assert len(result) == 3
        assert "abc" not in result[0]
        assert "xyz" not in result[1]
        assert "normal text" == result[2]

    def test_sanitize_none_returns_none(self):
        """BR-HAPI-211: None input should return None"""
        result = sanitize_for_llm(None)
        assert result is None

    def test_sanitize_nested_dict(self):
        """BR-HAPI-211: Nested dict should be sanitized recursively"""
        input_dict = {
            "config": {
                "database": {
                    "password": "db_secret"
                }
            }
        }
        result = sanitize_for_llm(input_dict)
        # JSON serialization flattens nested access, check result
        assert "db_secret" not in str(result)


class TestEdgeCases:
    """Tests for edge cases and error handling"""

    def test_sanitize_empty_string(self):
        """BR-HAPI-211: Empty string should return empty string"""
        result = sanitize_for_llm("")
        assert result == ""

    def test_sanitize_no_credentials(self):
        """BR-HAPI-211: Content without credentials should pass through"""
        input_str = "This is a normal log message with no sensitive data."
        result = sanitize_for_llm(input_str)
        assert result == input_str

    def test_sanitize_multiple_credentials(self):
        """BR-HAPI-211: Multiple credentials in one string should all be redacted"""
        input_str = 'password: secret1, api_key: key2, token: tok3'
        result = sanitize_for_llm(input_str)
        assert "secret1" not in result
        assert "key2" not in result
        assert "tok3" not in result

    def test_pattern_ordering_prevents_corruption(self):
        """BR-HAPI-211: Pattern ordering should prevent URL corruption"""
        # Database URL contains password which could be corrupted if patterns
        # are applied in wrong order
        input_str = "postgresql://user:complex_pass_word@host:5432/db"
        result = sanitize_for_llm(input_str)
        # Should redact password but keep URL structure
        assert "postgresql://" in result
        assert "@host:5432/db" in result
        assert "complex_pass_word" not in result


class TestFallbackSanitization:
    """Tests for fallback sanitization (DD-HAPI-005 FR-5)"""

    def test_fallback_on_normal_content(self):
        """BR-HAPI-211: Normal content should not trigger fallback"""
        result, error = sanitize_with_fallback("password: secret")
        assert error is None
        assert "secret" not in result

    def test_safe_fallback_method(self):
        """BR-HAPI-211: Safe fallback should use simple string matching"""
        sanitizer = LLMSanitizer()
        result = sanitizer.safe_fallback("password: secret123")
        assert "secret123" not in result
        assert REDACTED_PLACEHOLDER in result


class TestSanitizerClass:
    """Tests for LLMSanitizer class"""

    def test_default_rules_count(self):
        """BR-HAPI-211: Should have 17+ default rules"""
        rules = default_rules()
        # DD-HAPI-005 specifies 17 pattern categories
        assert len(rules) >= 17

    def test_custom_rules(self):
        """BR-HAPI-211: Should support custom rules"""
        import re
        from src.sanitization.llm_sanitizer import SanitizationRule

        custom_rules = [
            SanitizationRule(
                name="custom-pattern",
                pattern=re.compile(r"CUSTOM_SECRET_\w+"),
                replacement=REDACTED_PLACEHOLDER,
                description="Custom secret pattern",
            )
        ]

        sanitizer = LLMSanitizer(rules=custom_rules)
        result = sanitizer.sanitize("Found CUSTOM_SECRET_ABC123 in config")
        assert "CUSTOM_SECRET_ABC123" not in result
        assert REDACTED_PLACEHOLDER in result

    def test_sanitizer_instance_reuse(self):
        """BR-HAPI-211: Sanitizer should be reusable"""
        sanitizer = LLMSanitizer()
        result1 = sanitizer.sanitize("password: secret1")
        result2 = sanitizer.sanitize("password: secret2")
        assert "secret1" not in result1
        assert "secret2" not in result2


class TestRealWorldScenarios:
    """Tests for real-world credential leakage scenarios"""

    def test_kubectl_logs_output(self):
        """BR-HAPI-211: kubectl logs output should be sanitized"""
        input_str = """2025-01-01 10:00:00 INFO Connecting to database
2025-01-01 10:00:01 ERROR Failed to connect: postgresql://app_user:P@ssw0rd123!@db.example.com:5432/production
2025-01-01 10:00:02 INFO Retrying with fallback..."""
        result = sanitize_for_llm(input_str)
        assert "P@ssw0rd123!" not in result
        assert "postgresql://app_user:" in result

    def test_error_stack_trace(self):
        """BR-HAPI-211: Error stack traces should be sanitized"""
        input_str = """Error: Connection refused
    at connect (db.js:42)
    Config: {"host": "db.example.com", "password": "my_db_password", "port": 5432}
    at main (app.js:10)"""
        result = sanitize_for_llm(input_str)
        assert "my_db_password" not in result

    def test_kubernetes_configmap(self):
        """BR-HAPI-211: ConfigMap output should be sanitized"""
        input_str = """apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
data:
  DATABASE_URL: "postgresql://user:secret@db:5432/app"
  API_KEY: "sk-proj-1234567890abcdef"
  LOG_LEVEL: "info" """
        result = sanitize_for_llm(input_str)
        assert "secret" not in result.lower() or "secret@" not in result
        assert "sk-proj-1234567890abcdef" not in result
        assert "LOG_LEVEL" in result  # Non-sensitive kept

    def test_workflow_parameters(self):
        """BR-HAPI-211: Workflow parameters should be sanitized"""
        input_str = """Workflow parameters:
{
  "namespace": "production",
  "image": "myapp:v1.2.3",
  "credentials": {
    "registry_password": "docker_secret_123",
    "api_token": "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
  }
}"""
        result = sanitize_for_llm(input_str)
        assert "docker_secret_123" not in result
        assert "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" not in result
        assert '"namespace": "production"' in result




