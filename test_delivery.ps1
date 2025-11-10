# Test Output Delivery - Send HL7 Message to Test Interface1
# This will trigger: TCP Receive → Parse → Transform → **DELIVER TO FHIR RECEIVER**

$hl7Message = @"
MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20251019120000||ADT^A01|MSG_DELIVERY_TEST_001|P|2.5
EVN|A01|20251019120000
PID|1||67890^^^MRN||TestPatient^Delivery^T||19900101|F|||456 Test St^^TestCity^TX^75001^USA||555-9999|||S||MRN67890
PV1|1|O|ER^101^1|||||||||||||||URGENT
"@

# Add MLLP framing
$VT = [char]0x0B
$FS = [char]0x1C
$CR = [char]0x0D
$mllpMessage = "$VT$hl7Message$FS$CR"

Write-Host "Sending HL7 message to Test Interface1 (port 6661)..." -ForegroundColor Cyan
Write-Host "Message ID: MSG_DELIVERY_TEST_001" -ForegroundColor Yellow

# Create TCP client
$client = New-Object System.Net.Sockets.TcpClient("127.0.0.1", 6661)
$stream = $client.GetStream()
$writer = New-Object System.IO.StreamWriter($stream)
$reader = New-Object System.IO.StreamReader($stream)

# Send message
$writer.Write($mllpMessage)
$writer.Flush()

Write-Host "✅ Message sent! Waiting for ACK..." -ForegroundColor Green

# Read ACK response
$ack = $reader.ReadLine()
Write-Host "📥 ACK received: $ack" -ForegroundColor Green

# Close connection
$writer.Close()
$reader.Close()
$client.Close()

Write-Host ""
Write-Host "✅ Test message sent successfully!" -ForegroundColor Green
Write-Host ""
Write-Host "Expected flow:" -ForegroundColor Cyan
Write-Host "  1. TCP Connector receives message" -ForegroundColor White
Write-Host "  2. Parse to JSON" -ForegroundColor White
Write-Host "  3. Transform HL7 to FHIR" -ForegroundColor White
Write-Host "  4. Store output" -ForegroundColor White
Write-Host "  5. TRIGGER DELIVERY to FHIR Receiver" -ForegroundColor Yellow
Write-Host "  6. POST to http://localhost:8081/Patient" -ForegroundColor White
Write-Host "  7. Update delivery status" -ForegroundColor White
Write-Host ""
Write-Host "Check logs with: docker-compose logs -f app" -ForegroundColor Cyan
