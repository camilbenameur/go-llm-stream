package stream

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestReader_BasicJSON(t *testing.T) {
	input := `{"name": "test", "value": 42}`
	reader := NewReader(context.Background(), strings.NewReader(input))
	defer reader.Close()

	var tokens []Token
	for token := range reader.Tokens() {
		if token.Kind == TokenError {
			t.Fatalf("unexpected error: %v", token.Err)
		}
		if token.Kind == TokenEOF {
			break
		}
		tokens = append(tokens, token)
	}

	// Should have: { "name" : "test" , "value" : 42 }
	if len(tokens) < 5 {
		t.Errorf("expected at least 5 tokens, got %d", len(tokens))
	}

	// First token should be object start
	if tokens[0].Kind != TokenObjectStart {
		t.Errorf("expected TokenObjectStart, got %s", tokens[0].Kind)
	}
}

func TestReader_NextToken(t *testing.T) {
	input := `[1, 2, 3]`
	reader := NewReader(context.Background(), strings.NewReader(input))
	defer reader.Close()

	token := reader.NextToken()
	if token.Kind != TokenArrayStart {
		t.Errorf("expected TokenArrayStart, got %s", token.Kind)
	}

	token = reader.NextToken()
	if token.Kind != TokenNumber {
		t.Errorf("expected TokenNumber, got %s", token.Kind)
	}
	if string(token.Raw) != "1" {
		t.Errorf("expected '1', got %q", token.Raw)
	}
}

func TestReader_BytesConsumed(t *testing.T) {
	input := `{"key": "value"}`
	reader := NewReader(context.Background(), strings.NewReader(input))
	defer reader.Close()

	// Consume all tokens
	for token := range reader.Tokens() {
		if token.Kind == TokenEOF {
			break
		}
	}

	if reader.BytesConsumed() != int64(len(input)) {
		t.Errorf("expected %d bytes consumed, got %d", len(input), reader.BytesConsumed())
	}
}

func TestHealer_BasicJSON(t *testing.T) {
	input := `{"complete": true}`
	healer := NewHealer(context.Background(), strings.NewReader(input),
		WithStripMarkdown(false)) // Disable markdown for simple test
	defer healer.Close()

	var tokens []Token
	for token := range healer.Tokens() {
		if token.Kind == TokenError {
			t.Fatalf("unexpected error: %v", token.Err)
		}
		if token.Kind == TokenEOF {
			break
		}
		tokens = append(tokens, token)
	}

	if len(tokens) < 4 {
		t.Errorf("expected at least 4 tokens, got %d", len(tokens))
	}
}

func TestHealer_TruncatedJSON(t *testing.T) {
	// Truncated JSON - missing closing brace
	input := `{"name": "test"`

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	healer := NewHealer(ctx, strings.NewReader(input),
		WithStripMarkdown(false)) // Disable markdown for simple test
	defer healer.Close()

	var tokens []Token
	for {
		token := healer.NextToken()
		if token.Kind == TokenError {
			if token.Err == context.DeadlineExceeded {
				t.Fatal("test timed out")
			}
			t.Fatalf("unexpected error: %v", token.Err)
		}
		if token.Kind == TokenEOF {
			break
		}
		tokens = append(tokens, token)
		if len(tokens) > 20 {
			t.Fatal("too many tokens, likely infinite loop")
		}
	}

	// Should have added closing brace
	if len(tokens) == 0 {
		t.Fatal("expected tokens")
	}

	// Last non-EOF token should be object end (healed)
	lastToken := tokens[len(tokens)-1]
	if lastToken.Kind != TokenObjectEnd {
		t.Errorf("expected healed TokenObjectEnd, got %s", lastToken.Kind)
	}
}

func TestHealer_WithMarkdown(t *testing.T) {
	// JSON wrapped in markdown code block
	input := "```json\n{\"key\": \"value\"}\n```"
	healer := NewHealer(context.Background(), strings.NewReader(input),
		WithStripMarkdown(true))
	defer healer.Close()

	var tokens []Token
	for token := range healer.Tokens() {
		if token.Kind == TokenError {
			t.Fatalf("unexpected error: %v", token.Err)
		}
		if token.Kind == TokenEOF {
			break
		}
		tokens = append(tokens, token)
	}

	// Should parse the JSON inside the code block
	if len(tokens) < 4 {
		t.Errorf("expected at least 4 tokens, got %d", len(tokens))
	}
}

func TestHealer_WithOptions(t *testing.T) {
	input := `{"test": true}`
	healer := NewHealer(context.Background(), strings.NewReader(input),
		WithStripMarkdown(false),
		WithAutoClose(true),
		WithIgnoreTrailingJunk(true))
	defer healer.Close()

	// Just verify it works with options
	token := healer.NextToken()
	if token.Kind != TokenObjectStart {
		t.Errorf("expected TokenObjectStart, got %s", token.Kind)
	}
}

func TestOptions_Defaults(t *testing.T) {
	opts := DefaultOptions()

	if !opts.StripMarkdown {
		t.Error("expected StripMarkdown true")
	}
	if !opts.AutoClose {
		t.Error("expected AutoClose true")
	}
	if !opts.IgnoreTrailingJunk {
		t.Error("expected IgnoreTrailingJunk true")
	}
}

func TestOptions_WithFunctions(t *testing.T) {
	opts := applyOptions(
		WithStripMarkdown(false),
		WithAutoClose(false),
		WithIgnoreTrailingJunk(false),
	)

	if opts.StripMarkdown {
		t.Error("expected StripMarkdown false")
	}
	if opts.AutoClose {
		t.Error("expected AutoClose false")
	}
	if opts.IgnoreTrailingJunk {
		t.Error("expected IgnoreTrailingJunk false")
	}
}

// Benchmark tests
func BenchmarkReader_SmallJSON(b *testing.B) {
	input := `{"name": "test", "value": 42}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := NewReader(context.Background(), strings.NewReader(input))
		for range reader.Tokens() {
		}
		reader.Close()
	}
}

func BenchmarkHealer_SmallJSON(b *testing.B) {
	input := `{"name": "test", "value": 42}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		healer := NewHealer(context.Background(), strings.NewReader(input))
		for range healer.Tokens() {
		}
		healer.Close()
	}
}
