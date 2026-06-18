package scanner

import (
	"errors"
	"sync"
)

// errTrailingData is reported when input contains non-whitespace content after
// the top-level JSON value and the tokenizer is configured to reject it.
var errTrailingData = errors.New("unexpected content after top-level JSON value")

// TokenKind represents the type of a JSON token.
type TokenKind uint8

const (
	TokenObjectStart TokenKind = iota // {
	TokenObjectEnd                    // }
	TokenArrayStart                   // [
	TokenArrayEnd                     // ]
	TokenString                       // "..."
	TokenNumber                       // 123, 1.5e-10
	TokenBool                         // true, false
	TokenNull                         // null
	TokenColon                        // :
	TokenComma                        // ,
	TokenEOF                          // End of input
	TokenIncomplete                   // Need more data
	TokenError                        // Invalid token
)

// String returns a human-readable name for the token kind.
func (k TokenKind) String() string {
	switch k {
	case TokenObjectStart:
		return "ObjectStart"
	case TokenObjectEnd:
		return "ObjectEnd"
	case TokenArrayStart:
		return "ArrayStart"
	case TokenArrayEnd:
		return "ArrayEnd"
	case TokenString:
		return "String"
	case TokenNumber:
		return "Number"
	case TokenBool:
		return "Bool"
	case TokenNull:
		return "Null"
	case TokenColon:
		return "Colon"
	case TokenComma:
		return "Comma"
	case TokenEOF:
		return "EOF"
	case TokenIncomplete:
		return "Incomplete"
	case TokenError:
		return "Error"
	default:
		return "Unknown"
	}
}

// Token represents a single JSON token with its metadata.
type Token struct {
	Kind TokenKind

	// Raw contains the raw bytes of the token (for strings, numbers, bools, null).
	// This slice is only valid until the next call to NextToken or Append.
	// Copy it if you need to keep it.
	Raw []byte

	// Start is the byte offset where this token begins.
	Start int64

	// End is the byte offset where this token ends (exclusive).
	End int64

	// Completed indicates whether this token is complete.
	// For TokenIncomplete, this is always false.
	Completed bool

	// Err contains any error if Kind is TokenError.
	Err error

	// IsKey indicates if this string token is an object key.
	IsKey bool
}

// Tokenizer wraps a Scanner and produces JSON tokens.
// It handles buffering and incomplete token management.
type Tokenizer struct {
	scanner *Scanner

	// Buffer for input data
	buffer []byte

	// Current position in buffer
	position int

	// Byte offset at start of buffer (for absolute positioning)
	bufferOffset int64

	// Start position of current token being parsed
	tokenStart int

	// Track if we're in a literal (string/number/bool/null)
	inLiteral bool

	// Track the kind of literal we're parsing
	literalKind TokenKind

	// Track if current string is an object key
	isKey bool

	// Buffer for accumulating literal content across chunks
	literalBuf []byte

	// Pending token to emit (for multi-token transitions)
	pendingToken *Token

	// rejectTrailing reports non-whitespace content after the root value as an
	// error instead of silently ending the stream.
	rejectTrailing bool
}

// Pool for tokenizer reuse
var tokenizerPool = sync.Pool{
	New: func() any {
		return &Tokenizer{
			buffer:     make([]byte, 0, 4096),
			literalBuf: make([]byte, 0, 256),
		}
	},
}

// NewTokenizer returns a new tokenizer from the pool.
func NewTokenizer() *Tokenizer {
	t := tokenizerPool.Get().(*Tokenizer)
	t.Reset()
	return t
}

// Free returns the tokenizer to the pool for reuse.
func (t *Tokenizer) Free() {
	if t.scanner != nil {
		t.scanner.Free()
		t.scanner = nil
	}
	if cap(t.buffer) > 65536 {
		t.buffer = nil
	}
	if cap(t.literalBuf) > 4096 {
		t.literalBuf = nil
	}
	tokenizerPool.Put(t)
}

// Reset resets the tokenizer to its initial state.
func (t *Tokenizer) Reset() {
	if t.scanner == nil {
		t.scanner = New()
	} else {
		t.scanner.Reset()
	}
	t.buffer = t.buffer[:0]
	t.position = 0
	t.bufferOffset = 0
	t.tokenStart = 0
	t.inLiteral = false
	t.literalKind = TokenError
	t.isKey = false
	t.literalBuf = t.literalBuf[:0]
	t.pendingToken = nil
	t.rejectTrailing = false
}

// SetRejectTrailing controls whether non-whitespace content after the top-level
// value is reported as an error (true) or silently ignored (false, the default).
func (t *Tokenizer) SetRejectTrailing(reject bool) {
	t.rejectTrailing = reject
}

