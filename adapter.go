// Package adk provides an adapter for Google ADK (Agent Development Kit)
// to work with the AG-UI protocol.
//
// This package converts ADK session.Event to AG-UI events and provides
// an HTTP handler that wraps ADK agents.
package aguigo

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// ADKConverter converts ADK session.Event to AG-UI events
type ADKConverter struct {
	*BaseConverter
}

// NewADKConverter creates a new ADK-specific converter
func NewADKConverter(threadID, runID string, opts ...Option) *ADKConverter {
	return &ADKConverter{
		BaseConverter: NewBaseConverter(threadID, runID, opts...),
	}
}

// ConvertEvent converts an ADK session.Event to AG-UI events
func (c *ADKConverter) ConvertEvent(adkEvent *session.Event) []Event {
	var events []Event

	// Handle content (text responses, function calls, function responses)
	if adkEvent.Content != nil && len(adkEvent.Content.Parts) > 0 {
		for _, part := range adkEvent.Content.Parts {
			// Handle thinking/reasoning (Thought flag on text parts)
			if part.Thought && part.Text != "" {
				thoughtEvents := c.handleThought(adkEvent, part.Text)
				events = append(events, thoughtEvents...)
				continue // Don't process as regular text
			}

			// Handle text content
			if part.Text != "" {
				textEvents := c.handleTextPart(adkEvent, part.Text)
				events = append(events, textEvents...)
			}

			// Handle function calls (tool invocations)
			if part.FunctionCall != nil {
				toolEvents := c.handleFunctionCall(part.FunctionCall)
				events = append(events, toolEvents...)
			}

			// Handle function responses (tool results)
			if part.FunctionResponse != nil {
				toolEvents := c.handleFunctionResponse(part.FunctionResponse)
				events = append(events, toolEvents...)
			}
		}
	}

	// Handle state changes via actions
	stateEvents := c.handleActions(&adkEvent.Actions)
	events = append(events, stateEvents...)

	return events
}

// handleThought processes thinking/reasoning content from ADK events
func (c *ADKConverter) handleThought(adkEvent *session.Event, thought string) []Event {
	var events []Event

	opts := c.GetOptions()
	if !opts.EmitStepEvents {
		// If step events disabled, emit as custom event instead
		events = append(events, c.CreateCustomEvent("thinking", map[string]any{
			"content": thought,
			"author":  adkEvent.Author,
		}))
		return events
	}

	// Create a step for the thinking process
	stepID := uuid.New().String()
	stepName := "thinking"
	if adkEvent.Author != "" {
		stepName = fmt.Sprintf("%s_thinking", adkEvent.Author)
	}

	// Emit step started
	events = append(events, c.CreateStepEvent(stepName, stepID, true))

	// Emit activity delta if enabled
	if opts.EmitActivityEvents {
		events = append(events, c.CreateActivityEvent(Activity{
			ID:          stepID,
			Type:        "thinking",
			Status:      "running",
			Description: thought,
		}))
	}

	// Emit step finished
	events = append(events, c.CreateStepEvent(stepName, stepID, false))

	return events
}

// handleTextPart processes text content from ADK events
func (c *ADKConverter) handleTextPart(adkEvent *session.Event, text string) []Event {
	var events []Event

	// Start a new message if needed
	if !c.IsMessageStarted() {
		role := "assistant"
		if adkEvent.Author == "user" {
			role = "user"
		}
		events = append(events, c.StartMessage(role)...)
	}

	// Add content chunk
	events = append(events, c.AddMessageContent(text))

	return events
}

// handleFunctionCall processes function call requests from ADK
func (c *ADKConverter) handleFunctionCall(fc *genai.FunctionCall) []Event {
	var events []Event

	toolCallID := fc.ID
	if toolCallID == "" {
		toolCallID = uuid.New().String()
	}

	// Start tool call (this also closes any open message)
	events = append(events, c.StartToolCall(fc.Name, toolCallID)...)

	// Emit TOOL_CALL_ARGS with the arguments
	if fc.Args != nil {
		argsJSON, err := json.Marshal(fc.Args)
		if err == nil && len(argsJSON) > 0 {
			events = append(events, c.AddToolCallArgs(toolCallID, string(argsJSON)))
		}
	}

	// Emit TOOL_CALL_END
	events = append(events, c.EndToolCall(toolCallID))

	return events
}

