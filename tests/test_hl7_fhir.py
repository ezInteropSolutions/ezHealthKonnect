#!/usr/bin/env python3
"""
Simple HL7 to FHIR Transformation Test Script
Windows-compatible version without Unicode emojis
"""

import json
import requests
import sys
from datetime import datetime

def load_json_file(file_path):
    """Load JSON file safely"""
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            return json.load(f)
    except FileNotFoundError:
        print(f"ERROR: File {file_path} not found")
        return None
    except json.JSONDecodeError as e:
        print(f"ERROR: Invalid JSON - {e}")
        return None

def check_service_status(base_url):
    """Check if the service is running"""
    try:
        response = requests.get(f"{base_url}/api/fhir/transform/status", timeout=10)
        if response.status_code == 200:
            print("OK: Service is running")
            return True
        else:
            print(f"ERROR: Service returned status {response.status_code}")
            return False
    except requests.exceptions.RequestException as e:
        print(f"ERROR: Cannot connect to service - {e}")
        return False

def test_transformation(base_url, parsed_hl7_data):
    """Test the HL7 to FHIR transformation"""
    
    # Create request payload
    request_data = {
        "parsedHL7Data": parsed_hl7_data,
        "targetProfile": "base",
        "fhirVersion": "R4",
        "createBundle": True,
        "validationMode": "basic",
        "requestId": f"test_{datetime.now().strftime('%Y%m%d_%H%M%S')}",
        "interfaceId": "test_interface"
    }
    
    # Send request
    try:
        print("Sending transformation request...")
        response = requests.post(
            f"{base_url}/api/fhir/transform",
            json=request_data,
            headers={"Content-Type": "application/json"},
            timeout=30
        )
        
        if response.status_code in [200, 201]:
            return response.json()
        else:
            print(f"ERROR: Request failed with status {response.status_code}")
            try:
                error_data = response.json()
                print("Error details:")
                print(json.dumps(error_data, indent=2))
            except:
                print(f"Response: {response.text}")
            return None
            
    except requests.exceptions.RequestException as e:
        print(f"ERROR: Request failed - {e}")
        return None

def display_results(result):
    """Display transformation results"""
    if not result:
        return
    
    print("\n" + "="*60)
    print("HL7 TO FHIR TRANSFORMATION RESULTS")
    print("="*60)
    
    # Basic info
    print(f"Success: {result.get('success', False)}")
    print(f"Request ID: {result.get('requestId', 'N/A')}")
    print(f"Message Type: {result.get('messageType', 'N/A')}")
    
    # Performance
    perf = result.get('performance', {})
    if perf:
        print(f"\nPerformance:")
        print(f"  Total Time: {perf.get('totalTime', 'N/A')}")
        print(f"  Resources Created: {perf.get('resourcesCreated', 0)}")
    
    # Resources
    resources = result.get('fhirResources', [])
    if resources:
        print(f"\nFHIR Resources Created ({len(resources)} total):")
        for i, res in enumerate(resources, 1):
            res_type = res.get('resourceType', 'Unknown')
            res_id = res.get('id', 'No ID')
            print(f"  {i}. {res_type} (ID: {res_id})")
            
            # Show patient details
            if res_type == "Patient":
                names = res.get('name', [])
                if names:
                    given = names[0].get('given', [])
                    family = names[0].get('family', '')
                    print(f"     Name: {' '.join(given)} {family}")
                identifiers = res.get('identifier', [])
                if identifiers:
                    print(f"     ID: {identifiers[0].get('value', 'N/A')}")
    
    # Bundle
    bundle = result.get('bundle')
    if bundle:
        print(f"\nFHIR Bundle:")
        print(f"  Bundle ID: {bundle.get('id', 'N/A')}")
        print(f"  Type: {bundle.get('type', 'N/A')}")
        print(f"  Entries: {len(bundle.get('entry', []))}")
    
    # Warnings and Errors
    warnings = result.get('warnings', [])
    if warnings:
        print(f"\nWarnings ({len(warnings)}):")
        for w in warnings[:3]:
            print(f"  - {w}")
        if len(warnings) > 3:
            print(f"  ... and {len(warnings) - 3} more")
    
    errors = result.get('errors', [])
    if errors:
        print(f"\nErrors ({len(errors)}):")
        for e in errors:
            print(f"  - {e}")

def save_results(result, filename):
    """Save results to file"""
    try:
        with open(filename, 'w', encoding='utf-8') as f:
            json.dump(result, f, indent=2)
        print(f"\nResults saved to: {filename}")
    except Exception as e:
        print(f"ERROR: Could not save results - {e}")

def main():
    if len(sys.argv) < 2:
        print("Usage: python simple_test.py <parsed_hl7_file.json> [server_url]")
        print("Example: python simple_test.py parsedhl7.json")
        print("Example: python simple_test.py parsedhl7.json http://localhost:8080")
        sys.exit(1)
    
    hl7_file = sys.argv[1]
    base_url = sys.argv[2] if len(sys.argv) > 2 else "http://localhost:8080"
    
    print("Starting HL7 to FHIR Transformation Test")
    print("="*50)
    
    # Check service
    if not check_service_status(base_url):
        print("Cannot continue - service is not available")
        sys.exit(1)
    
    # Load HL7 data
    print(f"\nLoading HL7 data from: {hl7_file}")
    hl7_data = load_json_file(hl7_file)
    if not hl7_data:
        sys.exit(1)
    
    if not hl7_data.get('success'):
        print("ERROR: HL7 data indicates parsing was not successful")
        sys.exit(1)
    
    # Test transformation
    result = test_transformation(base_url, hl7_data)
    
    if result:
        # Display results
        display_results(result)
        
        # Save results
        timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
        output_file = f"fhir_result_{timestamp}.json"
        save_results(result, output_file)
        
        print("\nTest completed successfully!")
    else:
        print("Test failed - no valid response")
        sys.exit(1)

if __name__ == "__main__":
    main()