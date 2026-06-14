package openai

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestStream_NextDelta(t *testing.T) {
	input := `data: {"id":"chatcmpl-123","choices":[{"index":0,"delta":{"content":"Hello"}}]}

data: {"id":"chatcmpl-123","choices":[{"index":0,"delta":{"content":" World"}}]}

data: [DONE]

`
	stream := NewStream(context.Background(), strings.NewReader(input))

	delta1, err := stream.NextDelta()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delta1 != "Hello" {
		t.Errorf("expected 'Hello', got %q", delta1)
	}

	delta2, err := stream.NextDelta()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delta2 != " World" {
		t.Errorf("expected ' World', got %q", delta2)
	}

	// Should get EOF after [DONE]
	_, err = stream.NextDelta()
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestStream_NextChunk(t *testing.T) {
	input := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: [DONE]

`
	stream := NewStream(context.Background(), strings.NewReader(input))

	chunk1, err := stream.NextChunk()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chunk1.ID != "chatcmpl-123" {
		t.Errorf("expected id 'chatcmpl-123', got %q", chunk1.ID)
	}
	if chunk1.Model != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got %q", chunk1.Model)
	}
	if len(chunk1.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(chunk1.Choices))
	}
	if chunk1.Choices[0].Delta.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", chunk1.Choices[0].Delta.Role)
	}

	chunk2, err := stream.NextChunk()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chunk2.Choices[0].Delta.Content != "Hello" {
		t.Errorf("expected content 'Hello', got %q", chunk2.Choices[0].Delta.Content)
	}

	// Should get EOF after [DONE]
	_, err = stream.NextChunk()
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestStream_CustomDeltaPath(t *testing.T) {
	// Anthropic-style response
	input := `data: {"type":"content_block_delta","delta":{"text":"Hello"}}

data: [DONE]

`
	stream := NewStream(context.Background(), strings.NewReader(input),
		WithDeltaPath("delta.text"))

	delta, err := stream.NextDelta()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delta != "Hello" {
		t.Errorf("expected 'Hello', got %q", delta)
	}
}

func TestStream_DeltasChannel(t *testing.T) {
	input := `data: {"choices":[{"delta":{"content":"A"}}]}

data: {"choices":[{"delta":{"content":"B"}}]}

data: {"choices":[{"delta":{"content":"C"}}]}

data: [DONE]

`
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream := NewStream(ctx, strings.NewReader(input))

	var deltas []string
	for delta := range stream.Deltas() {
		deltas = append(deltas, delta)
	}

	if len(deltas) != 3 {
		t.Fatalf("expected 3 deltas, got %d", len(deltas))
	}
	if deltas[0] != "A" || deltas[1] != "B" || deltas[2] != "C" {
		t.Errorf("unexpected deltas: %v", deltas)
	}
}

func TestStream_ContextCancellation(t *testing.T) {
	// Use a slow reader that blocks
	slowReader := &slowReader{
		data: `data: {"choices":[{"delta":{"content":"Hello"}}]}

`,
		delay: 10 * time.Second, // Very slow
	}

	ctx, cancel := context.WithCancel(context.Background())
	stream := NewStream(ctx, slowReader)

	// Cancel immediately
	cancel()

	_, err := stream.NextDelta()
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

type slowReader struct {
	data  string
	delay time.Duration
	read  bool
}

func (r *slowReader) Read(p []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	time.Sleep(r.delay)
	r.read = true
	n := copy(p, r.data)
	return n, nil
}

func TestStream_MissingContent(t *testing.T) {
	// Event without content field (e.g., role-only first message)
	input := `data: {"choices":[{"delta":{"role":"assistant"}}]}

data: {"choices":[{"delta":{"content":"Hello"}}]}

data: [DONE]

`
	stream := NewStream(context.Background(), strings.NewReader(input))

	// Should skip the role-only message and return "Hello"
	delta, err := stream.NextDelta()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delta != "Hello" {
		t.Errorf("expected 'Hello', got %q", delta)
	}
}

func TestStream_CustomDoneMarker(t *testing.T) {
	input := `data: {"choices":[{"delta":{"content":"Hi"}}]}

data: STREAM_END

`
	stream := NewStream(context.Background(), strings.NewReader(input),
		WithDoneMarker("STREAM_END"))

	delta, err := stream.NextDelta()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delta != "Hi" {
		t.Errorf("expected 'Hi', got %q", delta)
	}

	_, err = stream.NextDelta()
	if err != io.EOF {
		t.Errorf("expected io.EOF after custom done marker, got %v", err)
	}
}

func TestStream_HealJSON(t *testing.T) {
	// Truncated JSON payload
	input := `data: {"choices":[{"delta":{"content":"Hello

data: [DONE]

`
	stream := NewStream(context.Background(), strings.NewReader(input),
		WithHealJSON(true))

	// With healing enabled, should attempt to recover
	// Note: In practice the truncated JSON parsing is complex,
	// this just tests that the option is respected
	delta, err := stream.NextDelta()
	if err != nil && err != io.EOF {
		// Healing may or may not succeed on such severely truncated input
		t.Logf("healing result: delta=%q, err=%v", delta, err)
	}
}

func TestStream_HealJSON_TruncatedPayload_Recovered(t *testing.T) {
	// A truncated SSE event: the JSON object is cut off mid-string,
	// with no closing quote or braces.
	input := `data: {"choices":[{"delta":{"content":"hel

data: [DONE]

`
	stream := NewStream(context.Background(), strings.NewReader(input),
		WithHealJSON(true))

	delta, err := stream.NextDelta()
	if err != nil {
		t.Fatalf("expected healing to recover content, got error: %v", err)
	}
	if delta != "hel" {
		t.Errorf("expected healed content 'hel', got %q", delta)
	}
}

func TestStream_NoHealJSON_TruncatedPayload_Skipped(t *testing.T) {
	// Same truncated payload, but healing disabled: the malformed event
	// should be skipped, leaving only [DONE] -> io.EOF.
	input := `data: {"choices":[{"delta":{"content":"hel

data: [DONE]

`
	stream := NewStream(context.Background(), strings.NewReader(input))

	_, err := stream.NextDelta()
	if err != io.EOF {
		t.Errorf("expected truncated payload to be skipped resulting in io.EOF, got %v", err)
	}
}

func TestStream_NextEvent(t *testing.T) {
	input := `event: ping
data: keep-alive

data: {"choices":[{"delta":{"content":"Hi"}}]}

data: [DONE]

`
	stream := NewStream(context.Background(), strings.NewReader(input))

	// First event is the ping
	event1, err := stream.NextEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event1.Event != "ping" {
		t.Errorf("expected event type 'ping', got %q", event1.Event)
	}

	// Second event has content
	event2, err := stream.NextEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(event2.Data, "Hi") {
		t.Errorf("expected data to contain 'Hi'")
	}
}

// Benchmark tests
func BenchmarkStream_NextDelta(b *testing.B) {
	input := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: [DONE]

`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream := NewStream(context.Background(), strings.NewReader(input))
		_, _ = stream.NextDelta()
	}
}

func BenchmarkStream_NextChunk(b *testing.B) {
	input := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: [DONE]

`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream := NewStream(context.Background(), strings.NewReader(input))
		_, _ = stream.NextChunk()
	}
}
