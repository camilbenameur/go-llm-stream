package scanner

import (
	"bytes"
	"testing"
)

func TestScannerBasicTypes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []ScanResult
	}{
		{
			name:  "empty object",
			input: "{}",
			want:  []ScanResult{ScanBeginObject, ScanEndObject},
		},
		{
			name:  "empty array",
			input: "[]",
			want:  []ScanResult{ScanBeginArray, ScanEndArray},
		},
		{
			name:  "string value",
			input: `"hello"`,
			want:  []ScanResult{ScanBeginLiteral, ScanContinue, ScanContinue, ScanContinue, ScanContinue, ScanContinue, ScanContinue},
		},
		{
			name:  "number value",
			input: "123",
			want:  []ScanResult{ScanBeginLiteral, ScanContinue, ScanContinue},
		},
		{
			name:  "true literal",
			input: "true",
			want:  []ScanResult{ScanBeginLiteral, ScanContinue, ScanContinue, ScanContinue},
		},
		{
			name:  "false literal",
			input: "false",
			want:  []ScanResult{ScanBeginLiteral, ScanContinue, ScanContinue, ScanContinue, ScanContinue},
		},
		{
			name:  "null literal",
			input: "null",
			want:  []ScanResult{ScanBeginLiteral, ScanContinue, ScanContinue, ScanContinue},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			defer s.Free()

			var results []ScanResult
			for _, c := range []byte(tt.input) {
				r := s.Step(c)
				if r != ScanSkipSpace {
					results = append(results, r)
				}
			}

			if len(results) != len(tt.want) {
				t.Errorf("got %d results, want %d", len(results), len(tt.want))
				t.Errorf("got: %v", results)
				return
			}

			for i, r := range results {
				if r != tt.want[i] {
					t.Errorf("result[%d] = %v, want %v", i, r, tt.want[i])
				}
			}
		})
	}
}

func TestScannerNestedObjects(t *testing.T) {
	s := New()
	defer s.Free()

	input := `{"a": {"b": 1}}`

	for _, c := range []byte(input) {
		r := s.Step(c)
		if r == ScanError {
			t.Fatalf("unexpected error: %v", s.Err())
		}
	}

	if !s.EndTop() {
		t.Error("expected EndTop to be true")
	}
}

func TestScannerResumeFromSnapshot(t *testing.T) {
	// Parse partial input
	s1 := New()
	input1 := `{"key": "val`

	for _, c := range []byte(input1) {
		r := s1.Step(c)
		if r == ScanError {
			t.Fatalf("unexpected error in first part: %v", s1.Err())
		}
	}

	// Take a snapshot
	snap := s1.Snapshot()
	s1.Free()

	// Create new scanner and restore
	s2 := New()
	s2.Restore(snap)

	// Continue with remaining input
	input2 := `ue"}`

	for _, c := range []byte(input2) {
		r := s2.Step(c)
		if r == ScanError {
			t.Fatalf("unexpected error in second part: %v", s2.Err())
		}
	}

	if !s2.EndTop() {
		t.Error("expected EndTop to be true after resume")
	}
	s2.Free()
}

func TestScannerDepth(t *testing.T) {
	s := New()
	defer s.Free()

	input := `[[[{"a":[1]}]]]`

	maxDepth := 0
	for _, c := range []byte(input) {
		s.Step(c)
		if s.Depth() > maxDepth {
			maxDepth = s.Depth()
		}
	}

	if maxDepth != 5 { // 3 arrays + 1 object + 1 array
		t.Errorf("maxDepth = %d, want 5", maxDepth)
	}
}

