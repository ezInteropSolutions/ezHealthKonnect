// services/connectors/hl7_mllp_session.go
// Transport-agnostic HL7 v2 MLLP framing and acknowledgement, shared by every
// connector that receives HL7 v2 from a device: tcp_mllp_inbound.go (network)
// and serial_inbound.go in "hl7_mllp" protocol mode (RS232). Written against
// *bufio.Reader / io.Writer only — both net.Conn and go.bug.st/serial's
// serial.Port satisfy those — the same "one implementation, any byte stream"
// approach astm_framing.go already takes for ASTM.
//
// A serial link and a TCP connection differ only in what happens AROUND a
// frame (TCP: per-connection deadlines, TLS, host-query; serial: one
// long-lived port, line noise). The frame bytes (VT ... FS CR) and the ACK
// the device expects back (MSH + MSA, AA/AE/AR) are identical, so they live
// here once.
package connectors

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/dop251/goja"
)

// ReadMLLPFrameFrom reads one MLLP frame (VT payload FS CR) from r.
//
// r MUST be one *bufio.Reader the caller keeps for the whole connection/port —
// never a fresh one per call, or bytes the reader already buffered for the
// NEXT frame are lost (see astm_framing.go's ReceiveMessage for the deadlock
// this caused there).
//
// skipPreamble controls bytes before the start byte: true discards them
// (serial lines pick up noise when a cable is plugged in or a device powers
// up); false treats any non-VT first byte as a framing error, the TCP
// listener's long-standing behavior.
//
// An FS not followed by CR is kept as payload, and a frame larger than
// maxBytes returns errMLLPFrameTooLarge (bounds memory against a sender that
// never emits the end marker).
func ReadMLLPFrameFrom(r *bufio.Reader, maxBytes int, skipPreamble bool) (string, error) {
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == MLLPStartByte {
			break
		}
		if !skipPreamble {
			return "", fmt.Errorf("invalid MLLP start byte: 0x%02X (expected 0x0B)", b)
		}
	}

	var message strings.Builder
	for {
		if maxBytes > 0 && message.Len() >= maxBytes {
			return "", errMLLPFrameTooLarge
		}
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if b != MLLPEndByte1 {
			message.WriteByte(b)
			continue
		}
		next, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if next == MLLPEndByte2 {
			return message.String(), nil
		}
		message.WriteByte(b)
		message.WriteByte(next)
	}
}

// WriteMLLPFrameTo wraps message in MLLP framing and writes it in one call.
func WriteMLLPFrameTo(w io.Writer, message string) error {
	frame := make([]byte, 0, len(message)+3)
	frame = append(frame, MLLPStartByte)
	frame = append(frame, message...)
	frame = append(frame, MLLPEndByte1, MLLPEndByte2)
	_, err := w.Write(frame)
	return err
}

// isHL7v2Message reports whether a frame starts with a structurally valid MSH
// segment: optional leading BOM/whitespace, then "MSH", then a field separator
// (any non-alphanumeric, non-whitespace character — HL7 does not mandate "|")
// followed by at least the encoding characters.
func isHL7v2Message(frame string) bool {
	trimmed := strings.TrimLeft(frame, "\ufeff \t\r\n")
	if len(trimmed) < 8 || !strings.HasPrefix(trimmed, "MSH") {
		return false
	}
	sep := rune(trimmed[3])
	if unicode.IsLetter(sep) || unicode.IsDigit(sep) || unicode.IsSpace(sep) {
		return false
	}
	return true
}

// hl7MSHFields returns the pipe-split fields of the message's first segment
// when it is an MSH segment (both \r and \r\n segment terminators accepted).
func hl7MSHFields(hl7Message string) []string {
	first := strings.SplitN(strings.ReplaceAll(hl7Message, "\r\n", "\r"), "\r", 2)[0]
	first = strings.TrimSuffix(first, "\n")
	if !strings.HasPrefix(first, "MSH") {
		return nil
	}
	return strings.Split(first, "|")
}

// HL7MSHField returns MSH field `position` counted the way strings.Split on
// "|" counts it (index 8 = MSH-9, index 9 = MSH-10), or "" when absent.
func HL7MSHField(hl7Message string, position int) string {
	fields := hl7MSHFields(hl7Message)
	if len(fields) > position {
		return fields[position]
	}
	return ""
}

// HL7MessageType returns MSH-9 (e.g. "ORU^R01"), or "UNKNOWN".
func HL7MessageType(hl7Message string) string {
	if t := HL7MSHField(hl7Message, 8); t != "" {
		return t
	}
	return "UNKNOWN"
}

// HL7MessageControlID returns MSH-10, or "".
func HL7MessageControlID(hl7Message string) string {
	return HL7MSHField(hl7Message, 9)
}

