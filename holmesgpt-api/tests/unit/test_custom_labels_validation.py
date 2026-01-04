"""
Unit Tests for Custom Labels Validation (DD-HAPI-001)

Business Requirement: BR-HAPI-250 - Workflow Catalog Search
Design Decision: DD-HAPI-001 - Custom Labels Auto-Append Architecture

These are UNIT TESTS that validate Pydantic model validation and data structures.
No external services required.
"""



class TestCustomLabelsModelValidation:
    """Unit tests for custom_labels Pydantic model validation (DD-HAPI-001)"""

    def test_custom_labels_subdomain_structure_validated(self):
        """DD-HAPI-001: Verify custom_labels use subdomain-based structure"""
        from src.models.incident_models import EnrichmentResults

        # Valid subdomain structure
        valid_labels = {
            "constraint": ["cost-constrained"],
            "team": ["name=payments"],
            "region": ["zone=us-east-1"]
        }

        enrichment = EnrichmentResults(customLabels=valid_labels)
        assert enrichment.customLabels == valid_labels

        # Each key should be a subdomain, each value should be a list of strings
        for subdomain, values in enrichment.customLabels.items():
            assert isinstance(subdomain, str)
            assert isinstance(values, list)
            for value in values:
                assert isinstance(value, str)

    def test_custom_labels_boolean_and_keyvalue_formats(self):
        """DD-HAPI-001: Verify both boolean and key=value formats are supported"""
        from src.models.incident_models import EnrichmentResults

        # Mix of boolean (presence = true) and key=value formats
        mixed_labels = {
            "constraint": ["cost-constrained", "stateful-safe"],  # Boolean keys
            "team": ["name=payments", "owner=sre"],  # Key=value pairs
            "mixed": ["active", "priority=high"]  # Both in same subdomain
        }

        enrichment = EnrichmentResults(customLabels=mixed_labels)
        assert enrichment.customLabels == mixed_labels

        # Verify boolean format (no '=')
        assert "cost-constrained" in enrichment.customLabels["constraint"]
        assert "=" not in "cost-constrained"

        # Verify key=value format
        assert "name=payments" in enrichment.customLabels["team"]
        assert "=" in "name=payments"





