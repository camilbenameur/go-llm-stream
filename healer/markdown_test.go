package healer

import (
	"strings"
	"testing"
)

func TestMarkdownFilter_StripCodeBlock(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no markdown",
			input:    `{"name": "John"}`,
			expected: `{"name": "John"}`,
		},
		{
			name:     "json code block",
			input:    "```json\n{\"name\": \"John\"}\n```",
			expected: "{\"name\": \"John\"}\n",
		},
		{
			name:     "code block with trailing text",
			input:    "```json\n{\"name\": \"John\"}\n```\nHere's the JSON output!",
			expected: "{\"name\": \"John\"}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripMarkdownString(tt.input)

			if got != tt.expected {
				t.Errorf("StripMarkdownString() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestMarkdownFilter_Streaming(t *testing.T) {
	// Simulate streaming where data arrives in chunks
	chunks := []string{
		"```js",
		"on\n",
		`{"name"`,
		`: "John"`,
		"}\n``",
		"`\nDone!",
	}

	f := NewMarkdownFilter()
	defer f.Free()

	var result strings.Builder
	for _, chunk := range chunks {
		filtered := f.Filter([]byte(chunk))
		result.Write(filtered)
	}
	result.Write(f.Flush())

	got := strings.TrimSpace(result.String())
	expected := `{"name": "John"}`

	if got != expected {
		t.Errorf("Streaming result = %q, want %q", got, expected)
	}
}

func TestMarkdownFilter_NoClosingDelimiter(t *testing.T) {
	// Handle case where LLM truncates before closing ```
	input := "```json\n{\"name\": \"John\"}"

	got := StripMarkdownString(input)
	expected := `{"name": "John"}`

	if got != expected {
		t.Errorf("StripMarkdownString() = %q, want %q", got, expected)
	}
}

func TestMarkdownFilter_MultipleCodeBlocks(t *testing.T) {
	// Only the first code block is extracted
	input := "```json\n{\"first\": true}\n```\nText\n```json\n{\"second\": true}\n```"

	got := StripMarkdownString(input)
	expected := "{\"first\": true}\n" // Includes newline before closing ```

	if got != expected {
		t.Errorf("StripMarkdownString() = %q, want %q", got, expected)
	}
}

func TestMarkdownFilter_Reset(t *testing.T) {
	f := NewMarkdownFilter()
	defer f.Free()

	// First use
	_ = f.Filter([]byte("```json\n{\"a\": 1}\n```"))
	f.Flush()

	// Reset and reuse
	f.Reset()

	result := f.Filter([]byte("```json\n{\"b\": 2}\n```"))
	result = append(result, f.Flush()...)

	got := strings.TrimSpace(string(result))
	expected := `{"b": 2}`

	if got != expected {
		t.Errorf("After Reset() = %q, want %q", got, expected)
	}
}

func BenchmarkStripMarkdown(b *testing.B) {
	input := []byte("```json\n{\"users\": [{\"name\": \"John\", \"age\": 30}, {\"name\": \"Jane\", \"age\": 25}]}\n```")

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = StripMarkdown(input)
	}
}

func BenchmarkMarkdownFilter_Streaming(b *testing.B) {
	chunks := [][]byte{
		[]byte("```json\n"),
		[]byte(`{"name": "John", "age": 30, "items": [`),
		[]byte(`1, 2, 3, 4, 5`),
		[]byte(`]}`),
		[]byte("\n```"),
	}

	b.ReportAllocs()

	totalBytes := 0
	for _, c := range chunks {
		totalBytes += len(c)
	}
	b.SetBytes(int64(totalBytes))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		f := NewMarkdownFilter()
		for _, chunk := range chunks {
			_ = f.Filter(chunk)
		}
		_ = f.Flush()
		f.Free()
	}
}
