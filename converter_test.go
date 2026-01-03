package aguigo

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaseConverter_New(t *testing.T) {
	t.Run("with IDs", func(t *testing.T) {
		conv := NewBaseConverter("thread-1", "run-1")
		assert.Equal(t, "thread-1", conv.GetThreadID())
		assert.Equal(t, "run-1", conv.GetRunID())
	})

	t.Run("without IDs", func(t *testing.T) {
		conv := NewBaseConverter("", "")
		assert.NotEmpty(t, conv.GetThreadID())
		assert.NotEmpty(t, conv.GetRunID())
	})

	t.Run("with options", func(t *testing.T) {
		conv := NewBaseConverter("", "",
			WithRawEvents(true),
			WithStepEvents(true),
			WithActivityEvents(true),
		)
		opts := conv.GetOptions()
		assert.True(t, opts.IncludeRawEvents)
		assert.True(t, opts.EmitStepEvents)
		assert.True(t, opts.EmitActivityEvents)
	})
}

func TestBaseConverter_Lifecycle(t *testing.T) {
	conv := NewBaseConverter("thread-1", "run-1")

	t.Run("StartRun", func(t *testing.T) {
		event := conv.StartRun()
		assert.Equal(t, TypeRunStarted, event.GetType())
		runStarted := event.(RunStarted)
		assert.Equal(t, "thread-1", runStarted.ThreadID)
		assert.Equal(t, "run-1", runStarted.RunID)
	})

	t.Run("FinishRun", func(t *testing.T) {
		events := conv.FinishRun()
		assert.Len(t, events, 1)
		assert.Equal(t, TypeRunFinished, events[0].GetType())
	})

	t.Run("FinishRun with open message", func(t *testing.T) {
		conv.StartMessage("assistant")
		events := conv.FinishRun()
		assert.Len(t, events, 2)
		assert.Equal(t, TypeTextMessageEnd, events[0].GetType())
		assert.Equal(t, TypeRunFinished, events[1].GetType())
		assert.False(t, conv.IsMessageStarted())
	})

	t.Run("ErrorRun", func(t *testing.T) {
		events := conv.ErrorRun(errors.New("test error"))
		assert.Len(t, events, 1)
		assert.Equal(t, TypeRunError, events[0].GetType())
	})
}

func TestBaseConverter_Messages(t *testing.T) {
	conv := NewBaseConverter("thread-1", "run-1")

	t.Run("StartMessage", func(t *testing.T) {
		events := conv.StartMessage("assistant")
		assert.Len(t, events, 1)
		assert.Equal(t, TypeTextMessageStart, events[0].GetType())
		assert.True(t, conv.IsMessageStarted())
		assert.NotEmpty(t, conv.GetCurrentMessageID())
	})

	t.Run("StartMessage closes previous", func(t *testing.T) {
		firstMsgID := conv.GetCurrentMessageID()
		events := conv.StartMessage("user")
		assert.Len(t, events, 2)
		assert.Equal(t, TypeTextMessageEnd, events[0].GetType())
		assert.Equal(t, firstMsgID, events[0].(TextMessageEnd).MessageID)
		assert.Equal(t, TypeTextMessageStart, events[1].GetType())
	})

	t.Run("AddMessageContent", func(t *testing.T) {
		event := conv.AddMessageContent("hello")
		assert.Equal(t, TypeTextMessageContent, event.GetType())
		content := event.(TextMessageContent)
		assert.Equal(t, "hello", content.Delta)
		assert.Equal(t, conv.GetCurrentMessageID(), content.MessageID)
	})

	t.Run("EndMessage", func(t *testing.T) {
		event := conv.EndMessage()
		assert.Equal(t, TypeTextMessageEnd, event.GetType())
		assert.False(t, conv.IsMessageStarted())
	})
}

func TestBaseConverter_ToolCalls(t *testing.T) {
	conv := NewBaseConverter("thread-1", "run-1")
	conv.StartMessage("assistant")

	t.Run("StartToolCall", func(t *testing.T) {
		events := conv.StartToolCall("test_tool", "tool-1")
		assert.Len(t, events, 2)
		assert.Equal(t, TypeTextMessageEnd, events[0].GetType())
		assert.Equal(t, TypeToolCallStart, events[1].GetType())
		startEvent := events[1].(ToolCallStart)
		assert.Equal(t, "tool-1", startEvent.ToolCallID)
		assert.Equal(t, "test_tool", startEvent.ToolCallName)
		assert.False(t, conv.IsMessageStarted())
	})

	t.Run("AddToolCallArgs", func(t *testing.T) {
		event := conv.AddToolCallArgs("tool-1", `{"arg":"val"}`)
		assert.Equal(t, TypeToolCallArgs, event.GetType())
		argsEvent := event.(ToolCallArgs)
		assert.Equal(t, `{"arg":"val"}`, argsEvent.Delta)
	})

	t.Run("EndToolCall", func(t *testing.T) {
		event := conv.EndToolCall("tool-1")
		assert.Equal(t, TypeToolCallEnd, event.GetType())
	})

	t.Run("AddToolCallResult", func(t *testing.T) {
		event := conv.AddToolCallResult("tool-1", "result")
		assert.Equal(t, TypeToolCallResult, event.GetType())
		resultEvent := event.(ToolCallResult)
		assert.Equal(t, "tool-1", resultEvent.ToolCallID)
		assert.Equal(t, "result", resultEvent.Content)
		assert.NotEmpty(t, resultEvent.MessageID)
	})
}

func TestBaseConverter_EventCreators(t *testing.T) {
	conv := NewBaseConverter("", "")

	t.Run("CreateStepEvent", func(t *testing.T) {
		startEvent := conv.CreateStepEvent("step-1", "id-1", true).(StepStarted)
		assert.Equal(t, "step-1", startEvent.StepName)
		assert.Equal(t, "id-1", startEvent.StepID)

		endEvent := conv.CreateStepEvent("step-1", "id-1", false).(StepFinished)
		assert.Equal(t, "id-1", endEvent.StepID)
	})

	t.Run("CreateActivityEvent", func(t *testing.T) {
		activity := Activity{ID: "act-1", Type: "test"}
		event := conv.CreateActivityEvent(activity).(ActivityDelta)
		assert.Equal(t, "act-1", event.Activity.ID)
	})

	t.Run("CreateStateDeltaEvent", func(t *testing.T) {
		delta := map[string]any{"key": "value"}
		event := conv.CreateStateDeltaEvent(delta).(StateDelta)
		assert.Equal(t, "value", event.Delta["key"])
	})

	t.Run("CreateCustomEvent", func(t *testing.T) {
		data := map[string]any{"foo": "bar"}
		event := conv.CreateCustomEvent("my-event", data).(Custom)
		assert.Equal(t, "my-event", event.Name)
		assert.Equal(t, "bar", event.Data.(map[string]any)["foo"])
	})
}