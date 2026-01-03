// Package event defines all AG-UI protocol event types.
package aguigo

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// EventType represents all possible AG-UI event types
type EventType string

const (
	// Lifecycle events
	TypeRunStarted  EventType = "RUN_STARTED"
	TypeRunFinished EventType = "RUN_FINISHED"
	TypeRunError    EventType = "RUN_ERROR"

	// Step events
	TypeStepStarted  EventType = "STEP_STARTED"
	TypeStepFinished EventType = "STEP_FINISHED"

	// Text message events
	TypeTextMessageStart   EventType = "TEXT_MESSAGE_START"
	TypeTextMessageContent EventType = "TEXT_MESSAGE_CONTENT"
	TypeTextMessageEnd     EventType = "TEXT_MESSAGE_END"

	// Tool call events
	TypeToolCallStart  EventType = "TOOL_CALL_START"
	TypeToolCallArgs   EventType = "TOOL_CALL_ARGS"
	TypeToolCallEnd    EventType = "TOOL_CALL_END"
	TypeToolCallResult EventType = "TOOL_CALL_RESULT"

	// State management events
	TypeStateSnapshot    EventType = "STATE_SNAPSHOT"
	TypeStateDelta       EventType = "STATE_DELTA"
	TypeMessagesSnapshot EventType = "MESSAGES_SNAPSHOT"

	// Activity events
	TypeActivitySnapshot EventType = "ACTIVITY_SNAPSHOT"
	TypeActivityDelta    EventType = "ACTIVITY_DELTA"

	// Special events
	TypeRaw    EventType = "RAW"
	TypeCustom EventType = "CUSTOM"
)

// Event is the interface that all AG-UI events must implement
type Event interface {
	// GetType returns the event type
	GetType() EventType

	// GetTimestamp returns the event timestamp in milliseconds
	GetTimestamp() int64

	// ToJSON serializes the event to JSON
	ToJSON() ([]byte, error)
}

// Base contains common fields for all AG-UI events
type Base struct {
	Type      EventType `json:"type"`
	Timestamp int64     `json:"timestamp,omitempty"`
	RawEvent  any       `json:"rawEvent,omitempty"`
}

// NewBase creates a new Base with the current timestamp
func NewBase(eventType EventType) Base {
	return Base{
		Type:      eventType,
		Timestamp: time.Now().UnixMilli(),
	}
}

// GetType returns the event type
func (e Base) GetType() EventType {
	return e.Type
}

// GetTimestamp returns the event timestamp
func (e Base) GetTimestamp() int64 {
	return e.Timestamp
}

// RunStarted signals the start of an agent run
type RunStarted struct {
	Base
	ThreadID    string `json:"threadId"`
	RunID       string `json:"runId"`
	ParentRunID string `json:"parentRunId,omitempty"`
}

// ToJSON serializes the event to JSON
func (e RunStarted) ToJSON() ([]byte, error) { return json.Marshal(e) }

// RunFinished signals the completion of an agent run
type RunFinished struct {
	Base
	ThreadID string `json:"threadId"`
	RunID    string `json:"runId"`
}

// ToJSON serializes the event to JSON
func (e RunFinished) ToJSON() ([]byte, error) { return json.Marshal(e) }

// RunError signals an error during agent execution
type RunError struct {
	Base
	ThreadID string `json:"threadId,omitempty"`
	RunID    string `json:"runId,omitempty"`
	Message  string `json:"message"`
	Code     string `json:"code,omitempty"`
}

// ToJSON serializes the event to JSON
func (e RunError) ToJSON() ([]byte, error) { return json.Marshal(e) }

// StepStarted signals the start of a processing step
type StepStarted struct {
	Base
	StepName string `json:"stepName"`
	StepID   string `json:"stepId"`
}

// ToJSON serializes the event to JSON
func (e StepStarted) ToJSON() ([]byte, error) { return json.Marshal(e) }

// StepFinished signals the completion of a processing step
type StepFinished struct {
	Base
	StepID string `json:"stepId"`
}

// ToJSON serializes the event to JSON
func (e StepFinished) ToJSON() ([]byte, error) { return json.Marshal(e) }

// TextMessageStart signals the start of a text message
type TextMessageStart struct {
	Base
	MessageID string `json:"messageId"`
	Role      string `json:"role"` // "assistant" or "user"
}

// ToJSON serializes the event to JSON
func (e TextMessageStart) ToJSON() ([]byte, error) { return json.Marshal(e) } // TextMessageContent contains a chunk of text content

// TextMessageContent contains a chunk of text content
type TextMessageContent struct {
	Base
	MessageID string `json:"messageId"`
	Delta     string `json:"delta"` // Must not be empty
}

