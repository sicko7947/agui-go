package aguigo

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewBase(t *testing.T) {
	base := NewBase(TypeRunStarted)
	assert.Equal(t, TypeRunStarted, base.Type)
	assert.WithinDuration(t, time.Now(), time.UnixMilli(base.Timestamp), 100*time.Millisecond)
}

func TestBaseMethods(t *testing.T) {
	base := NewBase(TypeRunFinished)
	assert.Equal(t, TypeRunFinished, base.GetType())
	assert.NotZero(t, base.GetTimestamp())
}

func TestEventToJSON(t *testing.T) {
	testCases := []struct {
		name  string
		event Event
		want  string
	}{
		{
			name: "RunStarted",
			event: &RunStarted{
				Base:     NewBase(TypeRunStarted),
				ThreadID: "thread-1",
				RunID:    "run-1",
			},
			want: `{"type":"RUN_STARTED","threadId":"thread-1","runId":"run-1"}`,
		},
		{
			name: "RunFinished",
			event: &RunFinished{
				Base:     NewBase(TypeRunFinished),
				ThreadID: "thread-1",
				RunID:    "run-1",
			},
			want: `{"type":"RUN_FINISHED","threadId":"thread-1","runId":"run-1"}`,
		},
		{
			name: "RunError",
			event: &RunError{
				Base:    NewBase(TypeRunError),
				Message: "test error",
			},
			want: `{"type":"RUN_ERROR","message":"test error"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jsonBytes, err := tc.event.ToJSON()
			assert.NoError(t, err)

			// We need to unmarshal and remarshal to get a consistent key order
			var got map[string]interface{}
			err = json.Unmarshal(jsonBytes, &got)
			assert.NoError(t, err)
			delete(got, "timestamp") // Remove timestamp for comparison
			delete(got, "rawEvent")  // Remove rawEvent for comparison

			var want map[string]interface{}
			err = json.Unmarshal([]byte(tc.want), &want)
			assert.NoError(t, err)

			assert.Equal(t, want, got)
		})
	}
}

func TestContentUnmarshal(t *testing.T) {
	t.Run("from string", func(t *testing.T) {
		var content Content
		err := json.Unmarshal([]byte(`"hello"`), &content)
		assert.NoError(t, err)
		assert.Equal(t, Content{{Type: "text", Text: "hello"}}, content)
	})

	t.Run("from array", func(t *testing.T) {
		var content Content
		jsonData := `[{"type": "text", "text": "hello"}, {"type": "image", "url": "image.png"}]`
		err := json.Unmarshal([]byte(jsonData), &content)
		assert.NoError(t, err)
		assert.Equal(t, Content{
			{Type: "text", Text: "hello"},
			{Type: "image", URL: "image.png"},
		}, content)
	})

	t.Run("from null", func(t *testing.T) {
		var content Content
		err := json.Unmarshal([]byte(`null`), &content)
		assert.NoError(t, err)
		assert.Nil(t, content)
	})
}

func TestContentMarshal(t *testing.T) {
	t.Run("from string-like content", func(t *testing.T) {
		content := Content{{Type: "text", Text: "hello"}}
		jsonBytes, err := json.Marshal(content)
		assert.NoError(t, err)
		assert.JSONEq(t, `[{"type":"text","text":"hello"}]`, string(jsonBytes))
	})

	t.Run("from multi-part content", func(t *testing.T) {
		content := Content{
			{Type: "text", Text: "hello"},
			{Type: "image", URL: "image.png"},
		}
		jsonBytes, err := json.Marshal(content)
		assert.NoError(t, err)
		assert.JSONEq(t, `[{"type":"text","text":"hello"},{"type":"image","url":"image.png"}]`, string(jsonBytes))
	})

	t.Run("from nil", func(t *testing.T) {
		var content Content
		jsonBytes, err := json.Marshal(content)
		assert.NoError(t, err)
		assert.Equal(t, "null", string(jsonBytes))
	})
}

func TestContentGetText(t *testing.T) {
	content := Content{
		{Type: "text", Text: "Hello "},
		{Type: "image", URL: "image.png"},
		{Type: "text", Text: "world!"},
	}
	assert.Equal(t, "Hello world!", content.GetText())
}

func TestContentGetTextParts(t *testing.T) {
	content := Content{
		{Type: "text", Text: "Hello"},
		{Type: "image", URL: "image.png"},
		{Type: "text", Text: "world"},
	}
	parts := content.GetTextParts()
	assert.Len(t, parts, 2)
	assert.Equal(t, "Hello", parts[0].Text)
	assert.Equal(t, "world", parts[1].Text)
}