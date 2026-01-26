package sse

import (
	"io"
	"strings"
	"testing"
)

func TestDecoder_BasicEvent(t *testing.T) {
	input := "data: hello world\n\n"
	d := NewDecoder(strings.NewReader(input))

	event, err := d.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Event != "message" {
		t.Errorf("expected event type 'message', got %q", event.Event)
	}
	if event.Data != "hello world" {
		t.Errorf("expected data 'hello world', got %q", event.Data)
	}

	// Should return EOF on next call
	_, err = d.Next()
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestDecoder_MultipleDataLines(t *testing.T) {
	input := "data: line 1\ndata: line 2\ndata: line 3\n\n"
	d := NewDecoder(strings.NewReader(input))

	event, err := d.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "line 1\nline 2\nline 3"
	if event.Data != expected {
		t.Errorf("expected data %q, got %q", expected, event.Data)
	}
}

func TestDecoder_EventType(t *testing.T) {
	input := "event: custom\ndata: test\n\n"
	d := NewDecoder(strings.NewReader(input))

	event, err := d.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Event != "custom" {
		t.Errorf("expected event type 'custom', got %q", event.Event)
	}
}

func TestDecoder_EventID(t *testing.T) {
	input := "id: 123\ndata: test\n\n"
	d := NewDecoder(strings.NewReader(input))

	event, err := d.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.ID != "123" {
		t.Errorf("expected id '123', got %q", event.ID)
	}
	if d.LastEventID() != "123" {
		t.Errorf("expected LastEventID '123', got %q", d.LastEventID())
	}
}

func TestDecoder_IDPersists(t *testing.T) {
	input := "id: first\ndata: one\n\ndata: two\n\n"
	d := NewDecoder(strings.NewReader(input))

	event1, _ := d.Next()
	event2, _ := d.Next()

	if event1.ID != "first" {
		t.Errorf("expected first event id 'first', got %q", event1.ID)
	}
	if event2.ID != "first" {
		t.Errorf("expected second event id 'first' (inherited), got %q", event2.ID)
	}
}

func TestDecoder_Retry(t *testing.T) {
	input := "retry: 5000\ndata: test\n\n"
	d := NewDecoder(strings.NewReader(input))

	event, err := d.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Retry != 5000 {
		t.Errorf("expected retry 5000, got %d", event.Retry)
	}
}

func TestDecoder_InvalidRetryIgnored(t *testing.T) {
	input := "retry: abc\ndata: test\n\n"
	d := NewDecoder(strings.NewReader(input))

	event, err := d.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Retry != 0 {
		t.Errorf("expected retry 0 (invalid ignored), got %d", event.Retry)
	}
}

func TestDecoder_Comments(t *testing.T) {
	input := ": this is a comment\ndata: actual data\n: another comment\n\n"
	d := NewDecoder(strings.NewReader(input))

	event, err := d.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Data != "actual data" {
		t.Errorf("expected data 'actual data', got %q", event.Data)
	}
}

func TestDecoder_KeepAlive(t *testing.T) {
	// Keep-alives are just empty comment lines
	input := ":\n:\ndata: after keepalive\n\n"
	d := NewDecoder(strings.NewReader(input))

	event, err := d.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Data != "after keepalive" {
		t.Errorf("expected data 'after keepalive', got %q", event.Data)
	}
}

func TestDecoder_NoSpaceAfterColon(t *testing.T) {
	input := "data:no space\n\n"
	d := NewDecoder(strings.NewReader(input))

	event, err := d.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Data != "no space" {
		t.Errorf("expected data 'no space', got %q", event.Data)
	}
}

func TestDecoder_ExtraSpaceAfterColon(t *testing.T) {
	// Only first space is stripped
	input := "data:  two spaces\n\n"
	d := NewDecoder(strings.NewReader(input))

	event, err := d.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Data != " two spaces" {
		t.Errorf("expected data ' two spaces', got %q", event.Data)
	}
}

func TestDecoder_MultipleEvents(t *testing.T) {
	input := "data: first\n\ndata: second\n\ndata: third\n\n"
	d := NewDecoder(strings.NewReader(input))

	events := make([]Event, 0, 3)
	for {
		event, err := d.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		events = append(events, event)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Data != "first" {
		t.Errorf("expected first event data 'first', got %q", events[0].Data)
	}
	if events[1].Data != "second" {
		t.Errorf("expected second event data 'second', got %q", events[1].Data)
	}
	if events[2].Data != "third" {
		t.Errorf("expected third event data 'third', got %q", events[2].Data)
	}
}

func TestDecoder_EmptyData(t *testing.T) {
	input := "data:\n\n"
	d := NewDecoder(strings.NewReader(input))

	event, err := d.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Data != "" {
		t.Errorf("expected empty data, got %q", event.Data)
	}
}

func TestDecoder_OpenAIFormat(t *testing.T) {
	// Simulate OpenAI streaming response format
	input := `data: {"id":"chatcmpl-123","choices":[{"delta":{"content":"Hello"}}]}

data: {"id":"chatcmpl-123","choices":[{"delta":{"content":" World"}}]}

data: [DONE]

`
	d := NewDecoder(strings.NewReader(input))

	events := make([]Event, 0, 3)
	for {
		event, err := d.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		events = append(events, event)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if !strings.Contains(events[0].Data, "Hello") {
		t.Errorf("expected first event to contain 'Hello'")
	}
	if !strings.Contains(events[1].Data, "World") {
		t.Errorf("expected second event to contain 'World'")
	}
	if events[2].Data != "[DONE]" {
		t.Errorf("expected third event data '[DONE]', got %q", events[2].Data)
	}
}

func TestDecoder_AnthropicFormat(t *testing.T) {
	// Simulate Anthropic streaming response format
	input := `event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" World"}}

event: message_stop
data: {"type":"message_stop"}

`
	d := NewDecoder(strings.NewReader(input))

	events := make([]Event, 0, 3)
	for {
		event, err := d.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		events = append(events, event)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Event != "content_block_delta" {
		t.Errorf("expected event type 'content_block_delta', got %q", events[0].Event)
	}
	if events[2].Event != "message_stop" {
		t.Errorf("expected event type 'message_stop', got %q", events[2].Event)
	}
}

func TestDecoder_PartialFrame(t *testing.T) {
	// Test that partial frames work when stream ends without trailing newline
	input := "data: partial"
	d := NewDecoder(strings.NewReader(input))

	event, err := d.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Data != "partial" {
		t.Errorf("expected data 'partial', got %q", event.Data)
	}
}

func TestDecoder_UnknownFieldIgnored(t *testing.T) {
	input := "unknown: value\ndata: test\n\n"
	d := NewDecoder(strings.NewReader(input))

	event, err := d.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Data != "test" {
		t.Errorf("expected data 'test', got %q", event.Data)
	}
}

func TestDecoder_IDWithNull(t *testing.T) {
	// IDs containing null should be ignored
	input := "id: has\x00null\ndata: test\n\n"
	d := NewDecoder(strings.NewReader(input))

	event, err := d.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.ID != "" {
		t.Errorf("expected empty id (null in value), got %q", event.ID)
	}
}

func TestEventStream_Channel(t *testing.T) {
	input := "data: one\n\ndata: two\n\ndata: three\n\n"
	stream := NewEventStream(strings.NewReader(input))
	defer stream.Close()

	var events []Event
	for event := range stream.Events() {
		events = append(events, event)
	}

	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}
	if stream.Err() != nil {
		t.Errorf("unexpected error: %v", stream.Err())
	}
}

// Benchmark tests
func BenchmarkDecoder_SimpleEvent(b *testing.B) {
	input := "data: hello world\n\n"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := NewDecoder(strings.NewReader(input))
		_, _ = d.Next()
	}
}

func BenchmarkDecoder_MultiLineData(b *testing.B) {
	input := "data: line 1\ndata: line 2\ndata: line 3\ndata: line 4\ndata: line 5\n\n"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := NewDecoder(strings.NewReader(input))
		_, _ = d.Next()
	}
}

func BenchmarkDecoder_OpenAIPayload(b *testing.B) {
	input := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := NewDecoder(strings.NewReader(input))
		_, _ = d.Next()
	}
}
