// Package handler provides HTTP handlers for the AG-UI protocol.
package aguigo

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/google/uuid"
)

// RunAgentInput represents the AG-UI protocol input format
type RunAgentInput struct {
	ThreadID string    `json:"threadId"`
	RunID    string    `json:"runId,omitempty"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
	Context  any       `json:"context,omitempty"`
	State    any       `json:"state,omitempty"`
}

// Tool represents a tool definition in AG-UI format
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters,omitempty"`
}

// EventSource is the interface that agent implementations must satisfy
// to work with the AG-UI handler.
type EventSource interface {
	// Run executes the agent and returns a channel of events.
	// The channel should be closed when the agent finishes.
	// If an error occurs, it should be sent as a RunError
	Run(ctx Context, input RunAgentInput) <-chan Event
}

// Context provides context for the agent run
type Context struct {
	// ThreadID is the conversation thread ID
	ThreadID string

	// RunID is the unique ID for this run
	RunID string

	// UserID is the user identifier (if available)
	UserID string

	// Request is the original HTTP request
	Request *http.Request
}

// Config configures the handler
type Config struct {
	// EventSource is the agent event source
	EventSource EventSource

	// AppName is the application name
	AppName string

	// Logger is an optional logger
	Logger Logger
}

// Logger interface for logging
type Logger interface {
	Printf(format string, v ...any)
}

// defaultLogger is a no-op logger
type defaultLogger struct{}

func (defaultLogger) Printf(format string, v ...any) {}

// Handler handles AG-UI protocol requests
type Handler struct {
	eventSource EventSource
	appName     string
	logger      Logger
}

// New creates a new AG-UI handler
func New(config Config) *Handler {
	logger := config.Logger
	if logger == nil {
		logger = defaultLogger{}
	}

	return &Handler{
		eventSource: config.EventSource,
		appName:     config.AppName,
		logger:      logger,
	}
}

// ServeHTTP handles AG-UI protocol requests
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.logger.Printf("[AG-UI] Received %s request from %s", r.Method, r.RemoteAddr)

	if r.Method == http.MethodOptions {
		h.logger.Printf("[AG-UI] Handling CORS preflight request")
		h.handleCORS(w)
		return
	}

	if r.Method != http.MethodPost {
		h.logger.Printf("[AG-UI] Method not allowed: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse input
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Printf("[AG-UI] Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	h.logger.Printf("[AG-UI] Request body: %s", string(body))

	var input RunAgentInput
	if err := json.Unmarshal(body, &input); err != nil {
		h.logger.Printf("[AG-UI] Invalid JSON: %v", err)
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Ensure IDs exist
	if input.ThreadID == "" {
		input.ThreadID = uuid.New().String()
		h.logger.Printf("[AG-UI] Generated new threadId: %s", input.ThreadID)
	}
	if input.RunID == "" {
		input.RunID = uuid.New().String()
		h.logger.Printf("[AG-UI] Generated new runId: %s", input.RunID)
	}

	h.logger.Printf("[AG-UI] Processing request - threadId: %s, runId: %s, messages: %d",
		input.ThreadID, input.RunID, len(input.Messages))

	// Determine encoding based on Accept header
	accept := r.Header.Get("Accept")
	contentType := ParseAcceptHeader(accept)
	h.logger.Printf("[AG-UI] Accept header: %s, using content-type: %s", accept, contentType)

	// Create context
	ctx := Context{
		ThreadID: input.ThreadID,
		RunID:    input.RunID,
		UserID:   r.Header.Get("X-User-ID"),
		Request:  r,
	}

	// Handle streaming vs non-streaming
	if contentType == "text/event-stream" {
		h.logger.Printf("[AG-UI] Starting SSE streaming response")
		h.handleSSE(w, ctx, input)
	} else {
		h.logger.Printf("[AG-UI] Starting JSON response")
		h.handleJSON(w, ctx, input)
	}
}

func (h *Handler) handleCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Authorization, X-User-ID")
	w.WriteHeader(http.StatusOK)
}

// handleSSE handles Server-Sent Events streaming
func (h *Handler) handleSSE(w http.ResponseWriter, ctx Context, input RunAgentInput) {
	SetSSEHeaders(w)
	enc := NewSSE(w)

	// Get events from the event source
	events := h.eventSource.Run(ctx, input)

	// Stream events
	for evt := range events {
		if err := enc.Encode(evt); err != nil {
			h.logger.Printf("[AG-UI] Failed to send event: %v (client disconnected?)", err)
			return
		}
	}

	h.logger.Printf("E response completed")
}

// handleJSON handles non-streaming JSON responses
func (h *Handler) handleJSON(w http.ResponseWriter, ctx Context, input RunAgentInput) {
	var allEvents []Event

	// Get events from the event source
	events := h.eventSource.Run(ctx, input)

	// Collect all events
	for evt := range events {
		allEvents = append(allEvents, evt)
	}

	h.logger.Printf("[AG-UI] Total events to send: %d", len(allEvents))

	// Send all events as JSON array
	data, err := MarshalEvents(allEvents)
	if err != nil {
		h.logger.Printf("[AG-UI] Failed to mars: %v", err)
		http.Error(w, "Failed to marshal events", http.StatusInternalServerError)
		return
	}

	h.logger.Printf("[AG-UI] Sending JSON response")

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// HealthHandler returns a simple health check endpoint
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "healthy",
		"protocol": "ag-ui",
		"version":  "1.0.0",
	})
}

// CORSMiddleware adds CORS headers to responses
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Authorization, X-User-ID")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// StdLogger wraps the standard log package
type StdLogger struct{}

// Printf implements Logger
func (StdLogger) Printf(format string, v ...any) {
	log.Printf(format, v...)
}
