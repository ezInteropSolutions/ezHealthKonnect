// services/connectors/astm_framing.go
// The ASTM E1394-97 ENQ/ACK/STX/ETX/EOT transport handshake, written ONCE
// against a plain io.ReadWriter — not net.Conn, not a serial-specific type —
// since both go.bug.st/serial's serial.Port and a plain net.Conn satisfy
// that interface with zero adapter code. Built against the serial connector
// first (this feature's own Phase A), reused unchanged by the ASTM-over-
// TCP/IP connector later (Phase B), which only differs in what concrete
// transport it opens before calling into this file.
//
// Deliberately transport- and deadline-agnostic: a caller (serial_inbound.go,
// astm_tcp_inbound.go) is responsible for setting its own read/write
// deadlines on the REAL underlying connection object before calling
// ReceiveMessage/SendMessage, using whichever mechanism that concrete type
// provides (net.Conn.SetDeadline, serial.Port.SetReadTimeout) — this file
// never type-asserts rw back to a concrete type to do that itself, keeping
// it genuinely reusable across both transports.
package connectors

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"

	"ezhealthkonnect/astm"

	"github.com/google/uuid"
)

// ASTMFramingConfig controls checksum-mismatch severity — everything else
// about the handshake is fixed by the standard itself, not configurable.
type ASTMFramingConfig struct {
	// ChecksumSeverity is "error" (default) — a checksum mismatch sends NAK,
	// asking the sender to retransmit the same frame, matching strict ASTM
	// behavior — or "warning" — a checksum mismatch is logged but the frame
	// is accepted anyway (ACK sent regardless), for sites on genuinely noisy
	// serial links where strict retransmission would stall the instrument.
	ChecksumSeverity string
	// MaxRetries bounds how many times SendMessage will retransmit one frame
	// after a NAK before giving up (default 6, matching ASTM's own rolling
	// 0-7 frame-number range — more retries than that would start reusing a
	// frame number that was never actually a retransmission).
	MaxRetries int
}

func (c ASTMFramingConfig) maxRetries() int {
	if c.MaxRetries > 0 {
		return c.MaxRetries
	}
	return 6
}

func (c ASTMFramingConfig) treatChecksumAsWarning() bool {
	return strings.EqualFold(c.ChecksumSeverity, "warning")
}

// ReceiveMessage plays ASTM's RECEIVER role — the common real-world shape
// for an inbound connector, since the instrument is almost always the one
// that initiates (see ASTMFramingConfig's own InitiateRole precedent in the
// connector config, not here): wait for ENQ, ACK it, read frames until EOT,
// returning the assembled, destuffed record text (ready for
// astm.ParseMessage, which expects exactly this already-destuffed shape).
//
// r is a *bufio.Reader the CALLER constructs once per connection/port and
// reuses across every ReceiveMessage call on that same connection — never a
// fresh bufio.NewReader per call. A bufio.Reader reads ahead from the
// underlying connection in chunks; if a caller (serial_inbound.go's readLoop,
// astm_tcp_inbound.go's handleConnection — both call ReceiveMessage
// repeatedly on ONE long-lived connection, by design, to support multiple
// sequential ENQ...EOT exchanges per session) constructed a NEW bufio.Reader
// on every call instead, any bytes the previous call's reader had already
// read ahead into its own internal buffer but not yet consumed (e.g. the
// NEXT message's own ENQ, sent by a fast real instrument immediately after
// the previous EOT) would be silently discarded when that reader went out of
// scope — the connector would then hang forever waiting for an ENQ that was
// already received and lost. Found as a real, reproducible deadlock (not a
// test artifact) while writing this feature's own multi-exchange test.
func ReceiveMessage(r *bufio.Reader, w io.Writer, cfg ASTMFramingConfig) (string, error) {
	first, err := r.ReadByte()
	if err != nil {
		return "", fmt.Errorf("astm: waiting for ENQ: %w", err)
	}
	if first != astm.ENQ {
		return "", fmt.Errorf("astm: expected ENQ, got byte 0x%02X", first)
	}
	if err := writeByte(w, astm.ACK); err != nil {
		return "", fmt.Errorf("astm: sending ACK for ENQ: %w", err)
	}

	var content strings.Builder
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", fmt.Errorf("astm: waiting for frame or EOT: %w", err)
		}
		if b == astm.EOT {
			return content.String(), nil
		}
		if b != astm.STX {
			return "", fmt.Errorf("astm: expected STX or EOT, got byte 0x%02X", b)
		}

		frame, terminator, checksumOK, err := readFrame(r)
		if err != nil {
			return "", fmt.Errorf("astm: reading frame: %w", err)
		}

		if !checksumOK {
			if cfg.treatChecksumAsWarning() {
				log.Printf("⚠️ ASTM: checksum mismatch on frame %q — accepting anyway (checksum severity = warning)", frame)
			} else {
				if err := writeByte(w, astm.NAK); err != nil {
					return "", fmt.Errorf("astm: sending NAK: %w", err)
				}
				continue // sender is expected to retransmit the same frame
			}
		}

		// frame is "<frameNumber><record text>" — strip the leading
		// single-digit frame number before accumulating record content.
		recordPart := frame
		if len(frame) > 0 {
			recordPart = frame[1:]
		}
		content.WriteString(recordPart)
		if terminator == astm.ETX {
			content.WriteByte('\r')
		}
		// ETB means more frames follow for this same logical record — no CR
		// inserted, so the continued frame's content concatenates onto the
		// same record line rather than starting a new one.

		if err := writeByte(w, astm.ACK); err != nil {
			return "", fmt.Errorf("astm: sending ACK for frame: %w", err)
		}
	}
}

