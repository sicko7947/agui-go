package aguigo

import (
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func TestADKConverter_NewADKConverter(t *testing.T) {
	t.Run("generates IDs when empty", func(t *testing.T) {
		conv := NewADKConverter("", "")
		assert.NotEmpty(t, conv.GetThreadID())
		assert.NotEmpty(t, conv.GetRunID())
	})

	t.Run("uses provided IDs", func(t *testing.T) {
		conv := NewADKConverter("thread-123", "run-456")
		assert.Equal(t, "thread-123", conv.GetThreadID())
		assert.Equal(t, "run-456", conv.GetRunID())
	})

	t.Run("applies options", func(t *testing.T) {
		conv := NewADKConverter("", "", WithStepEvents(true), WithActivityEvents(true), WithRawEvents(true))
		opts := conv.GetOptions()
		assert.True(t, opts.EmitStepEvents)
		assert.True(t, opts.EmitActivityEvents)
		assert.True(t, opts.IncludeRawEvents)
	})
}

func TestADKConverter_StartRun(t *testing.T) {
	conv := NewADKConverter("thread-1", "run-1")
	evt := conv.StartRun()

	assert.Equal(t, events.EventTypeRunStarted, evt.Type())
}

func TestADKConverter_FinishRun(t *testing.T) {
	conv := NewADKConverter("thread-1", "run-1")
	evts := conv.FinishRun()

	require.Len(t, evts, 1)
	assert.Equal(t, events.EventTypeRunFinished, evts[0].Type())
}

func TestADKConverter_ErrorRun(t *testing.T) {
	conv := NewADKConverter("thread-1", "run-1")
	evts := conv.ErrorRun(assert.AnError)

	require.Len(t, evts, 1)
	assert.Equal(t, events.EventTypeRunError, evts[0].Type())
}

func TestADKConverter_HandleThought(t *testing.T) {
	t.Run("emits THINKING events for thought content", func(t *testing.T) {
		conv := NewADKConverter("thread-1", "run-1")

		adkEvent := &session.Event{
			Author: "assistant",
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{
							Text:    "Let me think about this...",
							Thought: true,
						},
					},
				},
			},
		}

		evts := conv.ConvertEvent(adkEvent)

		// Should emit: THINKING_START, THINKING_TEXT_MESSAGE_START, THINKING_TEXT_MESSAGE_CONTENT, THINKING_TEXT_MESSAGE_END, THINKING_END
		require.Len(t, evts, 5)
		assert.Equal(t, events.EventTypeThinkingStart, evts[0].Type())
		assert.Equal(t, events.EventTypeThinkingTextMessageStart, evts[1].Type())
		assert.Equal(t, events.EventTypeThinkingTextMessageContent, evts[2].Type())
		assert.Equal(t, events.EventTypeThinkingTextMessageEnd, evts[3].Type())
		assert.Equal(t, events.EventTypeThinkingEnd, evts[4].Type())

		// Verify the content event has the thought text
		contentEvt, ok := evts[2].(*events.ThinkingTextMessageContentEvent)
		require.True(t, ok)
		assert.Equal(t, "Let me think about this...", contentEvt.Delta)
	})

	t.Run("skips empty thoughts", func(t *testing.T) {
		conv := NewADKConverter("thread-1", "run-1")

		adkEvent := &session.Event{
			Author: "assistant",
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{
							Text:    "",
							Thought: true,
						},
					},
				},
			},
		}

		evts := conv.ConvertEvent(adkEvent)
		assert.Empty(t, evts)
	})
}

func TestADKConverter_HandleTextPart(t *testing.T) {
	t.Run("emits TEXT_MESSAGE events for text content", func(t *testing.T) {
		conv := NewADKConverter("thread-1", "run-1")

		adkEvent := &session.Event{
			Author: "assistant",
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: "Hello, world!"},
					},
				},
			},
		}

		evts := conv.ConvertEvent(adkEvent)

		// Should emit: TEXT_MESSAGE_START, TEXT_MESSAGE_CONTENT
		require.Len(t, evts, 2)
		assert.Equal(t, events.EventTypeTextMessageStart, evts[0].Type())
		assert.Equal(t, events.EventTypeTextMessageContent, evts[1].Type())
	})

	t.Run("continues existing message for subsequent text", func(t *testing.T) {
		conv := NewADKConverter("thread-1", "run-1")

		// First text part
		adkEvent1 := &session.Event{
			Author: "assistant",
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: "Hello"},
					},
				},
			},
		}
		evts1 := conv.ConvertEvent(adkEvent1)
		require.Len(t, evts1, 2) // START + CONTENT

		// Second text part - should only emit CONTENT
		adkEvent2 := &session.Event{
			Author: "assistant",
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: ", world!"},
					},
				},
			},
		}
		evts2 := conv.ConvertEvent(adkEvent2)
		require.Len(t, evts2, 1) // Only CONTENT
		assert.Equal(t, events.EventTypeTextMessageContent, evts2[0].Type())
	})
}

