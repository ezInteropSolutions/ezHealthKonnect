package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Reporter receives installer progress. The web UI streams it over SSE; the
// headless command line prints it.
type Reporter interface {
	Step(n int, title string)
	Info(format string, args ...any)
	OK(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
}

// event is one progress message as sent to the web UI.
type event struct {
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

// Hub fans progress events out to every SSE subscriber and keeps a history,
// so a browser that connects late (or reconnects) still sees the whole log.
type Hub struct {
	mu      sync.Mutex
	history []event
	subs    map[chan event]bool
	closed  bool
}

// NewHub creates an empty Hub.
func NewHub() *Hub { return &Hub{subs: map[chan event]bool{}} }

func (h *Hub) publish(e event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.history = append(h.history, e)
	for ch := range h.subs {
		select {
		case ch <- e:
		default: // slow subscriber; it still gets the history on reconnect
		}
	}
}

// Subscribe returns the history so far plus a channel of new events; the
// channel is closed when the run finishes.
func (h *Hub) Subscribe() ([]event, chan event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan event, 256)
	past := append([]event(nil), h.history...)
	if h.closed {
		close(ch)
		return past, ch
	}
	h.subs[ch] = true
	return past, ch
}

// Unsubscribe removes a subscriber.
func (h *Hub) Unsubscribe(ch chan event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[ch] {
		delete(h.subs, ch)
		close(ch)
	}
}

// Finish publishes the final event ("done" or "failed") and closes subscribers.
func (h *Hub) Finish(level, msg string) {
	h.publish(event{Level: level, Msg: msg})
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for ch := range h.subs {
		close(ch)
		delete(h.subs, ch)
	}
}

// Reset clears the history for a new run.
func (h *Hub) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.history = nil
	h.closed = false
}

// HubReporter adapts a Hub to the Reporter interface and also mirrors to a
// console writer (the installer's own terminal / service log).
type HubReporter struct {
	Hub     *Hub
	Console io.Writer
}

func (r HubReporter) emit(level, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	r.Hub.publish(event{Level: level, Msg: msg})
	if r.Console != nil {
		fmt.Fprintf(r.Console, "[%s] %s\n", strings.ToUpper(level), msg)
	}
}

func (r HubReporter) Step(n int, title string) { r.emit("step", "Step %d: %s", n, title) }
func (r HubReporter) Info(f string, a ...any)  { r.emit("info", f, a...) }
func (r HubReporter) OK(f string, a ...any)    { r.emit("ok", f, a...) }
func (r HubReporter) Warn(f string, a ...any)  { r.emit("warn", f, a...) }
func (r HubReporter) Error(f string, a ...any) { r.emit("error", f, a...) }

// ConsoleReporter prints progress for headless runs.
type ConsoleReporter struct{ Out io.Writer }

func (r ConsoleReporter) Step(n int, title string) {
	fmt.Fprintf(r.Out, "\n== Step %d: %s\n", n, title)
}
func (r ConsoleReporter) Info(f string, a ...any) {
	fmt.Fprintf(r.Out, "   %s\n", fmt.Sprintf(f, a...))
}
func (r ConsoleReporter) OK(f string, a ...any) {
	fmt.Fprintf(r.Out, " ✔ %s\n", fmt.Sprintf(f, a...))
}
func (r ConsoleReporter) Warn(f string, a ...any) {
	fmt.Fprintf(r.Out, " ! %s\n", fmt.Sprintf(f, a...))
}
func (r ConsoleReporter) Error(f string, a ...any) {
	fmt.Fprintf(r.Out, " ✖ %s\n", fmt.Sprintf(f, a...))
}

// marshalEvent renders an event for SSE.
func marshalEvent(e event) string {
	b, _ := json.Marshal(e)
	return string(b)
}
