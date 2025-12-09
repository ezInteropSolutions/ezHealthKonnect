#!/usr/bin/env python3
"""Send test HL7 message to Test Interface3"""
import socket
import time

# HL7 message
hl7_message = """MSH|^~\\&|SendingApp|SendingFac|ReceivingApp|ReceivingFac|20251116103500||ADT^A01|MSG00005|P|2.5
EVN||20251116103500
PID|||67890^^^MRN||TestUser^Mike^B||19851215|M|||789 Oak St^^CityName^CA^90001^USA||(555)555-7777|||M||9876543|456-78-9012
PV1||E|ER^201^01||||DOC789^Brown^Alice^N^MD^^^L|||ORTHO||||U|||DOC789^Brown^Alice^N^MD^^^L||VIS789|INS3||||||||||||||||||||||||||20251116103500
"""

# MLLP framing
start_block = b'\x0B'
end_block = b'\x1C\x0D'
message = start_block + hl7_message.encode('utf-8').replace(b'\n', b'\r') + end_block

# Send to Test Interface3 on port 6663
try:
    client = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    client.settimeout(5)
    client.connect(('localhost', 6663))

    print("Connected to Test Interface3 on port 6663")
    client.sendall(message)
    print(f"Sent {len(message)} bytes")

    # Wait for ACK
    response = client.recv(1024)
    print(f"Received response ({len(response)} bytes):")
    print(response.decode('utf-8', errors='ignore'))

    client.close()
    print("\nTest message sent successfully!")
except Exception as e:
    print(f"Error: {e}")
