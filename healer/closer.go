package healer

import (
	"bytes"
	"sync"
)

// ClosureKind represents the type of JSON container that needs closing.
type ClosureKind uint8

const (
	ClosureObject ClosureKind = iota // Needs '}'
	ClosureArray                     // Needs ']'
	ClosureString                    // Needs '"'
)

// String returns a human-readable name for the closure kind.
func (c ClosureKind) String() string {
	switch c {
	case ClosureObject:
		return "Object"
	case ClosureArray:
		return "Array"
	case ClosureString:
		return "String"
	default:
		return "Unknown"
	}
}

// ClosingByte returns the byte needed to close this container.
func (c ClosureKind) ClosingByte() byte {
	switch c {
	case ClosureObject:
		return '}'
	case ClosureArray:
		return ']'
	case ClosureString:
		return '"'
	default:
		return 0
	}
}

// Closer implements the Minimal Closure Algorithm for JSON healing.
// It tracks the current parse state and can generate the minimal
// sequence of bytes needed to make incomplete JSON valid.
type Closer struct {
	// Stack of open containers (objects and arrays)
	stack []ClosureKind

	// Current parsing state
	inString    bool // Inside a string literal
	inEscape    bool // After a backslash in a string
	afterColon  bool // After ':' expecting a value
	afterComma  bool // After ',' in object (expecting key) or array (expecting value)
	inNumber    bool // Inside a number literal
	inLiteral   bool // Inside a literal (true, false, null)
	literalBuf  []byte
	escapeCount int  // Count of hex digits after \u
	pendingKey  bool // Innermost object has an open/completed key with no ':value' yet
}

// Pool for closer reuse
var closerPool = sync.Pool{
	New: func() any {
		return &Closer{
			stack:      make([]ClosureKind, 0, 16),
			literalBuf: make([]byte, 0, 8),
		}
	},
}

// NewCloser returns a new Closer from the pool.
func NewCloser() *Closer {
	c := closerPool.Get().(*Closer)
	c.Reset()
	return c
}

// Free returns the Closer to the pool.
func (c *Closer) Free() {
	if cap(c.stack) > 1024 {
		c.stack = nil
	}
	closerPool.Put(c)
}

// Reset resets the Closer to its initial state.
func (c *Closer) Reset() {
	c.stack = c.stack[:0]
	c.inString = false
	c.inEscape = false
	c.afterColon = false
	c.afterComma = false
	c.inNumber = false
	c.inLiteral = false
	c.literalBuf = c.literalBuf[:0]
	c.escapeCount = 0
	c.pendingKey = false
}

// Feed processes input bytes and updates the internal state.
func (c *Closer) Feed(data []byte) {
	for _, b := range data {
		c.feedByte(b)
	}
}

// feedByte processes a single byte.
func (c *Closer) feedByte(b byte) {
	// Handle string state
	if c.inString {
		if c.inEscape {
			c.inEscape = false
			if b == 'u' {
				c.escapeCount = 4
			}
			return
		}
		if c.escapeCount > 0 {
			c.escapeCount--
			return
		}
		if b == '\\' {
			c.inEscape = true
			return
		}
		if b == '"' {
			c.inString = false
			if !c.afterColon && len(c.stack) > 0 && c.stack[len(c.stack)-1] == ClosureObject {
				// This string just closed in object-key position: it is a
				// key awaiting ':value'.
				c.pendingKey = true
			} else {
				c.pendingKey = false
			}
			c.afterColon = false
			c.afterComma = false
		}
		return
	}

	// Handle number state
	if c.inNumber {
		if isNumberChar(b) {
			return
		}
		c.inNumber = false
		c.afterColon = false
		c.afterComma = false
		// Fall through to process this byte
	}

	// Handle literal state (true, false, null)
	if c.inLiteral {
		if b >= 'a' && b <= 'z' {
			c.literalBuf = append(c.literalBuf, b)
			return
		}
		c.inLiteral = false
		c.literalBuf = c.literalBuf[:0]
		c.afterColon = false
		c.afterComma = false
		// Fall through to process this byte
	}

	// Skip whitespace
	if isSpace(b) {
		return
	}

	switch b {
	case '{':
		c.stack = append(c.stack, ClosureObject)
		c.afterColon = false
		c.afterComma = false

	case '[':
		c.stack = append(c.stack, ClosureArray)
		c.afterColon = false
		c.afterComma = false

	case '}':
		if len(c.stack) > 0 && c.stack[len(c.stack)-1] == ClosureObject {
			c.stack = c.stack[:len(c.stack)-1]
		}
		c.afterColon = false
		c.afterComma = false
		c.pendingKey = false

	case ']':
		if len(c.stack) > 0 && c.stack[len(c.stack)-1] == ClosureArray {
			c.stack = c.stack[:len(c.stack)-1]
		}
		c.afterColon = false
		c.afterComma = false
		c.pendingKey = false

	case '"':
		c.inString = true
		c.inEscape = false
		c.escapeCount = 0

	case ':':
		c.afterColon = true
		c.afterComma = false
		c.pendingKey = false

	case ',':
		c.afterComma = true
		c.afterColon = false

	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		c.inNumber = true
		c.afterColon = false
		c.afterComma = false

	case 't', 'f', 'n':
		c.inLiteral = true
		c.literalBuf = c.literalBuf[:0]
		c.literalBuf = append(c.literalBuf, b)
		c.afterColon = false
		c.afterComma = false
	}
}