// readFrame reads everything after STX through the terminating ETX/ETB plus
// its 2-hex-digit checksum and trailing CR LF, returning the frame's own
// content (frame number + record text, with the CR that preceded
// ETX/ETB already excluded), which terminator byte ended it, and whether
// the checksum matched.
func readFrame(r *bufio.Reader) (content string, terminator byte, checksumOK bool, err error) {
	var raw []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", 0, false, err
		}
		if b == astm.ETX || b == astm.ETB {
			terminator = b
			break
		}
		raw = append(raw, b)
	}

	checksumBytes := make([]byte, 2)
	if _, err := io.ReadFull(r, checksumBytes); err != nil {
		return "", 0, false, fmt.Errorf("reading checksum: %w", err)
	}
	// Trailing CR LF after the checksum.
	if _, err := r.ReadByte(); err != nil { // CR
		return "", 0, false, fmt.Errorf("reading trailing CR: %w", err)
	}
	if _, err := r.ReadByte(); err != nil { // LF
		return "", 0, false, fmt.Errorf("reading trailing LF: %w", err)
	}

	frameAndContent := append(append([]byte{}, raw...), terminator)
	expected := astm.Checksum(frameAndContent)
	got := string(checksumBytes)
	ok := strings.EqualFold(expected, got)

	// Strip the trailing CR (if any) that precedes ETX/ETB from the content
	// returned to the caller — it's a frame-delimiter artifact, not part of
	// the record text itself.
	contentStr := string(raw)
	contentStr = strings.TrimSuffix(contentStr, "\r")

	return contentStr, terminator, ok, nil
}

// SendMessage plays ASTM's INITIATOR role — used by an outbound connector
// (this transport's rarer real-world direction, but a real one: a host
// pushing orders down to an instrument, or an outbound ASTM-over-TCP/IP
// delivery). content is the destuffed record text (CR-separated records,
// the same shape astm/builder.BuildDocument produces).
//
// r is a *bufio.Reader the caller constructs and reuses across calls on the
// same connection — see ReceiveMessage's own doc comment for why a fresh
// bufio.Reader per call is unsafe whenever a connection carries more than
// one exchange. Every current production caller (serial_outbound.go,
// astm_tcp_outbound.go) opens a brand-new connection per Send(), so a
// single-use bufio.Reader there is already correct by construction — this
// signature exists so a caller that DOES reuse a connection (this package's
// own multi-exchange tests, and any future persistent-connection outbound
// mode) gets the same safety ReceiveMessage does, for free.
func SendMessage(r *bufio.Reader, w io.Writer, content string, cfg ASTMFramingConfig) error {
	if err := writeByte(w, astm.ENQ); err != nil {
		return fmt.Errorf("astm: sending ENQ: %w", err)
	}
	resp, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("astm: waiting for ACK/NAK to ENQ: %w", err)
	}
	if resp == astm.NAK {
		return fmt.Errorf("astm: receiver NAK'd ENQ — not ready to receive")
	}
	if resp != astm.ACK {
		return fmt.Errorf("astm: expected ACK/NAK for ENQ, got byte 0x%02X", resp)
	}

	records := strings.Split(strings.TrimSuffix(content, "\r"), "\r")
	frameNum := 1
	for i, record := range records {
		if record == "" {
			continue
		}
		terminator := astm.ETX // one record per frame — the "unpacked" shape, always spec-valid
		isLast := i == len(records)-1

		if err := sendFrameWithRetry(w, r, frameNum, record, terminator, cfg); err != nil {
			return fmt.Errorf("astm: record %d: %w", i+1, err)
		}
		frameNum = astm.NextFrameNumber(frameNum)
		_ = isLast
	}

	if err := writeByte(w, astm.EOT); err != nil {
		return fmt.Errorf("astm: sending EOT: %w", err)
	}
	return nil
}

func sendFrameWithRetry(w io.Writer, r *bufio.Reader, frameNum int, record string, terminator byte, cfg ASTMFramingConfig) error {
	frameBody := []byte(strconv.Itoa(frameNum) + record + "\r")
	frameAndContent := append(append([]byte{}, frameBody...), terminator)
	checksum := astm.Checksum(frameAndContent)

	wire := append([]byte{astm.STX}, frameAndContent...)
	wire = append(wire, []byte(checksum)...)
	wire = append(wire, astm.CR, astm.LF)

	for attempt := 0; attempt <= cfg.maxRetries(); attempt++ {
		if _, err := w.Write(wire); err != nil {
			return fmt.Errorf("writing frame: %w", err)
		}
		resp, err := r.ReadByte()
		if err != nil {
			return fmt.Errorf("waiting for ACK/NAK: %w", err)
		}
		if resp == astm.ACK {
			return nil
		}
		if resp != astm.NAK {
			return fmt.Errorf("expected ACK/NAK, got byte 0x%02X", resp)
		}
		// NAK — retry the same frame.
	}
	return fmt.Errorf("exceeded %d retries without receiving ACK", cfg.maxRetries())
}

func writeByte(w io.Writer, b byte) error {
	_, err := w.Write([]byte{b})
	return err
}

// generateASTMMessageID returns a unique, transport-labeled message ID —
// shared by serial_inbound.go and astm_tcp_inbound.go. Uses a real UUID
// rather than a shared package-level counter: a plain `counter++` here would
// be an unguarded data race the moment two ASTM connections (two instruments,
// or two sequential messages on two TCP sessions) receive content
// concurrently — each goroutine calling this function independently, with no
// shared state to coordinate — exactly the real-world case this connector
// pair is built for. Matches the UUID convention already used elsewhere in
// this package (as2_outbound.go, direct_messaging_inbound.go) rather than
// inventing a second ID scheme.
func generateASTMMessageID(transportPrefix string) string {
	return fmt.Sprintf("%s_%s", transportPrefix, uuid.New().String())
}