// handleFunctionResponse processes function/tool responses from ADK
func (c *ADKConverter) handleFunctionResponse(fr *genai.FunctionResponse) []Event {
	var events []Event

	toolCallID := fr.ID
	if toolCallID == "" {
		toolCallID = uuid.New().String()
	}

	// Serialize the response
	var content string
	if fr.Response != nil {
		responseJSON, err := json.Marshal(fr.Response)
		if err == nil {
			content = string(responseJSON)
		} else {
			content = fmt.Sprintf("%v", fr.Response)
		}
	}

	events = append(events, c.AddToolCallResult(toolCallID, content))

	return events
}

// handleActions processes ADK action signals (state changes, transfers, etc.)
func (c *ADKConverter) handleActions(actions *session.EventActions) []Event {
	var events []Event

	// Handle state delta
	if len(actions.StateDelta) > 0 {
		events = append(events, c.CreateStateDeltaEvent(actions.StateDelta))
	}

	// Handle artifact delta as a custom event
	if len(actions.ArtifactDelta) > 0 {
		events = append(events, c.CreateCustomEvent("artifact_delta", actions.ArtifactDelta))
	}

	// Handle agent transfer as a custom event
	if actions.TransferToAgent != "" {
		events = append(events, c.CreateCustomEvent("agent_transfer", map[string]string{
			"targetAgent": actions.TransferToAgent,
		}))
	}

	// Handle escalation as a custom event
	if actions.Escalate {
		events = append(events, c.CreateCustomEvent("escalation", map[string]any{
			"escalate": true,
		}))
	}

	return events
}

// ADKHandler handles AG-UI protocol requests for ADK agents
type ADKHandler struct {
	runner          *runner.Runner
	sessionService  session.Service
	appName         string
	converterOpts   []Option
}

