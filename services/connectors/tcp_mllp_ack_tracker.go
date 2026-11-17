// services/connectors/tcp_mllp_ack_tracker.go
//
// Deferred MLLP acknowledgements.
//
// In ack mode "immediate" the MLLP listener answers AA as soon as a message is
// queued. In "after_store" and "after_processing" it holds the reply until the
// engine reports what happened, so a sender that checks the ACK learns about a
// rejected or failed message and can send it again:
//
//	after_store       AA once the message is durably stored; AE when the engine
//	                  rejects it (family filter, full queue) or cannot store it.
//	after_processing  AA once the pipeline finished (processed, or skipped
//	                  because none is configured); AE when it failed or the
//	                  message was rejected.
//
// The engine reports through two existing channels — ValidationFeedback
// (accepted / rejected, after the store) and MessageOutcome (the end of the
// message's trip) — once per message, or once per "-partN" when it splits a
// frame that holds several MSH segments. The tracker turns those signals into
// one verdict per frame. The connection's goroutine waits for it, so ACKs stay
// in the order the messages arrived.
package connectors

import (
	"regexp"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/hl7"
	"ezhealthkonnect/models"
)

// MLLP acknowledgement modes (ack.mode).
const (
	// ACKModeImmediate answers AA as soon as the message is queued, before it
	// is stored. A rejection or store failure after that cannot reach the sender.
	ACKModeImmediate = "immediate"
	// ACKModeAfterStore answers once the message is durably stored. The default.
	ACKModeAfterStore = "after_store"
	// ACKModeAfterProcessing answers once the message's pipeline has finished.
	ACKModeAfterProcessing = "after_processing"

	// defaultACKWait bounds how long a deferred ACK waits for the engine.
	defaultACKWait = 30 * time.Second
	minACKWait     = 1 * time.Second
	maxACKWait     = 10 * time.Minute
)

// normalizeMLLPACKMode maps a configured ack.mode to a known mode. An empty or
// unknown value (including the retired "none") becomes the default, after_store.
func normalizeMLLPACKMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case ACKModeImmediate:
		return ACKModeImmediate
	case ACKModeAfterProcessing:
		return ACKModeAfterProcessing
	default:
		return ACKModeAfterStore
	}
}

// clampACKWait keeps a configured wait inside sane bounds; zero means default.
func clampACKWait(seconds int) time.Duration {
	if seconds <= 0 {
		return defaultACKWait
	}
	d := time.Duration(seconds) * time.Second
	if d < minACKWait {
		return minACKWait
	}
	if d > maxACKWait {
		return maxACKWait
	}
	return d
}

// isDeferred reports whether the mode holds the reply for the engine.
func (a ACKConfig) isDeferred() bool {
	return a.Mode == ACKModeAfterStore || a.Mode == ACKModeAfterProcessing
}

// ackVerdict is the single result of a frame's acknowledgement.
type ackVerdict struct {
	OK     bool
	Reason string // for the log only; never sent to the sender
}

// pendingAck is one frame waiting for its verdict.
type pendingAck struct {
	mode     string
	expected int             // messages the engine will report on (parts)
	settled  map[string]bool // message IDs already counted
	done     chan ackVerdict // capacity 1; written once
	finished bool
}

// ackTracker holds the frames waiting for a verdict, keyed by the ID the
// connector gave the frame.
type ackTracker struct {
	mu      sync.Mutex
	pending map[string]*pendingAck
}

func newACKTracker() *ackTracker {
	return &ackTracker{pending: make(map[string]*pendingAck)}
}

// register starts tracking a frame. It must be called BEFORE the frame is
// handed to the engine so a fast signal cannot arrive before anyone waits.
func (t *ackTracker) register(id, mode string, expected int) *pendingAck {
	if expected < 1 {
		expected = 1
	}
	p := &pendingAck{
		mode:     mode,
		expected: expected,
		settled:  make(map[string]bool, expected),
		done:     make(chan ackVerdict, 1),
	}
	t.mu.Lock()
	t.pending[id] = p
	t.mu.Unlock()
	return p
}

// cancel stops tracking a frame (timeout, queue full, shutdown).
func (t *ackTracker) cancel(id string) {
	t.mu.Lock()
	delete(t.pending, id)
	t.mu.Unlock()
}

// size reports how many frames are waiting (tests, diagnostics).
func (t *ackTracker) size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.pending)
}

var partSuffix = regexp.MustCompile(`-part\d+$`)

// frameID returns the ID of the frame a message belongs to: the engine names
// the parts of a split frame "<frame id>-partN".
func frameID(messageID string) string {
	return partSuffix.ReplaceAllString(messageID, "")
}

// onFeedback receives the engine's accepted/rejected feedback for a message.
// after_processing ignores it: acceptance is not the end of the message.
func (t *ackTracker) onFeedback(messageID string, accepted bool, reason string) {
	t.settle(messageID, ACKModeAfterStore, accepted, reason)
}

// onOutcome receives the end-of-processing outcome for a message.
func (t *ackTracker) onOutcome(o models.MessageOutcome) {
	ok := o.Status == models.OutcomeProcessed || o.Status == models.OutcomeSkipped
	reason := o.Status
	if o.Error != "" {
		reason = o.Status + ": " + o.Error
	}
	t.settle(o.MessageID, ACKModeAfterProcessing, ok, reason)
}

// settle counts one message of a frame. source says which signal this is;
// after_store accepts both (a store failure only ever produces an outcome),
// after_processing accepts the outcome alone. A failure settles the frame at
// once; success waits for every part.
func (t *ackTracker) settle(messageID, source string, ok bool, reason string) {
	id := frameID(messageID)
	t.mu.Lock()
	p, found := t.pending[id]
	if !found || p.finished {
		t.mu.Unlock()
		return
	}
	if p.mode == ACKModeAfterProcessing && source == ACKModeAfterStore {
		t.mu.Unlock()
		return
	}
	// An outcome after after_store's own feedback is the same message again.
	if p.settled[messageID] {
		t.mu.Unlock()
		return
	}
	if !ok {
		p.finished = true
		delete(t.pending, id)
		t.mu.Unlock()
		p.done <- ackVerdict{OK: false, Reason: reason}
		return
	}
	p.settled[messageID] = true
	if len(p.settled) < p.expected {
		t.mu.Unlock()
		return
	}
	p.finished = true
	delete(t.pending, id)
	t.mu.Unlock()
	p.done <- ackVerdict{OK: true}
}

// ExpectedHL7Parts returns how many messages the engine will report on for a
// frame: the engine splits a frame with several MSH segments into one message
// per segment. It mirrors the engine's own split decision (processing
// splitBatchPayload, HL7 branch); a test keeps the two in step.
func ExpectedHL7Parts(frame string) int {
	trimmed := strings.TrimSpace(frame)
	hl7Content := strings.HasPrefix(trimmed, "MSH|") ||
		strings.Contains(trimmed, "\rMSH|") ||
		strings.Contains(trimmed, "\nMSH|") ||
		strings.HasPrefix(trimmed, "BHS|") ||
		strings.HasPrefix(trimmed, "FHS|")
	if !hl7Content || hl7.CountMessages(trimmed) <= 1 {
		return 1
	}
	if n := len(hl7.SplitMessages(trimmed)); n > 1 {
		return n
	}
	return 1
}

// closeAll ends every wait without a verdict (the connector is stopping). The
// waiting goroutines see their channel closed and give up on the connection.
func (t *ackTracker) closeAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, p := range t.pending {
		if !p.finished {
			p.finished = true
			close(p.done)
		}
		delete(t.pending, id)
	}
}