// parseACKConfig reads the nested "ack" config object every HL7-receiving
// connector shares. "mode: none" is upgraded to "immediate": a device
// waiting for an MLLP reply would block forever without one.
func parseACKConfig(ackMap map[string]interface{}) ACKConfig {
	mode := getStringFromMap(ackMap, "mode", "immediate")
	if mode == "none" {
		mode = "immediate"
	}
	return ACKConfig{
		Mode:            mode,
		OnError:         getStringFromMap(ackMap, "on_error", "nack"),
		SendingApp:      getStringFromMap(ackMap, "sending_app", "ezHealthKonnect"),
		SendingFacility: getStringFromMap(ackMap, "sending_facility", "EHK"),
		TextSuccess:     getStringFromMap(ackMap, "text_success", "Message received successfully"),
		TextError:       getStringFromMap(ackMap, "text_error", "Message processing error"),
		Script:          getStringFromMap(ackMap, "script", ""),
	}
}

// HL7Acknowledger builds the HL7 ACK a device receives for each message,
// applying the configured sender identity, text and optional buildACK script.
type HL7Acknowledger struct {
	Config ACKConfig
	// Now is the clock used for MSH-7; nil means time.Now.
	Now func() time.Time
}

// NewHL7Acknowledger creates an acknowledger for the given configuration.
func NewHL7Acknowledger(cfg ACKConfig) *HL7Acknowledger {
	return &HL7Acknowledger{Config: cfg}
}

// Resolve returns the ACK code and text after the optional script override.
// A script error falls back to the defaults (never fails the reply).
func (a *HL7Acknowledger) Resolve(originalMessage, defaultCode, defaultText string) (string, string, error) {
	if a.Config.Script == "" {
		return defaultCode, defaultText, nil
	}
	code, text, err := a.RunScript(originalMessage, defaultCode, defaultText)
	if err != nil {
		return defaultCode, defaultText, err
	}
	return code, text, nil
}

// Build returns the ACK message text (MSH + MSA, CRLF-terminated) for an
// already-resolved code and text.
func (a *HL7Acknowledger) Build(originalMessage, ackCode, textMessage string) string {
	controlID := HL7MessageControlID(originalMessage)
	now := time.Now
	if a.Now != nil {
		now = a.Now
	}
	sendingApp := a.Config.SendingApp
	if sendingApp == "" {
		sendingApp = "ezHealthKonnect"
	}
	sendingFacility := a.Config.SendingFacility
	if sendingFacility == "" {
		sendingFacility = "EHK"
	}
	ack := fmt.Sprintf("MSH|^~\\&|%s|%s|SENDER|SENDER|%s||ACK|%s|P|2.5\r\n",
		sendingApp, sendingFacility, now().Format("20060102150405"), controlID)
	ack += fmt.Sprintf("MSA|%s|%s|%s\r\n", ackCode, controlID, textMessage)
	return ack
}

// RunScript executes the configured `function buildACK(msg)` and returns its
// {ackCode, textMessage}. msg carries raw, controlID, messageType,
// sendingApp, sendingFacility, defaultCode and defaultText.
func (a *HL7Acknowledger) RunScript(originalMessage, defaultCode, defaultText string) (string, string, error) {
	vm := goja.New()
	msgObj := map[string]interface{}{
		"raw":             originalMessage,
		"controlID":       HL7MessageControlID(originalMessage),
		"messageType":     HL7MessageType(originalMessage),
		"sendingApp":      HL7MSHField(originalMessage, 2),
		"sendingFacility": HL7MSHField(originalMessage, 3),
		"defaultCode":     defaultCode,
		"defaultText":     defaultText,
	}
	if err := vm.Set("msg", msgObj); err != nil {
		return defaultCode, defaultText, err
	}
	if _, err := vm.RunString(a.Config.Script); err != nil {
		return defaultCode, defaultText, fmt.Errorf("script parse error: %w", err)
	}
	buildACK, ok := goja.AssertFunction(vm.Get("buildACK"))
	if !ok {
		return defaultCode, defaultText, errors.New("script must define function buildACK(msg)")
	}
	result, err := buildACK(goja.Undefined(), vm.ToValue(msgObj))
	if err != nil {
		return defaultCode, defaultText, fmt.Errorf("script execution error: %w", err)
	}
	obj := result.ToObject(vm)
	if obj == nil {
		return defaultCode, defaultText, errors.New("buildACK must return an object")
	}
	ackCode, textMessage := defaultCode, defaultText
	if v := obj.Get("ackCode"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		ackCode = v.String()
	}
	if v := obj.Get("textMessage"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		textMessage = v.String()
	}
	return ackCode, textMessage, nil
}

// Reply resolves (script override included) and builds the ACK in one step.
// The error, when non-nil, is a script failure the caller may log; the
// returned ACK is always usable.
func (a *HL7Acknowledger) Reply(originalMessage, defaultCode, defaultText string) (ack string, ackCode string, scriptErr error) {
	code, text, err := a.Resolve(originalMessage, defaultCode, defaultText)
	return a.Build(originalMessage, code, text), code, err
}