func TestScannerEscapeSequences(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"newline", `"hello\nworld"`},
		{"tab", `"hello\tworld"`},
		{"backslash", `"hello\\world"`},
		{"quote", `"hello\"world"`},
		{"unicode", `"hello\u0041world"`},
		{"all escapes", `"\b\f\n\r\t\\\/\""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			defer s.Free()

			for _, c := range []byte(tt.input) {
				r := s.Step(c)
				if r == ScanError {
					t.Fatalf("unexpected error: %v", s.Err())
				}
			}
		})
	}
}

func TestScannerNumbers(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"integer", "123"},
		{"negative", "-456"},
		{"zero", "0"},
		{"decimal", "123.456"},
		{"exponent", "1e10"},
		{"negative exponent", "1e-10"},
		{"full", "-123.456e+10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			defer s.Free()

			for _, c := range []byte(tt.input) {
				r := s.Step(c)
				if r == ScanError {
					t.Fatalf("unexpected error for %q: %v", tt.input, s.Err())
				}
			}
		})
	}
}

func TestScannerInvalidJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		// Incomplete literals are NOT errors - they're just incomplete
		// (waiting for more data in a stream)
		{"incomplete true", "tru", false},
		// At top level, number followed by letter is valid (number ends, new value starts)
		// But inside an array, it would trigger error when trying to parse 'a' as value
		{"number then letter at top", "123a", false}, // 'a' triggers end of number, then error on 'a' as value
		// Invalid escape sequence
		{"bad escape", `"\x00"`, true},
		// Unclosed string is just incomplete, not an error
		{"unclosed string", `"hello`, false},
		// Bad literal with terminator triggers error
		{"bad true terminated", "trux", true},
		// Inside an array, number followed by letter is an error
		{"bad number in array", "[123a]", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			defer s.Free()

			var gotError bool
			for _, c := range []byte(tt.input) {
				r := s.Step(c)
				if r == ScanError {
					gotError = true
					break
				}
			}

			if gotError != tt.wantError {
				if tt.wantError {
					t.Errorf("expected error for %q but got none", tt.input)
				} else {
					t.Errorf("unexpected error for %q: %v", tt.input, s.Err())
				}
			}
		})
	}
}

func TestScannerWhitespace(t *testing.T) {
	s := New()
	defer s.Free()

	// Input with lots of whitespace
	input := `  {  "key"  :  "value"  }  `

	for _, c := range []byte(input) {
		r := s.Step(c)
		if r == ScanError {
			t.Fatalf("unexpected error: %v", s.Err())
		}
	}

	if !s.EndTop() {
		t.Error("expected EndTop to be true")
	}
}

func TestScannerPoolReuse(t *testing.T) {
	// Get and free multiple scanners to test pool
	for i := 0; i < 100; i++ {
		s := New()
		input := `{"test": 123}`
		for _, c := range []byte(input) {
			s.Step(c)
		}
		s.Free()
	}
}

// TestScannerByteByByte is the critical test from the implementation guide:
// "Create a test case with a JSON string split into 1-byte chunks"
func TestScannerByteByByte(t *testing.T) {
	testCases := []string{
		`{"name": "John", "age": 30, "active": true, "balance": null}`,
		`[1, 2, 3, {"nested": [4, 5]}]`,
		`{"unicode": "\u0041\u0042\u0043"}`,
		`{"escape": "line1\nline2\ttab"}`,
		`{"deep": {"level1": {"level2": {"level3": "value"}}}}`,
		`[-123.456e-10, 0, 1.5e+10]`,
	}

	for _, input := range testCases {
		t.Run("", func(t *testing.T) {
			s := New()
			defer s.Free()

			// Feed one byte at a time
			for i, c := range []byte(input) {
				r := s.Step(c)
				if r == ScanError {
					t.Fatalf("error at byte %d (%c): %v", i, c, s.Err())
				}
			}
		})
	}
}

// Benchmark the scanner with various input sizes
func BenchmarkScannerSmall(b *testing.B) {
	input := []byte(`{"name": "John", "age": 30}`)
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		s := New()
		for _, c := range input {
			s.Step(c)
		}
		s.Free()
	}
}

func BenchmarkScannerMedium(b *testing.B) {
	// Build a larger JSON
	var buf bytes.Buffer
	buf.WriteString(`{"items": [`)
	for i := 0; i < 100; i++ {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(`{"id": `)
		buf.WriteString(itoa(int64(i)))
		buf.WriteString(`, "name": "item"}`)
	}
	buf.WriteString(`]}`)
	input := buf.Bytes()

	b.SetBytes(int64(len(input)))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		s := New()
		for _, c := range input {
			s.Step(c)
		}
		s.Free()
	}
}
