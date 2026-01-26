package scanner

import (
	"testing"
)

func TestTokenizerBasicObjects(t *testing.T) {
	tok := NewTokenizer()
	defer tok.Free()

	tok.Append([]byte(`{"name": "John", "age": 30}`))

	expected := []TokenKind{
		TokenObjectStart,
		TokenString, // "name" (key)
		TokenString, // "John" (value)
		TokenComma,
		TokenString, // "age" (key)
		TokenNumber, // 30
		TokenObjectEnd,
	}

	for i, want := range expected {
		token := tok.NextToken()
		if token.Kind != want {
			t.Errorf("token[%d] = %v, want %v", i, token.Kind, want)
		}
		if !token.Completed && want != TokenIncomplete {
			t.Errorf("token[%d] not completed", i)
		}
	}
}

func TestTokenizerBasicArray(t *testing.T) {
	tok := NewTokenizer()
	defer tok.Free()

	tok.Append([]byte(`[1, 2, 3]`))

	expected := []TokenKind{
		TokenArrayStart,
		TokenNumber, // 1
		TokenComma,
		TokenNumber, // 2
		TokenComma,
		TokenNumber, // 3
		TokenArrayEnd,
	}

	for i, want := range expected {
		token := tok.NextToken()
		if token.Kind != want {
			t.Errorf("token[%d] = %v, want %v", i, token.Kind, want)
		}
	}
}

func TestTokenizerBoolAndNull(t *testing.T) {
	tok := NewTokenizer()
	defer tok.Free()

	tok.Append([]byte(`[true, false, null]`))

	expected := []TokenKind{
		TokenArrayStart,
		TokenBool,
		TokenComma,
		TokenBool,
		TokenComma,
		TokenNull,
		TokenArrayEnd,
	}

	for i, want := range expected {
		token := tok.NextToken()
		if token.Kind != want {
			t.Errorf("token[%d] = %v, want %v", i, token.Kind, want)
		}
	}
}

// TestTokenizerChunkedInput is the critical resumability test:
// Split JSON into 1-byte chunks and verify it still parses correctly
func TestTokenizerChunkedInput(t *testing.T) {
	input := `{"name": "John", "age": 30, "active": true}`

	// Parse in single chunk as reference
	tokFull := NewTokenizer()
	tokFull.Append([]byte(input))

	var fullTokens []Token
	for {
		token := tokFull.NextToken()
		if token.Kind == TokenIncomplete || token.Kind == TokenEOF {
			break
		}
		if token.Kind == TokenError {
			t.Fatalf("error in full parse: %v", token.Err)
		}
		fullTokens = append(fullTokens, token)
	}
	tokFull.Free()

	// Parse in 1-byte chunks
	tokChunked := NewTokenizer()

	var chunkedTokens []Token
	for _, c := range []byte(input) {
		tokChunked.Append([]byte{c})

		// Drain all complete tokens
		for {
			token := tokChunked.NextToken()
			if token.Kind == TokenIncomplete {
				break
			}
			if token.Kind == TokenError {
				t.Fatalf("error in chunked parse: %v", token.Err)
			}
			if token.Kind == TokenEOF {
				break
			}
			chunkedTokens = append(chunkedTokens, token)
		}
	}
	tokChunked.Free()

	// Compare results
	if len(chunkedTokens) != len(fullTokens) {
		t.Fatalf("got %d tokens, want %d", len(chunkedTokens), len(fullTokens))
	}

	for i := range fullTokens {
		if chunkedTokens[i].Kind != fullTokens[i].Kind {
			t.Errorf("token[%d].Kind = %v, want %v",
				i, chunkedTokens[i].Kind, fullTokens[i].Kind)
		}
	}
}

func TestTokenizerIncompleteString(t *testing.T) {
	tok := NewTokenizer()
	defer tok.Free()

	// Incomplete string
	tok.Append([]byte(`{"key": "val`))

	// Should get ObjectStart, String (key), then Incomplete
	token := tok.NextToken()
	if token.Kind != TokenObjectStart {
		t.Errorf("expected ObjectStart, got %v", token.Kind)
	}

	token = tok.NextToken()
	if token.Kind != TokenString {
		t.Errorf("expected String (key), got %v", token.Kind)
	}

	token = tok.NextToken()
	if token.Kind != TokenIncomplete {
		t.Errorf("expected Incomplete, got %v", token.Kind)
	}

	// Complete the string
	tok.Append([]byte(`ue"}`))

	token = tok.NextToken()
	if token.Kind != TokenString {
		t.Errorf("expected String (value), got %v", token.Kind)
	}

	token = tok.NextToken()
	if token.Kind != TokenObjectEnd {
		t.Errorf("expected ObjectEnd, got %v", token.Kind)
	}
}