// Append adds more data to the tokenizer's buffer.
// This is the primary method for feeding streaming data.
func (t *Tokenizer) Append(data []byte) {
	// If we've consumed all the buffer, reset position
	if t.position >= len(t.buffer) {
		if t.inLiteral {
			// A literal's absolute start position (bufferOffset + tokenStart)
			// must remain stable across this reset. Since the whole buffer
			// is being discarded, shift tokenStart by the same amount
			// bufferOffset advances so the sum is unchanged. tokenStart may
			// become negative; that's fine, it's only ever used relative to
			// bufferOffset.
			t.tokenStart -= len(t.buffer)
		} else {
			t.tokenStart = 0
		}
		t.bufferOffset += int64(len(t.buffer))
		t.buffer = t.buffer[:0]
		t.position = 0
	}

	// If there's unconsumed data and we're starting a new token,
	// compact the buffer
	if t.position > 0 && t.tokenStart >= t.position {
		t.bufferOffset += int64(t.position)
		copy(t.buffer, t.buffer[t.position:])
		t.buffer = t.buffer[:len(t.buffer)-t.position]
		t.tokenStart -= t.position
		t.position = 0
	}

	t.buffer = append(t.buffer, data...)
}

// TokenizerSnapshot contains the complete state for resumption.
type TokenizerSnapshot struct {
	ScannerSnap  Snapshot
	InLiteral    bool
	LiteralKind  TokenKind
	IsKey        bool
	LiteralBuf   []byte
	Position     int
	TokenStart   int
	BufferOffset int64
}

// Snapshot returns the current tokenizer state.
func (t *Tokenizer) Snapshot() TokenizerSnapshot {
	literalCopy := make([]byte, len(t.literalBuf))
	copy(literalCopy, t.literalBuf)
	return TokenizerSnapshot{
		ScannerSnap:  t.scanner.Snapshot(),
		InLiteral:    t.inLiteral,
		LiteralKind:  t.literalKind,
		IsKey:        t.isKey,
		LiteralBuf:   literalCopy,
		Position:     t.position,
		TokenStart:   t.tokenStart,
		BufferOffset: t.bufferOffset,
	}
}

// Restore restores the tokenizer state from a snapshot.
func (t *Tokenizer) Restore(snap TokenizerSnapshot) {
	t.scanner.Restore(snap.ScannerSnap)
	t.inLiteral = snap.InLiteral
	t.literalKind = snap.LiteralKind
	t.isKey = snap.IsKey
	t.literalBuf = t.literalBuf[:0]
	t.literalBuf = append(t.literalBuf, snap.LiteralBuf...)
	t.position = snap.Position
	t.tokenStart = snap.TokenStart
	t.bufferOffset = snap.BufferOffset
}

