// Package converter provides interfaces and base implementations for
// converting external event sources to AG-UI protocol events.
package aguigo

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// ConverterInterface transforms external events to AG-UI events
type ConverterInterface interface {
	// GetThreadID returns the current thread ID
	GetThreadID() string

	// GetRunID returns the current run ID
	GetRunID() string

	// StartRun generates the RUN_STARTED event
	StartRun() Event

	// FinishRun generates the RUN_FINISHED event(s)
	FinishRun() []Event

	// ErrorRun generates the RUN_ERROR event(s)
	ErrorRun(err error) []Event
}

// Options configures the converter behavior
type Options struct {
	// IncludeRawEvents includes the original event in AG-UI events
	IncludeRawEvents bool

	// EmitStepEvents emits STEP_STARTED/STEP_FINISHED for sub-agent tracking
	EmitStepEvents bool

	// EmitActivityEvents emits ACTIVITY_DELTA for progress tracking
	EmitActivityEvents bool
}

// Option is a functional option for configuring the converter
type Option func(*Options)

// WithRawEvents enables including raw events
func WithRawEvents(include bool) Option {
	return func(o *Options) {
		o.IncludeRawEvents = include
	}
}

// WithStepEvents enables step event emission
func WithStepEvents(emit bool) Option {
	return func(o *Options) {
		o.EmitStepEvents = emit
	}
}

// WithActivityEvents enables activity event emission
func WithActivityEvents(emit bool) Option {
	return func(o *Options) {
		o.EmitActivityEvents = emit
	}
}

// toolCallState tracks the state of an active tool call
type toolCallState struct {
	toolCallID string
	toolName   string
	argsBuffer string
	started    bool
	ended      bool
}

// stepState tracks the state of an active step
type stepState struct {
	stepID    string
	stepName  string
	startedAt time.Time
}

// BaseConverter provides a base implementation of the ConverterInterface
// that can be embedded in framework-specific converters.
type BaseConverter struct {
	mu sync.Mutex

	// Current run context
	threadID string
	runID    string

	// Message tracking
	currentMessageID string
	messageStarted   bool
	lastAuthor       string

	// Tool call tracking
	activeToolCalls map[string]*toolCallState

	// Step/Activity tracking for sub-agents
	activeSteps map[string]*stepState

	// Options
	options Options
}

// NewBaseConverter creates a new base converter
func NewBaseConverter(threadID, runID string, opts ...Option) *BaseConverter {
	if threadID == "" {
		threadID = uuid.New().String()
	}
	if runID == "" {
		runID = uuid.New().String()
	}

	options := Options{}
	for _, opt := range opts {
		opt(&options)
	}

	return &BaseConverter{
		threadID:        threadID,
		runID:           runID,
		activeToolCalls: make(map[string]*toolCallState),
		activeSteps:     make(map[string]*stepState),
		options:         options,
	}
}

// GetThreadID returns the current thread ID
func (c *BaseConverter) GetThreadID() string {
	return c.threadID
}

// GetRunID returns the current run ID
func (c *BaseConverter) GetRunID() string {
	return c.runID
}

// GetOptions returns the converter options
func (c *BaseConverter) GetOptions() Options {
	return c.options
}

// StartRun generates the RUN_STARTED event
func (c *BaseConverter) StartRun() Event {
	return RunStarted{
		Base:     NewBase(TypeRunStarted),
		ThreadID: c.threadID,
		RunID:    c.runID,
	}
}

// FinishRun generates the RUN_FINISHED event
func (c *BaseConverter) FinishRun() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()

	var events []Event

	// Close any open message
	if c.messageStarted {
		events = append(events, TextMessageEnd{
			Base:      NewBase(TypeTextMessageEnd),
			MessageID: c.currentMessageID,
		})
		c.messageStarted = false
	}

	// Add run finished event
	events = append(events, RunFinished{
		Base:     NewBase(TypeRunFinished),
		ThreadID: c.threadID,
		RunID:    c.runID,
	})

	return events
}

// ErrorRun generates the RUN_ERROR event
func (c *BaseConverter) ErrorRun(err error) []Event {
	c.mu.Lock()
	defer c.mu.Unlock()

	var events []Event

	// Close any open message
	if c.messageStarted {
		events = append(events, TextMessageEnd{
			Base:      NewBase(TypeTextMessageEnd),
			MessageID: c.currentMessageID,
		})
		c.messageStarted = false
	}

	events = append(events, RunError{
		Base:     NewBase(TypeRunError),
		ThreadID: c.threadID,
		RunID:    c.runID,
		Message:  err.Error(),
	})

	return events
}

