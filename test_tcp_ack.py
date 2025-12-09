#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Test script to send HL7 message and capture ACK response
Usage: python test_tcp_ack.py
"""
import socket
import time
import sys
import io

# Fix Windows console encoding
if sys.platform == 'win32':
    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8')
    sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding='utf-8')

# MLLP framing bytes
MLLP_START = b'\x0b'
MLLP_END = b'\x1c\x0d'

# Sample HL7 ADT^A01 message
hl7_message = r"""MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20251123180000||ADT^A01|MSG00001|P|2.5
EVN|A01|20251123180000
PID|1||12345678||DOE^JOHN^A||19800101|M|||123 MAIN ST^^CITY^STATE^12345||555-1234|||S||ACCT12345|123-45-6789
PV1|1|I|WARD^ROOM^BED|"""

def send_hl7_and_get_ack(host='localhost', port=6667):
    """Send HL7 message and capture ACK"""
    print(f"🔌 Connecting to {host}:{port}...")

    try:
        # Create socket
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.settimeout(10)  # 10 second timeout

        # Connect
        sock.connect((host, port))
        print(f"✅ Connected to {host}:{port}")

        # Prepare MLLP-framed message
        framed_message = MLLP_START + hl7_message.encode('utf-8') + MLLP_END

        # Send message
        print(f"\n📤 Sending message ({len(framed_message)} bytes)...")
        print("=" * 60)
        print(hl7_message)
        print("=" * 60)

        sock.sendall(framed_message)
        print(f"✅ Message sent!")

        # IMPORTANT: Don't close write side - keep connection fully open
        # Some implementations shut down write after sending, but we need bidirectional

        # Wait for ACK
        print(f"\n⏳ Waiting for ACK response...")

        # Set a short timeout for receiving
        sock.settimeout(2.0)

        # Receive response (ACK should come back quickly)
        response = b''
        start_time = time.time()

        try:
            while time.time() - start_time < 5:  # Wait up to 5 seconds
                try:
                    chunk = sock.recv(1024)
                    if not chunk:
                        # Server closed connection
                        break
                    response += chunk

                    # Check if we got a complete MLLP frame
                    if MLLP_END in response:
                        break

                except socket.timeout:
                    # Timeout waiting for more data
                    if len(response) > 0:
                        # We got some data, break and process it
                        break
                    # No data yet, continue waiting
                    continue
        except Exception as e:
            print(f"   Exception during receive: {e}")

        if response:
            # Parse MLLP frame
            if response.startswith(MLLP_START):
                ack_message = response[1:]  # Remove start byte
                if MLLP_END in ack_message:
                    ack_message = ack_message.split(MLLP_END)[0]  # Remove end bytes
            else:
                ack_message = response

            print(f"\n✅ ACK RECEIVED ({len(response)} bytes)!")
            print("=" * 60)
            print(ack_message.decode('utf-8', errors='replace'))
            print("=" * 60)

            # Parse ACK
            lines = ack_message.decode('utf-8', errors='replace').split('\r')
            for line in lines:
                if line.startswith('MSA'):
                    fields = line.split('|')
                    if len(fields) >= 3:
                        ack_code = fields[1]
                        control_id = fields[2]
                        message = fields[3] if len(fields) > 3 else ''

                        if ack_code == 'AA':
                            print(f"\n🎉 SUCCESS: Application Accept (AA)")
                        elif ack_code == 'AE':
                            print(f"\n❌ ERROR: Application Error (AE)")
                        elif ack_code == 'AR':
                            print(f"\n⚠️  REJECT: Application Reject (AR)")

                        print(f"   Message Control ID: {control_id}")
                        if message:
                            print(f"   Message: {message}")
        else:
            print(f"\n❌ No ACK received within timeout period")
            print(f"   This could mean:")
            print(f"   - Server not sending ACK")
            print(f"   - Network/firewall blocking response")
            print(f"   - Connection closed before ACK sent")

        # Close socket
        sock.close()
        print(f"\n🔌 Connection closed")

    except ConnectionRefusedError:
        print(f"❌ Connection refused. Is the server listening on {host}:{port}?")
        return False
    except socket.timeout:
        print(f"❌ Connection timeout")
        return False
    except Exception as e:
        print(f"❌ Error: {e}")
        import traceback
        traceback.print_exc()
        return False

    return True

if __name__ == '__main__':
    print("="* 60)
    print("TCP/MLLP ACK Test Script")
    print("Testing Test Interface 7 on port 6667")
    print("=" * 60)
    print()

    send_hl7_and_get_ack()

    print("\n" + "=" * 60)
    print("Test complete!")
    print("=" * 60)