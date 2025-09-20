# Test Result Analysis Framework Usage Guide

## Overview
This framework provides reusable analysis modules following project guidelines:
- Avoids code duplication
- Provides strong business requirement validation
- Includes proper error handling and logging
- Uses business-aligned naming conventions

## Usage

### Command Line Analysis
```bash
# Analyze availability compliance (BR-PA-001)
python3 scripts/common/test_result_analyzer.py availability availability_detailed_results.json test_session_name

# Analyze concurrent handling (BR-PA-004)
python3 scripts/common/test_result_analyzer.py concurrent concurrent_test_results.json test_session_name

# Analyze LLM provider compliance (BR-PA-006)
python3 scripts/common/test_result_analyzer.py llm_provider llm_provider_results.json test_session_name
```

### Programmatic Usage
```python
from scripts.common.test_result_analyzer import AvailabilityAnalyzer, ReportGenerator

# Analyze availability
analyzer = AvailabilityAnalyzer('test_session_name')
results = analyzer.analyze_availability_compliance('results.json')

# Generate business requirement report
report_gen = ReportGenerator('test_session_name')
report_path = report_gen.generate_business_requirement_report([results], 'report.md')
```

## Business Requirements Supported
- **BR-PA-001**: Availability analysis and compliance validation
- **BR-PA-002** through **BR-PA-013**: Various test result validations
- Strong assertion validation following project guidelines
- Comprehensive error handling and logging

## Integration
This framework integrates with existing test infrastructure and follows project guidelines:
- Uses shared analysis patterns
- Provides business-focused validation
- Includes comprehensive error handling
- Generates detailed compliance reports
