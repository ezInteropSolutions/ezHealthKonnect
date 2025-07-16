import gzip, json, pprint
with gzip.open('./schemas/hl7/v2.5.1/ADT_A04.gz', 'rt') as f:
    data = json.load(f)
    if 'structure' in data:
        # Show MSH segment fields
        print('=== MSH SEGMENT FIELDS ===')
        if 'MSH' in data['structure'] and 'fields' in data['structure']['MSH']:
            msh_fields = data['structure']['MSH']['fields']
            print(f'MSH has {len(msh_fields)} fields:')
            for field_name in sorted(msh_fields.keys()):
                field_data = msh_fields[field_name]
                print(f'\\nField: {field_name}')
                pprint.pprint(field_data, depth=2, width=100)
        
        print('\\n\\n=== PID SEGMENT FIELDS ===')
        if 'PID' in data['structure'] and 'fields' in data['structure']['PID']:
            pid_fields = data['structure']['PID']['fields']
            print(f'PID has {len(pid_fields)} fields:')
            # Show just first 3 fields for brevity
            for i, field_name in enumerate(sorted(pid_fields.keys())):
                if i >= 3: break
                field_data = pid_fields[field_name]
                print(f'\\nField: {field_name}')
                pprint.pprint(field_data, depth=3, width=100)
