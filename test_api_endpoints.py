#!/usr/bin/env python3
"""
API Endpoint Testing Script for Interface-Centric Configuration Engine
Tests the MongoDB Configuration Manager API endpoints
"""

import json
import requests
import time
import os
from datetime import datetime
from uuid import uuid4

# Configuration
BASE_URL = "http://localhost:8080/api"
CONFIG_ENDPOINT = f"{BASE_URL}/config"
TIMEOUT = 30

def print_test_header(test_name):
    """Print a formatted test header"""
    print(f"\n{'='*60}")
    print(f"🧪 {test_name}")
    print(f"{'='*60}")

def print_result(success, message, data=None):
    """Print test result"""
    status = "✅ PASS" if success else "❌ FAIL"
    print(f"{status}: {message}")
    if data and isinstance(data, dict):
        print(f"📊 Response: {json.dumps(data, indent=2)}")
    elif data:
        print(f"📊 Response: {data}")

def test_configuration_health():
    """Test configuration health endpoint"""
    print_test_header("Configuration Health Check")

    try:
        response = requests.get(f"{CONFIG_ENDPOINT}/health", timeout=TIMEOUT)
        if response.status_code == 200:
            data = response.json()
            print_result(True, "Configuration health check successful", data)
            return True
        else:
            print_result(False, f"Health check failed with status {response.status_code}", response.text)
            return False
    except requests.exceptions.RequestException as e:
        print_result(False, f"Health check request failed: {e}")
        return False

def test_configuration_crud():
    """Test configuration CRUD operations"""
    print_test_header("Configuration CRUD Operations")

    # Generate unique interface ID
    interface_id = f"test-interface-{uuid4().hex[:8]}"

    # Test configuration
    test_config = {
        "interface_id": interface_id,
        "name": "Test HL7 to FHIR Interface",
        "description": "API test configuration for MongoDB Configuration Engine",
        "version": "1.0.0",
        "status": "active",
        "pipeline": {
            "input": {
                "type": "mllp",
                "connector_config": {
                    "host": "0.0.0.0",
                    "port": 2575,
                    "max_connections": 50
                },
                "validation": {
                    "enabled": True,
                    "rules": []
                },
                "preprocessing": {
                    "enabled": False,
                    "steps": []
                }
            },
            "validation": {
                "schema_validation": {
                    "enabled": True,
                    "schema_type": "hl7",
                    "strict_mode": False
                },
                "business_rules": [],
                "custom_validators": []
            },
            "transformation": {
                "engine": "hl7_to_fhir",
                "mapping_template": "standard_adt_v4",
                "custom_mappings": [],
                "post_processing": []
            },
            "business_logic": {
                "rules_engine": {
                    "enabled": False,
                    "rules": []
                },
                "workflow_automation": []
            },
            "destinations": [
                {
                    "destination_id": f"dest-{uuid4().hex[:8]}",
                    "type": "fhir_api",
                    "config": {
                        "base_url": "http://localhost:8080/fhir"
                    },
                    "routing_rules": [],
                    "error_handling": {
                        "retry_count": 3,
                        "retry_delay": 5000,
                        "dead_letter_queue": True
                    }
                }
            ]
        },
        "monitoring": {
            "metrics_enabled": True,
            "alert_thresholds": {
                "error_rate": 0.05,
                "processing_time_ms": 5000
            },
            "retention_policy": {
                "raw_messages": 90,
                "processed_messages": 30
            }
        }
    }

    try:
        # Test CREATE
        print("\n📝 Testing Create Configuration...")
        response = requests.post(
            f"{CONFIG_ENDPOINT}/interfaces",
            json=test_config,
            timeout=TIMEOUT
        )

        if response.status_code in [200, 201]:
            print_result(True, "Configuration created successfully", response.json())
            created_config = response.json()
        else:
            print_result(False, f"Create failed with status {response.status_code}", response.text)
            return False

        # Test READ
        print("\n📖 Testing Get Configuration...")
        response = requests.get(f"{CONFIG_ENDPOINT}/interfaces/{interface_id}", timeout=TIMEOUT)

        if response.status_code == 200:
            data = response.json()
            print_result(True, "Configuration retrieved successfully", data.get('data', {}))
        else:
            print_result(False, f"Read failed with status {response.status_code}", response.text)

        # Test UPDATE
        print("\n✏️ Testing Update Configuration...")
        test_config["description"] = "Updated API test configuration"
        test_config["version"] = "1.1.0"

        response = requests.put(
            f"{CONFIG_ENDPOINT}/interfaces/{interface_id}",
            json=test_config,
            timeout=TIMEOUT
        )

        if response.status_code == 200:
            print_result(True, "Configuration updated successfully", response.json())
        else:
            print_result(False, f"Update failed with status {response.status_code}", response.text)

        # Test LIST
        print("\n📋 Testing List Configurations...")
        response = requests.get(f"{CONFIG_ENDPOINT}/interfaces", timeout=TIMEOUT)

        if response.status_code == 200:
            data = response.json()
            config_count = len(data.get('data', {}).get('configurations', []))
            print_result(True, f"Configuration list retrieved: {config_count} configurations",
                        {"count": config_count})
        else:
            print_result(False, f"List failed with status {response.status_code}", response.text)

        return True

    except requests.exceptions.RequestException as e:
        print_result(False, f"CRUD test request failed: {e}")
        return False