func TestADKConverter_HandleFunctionCall(t *testing.T) {
	t.Run("emits TOOL_CALL events for function calls", func(t *testing.T) {
		conv := NewADKConverter("thread-1", "run-1")

		adkEvent := &session.Event{
			Author: "assistant",
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{
							FunctionCall: &genai.FunctionCall{
								ID:   "call-123",
								Name: "get_weather",
								Args: map[string]any{"city": "Sydney"},
							},
						},
					},
				},
			},
		}

		evts := conv.ConvertEvent(adkEvent)

		// Should emit: TOOL_CALL_START, TOOL_CALL_ARGS, TOOL_CALL_END
		require.Len(t, evts, 3)
		assert.Equal(t, events.EventTypeToolCallStart, evts[0].Type())
		assert.Equal(t, events.EventTypeToolCallArgs, evts[1].Type())
		assert.Equal(t, events.EventTypeToolCallEnd, evts[2].Type())
	})

	t.Run("closes open text message before tool call", func(t *testing.T) {
		conv := NewADKConverter("thread-1", "run-1")

		// First, start a text message
		textEvent := &session.Event{
			Author: "assistant",
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: "Let me check the weather..."},
					},
				},
			},
		}
		conv.ConvertEvent(textEvent)
		assert.True(t, conv.IsMessageStarted())

		// Then emit a function call
		fcEvent := &session.Event{
			Author: "assistant",
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{
							FunctionCall: &genai.FunctionCall{
								ID:   "call-123",
								Name: "get_weather",
							},
						},
					},
				},
			},
		}
		evts := conv.ConvertEvent(fcEvent)

		// Should emit: TEXT_MESSAGE_END, TOOL_CALL_START, TOOL_CALL_END
		require.Len(t, evts, 3)
		assert.Equal(t, events.EventTypeTextMessageEnd, evts[0].Type())
		assert.Equal(t, events.EventTypeToolCallStart, evts[1].Type())
		assert.False(t, conv.IsMessageStarted())
	})
}

func TestADKConverter_HandleFunctionResponse(t *testing.T) {
	t.Run("emits TOOL_CALL_RESULT for function responses", func(t *testing.T) {
		conv := NewADKConverter("thread-1", "run-1")

		adkEvent := &session.Event{
			Author: "tool",
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{
							FunctionResponse: &genai.FunctionResponse{
								ID:       "call-123",
								Name:     "get_weather",
								Response: map[string]any{"temperature": 25, "condition": "sunny"},
							},
						},
					},
				},
			},
		}

		evts := conv.ConvertEvent(adkEvent)

		require.Len(t, evts, 1)
		assert.Equal(t, events.EventTypeToolCallResult, evts[0].Type())
	})
}

func TestADKConverter_HandleStateDelta(t *testing.T) {
	t.Run("emits STATE_DELTA for state changes", func(t *testing.T) {
		conv := NewADKConverter("thread-1", "run-1")

		adkEvent := &session.Event{
			Author: "assistant",
			Actions: session.EventActions{
				StateDelta: map[string]any{
					"counter": 42,
					"status":  "active",
				},
			},
		}

		evts := conv.ConvertEvent(adkEvent)

		require.Len(t, evts, 1)
		assert.Equal(t, events.EventTypeStateDelta, evts[0].Type())
	})
}

func TestADKConverter_HandleAgentTransfer(t *testing.T) {
	t.Run("emits CUSTOM event for agent transfer", func(t *testing.T) {
		conv := NewADKConverter("thread-1", "run-1")

		adkEvent := &session.Event{
			Author: "assistant",
			Actions: session.EventActions{
				TransferToAgent: "specialist-agent",
			},
		}

		evts := conv.ConvertEvent(adkEvent)

		require.Len(t, evts, 1)
		assert.Equal(t, events.EventTypeCustom, evts[0].Type())
	})
}

func TestADKConverter_HandleEscalation(t *testing.T) {
	t.Run("emits CUSTOM event for escalation", func(t *testing.T) {
		conv := NewADKConverter("thread-1", "run-1")

		adkEvent := &session.Event{
			Author: "assistant",
			Actions: session.EventActions{
				Escalate: true,
			},
		}

		evts := conv.ConvertEvent(adkEvent)

		require.Len(t, evts, 1)
		assert.Equal(t, events.EventTypeCustom, evts[0].Type())
	})
}

func TestADKConverter_HandleExecutableCode(t *testing.T) {
	t.Run("emits CUSTOM event for executable code", func(t *testing.T) {
		conv := NewADKConverter("thread-1", "run-1")

		adkEvent := &session.Event{
			Author: "assistant",
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{
							ExecutableCode: &genai.ExecutableCode{
								Language: genai.LanguagePython,
								Code:     "print('Hello, World!')",
							},
						},
					},
				},
			},
		}

		evts := conv.ConvertEvent(adkEvent)

		require.Len(t, evts, 1)
		assert.Equal(t, events.EventTypeCustom, evts[0].Type())
	})
}

func TestADKConverter_HandleCodeExecutionResult(t *testing.T) {
	t.Run("emits CUSTOM event for code execution result", func(t *testing.T) {
		conv := NewADKConverter("thread-1", "run-1")

		adkEvent := &session.Event{
			Author: "assistant",
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{
							CodeExecutionResult: &genai.CodeExecutionResult{
								Outcome: genai.OutcomeOK,
								Output:  "Hello, World!",
							},
						},
					},
				},
			},
		}

		evts := conv.ConvertEvent(adkEvent)

		require.Len(t, evts, 1)
		assert.Equal(t, events.EventTypeCustom, evts[0].Type())
	})
}

func TestADKConverter_FinishRunClosesOpenMessage(t *testing.T) {
	conv := NewADKConverter("thread-1", "run-1")

	// Start a text message
	textEvent := &session.Event{
		Author: "assistant",
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{
				Parts: []*genai.Part{
					{Text: "Hello"},
				},
			},
		},
	}
	conv.ConvertEvent(textEvent)
	assert.True(t, conv.IsMessageStarted())

	// Finish the run
	evts := conv.FinishRun()

	// Should emit: TEXT_MESSAGE_END, RUN_FINISHED
	require.Len(t, evts, 2)
	assert.Equal(t, events.EventTypeTextMessageEnd, evts[0].Type())
	assert.Equal(t, events.EventTypeRunFinished, evts[1].Type())
	assert.False(t, conv.IsMessageStarted())
}
