package healer

import (
	"testing"
)

func TestCloser_BasicObjects(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "complete object",
			input:    `{"name": "John"}`,
			expected: "",
		},
		{
			name:     "unclosed object",
			input:    `{"name": "John"`,
			expected: "}",
		},
		{
			name:     "unclosed nested objects",
			input:    `{"user": {"name": "John"`,
			expected: "}}",
		},
		{
			name:     "unclosed object after colon",
			input:    `{"name":`,
			expected: "null}",
		},
		{
			name:     "unclosed object after comma",
			input:    `{"name": "John",`,
			expected: `"":"null"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCloser()
			defer c.Free()

			c.Feed([]byte(tt.input))
			got := string(c.Closure())

			if got != tt.expected {
				t.Errorf("Closure() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCloser_Arrays(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "complete array",
			input:    `[1, 2, 3]`,
			expected: "",
		},
		{
			name:     "unclosed array",
			input:    `[1, 2, 3`,
			expected: "]",
		},
		{
			name:     "unclosed nested arrays",
			input:    `[[1, 2], [3, 4`,
			expected: "]]",
		},
		{
			name:     "unclosed array after comma",
			input:    `[1, 2,`,
			expected: "null]",
		},
		{
			name:     "mixed array and object",
			input:    `[{"name": "John"`,
			expected: "}]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCloser()
			defer c.Free()

			c.Feed([]byte(tt.input))
			got := string(c.Closure())

			if got != tt.expected {
				t.Errorf("Closure() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCloser_Strings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "complete string",
			input:    `"hello"`,
			expected: "",
		},
		{
			name:     "unclosed string",
			input:    `{"name": "John`,
			expected: `"}`,
		},
		{
			name:     "unclosed string with escape",
			input:    `{"name": "John \"The`,
			expected: `"}`,
		},
		{
			name:     "unclosed string with unicode escape",
			input:    `{"name": "\u0048`,
			expected: `"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCloser()
			defer c.Free()

			c.Feed([]byte(tt.input))
			got := string(c.Closure())

			if got != tt.expected {
				t.Errorf("Closure() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCloser_Literals(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "partial true - t",
			input:    `{"flag": t`,
			expected: "rue}",
		},
		{
			name:     "partial true - tr",
			input:    `{"flag": tr`,
			expected: "ue}",
		},
		{
			name:     "partial true - tru",
			input:    `{"flag": tru`,
			expected: "e}",
		},
		{
			name:     "partial false - f",
			input:    `{"flag": f`,
			expected: "alse}",
		},
		{
			name:     "partial false - fa",
			input:    `{"flag": fa`,
			expected: "lse}",
		},
		{
			name:     "partial false - fal",
			input:    `{"flag": fal`,
			expected: "se}",
		},
		{
			name:     "partial false - fals",
			input:    `{"flag": fals`,
			expected: "e}",
		},
		{
			name:     "partial null - n",
			input:    `{"value": n`,
			expected: "ull}",
		},
		{
			name:     "partial null - nu",
			input:    `{"value": nu`,
			expected: "ll}",
		},
		{
			name:     "partial null - nul",
			input:    `{"value": nul`,
			expected: "l}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCloser()
			defer c.Free()

			c.Feed([]byte(tt.input))
			got := string(c.Closure())

			if got != tt.expected {
				t.Errorf("Closure() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCloser_Numbers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "complete number",
			input:    `{"count": 42}`,
			expected: "",
		},
		{
			name:     "unclosed after number",
			input:    `{"count": 42`,
			expected: "}",
		},
		{
			name:     "unclosed in array after number",
			input:    `[1, 2, 3`,
			expected: "]",
		},
		{
			name:     "negative number",
			input:    `{"value": -123`,
			expected: "}",
		},
		{
			name:     "decimal number",
			input:    `{"value": 3.14`,
			expected: "}",
		},
		{
			name:     "scientific notation",
			input:    `{"value": 1.5e10`,
			expected: "}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCloser()
			defer c.Free()

			c.Feed([]byte(tt.input))
			got := string(c.Closure())

			if got != tt.expected {
				t.Errorf("Closure() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestHeal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "complete JSON unchanged",
			input:    `{"name": "John"}`,
			expected: `{"name": "John"}`,
		},
		{
			name:     "unclosed object",
			input:    `{"name": "John"`,
			expected: `{"name": "John"}`,
		},
		{
			name:     "deeply nested unclosed",
			input:    `{"users": [{"name": "John", "address": {"city": "NYC"`,
			expected: `{"users": [{"name": "John", "address": {"city": "NYC"}}]}`,
		},
		{
			name:     "unclosed string",
			input:    `{"name": "Jo`,
			expected: `{"name": "Jo"}`,
		},
		{
			name:     "partial literal",
			input:    `{"active": tr`,
			expected: `{"active": true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HealString(tt.input)
			if got != tt.expected {
				t.Errorf("HealString() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCloser_Depth(t *testing.T) {
	c := NewCloser()
	defer c.Free()

	if c.Depth() != 0 {
		t.Errorf("Initial depth = %d, want 0", c.Depth())
	}

	c.Feed([]byte(`{`))
	if c.Depth() != 1 {
		t.Errorf("After { depth = %d, want 1", c.Depth())
	}

	c.Feed([]byte(`"items": [`))
	if c.Depth() != 2 {
		t.Errorf("After [ depth = %d, want 2", c.Depth())
	}

	c.Feed([]byte(`{"nested": true`))
	if c.Depth() != 3 {
		t.Errorf("After nested { depth = %d, want 3", c.Depth())
	}

	c.Feed([]byte(`}`))
	if c.Depth() != 2 {
		t.Errorf("After } depth = %d, want 2", c.Depth())
	}

	c.Feed([]byte(`]}`))
	if c.Depth() != 0 {
		t.Errorf("After ]} depth = %d, want 0", c.Depth())
	}
}

func BenchmarkCloser_Feed(b *testing.B) {
	input := []byte(`{"users": [{"name": "John", "age": 30}, {"name": "Jane", "age": 25}], "count": 2}`)

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c := NewCloser()
		c.Feed(input)
		c.Free()
	}
}

func BenchmarkHeal(b *testing.B) {
	input := []byte(`{"users": [{"name": "John", "age": 30}, {"name": "Jane", "age": 25}], "count": 2`)

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Heal(input)
	}
}
