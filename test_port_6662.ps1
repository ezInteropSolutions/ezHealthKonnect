# Send test HL7 message to port 6662 (Test Interface2)
$hl7Message = "MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20251109071000||ADT^A01|TEST_6662_001|P|2.5`r`nEVN|A01|20251109071000`r`nPID|1||12345^^^MRN||TestPatient^MVC^Pipeline||19850505|M|||789 Main St^^CityName^ST^12345^USA||555-5555|||M||MRN12345`r`nPV1|1|I|WARD^101^1|||||||||||||||V99999"

# Add MLLP framing
$VT = [char]0x0B
$FS = [char]0x1C
$CR = [char]0x0D
$mllpMessage = "$VT$hl7Message$FS$CR"

Write-Host "Sending test message to port 6662..." -ForegroundColor Cyan

# Create TCP client
$client = New-Object System.Net.Sockets.TcpClient("127.0.0.1", 6662)
$stream = $client.GetStream()
$writer = New-Object System.IO.StreamWriter($stream)
$reader = New-Object System.IO.StreamReader($stream)

# Send message
$writer.Write($mllpMessage)
$writer.Flush()

Write-Host "Message sent! Waiting for ACK..." -ForegroundColor Green

# Read ACK
Start-Sleep -Milliseconds 500
if ($stream.DataAvailable) {
    $ack = $reader.ReadLine()
    Write-Host "ACK received: $ack" -ForegroundColor Green
} else {
    Write-Host "No ACK received (timeout)" -ForegroundColor Yellow
}

# Cleanup
$writer.Close()
$reader.Close()
$client.Close()

Write-Host "Test complete! Check logs for MVC pipeline execution." -ForegroundColor Yellow
