# Test TCP connection to port 6662
# Send a simple HL7 ADT^A01 message wrapped in MLLP framing

$server = "localhost"
$port = 6662

Write-Host "Testing TCP connection to $server`:$port..." -ForegroundColor Yellow

try {
    $client = New-Object System.Net.Sockets.TcpClient
    $client.Connect($server, $port)

    if ($client.Connected) {
        Write-Host "✅ Successfully connected to $server`:$port" -ForegroundColor Green

        # HL7 ADT^A01 test message
        $hl7Message = @"
MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20250104171300||ADT^A01|MSG00001|P|2.5
EVN|A01|20250104171300
PID|1||123456^^^FACILITY^MR||DOE^JOHN^A||19800101|M|||123 MAIN ST^^CITY^ST^12345||(555)555-1212|||S||987654321
PV1|1|I|WARD^ROOM^BED||||DOCTOR^ATTENDING^A|||||||||||VISIT123|||||||||||||||||||||||||20250104171300
"@

        # MLLP framing: Start-of-Block (0x0B) + Message + End-of-Block (0x1C) + Carriage-Return (0x0D)
        $startBlock = [char]0x0B
        $endBlock = [char]0x1C
        $carriageReturn = [char]0x0D

        $mllpMessage = $startBlock + $hl7Message + $endBlock + $carriageReturn

        Write-Host "`nSending MLLP-wrapped HL7 message..." -ForegroundColor Yellow
        Write-Host "Message length: $($mllpMessage.Length) bytes" -ForegroundColor Cyan

        $stream = $client.GetStream()
        $writer = New-Object System.IO.StreamWriter($stream)
        $reader = New-Object System.IO.StreamReader($stream)

        # Send message
        $writer.Write($mllpMessage)
        $writer.Flush()

        Write-Host "✅ Message sent successfully" -ForegroundColor Green

        # Wait for ACK (timeout after 5 seconds)
        Write-Host "`nWaiting for ACK response..." -ForegroundColor Yellow
        $stream.ReadTimeout = 5000

        try {
            $response = $reader.ReadLine()
            if ($response) {
                Write-Host "✅ Received response:" -ForegroundColor Green
                Write-Host $response -ForegroundColor White
            }
        }
        catch {
            Write-Host "⚠️ No ACK received (timeout or connection closed)" -ForegroundColor Yellow
        }

        $client.Close()
        Write-Host "`n✅ Connection closed" -ForegroundColor Green
    }
    else {
        Write-Host "❌ Failed to connect to $server`:$port" -ForegroundColor Red
    }
}
catch {
    Write-Host "❌ Connection error: $_" -ForegroundColor Red
    Write-Host "`nPossible reasons:" -ForegroundColor Yellow
    Write-Host "  1. Port 6662 is not exposed in Docker container" -ForegroundColor White
    Write-Host "  2. Interface is not activated/started" -ForegroundColor White
    Write-Host "  3. Firewall blocking the connection" -ForegroundColor White
}
