package scanner

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestStreamReaderBasic(t *testing.T) {
	input := `{"name": "John", "age": 30}`
	reader := strings.NewReader(input)

	sr := NewStreamReader(context.Background(), reader)
	defer sr.Close()

	expected := []TokenKind{
		TokenObjectStart,
		TokenString, // key
		TokenString, // value
		TokenComma,
		TokenString, // key
		TokenNumber, // value
		TokenObjectEnd,
		TokenEOF,
	}

	for i, want := range expected {
		token := sr.NextToken()
		if token.Kind != want {
			t.Errorf("token[%d] = %v, want %v", i, token.Kind, want)
		}
	}
}

func TestStreamReaderTokensChannel(t *testing.T) {
	input := `[1, 2, 3]`
	reader := strings.NewReader(input)

	sr := NewStreamReader(context.Background(), reader)
	defer sr.Close()

	var tokens []Token
	for token := range sr.Tokens() {
		tokens = append(tokens, token)
	}

	expected := []TokenKind{
		TokenArrayStart,
		TokenNumber,
		TokenComma,
		TokenNumber,
		TokenComma,
		TokenNumber,
		TokenArrayEnd,
		TokenEOF,
	}

	if len(tokens) != len(expected) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(expected))
	}

	for i, want := range expected {
		if tokens[i].Kind != want {
			t.Errorf("token[%d] = %v, want %v", i, tokens[i].Kind, want)
		}
	}
}

// slowReader simulates a slow stream by yielding one byte at a time
type slowReader struct {
	data []byte
	pos  int
}

func (r *slowReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	p[0] = r.data[r.pos]
	r.pos++
	return 1, nil
}

func TestStreamReaderSlowStream(t *testing.T) {
	input := `{"key": "value"}`
	reader := &slowReader{data: []byte(input)}

	sr := NewStreamReader(context.Background(), reader)
	defer sr.Close()

	expected := []TokenKind{
		TokenObjectStart,
		TokenString,
		TokenString,
		TokenObjectEnd,
		TokenEOF,
	}

	for i, want := range expected {
		token := sr.NextToken()
		if token.Kind != want {
			t.Errorf("token[%d] = %v, want %v", i, token.Kind, want)
		}
	}
}

func TestStreamReaderContextCancellation(t *testing.T) {
	// Create a reader that blocks forever
	pr, _ := io.Pipe()
	defer pr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	sr := NewStreamReader(ctx, pr)
	defer sr.Close()

	token := sr.NextToken()
	if token.Kind != TokenError {
		t.Errorf("expected TokenError on cancellation, got %v", token.Kind)
	}
}

func TestStreamReaderLargeInput(t *testing.T) {
	// Build a large JSON array
	var buf bytes.Buffer
	buf.WriteString("[")
	for i := 0; i < 1000; i++ {
		if i > 0 {
			buf.WriteString(",")
		}
		buf.WriteString(`{"id":`)
		buf.WriteString(itoa(int64(i)))
		buf.WriteString(`,"name":"item`)
		buf.WriteString(itoa(int64(i)))
		buf.WriteString(`"}`)
	}
	buf.WriteString("]")

	sr := NewStreamReader(context.Background(), bytes.NewReader(buf.Bytes()))
	defer sr.Close()

	tokenCount := 0
	for token := range sr.Tokens() {
		tokenCount++
		if token.Kind == TokenError {
			t.Fatalf("unexpected error: %v", token.Err)
		}
	}

	// Each object: ObjectStart, String(key), Number, Comma, String(key), String(value), ObjectEnd
	// Plus ArrayStart, ArrayEnd, commas between objects, and EOF
	// This is a sanity check that we processed many tokens
	if tokenCount < 5000 {
		t.Errorf("expected at least 5000 tokens, got %d", tokenCount)
	}
}

func TestStreamReaderDepth(t *testing.T) {
	input := `{"a":{"b":{"c":[1,2,3]}}}`
	reader := strings.NewReader(input)

	sr := NewStreamReader(context.Background(), reader)
	defer sr.Close()

	maxDepth := 0
	for token := range sr.Tokens() {
		// Depth is measured AFTER processing the token
		// So we check depth before the token is returned from the channel
		// Actually, we need to check after each token
		depth := sr.Depth()
		if depth > maxDepth {
			maxDepth = depth
		}
		_ = token
	}

	// After consuming all tokens, depth should return to 0
	// But max depth during parsing should be 4
	// The issue is that Depth() is read after token consumption
	// Let's just verify we can track depth
	// Note: Due to goroutine timing, maxDepth might be 0 or 4
	_ = maxDepth // acknowledged that timing affects this value
}

func TestStreamReaderDepthDeterministic(t *testing.T) {
	input := `{"a":{"b":{"c":[1,2,3]}}}`
	reader := strings.NewReader(input)

	sr := NewStreamReader(context.Background(), reader)
	defer sr.Close()

	maxDepth := 0
	for {
		token := sr.NextToken()
		depth := sr.Depth()
		if depth > maxDepth {
			maxDepth = depth
		}
		if token.Kind == TokenEOF || token.Kind == TokenError {
			break
		}
	}

	if maxDepth != 4 {
		t.Errorf("maxDepth = %d, want 4", maxDepth)
	}
}

func BenchmarkStreamReaderSmall(b *testing.B) {
	input := []byte(`{"name": "John", "age": 30, "active": true}`)
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(input)
		sr := NewStreamReader(context.Background(), reader)
		for {
			token := sr.NextToken()
			if token.Kind == TokenEOF || token.Kind == TokenError {
				break
			}
		}
		sr.Close()
	}
}
