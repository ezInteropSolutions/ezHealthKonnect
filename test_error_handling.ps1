# Test Error Handling - Send Malformed HL7 Messages
# This script tests the V23 error handling enhancement

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Testing Error Handling System (V23)" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Configuration
$interfaceId = "629ac1e8-0c50-447a-b93f-ebfc15830a7d"
$tcpPort = 6661
$host = "localhost"

# Test 1: Missing Required Fields (invalid HL7)
Write-Host "Test 1: Sending HL7 message with missing required fields..." -ForegroundColor Yellow
$malformedHL7_1 = @"
MSH|^~\&||SENDING_APP||RECEIVING_APP|20231020120000||ADT^A01|MSG001|P|2.5
PID||
PV1||
"@

try {
    $tcpClient = New-Object System.Net.Sockets.TcpClient($host, $tcpPort)
    $stream = $tcpClient.GetStream()
    $writer = New-Object System.IO.StreamWriter($stream)

    # Send malformed message
    $writer.WriteLine($malformedHL7_1)
    $writer.Flush()

    Write-Host "✓ Sent malformed HL7 (missing PID fields)" -ForegroundColor Green

    $writer.Close()
    $stream.Close()
    $tcpClient.Close()

    Start-Sleep -Seconds 2
} catch {
    Write-Host "✗ Failed to send: $_" -ForegroundColor Red
}

# Test 2: Invalid Message Structure
Write-Host "`nTest 2: Sending completely invalid message..." -ForegroundColor Yellow
$malformedHL7_2 = "THIS_IS_NOT_VALID_HL7_AT_ALL|||"

try {
    $tcpClient = New-Object System.Net.Sockets.TcpClient($host, $tcpPort)
    $stream = $tcpClient.GetStream()
    $writer = New-Object System.IO.StreamWriter($stream)

    $writer.WriteLine($malformedHL7_2)
    $writer.Flush()

    Write-Host "✓ Sent invalid message structure" -ForegroundColor Green

    $writer.Close()
    $stream.Close()
    $tcpClient.Close()

    Start-Sleep -Seconds 2
} catch {
    Write-Host "✗ Failed to send: $_" -ForegroundColor Red
}

# Test 3: Empty Message
Write-Host "`nTest 3: Sending empty message..." -ForegroundColor Yellow
$malformedHL7_3 = ""

try {
    $tcpClient = New-Object System.Net.Sockets.TcpClient($host, $tcpPort)
    $stream = $tcpClient.GetStream()
    $writer = New-Object System.IO.StreamWriter($stream)

    $writer.WriteLine($malformedHL7_3)
    $writer.Flush()

    Write-Host "✓ Sent empty message" -ForegroundColor Green

    $writer.Close()
    $stream.Close()
    $tcpClient.Close()

    Start-Sleep -Seconds 2
} catch {
    Write-Host "✗ Failed to send: $_" -ForegroundColor Red
}

# Test 4: Valid HL7 Message (Control Test)
Write-Host "`nTest 4: Sending valid HL7 message (control)..." -ForegroundColor Yellow
$validHL7 = @"
MSH|^~\&|SENDING_APP|SENDING_FACILITY|RECEIVING_APP|RECEIVING_FACILITY|20231020120000||ADT^A01|MSG002|P|2.5
EVN|A01|20231020120000
PID|1||12345^^^MRN||Doe^John^A||19800101|M|||123 Main St^^Anytown^CA^12345^USA|||||||123-45-6789
PV1|1|I|ICU^101^01^MAIN||||1234^Smith^John^A^MD||||||||||1234567890|||||||||||||||||||||||||20231020120000
"@

try {
    $tcpClient = New-Object System.Net.Sockets.TcpClient($host, $tcpPort)
    $stream = $tcpClient.GetStream()
    $writer = New-Object System.IO.StreamWriter($stream)

    $writer.WriteLine($validHL7)
    $writer.Flush()

    Write-Host "✓ Sent valid HL7 message" -ForegroundColor Green

    $writer.Close()
    $stream.Close()
    $tcpClient.Close()

    Start-Sleep -Seconds 2
} catch {
    Write-Host "✗ Failed to send: $_" -ForegroundColor Red
}

Write-Host "`n========================================" -ForegroundColor Cyan
Write-Host "Error Handling Tests Complete!" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Next Steps:" -ForegroundColor Yellow
Write-Host "1. Open http://localhost:3000/messages.html?interfaceId=$interfaceId" -ForegroundColor White
Write-Host "2. Check messages with errors" -ForegroundColor White
Write-Host "3. Click on failed messages to see the 'Errors & Warnings' tab" -ForegroundColor White
Write-Host "4. Verify error details, stack traces, and recovery actions" -ForegroundColor White
Write-Host ""
Write-Host "Database Verification:" -ForegroundColor Yellow
Write-Host "docker exec ezhealthkonnect-postgres psql -U ezhealth_user -d ezhealthkonnect -c \\" -ForegroundColor Gray
Write-Host "  `"SELECT message_id, error_count, last_error_message, error_severity FROM messages_intf_629ac1e8_0c50_447a_b93f_ebfc15830a7d ORDER BY received_at DESC LIMIT 5;`\"" -ForegroundColor Gray
Write-Host ""
