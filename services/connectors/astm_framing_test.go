package connectors

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	"ezhealthkonnect/astm"
)

func TestASTMFraming_SendReceiveRoundTrip(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	content := "H|\\^&|MSGCTRL001||ANALYZER-1^1.0^SN12345|||||LIS||P|E1394-97|20231004120000\r" +
		"P|1||MRN12345||Doe^Jane||19800101|F\r" +
		"L|1|N\r"

	errCh := make(chan error, 1)
	go func() {
		errCh <- SendMessage(bufio.NewReader(clientConn), clientConn, content, ASTMFramingConfig{})
	}()

	received, err := ReceiveMessage(bufio.NewReader(serverConn), serverConn, ASTMFramingConfig{})
	if err != nil {
		t.Fatalf("ReceiveMessage: %v", err)
	}

	if sendErr := <-errCh; sendErr != nil {
		t.Fatalf("SendMessage: %v", sendErr)
	}

	if received != content {
		t.Errorf("round-tripped content mismatch:\n got:  %q\n want: %q", received, content)
	}
}

func TestASTMFraming_SendMessage_RetriesFrameAfterNAK_ThenSucceeds(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		r := bufio.NewReader(serverConn)

		b, _ := r.ReadByte() // ENQ
		if b != astm.ENQ {
			t.Errorf("expected ENQ, got 0x%02X", b)
		}
		serverConn.Write([]byte{astm.ACK})

		// NAK the first attempt at the one frame SendMessage will send...
		readFrame(r)
		serverConn.Write([]byte{astm.NAK})

		// ...then ACK the retransmit.
		readFrame(r)
		serverConn.Write([]byte{astm.ACK})

		eot, _ := r.ReadByte()
		if eot != astm.EOT {
			t.Errorf("expected EOT, got 0x%02X", eot)
		}
	}()

	err := SendMessage(bufio.NewReader(clientConn), clientConn, "H|\\^&|TEST\r", ASTMFramingConfig{})
	if err != nil {
		t.Fatalf("SendMessage should succeed after one NAK'd retry: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for receiver goroutine")
	}
}

func TestASTMFraming_SendMessage_GivesUpAfterMaxRetries(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		r := bufio.NewReader(serverConn)
		r.ReadByte() // ENQ
		serverConn.Write([]byte{astm.ACK})

		// Always NAK, forever (up to the caller's own retry budget).
		for {
			if _, err := r.ReadByte(); err != nil {
				return
			}
			// consume the rest of the frame crudely by reading until LF
			for {
				b, err := r.ReadByte()
				if err != nil || b == astm.LF {
					break
				}
			}
			if werr := func() error { _, e := serverConn.Write([]byte{astm.NAK}); return e }(); werr != nil {
				return
			}
		}
	}()

	err := SendMessage(bufio.NewReader(clientConn), clientConn, "H|\\^&|TEST\r", ASTMFramingConfig{MaxRetries: 2})
	if err == nil {
		t.Fatal("expected SendMessage to give up and return an error after exceeding MaxRetries")
	}

	clientConn.Close() // unblock the receiver goroutine's ReadByte
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

func TestASTMFraming_SendMessage_FrameNumbersIncrementAndWrapAcrossRecords(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// 9 records needs frame numbers 1,2,3,4,5,6,7,0,1 — covers the 7->0
	// wraparound mid-message, not just NextFrameNumber's own unit behavior.
	var records []string
	for i := 0; i < 9; i++ {
		records = append(records, "X|"+string(rune('A'+i)))
	}
	content := strings.Join(records, "\r") + "\r"

	var observedFrameNumbers []byte
	done := make(chan struct{})
	go func() {
		defer close(done)
		r := bufio.NewReader(serverConn)
		r.ReadByte() // ENQ
		serverConn.Write([]byte{astm.ACK})

		for i := 0; i < len(records); i++ {
			b, err := r.ReadByte() // STX
			if err != nil || b != astm.STX {
				return
			}
			frameNumByte, err := r.ReadByte()
			if err != nil {
				return
			}
			observedFrameNumbers = append(observedFrameNumbers, frameNumByte)
			// consume the rest of the frame crudely through the trailing LF
			for {
				bb, err := r.ReadByte()
				if err != nil || bb == astm.LF {
					break
				}
			}
			serverConn.Write([]byte{astm.ACK})
		}
		r.ReadByte() // EOT
	}()

	if err := SendMessage(bufio.NewReader(clientConn), clientConn, content, ASTMFramingConfig{}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for receiver goroutine")
	}

	want := "123456701" // 9 records -> frame numbers 1..7, then wraps to 0, 1
	got := string(observedFrameNumbers)
	if got != want {
		t.Errorf("observed frame number sequence = %q, want %q", got, want)
	}
}

func TestASTMFraming_ReceiveMessage_RejectsNonENQFirstByte(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		clientConn.Write([]byte{astm.STX})
	}()

	_, err := ReceiveMessage(bufio.NewReader(serverConn), serverConn, ASTMFramingConfig{})
	if err == nil {
		t.Error("expected an error when the first byte isn't ENQ")
	}
}

