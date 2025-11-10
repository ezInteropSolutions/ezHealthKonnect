# Send test HL7 message to Test Interface1
$hl7Message = "MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20251019120000||ADT^A01|MSG_DELIVERY_TEST_001|P|2.5`r`nEVN|A01|20251019120000`r`nPID|1||67890^^^MRN||TestPatient^Delivery^T||19900101|F|||456 Test St^^TestCity^TX^75001^USA||555-9999|||S||MRN67890`r`nPV1|1|O|ER^101^1|||||||||||||||URGENT"

# Add MLLP framing
$VT = [char]0x0B
$FS = [char]0x1C
$CR = [char]0x0D
$mllpMessage = "$VT$hl7Message$FS$CR"

Write-Host "Sending test message..." -ForegroundColor Cyan

# Create TCP client
$client = New-Object System.Net.Sockets.TcpClient("127.0.0.1", 6661)
$stream = $client.GetStream()
$writer = New-Object System.IO.StreamWriter($stream)
$reader = New-Object System.IO.StreamReader($stream)

# Send message
$writer.Write($mllpMessage)
$writer.Flush()

Write-Host "Message sent! Waiting for ACK..." -ForegroundColor Green

# Read ACK
$ack = $reader.ReadLine()
Write-Host "ACK received: $ack" -ForegroundColor Green

# Cleanup
$writer.Close()
$reader.Close()
$client.Close()

Write-Host "Test complete! Check logs for delivery status." -ForegroundColor Yellow