// NextToken returns the next token from the buffer.
// If there isn't enough data, it returns TokenIncomplete.
func (t *Tokenizer) NextToken() Token {
	// If we have a pending token, return it first
	if t.pendingToken != nil {
		token := *t.pendingToken
		t.pendingToken = nil
		return token
	}

	for t.position < len(t.buffer) {
		c := t.buffer[t.position]
		t.position++

		result := t.scanner.Step(c)

		switch result {
		case ScanSkipSpace:
			if !t.inLiteral {
				t.tokenStart = t.position
			}
			continue

		case ScanBeginObject:
			t.tokenStart = t.position
			return Token{
				Kind:      TokenObjectStart,
				Raw:       []byte{'{'},
				Start:     t.bufferOffset + int64(t.position-1),
				End:       t.bufferOffset + int64(t.position),
				Completed: true,
			}

		case ScanEndObject:
			// If we were in a literal (number at end of object), complete it first
			if t.inLiteral {
				token := t.completeLiteral()
				t.inLiteral = false
				// Queue the close brace for next call
				t.pendingToken = &Token{
					Kind:      TokenObjectEnd,
					Raw:       []byte{'}'},
					Start:     t.bufferOffset + int64(t.position-1),
					End:       t.bufferOffset + int64(t.position),
					Completed: true,
				}
				return token
			}
			return Token{
				Kind:      TokenObjectEnd,
				Raw:       []byte{'}'},
				Start:     t.bufferOffset + int64(t.position-1),
				End:       t.bufferOffset + int64(t.position),
				Completed: true,
			}

		case ScanBeginArray:
			t.tokenStart = t.position
			return Token{
				Kind:      TokenArrayStart,
				Raw:       []byte{'['},
				Start:     t.bufferOffset + int64(t.position-1),
				End:       t.bufferOffset + int64(t.position),
				Completed: true,
			}

		case ScanEndArray:
			// If we were in a literal (number at end of array), complete it first
			if t.inLiteral {
				token := t.completeLiteral()
				t.inLiteral = false
				// Queue the close bracket for next call
				t.pendingToken = &Token{
					Kind:      TokenArrayEnd,
					Raw:       []byte{']'},
					Start:     t.bufferOffset + int64(t.position-1),
					End:       t.bufferOffset + int64(t.position),
					Completed: true,
				}
				return token
			}
			return Token{
				Kind:      TokenArrayEnd,
				Raw:       []byte{']'},
				Start:     t.bufferOffset + int64(t.position-1),
				End:       t.bufferOffset + int64(t.position),
				Completed: true,
			}

		case ScanBeginLiteral:
			t.inLiteral = true
			t.tokenStart = t.position - 1
			t.literalBuf = t.literalBuf[:0]
			t.literalBuf = append(t.literalBuf, c)
			t.literalKind = t.classifyLiteralStart(c)
			// Check if we're expecting a key
			t.isKey = len(t.scanner.parseState) > 0 &&
				t.scanner.parseState[len(t.scanner.parseState)-1] == ParseObjectKey
			continue

		case ScanObjectKey:
			// Completed object key (just saw ':'), emit the string token
			token := t.completeLiteral()
			token.IsKey = true
			t.inLiteral = false
			t.tokenStart = t.position
			return token

		case ScanObjectValue:
			// After object value (just saw ','), emit the value first then comma
			if t.inLiteral {
				token := t.completeLiteral()
				t.inLiteral = false
				// Queue the comma for next call
				t.pendingToken = &Token{
					Kind:      TokenComma,
					Raw:       []byte{','},
					Start:     t.bufferOffset + int64(t.position-1),
					End:       t.bufferOffset + int64(t.position),
					Completed: true,
				}
				return token
			}
			// Value was already emitted (object or array value), just emit comma
			t.tokenStart = t.position
			return Token{
				Kind:      TokenComma,
				Raw:       []byte{','},
				Start:     t.bufferOffset + int64(t.position-1),
				End:       t.bufferOffset + int64(t.position),
				Completed: true,
			}

		case ScanArrayValue:
			// After array value (just saw ','), emit the value first then comma
			if t.inLiteral {
				token := t.completeLiteral()
				t.inLiteral = false
				// Queue the comma for next call
				t.pendingToken = &Token{
					Kind:      TokenComma,
					Raw:       []byte{','},
					Start:     t.bufferOffset + int64(t.position-1),
					End:       t.bufferOffset + int64(t.position),
					Completed: true,
				}
				return token
			}
			// Value was already emitted, just emit comma
			t.tokenStart = t.position
			return Token{
				Kind:      TokenComma,
				Raw:       []byte{','},
				Start:     t.bufferOffset + int64(t.position-1),
				End:       t.bufferOffset + int64(t.position),
				Completed: true,
			}

		case ScanContinue:
			if t.inLiteral {
				t.literalBuf = append(t.literalBuf, c)
			}
			continue

		case ScanEnd:
			// Top-level value complete
			if t.inLiteral {
				token := t.completeLiteral()
				t.inLiteral = false
				return token
			}
			// ScanEnd (not in a literal) only happens when a non-whitespace byte
			// appears after the root value — i.e. trailing data. Genuine
			// end-of-input exhausts the buffer instead and never reaches here.
			if t.rejectTrailing {
				return Token{
					Kind:      TokenError,
					Start:     t.bufferOffset + int64(t.position-1),
					End:       t.bufferOffset + int64(t.position),
					Completed: true,
					Err:       errTrailingData,
				}
			}
			return Token{
				Kind:      TokenEOF,
				Start:     t.bufferOffset + int64(t.position),
				End:       t.bufferOffset + int64(t.position),
				Completed: true,
			}

		case ScanError:
			return Token{
				Kind:      TokenError,
				Start:     t.bufferOffset + int64(t.position-1),
				End:       t.bufferOffset + int64(t.position),
				Completed: true,
				Err:       t.scanner.Err(),
			}
		}
	}

	// Buffer exhausted
	if t.inLiteral {
		// We're in the middle of a literal, return incomplete
		return Token{
			Kind:      TokenIncomplete,
			Raw:       t.literalBuf,
			Start:     t.bufferOffset + int64(t.tokenStart),
			End:       t.bufferOffset + int64(t.position),
			Completed: false,
			IsKey:     t.isKey,
		}
	}

	// No more tokens and not in a literal
	return Token{
		Kind:      TokenIncomplete,
		Completed: false,
	}
}

// classifyLiteralStart determines the token kind from the first character.
func (t *Tokenizer) classifyLiteralStart(c byte) TokenKind {
	switch c {
	case '"':
		return TokenString
	case 't', 'f':
		return TokenBool
	case 'n':
		return TokenNull
	default:
		return TokenNumber
	}
}

// completeLiteral finalizes a literal token.
func (t *Tokenizer) completeLiteral() Token {
	// Make a copy of the literal buffer since we reuse it
	raw := make([]byte, len(t.literalBuf))
	copy(raw, t.literalBuf)
	return Token{
		Kind:      t.literalKind,
		Raw:       raw,
		Start:     t.bufferOffset + int64(t.tokenStart),
		End:       t.bufferOffset + int64(t.position),
		Completed: true,
		IsKey:     t.isKey,
	}
}

// Depth returns the current nesting depth.
func (t *Tokenizer) Depth() int {
	return t.scanner.Depth()
}

// Err returns any error from the scanner.
func (t *Tokenizer) Err() error {
	return t.scanner.Err()
}

// BytesConsumed returns the total bytes processed.
func (t *Tokenizer) BytesConsumed() int64 {
	return t.scanner.Bytes()
}