// StartMessage starts a new text message and returns the events
func (c *BaseConverter) StartMessage(role string) []Event {
	c.mu.Lock()
	defer c.mu.Unlock()

	var events []Event

	// Close any existing message first
	if c.messageStarted {
		events = append(events, TextMessageEnd{
			Base:      NewBase(TypeTextMessageEnd),
			MessageID: c.currentMessageID,
		})
	}

	c.currentMessageID = uuid.New().String()
	c.messageStarted = true

	events = append(events, TextMessageStart{
		Base:      NewBase(TypeTextMessageStart),
		MessageID: c.currentMessageID,
		Role:      role,
	})

	return events
}

// AddMessageContent adds content to the current message
func (c *BaseConverter) AddMessageContent(text string) Event {
	c.mu.Lock()
	defer c.mu.Unlock()

	return TextMessageContent{
		Base:      NewBase(TypeTextMessageContent),
		MessageID: c.currentMessageID,
		Delta:     text,
	}
}

// EndMessage ends the current message
func (c *BaseConverter) EndMessage() Event {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.messageStarted = false
	return TextMessageEnd{
		Base:      NewBase(TypeTextMessageEnd),
		MessageID: c.currentMessageID,
	}
}

// IsMessageStarted returns whether a message is currently being streamed
func (c *BaseConverter) IsMessageStarted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.messageStarted
}

// GetCurrentMessageID returns the current message ID
func (c *BaseConverter) GetCurrentMessageID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentMessageID
}

// StartToolCall starts a new tool call
func (c *BaseConverter) StartToolCall(toolName, toolCallID string) []Event {
	c.mu.Lock()
	defer c.mu.Unlock()

	var events []Event

	if toolCallID == "" {
		toolCallID = uuid.New().String()
	}

	// Close any open text message before tool call
	if c.messageStarted {
		events = append(events, TextMessageEnd{
			Base:      NewBase(TypeTextMessageEnd),
			MessageID: c.currentMessageID,
		})
		c.messageStarted = false
	}

	// Track this tool call
	c.activeToolCalls[toolCallID] = &toolCallState{
		toolCallID: toolCallID,
		toolName:   toolName,
		started:    true,
	}

	events = append(events, ToolCallStart{
		Base:            NewBase(TypeToolCallStart),
		ToolCallID:      toolCallID,
		ToolCallName:    toolName,
		ParentMessageID: c.currentMessageID,
	})

	return events
}

// AddToolCallArgs adds arguments to a tool call
func (c *BaseConverter) AddToolCallArgs(toolCallID, argsJSON string) Event {
	return ToolCallArgs{
		Base:       NewBase(TypeToolCallArgs),
		ToolCallID: toolCallID,
		Delta:      argsJSON,
	}
}

// EndToolCall ends a tool call
func (c *BaseConverter) EndToolCall(toolCallID string) Event {
	return ToolCallEnd{
		Base:       NewBase(TypeToolCallEnd),
		ToolCallID: toolCallID,
	}
}

// AddToolCallResult adds a tool call result
func (c *BaseConverter) AddToolCallResult(toolCallID, content string) Event {
	c.mu.Lock()
	defer c.mu.Unlock()

	messageID := uuid.New().String()

	// Clean up tracked tool call
	delete(c.activeToolCalls, toolCallID)

	return ToolCallResult{
		Base:       NewBase(TypeToolCallResult),
		MessageID:  messageID,
		ToolCallID: toolCallID,
		Role:       "tool",
		Content:    content,
	}
}

// CreateStepEvent creates a step started/finished event
func (c *BaseConverter) CreateStepEvent(stepName, stepID string, started bool) Event {
	if started {
		return StepStarted{
			Base:     NewBase(TypeStepStarted),
			StepName: stepName,
			StepID:   stepID,
		}
	}
	return StepFinished{
		Base:   NewBase(TypeStepFinished),
		StepID: stepID,
	}
}

// CreateActivityEvent creates an activity update event
func (c *BaseConverter) CreateActivityEvent(activity Activity) Event {
	return ActivityDelta{
		Base:     NewBase(TypeActivityDelta),
		Activity: activity,
	}
}

// CreateStateDeltaEvent creates a state delta event
func (c *BaseConverter) CreateStateDeltaEvent(delta map[string]any) Event {
	return StateDelta{
		Base:  NewBase(TypeStateDelta),
		Delta: delta,
	}
}

// CreateCustomEvent creates a custom event for vendor-specific data
func (c *BaseConverter) CreateCustomEvent(name string, data any) Event {
	return Custom{
		Base: NewBase(TypeCustom),
		Name: name,
		Data: data,
	}
}