def test_configuration_validation():
    """Test configuration validation"""
    print_test_header("Configuration Validation")

    # Test valid configuration
    valid_config = {
        "name": "Valid Test Interface",
        "pipeline": {
            "input": {
                "type": "mllp"
            },
            "destinations": [
                {
                    "destination_id": "test_dest",
                    "type": "fhir_api"
                }
            ]
        }
    }

    try:
        print("\n✅ Testing Valid Configuration...")
        response = requests.post(
            f"{CONFIG_ENDPOINT}/interfaces/validate",
            json=valid_config,
            timeout=TIMEOUT
        )

        if response.status_code == 200:
            data = response.json()
            if data.get('valid', False):
                print_result(True, "Valid configuration passed validation", data)
            else:
                print_result(False, "Valid configuration failed validation", data)
        else:
            print_result(False, f"Validation request failed with status {response.status_code}", response.text)

        # Test invalid configuration
        print("\n❌ Testing Invalid Configuration...")
        invalid_config = {
            "name": "",  # Invalid: empty name
            "pipeline": {
                "input": {
                    "type": ""  # Invalid: empty type
                },
                "destinations": []  # Invalid: no destinations
            }
        }

        response = requests.post(
            f"{CONFIG_ENDPOINT}/interfaces/validate",
            json=invalid_config,
            timeout=TIMEOUT
        )

        if response.status_code == 200:
            data = response.json()
            if not data.get('valid', True):
                print_result(True, "Invalid configuration properly rejected", data)
                return True
            else:
                print_result(False, "Invalid configuration incorrectly accepted", data)
                return False
        else:
            print_result(False, f"Validation request failed with status {response.status_code}", response.text)
            return False

    except requests.exceptions.RequestException as e:
        print_result(False, f"Validation test request failed: {e}")
        return False

def test_runtime_monitoring():
    """Test runtime monitoring endpoints"""
    print_test_header("Runtime Monitoring")

    try:
        # Test runtime stats
        print("\n📊 Testing Runtime Stats...")
        response = requests.get(f"{CONFIG_ENDPOINT}/runtime/stats", timeout=TIMEOUT)

        if response.status_code == 200:
            data = response.json()
            print_result(True, "Runtime stats retrieved successfully", data)
        else:
            print_result(False, f"Runtime stats failed with status {response.status_code}", response.text)

        # Test active processes
        print("\n🔄 Testing Active Processes...")
        response = requests.get(f"{CONFIG_ENDPOINT}/runtime/active", timeout=TIMEOUT)

        if response.status_code == 200:
            data = response.json()
            print_result(True, "Active processes retrieved successfully", data)
        else:
            print_result(False, f"Active processes failed with status {response.status_code}", response.text)

        return True

    except requests.exceptions.RequestException as e:
        print_result(False, f"Monitoring test request failed: {e}")
        return False

def test_hot_reload():
    """Test hot-reload functionality"""
    print_test_header("Hot-Reload Functionality")

    try:
        # Test reload all configurations
        print("\n🔥 Testing Reload All Configurations...")
        response = requests.post(f"{CONFIG_ENDPOINT}/reload/all", timeout=TIMEOUT)

        if response.status_code == 200:
            data = response.json()
            print_result(True, "Reload all configurations successful", data)
            return True
        else:
            print_result(False, f"Reload all failed with status {response.status_code}", response.text)
            return False

    except requests.exceptions.RequestException as e:
        print_result(False, f"Hot-reload test request failed: {e}")
        return False

def test_mapping_templates():
    """Test mapping template endpoints"""
    print_test_header("Mapping Templates")

    try:
        # Test get mapping templates
        print("\n📋 Testing Get Mapping Templates...")
        response = requests.get(f"{CONFIG_ENDPOINT}/templates", timeout=TIMEOUT)

        if response.status_code == 200:
            data = response.json()
            print_result(True, "Mapping templates retrieved successfully", data)
            return True
        else:
            print_result(False, f"Get templates failed with status {response.status_code}", response.text)
            return False

    except requests.exceptions.RequestException as e:
        print_result(False, f"Template test request failed: {e}")
        return False

def main():
    """Run all tests"""
    print("🚀 Interface-Centric Configuration Engine - API Endpoint Tests")
    print("=" * 80)
    print(f"🕐 Test started at: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print(f"🌐 Base URL: {BASE_URL}")
    print(f"⚙️  Config URL: {CONFIG_ENDPOINT}")

    tests = [
        ("Configuration Health", test_configuration_health),
        ("Configuration CRUD", test_configuration_crud),
        ("Configuration Validation", test_configuration_validation),
        ("Runtime Monitoring", test_runtime_monitoring),
        ("Hot-Reload", test_hot_reload),
        ("Mapping Templates", test_mapping_templates),
    ]

    results = []
    total_tests = len(tests)

    for test_name, test_func in tests:
        try:
            result = test_func()
            results.append((test_name, result))
            time.sleep(1)  # Brief pause between tests
        except Exception as e:
            print_result(False, f"Test {test_name} crashed: {e}")
            results.append((test_name, False))

    # Summary
    print("\n" + "=" * 80)
    print("📊 TEST SUMMARY")
    print("=" * 80)

    passed = sum(1 for _, result in results if result)
    failed = total_tests - passed

    for test_name, result in results:
        status = "✅ PASS" if result else "❌ FAIL"
        print(f"{status}: {test_name}")

    print(f"\nTotal Tests: {total_tests}")
    print(f"Passed: {passed}")
    print(f"Failed: {failed}")
    print(f"Success Rate: {(passed/total_tests)*100:.1f}%")

    if failed == 0:
        print("\n🎉 All tests passed! Configuration Engine API is working correctly.")
        return 0
    else:
        print(f"\n⚠️  {failed} test(s) failed. Please check the server logs and MongoDB connection.")
        return 1

if __name__ == "__main__":
    exit(main())