func TestTokenizerIncompleteNumber(t *testing.T) {
	tok := NewTokenizer()
	defer tok.Free()

	tok.Append([]byte(`[123`))

	token := tok.NextToken()
	if token.Kind != TokenArrayStart {
		t.Errorf("expected ArrayStart, got %v", token.Kind)
	}

	token = tok.NextToken()
	if token.Kind != TokenIncomplete {
		t.Errorf("expected Incomplete for number, got %v", token.Kind)
	}

	// Complete with terminator
	tok.Append([]byte(`456]`))

	token = tok.NextToken()
	if token.Kind != TokenNumber {
		t.Errorf("expected Number, got %v", token.Kind)
	}
	// Check the full number
	if string(token.Raw) != "123456" {
		t.Errorf("number = %q, want %q", token.Raw, "123456")
	}

	token = tok.NextToken()
	if token.Kind != TokenArrayEnd {
		t.Errorf("expected ArrayEnd, got %v", token.Kind)
	}
}

func TestTokenizerKeyDetection(t *testing.T) {
	tok := NewTokenizer()
	defer tok.Free()

	tok.Append([]byte(`{"key": "value"}`))

	tok.NextToken() // ObjectStart

	keyToken := tok.NextToken()
	if !keyToken.IsKey {
		t.Error("expected key token to have IsKey=true")
	}
	if string(keyToken.Raw) != `"key"` {
		t.Errorf("key = %q, want %q", keyToken.Raw, `"key"`)
	}

	valueToken := tok.NextToken()
	if valueToken.IsKey {
		t.Error("expected value token to have IsKey=false")
	}
}

func TestTokenizerSnapshot(t *testing.T) {
	tok1 := NewTokenizer()

	// Parse partial
	tok1.Append([]byte(`{"key": "val`))
	tok1.NextToken() // ObjectStart
	tok1.NextToken() // String (key)
	tok1.NextToken() // Incomplete

	// Snapshot
	snap := tok1.Snapshot()
	tok1.Free()

	// Restore in new tokenizer
	tok2 := NewTokenizer()
	tok2.Restore(snap)
	tok2.Append([]byte(`ue"}`))

	token := tok2.NextToken()
	if token.Kind != TokenString {
		t.Errorf("expected String after restore, got %v", token.Kind)
	}

	token = tok2.NextToken()
	if token.Kind != TokenObjectEnd {
		t.Errorf("expected ObjectEnd, got %v", token.Kind)
	}
	tok2.Free()
}

func TestTokenizerDeepNesting(t *testing.T) {
	tok := NewTokenizer()
	defer tok.Free()

	tok.Append([]byte(`{"a":{"b":{"c":{"d":"value"}}}}`))

	depths := []int{}
	for {
		token := tok.NextToken()
		if token.Kind == TokenIncomplete || token.Kind == TokenEOF {
			break
		}
		depths = append(depths, tok.Depth())
	}

	// Should have increasing then decreasing depth
	maxDepth := 0
	for _, d := range depths {
		if d > maxDepth {
			maxDepth = d
		}
	}
	if maxDepth != 4 {
		t.Errorf("maxDepth = %d, want 4", maxDepth)
	}
}

func TestTokenizerUnicodeEscape(t *testing.T) {
	tok := NewTokenizer()
	defer tok.Free()

	// Unicode escape split across chunks
	chunks := []string{
		`{"text": "A\u00`,
		`41B"}`,
	}

	for _, chunk := range chunks {
		tok.Append([]byte(chunk))
	}

	// Drain tokens
	var tokens []Token
	for {
		token := tok.NextToken()
		if token.Kind == TokenIncomplete || token.Kind == TokenEOF {
			break
		}
		if token.Kind == TokenError {
			t.Fatalf("unexpected error: %v", token.Err)
		}
		tokens = append(tokens, token)
	}

	// Should have: ObjectStart, String(key), String(value), ObjectEnd
	if len(tokens) != 4 {
		t.Fatalf("got %d tokens, want 4", len(tokens))
	}
}

func TestTokenizerPoolReuse(t *testing.T) {
	for i := 0; i < 100; i++ {
		tok := NewTokenizer()
		tok.Append([]byte(`{"test": [1, 2, 3]}`))
		for {
			token := tok.NextToken()
			if token.Kind == TokenIncomplete || token.Kind == TokenEOF {
				break
			}
		}
		tok.Free()
	}
}

// Benchmark tokenizer throughput
func BenchmarkTokenizerSmall(b *testing.B) {
	input := []byte(`{"name": "John", "age": 30}`)
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tok := NewTokenizer()
		tok.Append(input)
		for {
			token := tok.NextToken()
			if token.Kind == TokenIncomplete || token.Kind == TokenEOF {
				break
			}
		}
		tok.Free()
	}
}

func BenchmarkTokenizerChunked(b *testing.B) {
	input := []byte(`{"name": "John", "age": 30, "items": [1, 2, 3, 4, 5]}`)
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tok := NewTokenizer()
		// Feed one byte at a time (worst case)
		for _, c := range input {
			tok.Append([]byte{c})
			for {
				token := tok.NextToken()
				if token.Kind == TokenIncomplete {
					break
				}
				if token.Kind == TokenEOF {
					break
				}
			}
		}
		tok.Free()
	}
}
