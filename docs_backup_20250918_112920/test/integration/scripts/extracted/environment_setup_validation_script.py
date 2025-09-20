#!/usr/bin/env python3
"""
Extracted script from ENVIRONMENT_SETUP.md
Business requirement: Extracted following project guidelines for reusability
"""
import json, sys
try:
    data = json.load(sys.stdin)
    models = [model['name'] for model in data.get('models', [])]
    print(f'Available models: {models}')
    if models:
        print('✅ Models are available')
        sys.exit(0)
    else:
        print('❌ No models available')
        sys.exit(1)
except Exception as e:
    print(f'❌ Error checking models: {e}')
    sys.exit(1)