// Closure returns the minimal sequence of bytes needed to close all
// open containers and make the JSON valid.
func (c *Closer) Closure() []byte {
	var buf bytes.Buffer
	buf.Grow(len(c.stack) + 4) // Pre-allocate for efficiency

	// Close incomplete string
	if c.inString {
		buf.WriteByte('"')
		if !c.afterColon && len(c.stack) > 0 && c.stack[len(c.stack)-1] == ClosureObject {
			// The string being closed is an object key with no ':value'
			// yet - supply a placeholder value to keep the pair valid.
			buf.WriteString(`:"null"`)
		}
		// String counts as a value, so clear afterColon/afterComma status
		// since the value was provided (the string itself)
	} else if c.inLiteral {
		// Complete incomplete literal
		suffix := c.literalSuffix()
		buf.WriteString(suffix)
	} else if c.inNumber {
		// Number is already complete, no suffix needed
	} else {
		// Handle trailing comma - need a placeholder value
		if c.afterComma && len(c.stack) > 0 {
			switch c.stack[len(c.stack)-1] {
			case ClosureArray:
				buf.WriteString("null")
			case ClosureObject:
				// After comma in object, we need a key-value pair
				buf.WriteString(`"":"null"`)
			}
		}

		// Handle after colon - need a value
		if c.afterColon {
			buf.WriteString("null")
		}

		// A completed object key with no ':value' yet (e.g. `{"a"`)
		// needs a placeholder value to form a valid key:value pair.
		if c.pendingKey {
			buf.WriteString(`:"null"`)
		}
	}

	// Close all open containers in reverse order
	for i := len(c.stack) - 1; i >= 0; i-- {
		buf.WriteByte(c.stack[i].ClosingByte())
	}

	return buf.Bytes()
}

// literalSuffix returns the remaining bytes to complete a literal.
func (c *Closer) literalSuffix() string {
	s := string(c.literalBuf)
	switch {
	case s == "t":
		return "rue"
	case s == "tr":
		return "ue"
	case s == "tru":
		return "e"
	case s == "f":
		return "alse"
	case s == "fa":
		return "lse"
	case s == "fal":
		return "se"
	case s == "fals":
		return "e"
	case s == "n":
		return "ull"
	case s == "nu":
		return "ll"
	case s == "nul":
		return "l"
	default:
		return ""
	}
}

// Depth returns the current nesting depth.
func (c *Closer) Depth() int {
	return len(c.stack)
}

// InString returns true if currently inside a string literal.
func (c *Closer) InString() bool {
	return c.inString
}

// isSpace returns true if b is a JSON whitespace character.
func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// isNumberChar returns true if b is valid in a JSON number.
func isNumberChar(b byte) bool {
	return (b >= '0' && b <= '9') || b == '.' || b == '-' || b == '+' || b == 'e' || b == 'E'
}

// Heal takes incomplete JSON bytes and returns valid JSON by appending
// the minimal closure sequence.
func Heal(data []byte) []byte {
	c := NewCloser()
	defer c.Free()

	c.Feed(data)
	closure := c.Closure()

	if len(closure) == 0 {
		return data
	}

	result := make([]byte, len(data)+len(closure))
	copy(result, data)
	copy(result[len(data):], closure)
	return result
}

// HealString is a convenience wrapper for string input.
func HealString(s string) string {
	return string(Heal([]byte(s)))
}