func TestASTMFraming_ReadFrame_DetectsChecksumMismatch(t *testing.T) {
	// Hand-craft a frame: "1H|\^&|TEST\r" + ETX + a deliberately wrong
	// checksum + CRLF.
	frameBody := "1H|\\^&|TEST\r"
	wrongChecksum := "00" // almost certainly not the real sum for this content
	raw := frameBody + string(astm.ETX) + wrongChecksum + "\r\n"

	r := bufio.NewReader(strings.NewReader(raw))
	content, terminator, ok, err := readFrame(r)
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if ok {
		t.Error("expected checksum mismatch to be detected (ok=false)")
	}
	if terminator != astm.ETX {
		t.Errorf("terminator = 0x%02X, want ETX", terminator)
	}
	if content != "1H|\\^&|TEST" {
		t.Errorf("content = %q, want %q", content, "1H|\\^&|TEST")
	}
}

func TestASTMFraming_ReadFrame_ValidChecksumPasses(t *testing.T) {
	frameBody := "1H|\\^&|TEST\r"
	frameAndContent := append([]byte(frameBody), astm.ETX)
	checksum := astm.Checksum(frameAndContent)
	raw := frameBody + string(astm.ETX) + checksum + "\r\n"

	r := bufio.NewReader(strings.NewReader(raw))
	_, _, ok, err := readFrame(r)
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if !ok {
		t.Error("expected a correctly-computed checksum to pass")
	}
}

func TestASTMFraming_ReceiveMessage_DefaultChecksumSeverity_NAKsThenAcceptsRetransmit(t *testing.T) {
	// The real-world counterpart to the warning-mode test below: the DEFAULT
	// severity ("error", the zero value of ASTMFramingConfig.ChecksumSeverity)
	// must NAK a bad-checksum frame and keep waiting — it must NOT silently
	// accept it the way warning mode does. This is the one half of the
	// checksum-severity behavior this file's own test suite had never
	// directly exercised (only the warning-mode accept path had a test).
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		var buf [1]byte
		clientConn.Write([]byte{astm.ENQ})
		clientConn.Read(buf[:]) // ACK for ENQ

		// First attempt: deliberately wrong checksum.
		frameBody := "1H|\\^&|TEST\r"
		badWire := append([]byte{astm.STX}, []byte(frameBody)...)
		badWire = append(badWire, astm.ETX)
		badWire = append(badWire, []byte("00")...)
		badWire = append(badWire, astm.CR, astm.LF)
		clientConn.Write(badWire)

		resp, _ := bufio.NewReader(clientConn).ReadByte()
		if resp != astm.NAK {
			t.Errorf("expected NAK for the bad-checksum frame under default (error) severity, got byte 0x%02X", resp)
		}

		// Retransmit the SAME frame number with the CORRECT checksum.
		frameAndContent := append([]byte(frameBody), astm.ETX)
		goodChecksum := astm.Checksum(frameAndContent)
		goodWire := append([]byte{astm.STX}, frameAndContent...)
		goodWire = append(goodWire, []byte(goodChecksum)...)
		goodWire = append(goodWire, astm.CR, astm.LF)
		clientConn.Write(goodWire)
		clientConn.Read(buf[:]) // ACK for the corrected retransmit

		clientConn.Write([]byte{astm.EOT})
	}()

	received, err := ReceiveMessage(bufio.NewReader(serverConn), serverConn, ASTMFramingConfig{}) // default severity — zero value means "error"
	if err != nil {
		t.Fatalf("ReceiveMessage: %v", err)
	}
	if received != "H|\\^&|TEST\r" {
		t.Errorf("received = %q, want %q", received, "H|\\^&|TEST\r")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sender goroutine")
	}
}

func TestASTMFraming_ReceiveMessage_ChecksumWarningMode_AcceptsBadFrame(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		var buf [1]byte
		clientConn.Write([]byte{astm.ENQ})
		clientConn.Read(buf[:]) // consume ACK for ENQ

		// Send one frame with an intentionally wrong checksum.
		frameBody := "1H|\\^&|TEST\r"
		wire := append([]byte{astm.STX}, []byte(frameBody)...)
		wire = append(wire, astm.ETX)
		wire = append(wire, []byte("00")...)
		wire = append(wire, astm.CR, astm.LF)
		clientConn.Write(wire)
		clientConn.Read(buf[:]) // consume ACK despite the bad checksum (warning mode)

		clientConn.Write([]byte{astm.EOT})
	}()

	received, err := ReceiveMessage(bufio.NewReader(serverConn), serverConn, ASTMFramingConfig{ChecksumSeverity: "warning"})
	if err != nil {
		t.Fatalf("ReceiveMessage: %v", err)
	}
	if received != "H|\\^&|TEST\r" {
		t.Errorf("received = %q, want %q", received, "H|\\^&|TEST\r")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sender goroutine")
	}
}
