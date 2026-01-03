package aguigo

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSSEEncoder(t *testing.T) {
	recorder := httptest.NewRecorder()
	encoder := NewSSE(recorder)

	t.Run("SetSSEHeaders", func(t *testing.T) {
		SetSSEHeaders(recorder)
		assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
		assert.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
		assert.Equal(t, "keep-alive", recorder.Header().Get("Connection"))
	})

	t.Run("Encode", func(t *testing.T) {
		event := &RunStarted{
			Base:     NewBase(TypeRunStarted),
			ThreadID: "thread-1",
			RunID:    "run-1",
		}
		err := encoder.Encode(event)
		assert.NoError(t, err)

		body := recorder.Body.String()
		assert.True(t, strings.HasPrefix(body, "data: {"), "Should start with 'data: {'")
		assert.True(t, strings.HasSuffix(body, "}\n\n"), "Should end with '}\n\n'")
	})

	t.Run("EncodeError", func(t *testing.T) {
		recorder.Body.Reset()
		err := encoder.EncodeError("thread-1", "run-1", errors.New("test error"))
		assert.NoError(t, err)
		body := recorder.Body.String()
		assert.Contains(t, body, "\"type\":\"RUN_ERROR\"")
		assert.Contains(t, body, "\"message\":\"test error\"")
	})

	t.Run("EncodeMultiple", func(t *testing.T) {
		recorder.Body.Reset()
		events := []Event{
			&RunStarted{Base: NewBase(TypeRunStarted)},
			&RunFinished{Base: NewBase(TypeRunFinished)},
		}
		err := encoder.EncodeMultiple(events)
		assert.NoError(t, err)
		body := recorder.Body.String()
		assert.Equal(t, 2, strings.Count(body, "data:"))
	})

	t.Run("Flush", func(t *testing.T) {
		err := encoder.Flush()
		assert.NoError(t, err)
		assert.True(t, recorder.Flushed)
	})
}

func TestJSONEncoder(t *testing.T) {
	recorder := httptest.NewRecorder()
	encoder := NewJSON(recorder)

	t.Run("SetJSONStreamHeaders", func(t *testing.T) {
		SetJSONStreamHeaders(recorder)
		assert.Equal(t, "application/x-ndjson", recorder.Header().Get("Content-Type"))
	})

	t.Run("Encode", func(t *testing.T) {
		event := &RunStarted{
			Base:     NewBase(TypeRunStarted),
			ThreadID: "thread-1",
			RunID:    "run-1",
		}
		err := encoder.Encode(event)
		assert.NoError(t, err)

		body := recorder.Body.String()
		assert.True(t, strings.HasPrefix(body, "{"), "Should start with '{'")
		assert.True(t, strings.HasSuffix(body, "}\n"), "Should end with '}\n'")
	})

	t.Run("EncodeMultiple", func(t *testing.T) {
		recorder.Body.Reset()
		events := []Event{
			&RunStarted{Base: NewBase(TypeRunStarted)},
			&RunFinished{Base: NewBase(TypeRunFinished)},
		}
		err := encoder.EncodeMultiple(events)
		assert.NoError(t, err)
		body := recorder.Body.String()
		assert.Equal(t, 2, strings.Count(body, "\n"))
	})
}

func TestParseAcceptHeader(t *testing.T) {
	testCases := []struct {
		name   string
		accept string
		want   string
	}{
		{"SSE", "text/event-stream", "text/event-stream"},
		{"NDJSON", "application/x-ndjson", "application/x-ndjson"},
		{"JSON", "application/json", "application/json"},
		{"Default", "", "text/event-stream"},
		{"Wildcard", "*/*", "text/event-stream"},
		{"Complex", "text/html, application/xhtml+xml, application/xml;q=0.9, image/webp, */*;q=0.8, text/event-stream", "text/event-stream"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseAcceptHeader(tc.accept)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMarshalEvents(t *testing.T) {
	events := []Event{
		&RunStarted{Base: NewBase(TypeRunStarted), RunID: "run-1"},
		&RunFinished{Base: NewBase(TypeRunFinished), RunID: "run-1"},
	}

	jsonBytes, err := MarshalEvents(events)
	assert.NoError(t, err)

	var rawMessages []json.RawMessage
	err = json.Unmarshal(jsonBytes, &rawMessages)
	assert.NoError(t, err)
	assert.Len(t, rawMessages, 2)
}