// ToJSON serializes the event to JSON
func (e TextMessageContent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// TextMessageEnd signals the end of a text message
type TextMessageEnd struct {
	Base
	MessageID string `json:"messageId"`
}

// ToJSON serializes the event to JSON
func (e TextMessageEnd) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ToolCallStart signale start of a tool call
type ToolCallStart struct {
	Base
	ToolCallID      string `json:"toolCallId"`
	ToolCallName    string `json:"toolCallName"`
	ParentMessageID string `json:"parentMessageId,omitempty"`
}

// ToJSON serializes the event to JSON
func (e ToolCallStart) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ToolCallArgs contains a chunk of tool call arguments
type ToolCallArgs struct {
	Base
	ToolCallID string `json:"toolCallId"`
	Delta      string `json:"delta"` // JSON fragment to append
}

// ToJSON serializes the event to JSON
func (e ToolCallArgs) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ToolCallEnd signals the end of a tool call
type ToolCallEnd struct {
	Base
	ToolCallID string `json:"toolCallId"`
}

// ToJSON serializes the event to JSON
func (e ToolCallEnd) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ToolCallResult contains the result of a tool execution
type ToolCallResult struct {
	Base
	MessageID  string `json:"messageId"`
	ToolCallID string `json:"toolCallId"`
	Role       string `json:"role,omitempty"` // Usually "tool"
	Content    string `json:"content"`
}

// ToJSON he event to JSON
func (e ToolCallResult) ToJSON() ([]byte, error) { return json.Marshal(e) }

// StateSnapshot contains a full state snapshot
type StateSnapshot struct {
	Base
	State map[string]any `json:"state"`
}

// ToJSON serializes the event to JSON
func (e StateSnapshot) ToJSON() ([]byte, error) { return json.Marshal(e) }

// StateDelta contains incremental state updates
type StateDelta struct {
	Base
	Delta map[string]any `json:"delta"`
}

// ToJSON serializes the event to JSON
func (e StateDelta) ToJSON() ([]byte, error) { return json.Marshal(e) }

// MessagesSnapshot contains the current message history
type MessagesSnapshot struct {
	Base
	Messages []Message `json:"messages"`
}

// ToJSON serializes the event to JSON
func (e MessagesSnapshot) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ActivitySnapshot contains activity/progress information
type ActivitySnapshot struct {
	Base
	Activities []Activity `json:"activities"`
}

// ToJSON serializes the event to JSON
func (e ActivitySnapshot) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ActivityDelta contains incremental activity updates
type ActivityDelta struct {
	Base
	Activity Activity `json:"activity"`
}

// ToJSON serializes the event to JSON
func (e ActivityDelta) ToJSON() ([]byte, error) { return json.Marshal(e) }

// Raw passes through raw, untyped data
type Raw struct {
	Base
	Data any `json:"data"`
}

// ToJSON serializes the event to JSON
func (e Raw) ToJSON() ([]byte, error) { return json.Marshal(e) }

// Custom is for vendor-specific extensions
type Custom struct {
	Base
	Name string `json:"name"`
	Data any    `json:"data"`
}

// ToJSON serializes the event to JSON
func (e Custom) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ContentPart represents a part of message content
type ContentPart struct {
	Type     string `json:"type"` // "text", "image", etc.
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
	URL      string `json:"url,omitempty"`
	ID       string `json:"id,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// Content can be either a string or []ContentPart
// This matches the AG-UI protocol which allows both formats
type Content []ContentPart

// UnmarshalJSON handles both string and array formats for content
func (c *Content) UnmarshalJSON(data []byte) error {
	// Handle null/empty
	if len(data) == 0 || string(data) == "null" {
		*c = nil
		return nil
	}

	// Try to unmarshal as string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		// Convert string to ContentPart array
		*c = []ContentPart{{Type: "text", Text: str}}
		return nil
	}

	// Try to unmarshal as array of ContentPart
	var parts []ContentPart
	if err := json.Unmarshal(data, &parts); err != nil {
		return fmt.Errorf("content must be string or array of ContentPart: %w", err)
	}
	*c = parts
	return nil
}

// MarshalJSON always marshals as array format
func (c Content) MarshalJSON() ([]byte, error) {
	if c == nil {
		return []byte("null"), nil
	}
	return json.Marshal([]ContentPart(c))
}

// GetText returns the concatenated text content from all text parts
func (c Content) GetText() string {
	var texts []string
	for _, part := range c {
		if part.Type == "text" && part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "")
}

// GetTextParts returns only the text content parts
func (c Content) GetTextParts() []ContentPart {
	var parts []ContentPart
	for _, part := range c {
		if part.Type == "text" {
			parts = append(parts, part)
		}
	}
	return parts
}

// Message represents a chat message in the history
type Message struct {
	ID        string  `json:"id"`
	Role      string  `json:"role"`
	Content   Content `json:"content"`
	Name      string  `json:"name,omitempty"`
	CreatedAt int64   `json:"createdAt,omitempty"`
}

// Activity represents an agent activity/step
type Activity struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Status      string `json:"status"` // "pending", "running", "completed", "failed"
	Description string `json:"description,omitempty"`
	StartedAt   int64  `json:"startedAt,omitempty"`
	CompletedAt int64  `json:"completedAt,omitempty"`
}
