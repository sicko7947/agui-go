// Package encoder provides utilities for encoding AG-UI events
// in various formats including SSE (Server-Sent Events) and JSON.
package aguigo

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Encoder is the interface for encoding AG-UI events
type Encoder interface {
	// Encode writes a single event
	Encode(e Event) error

	// EncodeMultiple writes multiple events
	EncodeMultiple(events []Event) error

	// Flush ensures all buffered data is written
	Flush() error
}

// SSE encodes AG-UI events as Server-Sent Events
type SSE struct {
	writer  io.Writer
	flusher http.Flusher
}

// NewSSE creates a new SSE encoder for the given response writer
func NewSSE(w http.ResponseWriter) *SSE {
	flusher, _ := w.(http.Flusher)
	return &SSE{
		writer:  w,
		flusher: flusher,
	}
}

// SetSSEHeaders sets the required headers for SSE streaming
func SetSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
}

// Encode writes an AG-UI event as an SSE message
func (e *SSE) Encode(evt Event) error {
	data, err := evt.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Write SSE format: "data: {json}\n\n"
	_, err = fmt.Fprintf(e.writer, "data: %s\n\n", data)
	if err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	// Flush immediately for real-time streaming
	if e.flusher != nil {
		e.flusher.Flush()
	}

	return nil
}

// EncodeMultiple writes multiple AG-UI events
func (e *SSE) EncodeMultiple(events []Event) error {
	for _, evt := range events {
		if err := e.Encode(evt); err != nil {
			return err
		}
	}
	return nil
}

// Flush ensures all buffered data is written
func (e *SSE) Flush() error {
	if e.flusher != nil {
		e.flusher.Flush()
	}
	return nil
}

// EncodeError writes an error event
func (e *SSE) EncodeError(threadID, runID string, err error) error {
	errorEvent := RunError{
		Base:     NewBase(TypeRunError),
		ThreadID: threadID,
		RunID:    runID,
		Message:  err.Error(),
	}
	return e.Encode(errorEvent)
}

// JSON encodes AG-UI events as newline-delimited JSON (NDJSON)
type JSON struct {
	writer  io.Writer
	flusher http.Flusher
}

// NewJSON creates a new NDJSON encoder
func NewJSON(w http.ResponseWriter) *JSON {
	flusher, _ := w.(http.Flusher)
	return &JSON{
		writer:  w,
		flusher: flusher,
	}
}

// SetJSONStreamHeaders sets headers for NDJSON streaming
func SetJSONStreamHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no")
}

// Encode writes an AG-UI event as a JSON line
func (e *JSON) Encode(evt Event) error {
	data, err := evt.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	_, err = fmt.Fprintf(e.writer, "%s\n", data)
	if err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	if e.flusher != nil {
		e.flusher.Flush()
	}

	return nil
}

// EncodeMultiple writes multiple AG-UI events
func (e *JSON) EncodeMultiple(events []Event) error {
	for _, evt := range events {
		if err := e.Encode(evt); err != nil {
			return err
		}
	}
	return nil
}

// Flush ensures all buffered data is written
func (e *JSON) Flush() error {
	if e.flusher != nil {
		e.flusher.Flush()
	}
	return nil
}

// Ensure encoders implement the interface
var (
	_ Encoder = (*SSE)(nil)
	_ Encoder = (*JSON)(nil)
)

// ParseAcceptHeader determines the preferred encoding from Accept header
func ParseAcceptHeader(accept string) string {
	// Default to SSE for AG-UI compatibility
	if accept == "" {
		return "text/event-stream"
	}

	// Check for specific preferences
	switch {
	case strings.Contains(accept, "text/event-stream"):
		return "text/event-stream"
	case strings.Contains(accept, "application/x-ndjson"):
		return "application/x-ndjson"
	case strings.Contains(accept, "application/json"):
		return "application/json"
	default:
		return "text/event-stream"
	}
}

// MarshalEvents converts multiple events to a JSON array (for non-streaming responses)
func MarshalEvents(events []Event) ([]byte, error) {
	var jsonEvents []json.RawMessage
	for _, evt := range events {
		data, err := evt.ToJSON()
		if err != nil {
			return nil, err
		}
		jsonEvents = append(jsonEvents, data)
	}
	return json.Marshal(jsonEvents)
}