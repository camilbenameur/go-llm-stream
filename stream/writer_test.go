package stream

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// collectReaderTokens reads all tokens (up to and including TokenEOF) from
// the reader-based API for input, returning them for comparison.
func collectReaderTokens(t *testing.T, input string) []Token {
	t.Helper()
	reader := NewReader(context.Background(), strings.NewReader(input))
	defer reader.Close()

	var tokens []Token
	for {
		tok := reader.NextToken()
		tokens = append(tokens, normalizeToken(tok))
		if tok.Kind == TokenEOF || tok.Kind == TokenError {
			break
		}
	}
	return tokens
}

// collectWriterTokens feeds input to a Writer in chunks defined by
// chunkSizes (cycled if shorter than input), then closes it, and returns
// all tokens observed via both OnToken and NextToken.
func collectWriterTokens(t *testing.T, input string, chunkSizes []int) []Token {
	t.Helper()

	w := NewWriter()

	var onTokenTokens []Token
	w.OnToken = func(tok Token) {
		onTokenTokens = append(onTokenTokens, normalizeToken(tok))
	}

	data := []byte(input)
	pos := 0
	chunkIdx := 0
	for pos < len(data) {
		size := chunkSizes[chunkIdx%len(chunkSizes)]
		if size <= 0 {
			size = 1
		}
		end := pos + size
		if end > len(data) {
			end = len(data)
		}
		n, err := w.Write(data[pos:end])
		if err != nil {
			t.Fatalf("Write returned error: %v", err)
		}
		if n != end-pos {
			t.Fatalf("Write returned n=%d, expected %d", n, end-pos)
		}
		pos = end
		chunkIdx++
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	// Drain NextToken and verify it matches OnToken exactly.
	var nextTokenTokens []Token
	for {
		tok, ok := w.NextToken()
		if !ok {
			break
		}
		nextTokenTokens = append(nextTokenTokens, normalizeToken(tok))
	}

	if len(onTokenTokens) != len(nextTokenTokens) {
		t.Fatalf("OnToken produced %d tokens but NextToken produced %d",
			len(onTokenTokens), len(nextTokenTokens))
	}
	for i := range onTokenTokens {
		if !tokensEqual(onTokenTokens[i], nextTokenTokens[i]) {
			t.Fatalf("OnToken[%d] = %+v != NextToken[%d] = %+v",
				i, onTokenTokens[i], i, nextTokenTokens[i])
		}
	}

	return onTokenTokens
}

// normalizeToken copies the Raw slice (since Tokenizer reuses buffers) and
// strips fields that aren't relevant for cross-implementation comparison
// (byte offsets depend only on input position, which is identical between
// reader and writer paths, so we keep those - but errors are not
// comparable directly so we normalize them to a boolean).
func normalizeToken(tok Token) Token {
	var raw []byte
	if tok.Raw != nil {
		raw = make([]byte, len(tok.Raw))
		copy(raw, tok.Raw)
	}
	out := tok
	out.Raw = raw
	if tok.Err != nil {
		// Preserve only the fact that an error occurred; error values
		// themselves are not comparable across implementations.
		out.Err = errSentinel
	}
	return out
}

var errSentinel = errors.New("sentinel")

func tokensEqual(a, b Token) bool {
	if a.Kind != b.Kind {
		return false
	}
	if string(a.Raw) != string(b.Raw) {
		return false
	}
	if a.Start != b.Start || a.End != b.End {
		return false
	}
	if a.Completed != b.Completed {
		return false
	}
	if a.IsKey != b.IsKey {
		return false
	}
	if (a.Err == nil) != (b.Err == nil) {
		return false
	}
	return true
}

// TestWriter_MatchesReader_ChunkBoundaries verifies that feeding identical
// input through the push-based Writer, split at various chunk boundaries
// (including mid-string-literal and mid-number splits), produces the exact
// same token sequence as the pull-based Reader.
func TestWriter_MatchesReader_ChunkBoundaries(t *testing.T) {
	inputs := map[string]string{
		"simple_object":    `{"name": "test", "value": 42}`,
		"nested":           `{"a": {"b": [1, 2, 3.14159], "c": "hello world"}, "d": null, "e": true, "f": false}`,
		"long_string":      `{"message": "the quick brown fox jumps over the lazy dog, repeatedly and at length"}`,
		"numbers":          `[0, -1, 1.5, -2.25, 1e10, -3.14e-7, 1000000]`,
		"escaped_string":   `{"text": "line1\nline2\ttabbed \"quoted\" \\backslash\\"}`,
		"array_of_objects": `[{"id": 1}, {"id": 2}, {"id": 3}]`,
	}

	chunkSizePatterns := map[string][]int{
		"whole_input_at_once": {1 << 20},
		"one_byte_at_a_time":  {1},
		"two_bytes":           {2},
		"three_bytes":         {3},
		"five_bytes":          {5},
		"varying_1_2_3":       {1, 2, 3},
		"varying_7_1":         {7, 1},
	}

	for inputName, input := range inputs {
		expected := collectReaderTokens(t, input)

		for patternName, chunks := range chunkSizePatterns {
			t.Run(inputName+"/"+patternName, func(t *testing.T) {
				got := collectWriterTokens(t, input, chunks)

				if len(got) != len(expected) {
					t.Fatalf("token count mismatch: writer=%d reader=%d\nwriter=%+v\nreader=%+v",
						len(got), len(expected), got, expected)
				}

				for i := range expected {
					if !tokensEqual(got[i], expected[i]) {
						t.Errorf("token %d mismatch:\n  writer = %+v (raw=%q)\n  reader = %+v (raw=%q)",
							i, got[i], string(got[i].Raw), expected[i], string(expected[i].Raw))
					}
				}
			})
		}
	}
}

// TestWriter_SplitInsideStringLiteral specifically checks that a string
// literal split mid-way across two Write calls is not emitted until it is
// complete.
func TestWriter_SplitInsideStringLiteral(t *testing.T) {
	w := NewWriter()

	var tokens []Token
	w.OnToken = func(tok Token) {
		tokens = append(tokens, tok)
	}

	// {"key": "hel  -- split --  lo world"}
	if _, err := w.Write([]byte(`{"key": "hel`)); err != nil {
		t.Fatalf("Write 1: %v", err)
	}

	// At this point the string literal is incomplete; it must not have
	// been emitted as a TokenString yet.
	for _, tok := range tokens {
		if tok.Kind == TokenString && !tok.IsKey {
			t.Fatalf("string value token emitted before complete: %+v", tok)
		}
	}

	if _, err := w.Write([]byte(`lo world"}`)); err != nil {
		t.Fatalf("Write 2: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Now we should have ObjectStart, "key" (IsKey), Colon, "hel lo world", ObjectEnd, EOF
	var stringValue *Token
	for i := range tokens {
		if tokens[i].Kind == TokenString && !tokens[i].IsKey {
			stringValue = &tokens[i]
		}
	}
	if stringValue == nil {
		t.Fatal("expected a string value token after Close")
	}
	if string(stringValue.Raw) != `"hello world"` {
		t.Errorf("expected raw %q, got %q", `"hello world"`, stringValue.Raw)
	}
}

// TestWriter_SplitInsideNumber checks that a number literal split mid-way
// across two Write calls is reassembled correctly.
func TestWriter_SplitInsideNumber(t *testing.T) {
	w := NewWriter()

	var tokens []Token
	w.OnToken = func(tok Token) {
		tokens = append(tokens, tok)
	}

	// [123.456]  -- split as "[12" + "3.45" + "6]"
	if _, err := w.Write([]byte(`[12`)); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	for _, tok := range tokens {
		if tok.Kind == TokenNumber {
			t.Fatalf("number token emitted before complete: %+v", tok)
		}
	}

	if _, err := w.Write([]byte(`3.45`)); err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	for _, tok := range tokens {
		if tok.Kind == TokenNumber {
			t.Fatalf("number token emitted before complete: %+v", tok)
		}
	}

	if _, err := w.Write([]byte(`6]`)); err != nil {
		t.Fatalf("Write 3: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	var numTok *Token
	for i := range tokens {
		if tokens[i].Kind == TokenNumber {
			numTok = &tokens[i]
		}
	}
	if numTok == nil {
		t.Fatal("expected a number token")
	}
	if string(numTok.Raw) != "123.456" {
		t.Errorf("expected raw %q, got %q", "123.456", numTok.Raw)
	}
}

// TestWriter_CloseTwice verifies Close is idempotent.
func TestWriter_CloseTwice(t *testing.T) {
	w := NewWriter()
	if _, err := w.Write([]byte(`{"a": 1}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close 1: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close 2: %v", err)
	}
}

// TestWriter_WriteAfterClose verifies Write returns an error after Close.
func TestWriter_WriteAfterClose(t *testing.T) {
	w := NewWriter()
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := w.Write([]byte("x")); err == nil {
		t.Fatal("expected error writing after Close")
	}
}

// TestWriter_EmptyInput verifies behavior with no data at all.
func TestWriter_EmptyInput(t *testing.T) {
	w := NewWriter()
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	tok, ok := w.NextToken()
	if !ok {
		t.Fatal("expected at least an EOF token")
	}
	if tok.Kind != TokenEOF {
		t.Errorf("expected TokenEOF, got %s", tok.Kind)
	}
}

// TestWriter_BytesConsumedAndDepth sanity-checks the helper accessors.
func TestWriter_BytesConsumedAndDepth(t *testing.T) {
	w := NewWriter()
	input := `{"a": [1, 2`
	if _, err := w.Write([]byte(input)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if w.Depth() != 2 {
		t.Errorf("expected depth 2, got %d", w.Depth())
	}
	if got := w.BytesConsumed(); got != int64(len(input)) {
		t.Errorf("expected BytesConsumed %d, got %d", len(input), got)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