// NewADKHandler creates a new AG-UI handler for an ADK agent.
// Optional converter options can be passed to configure event emission behavior:
//   - WithStepEvents(true) - Emit STEP_STARTED/STEP_FINISHED events for thinking/reasoning
//   - WithActivityEvents(true) - Emit ACTIVITY_DELTA events for progress tracking
//   - WithRawEvents(true) - Include original events in output
func NewADKHandler(ag agent.Agent, sessionService session.Service, appName string, opts ...Option) (*ADKHandler, error) {
	if appName == "" {
		appName = "adk-agent"
	}

	r, err := runner.New(runner.Config{
		AppName:        appName,
		Agent:          ag,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create runner: %w", err)
	}

	return &ADKHandler{
		runner:          r,
		sessionService:  sessionService,
		appName:         appName,
		converterOpts:   opts,
	}, nil
}

// ensureSession creates a session if it doesn't exist
func (h *ADKHandler) ensureSession(ctx context.Context, userID, sessionID string) error {
	// Try to get the session first
	_, err := h.sessionService.Get(ctx, &session.GetRequest{
		AppName:   h.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err == nil {
		// Session exists
		return nil
	}

	// Session doesn't exist, create it
	_, err = h.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   h.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

// ServeHTTP handles AG-UI protocol requests
func (h *ADKHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[AG-UI] Received %s request from %s", r.Method, r.RemoteAddr)

	if r.Method == http.MethodOptions {
		h.handleCORS(w)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse input
	var input RunAgentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Ensure IDs exist
	if input.ThreadID == "" {
		input.ThreadID = uuid.New().String()
	}
	if input.RunID == "" {
		input.RunID = uuid.New().String()
	}

	// Determine encoding based on Accept header
	accept := r.Header.Get("Accept")
	contentType := ParseAcceptHeader(accept)

	// Handle streaming vs non-streaming
	if contentType == "text/event-stream" {
		h.handleSSE(w, r.Context(), input)
	} else {
		h.handleJSON(w, r.Context(), input)
	}
}

func (h *ADKHandler) handleCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-control-allow-methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Authorization")
	w.WriteHeader(http.StatusOK)
}

// handleSSE handles Server-Sent Events streaming
func (h *ADKHandler) handleSSE(w http.ResponseWriter, ctx context.Context, input RunAgentInput) {
	SetSSEHeaders(w)
	enc := NewSSE(w)
	conv := NewADKConverter(input.ThreadID, input.RunID, h.converterOpts...)

	// Send RUN_STARTED
	if err := enc.Encode(conv.StartRun()); err != nil {
		return
	}

	// Convert AG-UI messages to ADK content
	adkContent := convertMessagesToADKContent(input.Messages)

	// Use threadID as sessionID for simplicity
	userID := "default-user"
	sessionID := input.ThreadID

	// Ensure session exists before running agent
	if err := h.ensureSession(ctx, userID, sessionID); err != nil {
		enc.EncodeError(input.ThreadID, input.RunID, err)
		return
	}

	// Track if an error occurred
	errorOccurred := false

	// Run the agent and stream events
	for adkEvent, err := range h.runner.Run(ctx, userID, sessionID, adkContent, agent.RunConfig{}) {
		if err != nil {
			enc.EncodeError(input.ThreadID, input.RunID, err)
			errorOccurred = true
			break
		}

		// Convert ADK event to AG-UI events
		aguiEvents := conv.ConvertEvent(adkEvent)

		// Send each AG-UI event
		for _, aguiEvent := range aguiEvents {
			if err := enc.Encode(aguiEvent); err != nil {
				return
			}
		}
	}

	// Only send RUN_FINISHED if no error occurred
	if !errorOccurred {
		enc.EncodeMultiple(conv.FinishRun())
	}
}

// handleJSON handles non-streaming JSON responses
func (h *ADKHandler) handleJSON(w http.ResponseWriter, ctx context.Context, input RunAgentInput) {
	conv := NewADKConverter(input.ThreadID, input.RunID, h.converterOpts...)
	var allEvents []Event

	// Add RUN_STARTED
	allEvents = append(allEvents, conv.StartRun())

	// Convert messages and run
	adkContent := convertMessagesToADKContent(input.Messages)

	userID := "default-user"
	sessionID := input.ThreadID

	// Ensure session exists before running agent
	if err := h.ensureSession(ctx, userID, sessionID); err != nil {
		allEvents = append(allEvents, RunError{
			Base:     NewBase(TypeRunError),
			ThreadID: input.ThreadID,
			RunID:    input.RunID,
			Message:  fmt.Sprintf("failed to ensure session: %v", err),
		})
		data, _ := MarshalEvents(allEvents)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
		return
	}

	// Track if an error occurred
	errorOccurred := false

	for adkEvent, err := range h.runner.Run(ctx, userID, sessionID, adkContent, agent.RunConfig{}) {
		if err != nil {
			allEvents = append(allEvents, RunError{
				Base:     NewBase(TypeRunError),
				ThreadID: input.ThreadID,
				RunID:    input.RunID,
				Message:  err.Error(),
			})
			errorOccurred = true
			break
		}

		aguiEvents := conv.ConvertEvent(adkEvent)
		allEvents = append(allEvents, aguiEvents...)
	}

	// Only add RUN_FINISHED if no error occurred
	if !errorOccurred {
		allEvents = append(allEvents, conv.FinishRun()...)
	}

	// Send all events as JSON array
	data, err := MarshalEvents(allEvents)
	if err != nil {
		http.Error(w, "Failed to marshal events", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// convertMessagesToADKContent converts AG-UI messages to ADK content format
func convertMessagesToADKContent(messages []Message) *genai.Content {
	if len(messages) == 0 {
		return nil
	}

	// Get the last user message
	var lastUserMessage *Message
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUserMessage = &messages[i]
			break
		}
	}

	if lastUserMessage == nil {
		return nil
	}

	// Convert to ADK content
	var parts []*genai.Part
	for _, content := range lastUserMessage.Content {
		if content.Type == "text" && content.Text != "" {
			parts = append(parts, &genai.Part{
				Text: content.Text,
			})
		}
	}

	if len(parts) == 0 {
		return nil
	}

	return &genai.Content{
		Role:  genai.RoleUser,
		Parts: parts,
	}
}
