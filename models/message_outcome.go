// models/message_outcome.go
//
// MessageOutcome is the final result of one inbound message's trip through
// the processing engine: rejected before storage, failed somewhere between
// storage and the end of its pipeline, or processed. The engine reports it
// exactly once per stored-or-rejected message to the inbound connector that
// produced the message, when that connector opts in (see
// connectors.OutcomeAwareConnector). A connector that must not acknowledge
// its sender until the work is really done — for example a webhook receiver
// whose sender delivers strictly in order and waits for the reply — uses it
// to hold its reply.
package models

// Message outcome statuses.
const (
	// OutcomeProcessed: the pipeline ran to completion.
	OutcomeProcessed = "processed"
	// OutcomeFailed: the message was accepted but storage, parsing or a
	// pipeline step failed.
	OutcomeFailed = "failed"
	// OutcomeRejected: the message was refused before processing (message
	// family filter, full queue).
	OutcomeRejected = "rejected"
	// OutcomeSkipped: no pipeline is configured for the message's type, so
	// nothing ran.
	OutcomeSkipped = "skipped"
)

// MessageOutcome reports what finally happened to one inbound message.
type MessageOutcome struct {
	InterfaceID string
	MessageID   string
	Status      string // one of the Outcome* constants
	Error       string // empty unless Status is failed or rejected
}

// Succeeded reports whether the message was fully processed.
func (o MessageOutcome) Succeeded() bool { return o.Status == OutcomeProcessed }
