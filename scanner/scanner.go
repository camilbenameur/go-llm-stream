package scanner

import (
	"sync"
)

// MaxNestingDepth is the maximum allowed nesting depth for objects and arrays.
const MaxNestingDepth = 10000

// Scanner is a resumable JSON scanner that processes input byte-by-byte.
// It can be paused at any point and resumed later, making it ideal for
// streaming scenarios where data arrives in chunks.
type Scanner struct {
	// Current state of the scanner (serializable)
	state State

	// Stack of parse contexts (object key/value, array value)
	parseState []ParseContext

	// Whether we've completed the top-level value
	endTop bool

	// Last error encountered
	err error

	// Total bytes consumed
	bytes int64
}

// Pool for scanner reuse to minimize allocations
var scannerPool = sync.Pool{
	New: func() any {
		return &Scanner{
			parseState: make([]ParseContext, 0, 16),
		}
	},
}

// New returns a new scanner from the pool, ready for use.
func New() *Scanner {
	s := scannerPool.Get().(*Scanner)
	s.Reset()
	return s
}

// Free returns the scanner to the pool for reuse.
// The scanner must not be used after calling Free.
func (s *Scanner) Free() {
	// Don't hold huge stacks in the pool
	if cap(s.parseState) > 1024 {
		s.parseState = nil
	}
	scannerPool.Put(s)
}

// Reset resets the scanner to its initial state.
func (s *Scanner) Reset() {
	s.state = StateBeginValue
	s.parseState = s.parseState[:0]
	s.endTop = false
	s.err = nil
	s.bytes = 0
}

// State returns the current scanner state.
func (s *Scanner) State() State {
	return s.state
}

// Bytes returns the total number of bytes consumed.
func (s *Scanner) Bytes() int64 {
	return s.bytes
}

// Err returns any error encountered during scanning.
func (s *Scanner) Err() error {
	return s.err
}

// EndTop returns true if the top-level value has been completed.
func (s *Scanner) EndTop() bool {
	return s.endTop
}

// Depth returns the current nesting depth.
func (s *Scanner) Depth() int {
	return len(s.parseState)
}

// Snapshot returns a serializable snapshot of the scanner state.
// This can be used to save and restore the scanner for resumability.
type Snapshot struct {
	State      State
	ParseState []ParseContext
	EndTop     bool
	Bytes      int64
}

// Snapshot returns the current scanner state as a serializable snapshot.
func (s *Scanner) Snapshot() Snapshot {
	// Make a copy of parseState to avoid sharing the slice
	ps := make([]ParseContext, len(s.parseState))
	copy(ps, s.parseState)
	return Snapshot{
		State:      s.state,
		ParseState: ps,
		EndTop:     s.endTop,
		Bytes:      s.bytes,
	}
}

// Restore restores the scanner state from a snapshot.
func (s *Scanner) Restore(snap Snapshot) {
	s.state = snap.State
	s.parseState = s.parseState[:0]
	s.parseState = append(s.parseState, snap.ParseState...)
	s.endTop = snap.EndTop
	s.bytes = snap.Bytes
	s.err = nil
}

// Step processes a single byte and returns the scan result.
// This is the core method implementing the state machine.
func (s *Scanner) Step(c byte) ScanResult {
	s.bytes++
	return s.step(c)
}

// step is the internal dispatch to state handlers.
func (s *Scanner) step(c byte) ScanResult {
	switch s.state {
	case StateBeginValue:
		return s.stepBeginValue(c)
	case StateBeginValueOrEmpty:
		return s.stepBeginValueOrEmpty(c)
	case StateBeginStringOrEmpty:
		return s.stepBeginStringOrEmpty(c)
	case StateBeginString:
		return s.stepBeginString(c)
	case StateInString:
		return s.stepInString(c)
	case StateInStringEsc:
		return s.stepInStringEsc(c)
	case StateInStringEscU:
		return s.stepInStringEscU(c)
	case StateInStringEscU1:
		return s.stepInStringEscU1(c)
	case StateInStringEscU2:
		return s.stepInStringEscU2(c)
	case StateInStringEscU3:
		return s.stepInStringEscU3(c)
	case StateNeg:
		return s.stepNeg(c)
	case State0:
		return s.step0(c)
	case State1:
		return s.step1(c)
	case StateDot:
		return s.stepDot(c)
	case StateDot0:
		return s.stepDot0(c)
	case StateE:
		return s.stepE(c)
	case StateESign:
		return s.stepESign(c)
	case StateE0:
		return s.stepE0(c)
	case StateT:
		return s.stepT(c)
	case StateTr:
		return s.stepTr(c)
	case StateTru:
		return s.stepTru(c)
	case StateF:
		return s.stepF(c)
	case StateFa:
		return s.stepFa(c)
	case StateFal:
		return s.stepFal(c)
	case StateFals:
		return s.stepFals(c)
	case StateN:
		return s.stepN(c)
	case StateNu:
		return s.stepNu(c)
	case StateNul:
		return s.stepNul(c)
	case StateEndValue:
		return s.stepEndValue(c)
	case StateEndTop:
		return s.stepEndTop(c)
	case StateError:
		return ScanError
	default:
		return s.error(c, "unknown state")
	}
}

// isSpace returns true if c is a JSON whitespace character.
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// isDigit returns true if c is a digit 0-9.
func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// isHex returns true if c is a hexadecimal digit.
func isHex(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// error records an error and transitions to the error state.
func (s *Scanner) error(c byte, context string) ScanResult {
	s.state = StateError
	s.err = &ScannerError{
		Byte:    c,
		Context: context,
		Offset:  s.bytes,
	}
	return ScanError
}

// ScannerError represents a scanning error.
type ScannerError struct {
	Byte    byte
	Context string
	Offset  int64
}

func (e *ScannerError) Error() string {
	return "invalid character '" + string(e.Byte) + "' " + e.Context + " at offset " + itoa(e.Offset)
}

// itoa is a simple int64 to string without importing strconv.
func itoa(i int64) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// pushParseState pushes a new parse context onto the stack.
func (s *Scanner) pushParseState(ctx ParseContext) ScanResult {
	s.parseState = append(s.parseState, ctx)
	if len(s.parseState) > MaxNestingDepth {
		return s.error(0, "exceeded max depth")
	}
	return ScanContinue
}

// popParseState pops the parse context stack and sets appropriate next state.
func (s *Scanner) popParseState() {
	n := len(s.parseState) - 1
	s.parseState = s.parseState[:n]
	if n == 0 {
		s.state = StateEndTop
		s.endTop = true
	} else {
		s.state = StateEndValue
	}
}

// stepBeginValue handles the start of any JSON value.
func (s *Scanner) stepBeginValue(c byte) ScanResult {
	if isSpace(c) {
		return ScanSkipSpace
	}
	switch c {
	case '{':
		s.state = StateBeginStringOrEmpty
		s.pushParseState(ParseObjectKey)
		return ScanBeginObject
	case '[':
		s.state = StateBeginValueOrEmpty
		s.pushParseState(ParseArrayValue)
		return ScanBeginArray
	case '"':
		s.state = StateInString
		return ScanBeginLiteral
	case '-':
		s.state = StateNeg
		return ScanBeginLiteral
	case '0':
		s.state = State0
		return ScanBeginLiteral
	case 't':
		s.state = StateT
		return ScanBeginLiteral
	case 'f':
		s.state = StateF
		return ScanBeginLiteral
	case 'n':
		s.state = StateN
		return ScanBeginLiteral
	}
	if c >= '1' && c <= '9' {
		s.state = State1
		return ScanBeginLiteral
	}
	return s.error(c, "looking for beginning of value")
}

// stepBeginValueOrEmpty handles after '[', expecting value or ']'.
func (s *Scanner) stepBeginValueOrEmpty(c byte) ScanResult {
	if isSpace(c) {
		return ScanSkipSpace
	}
	if c == ']' {
		s.popParseState()
		return ScanEndArray
	}
	return s.stepBeginValue(c)
}

// stepBeginStringOrEmpty handles after '{', expecting key or '}'.
func (s *Scanner) stepBeginStringOrEmpty(c byte) ScanResult {
	if isSpace(c) {
		return ScanSkipSpace
	}
	if c == '}' {
		s.popParseState()
		return ScanEndObject
	}
	return s.stepBeginString(c)
}

// stepBeginString expects an object key (string).
func (s *Scanner) stepBeginString(c byte) ScanResult {
	if isSpace(c) {
		return ScanSkipSpace
	}
	if c == '"' {
		s.state = StateInString
		return ScanBeginLiteral
	}
	return s.error(c, "looking for beginning of object key string")
}

// String parsing states

// stepInString handles bytes inside a string literal.
func (s *Scanner) stepInString(c byte) ScanResult {
	if c == '"' {
		s.state = StateEndValue
		return ScanContinue
	}
	if c == '\\' {
		s.state = StateInStringEsc
		return ScanContinue
	}
	if c < 0x20 {
		return s.error(c, "in string literal")
	}
	return ScanContinue
}

// stepInStringEsc handles after '\' in string.
func (s *Scanner) stepInStringEsc(c byte) ScanResult {
	switch c {
	case 'b', 'f', 'n', 'r', 't', '\\', '/', '"':
		s.state = StateInString
		return ScanContinue
	case 'u':
		s.state = StateInStringEscU
		return ScanContinue
	}
	return s.error(c, "in string escape code")
}

// stepInStringEscU handles after '\u'.
func (s *Scanner) stepInStringEscU(c byte) ScanResult {
	if isHex(c) {
		s.state = StateInStringEscU1
		return ScanContinue
	}
	return s.error(c, "in \\u hexadecimal character escape")
}

// stepInStringEscU1 handles after '\uX'.
func (s *Scanner) stepInStringEscU1(c byte) ScanResult {
	if isHex(c) {
		s.state = StateInStringEscU2
		return ScanContinue
	}
	return s.error(c, "in \\u hexadecimal character escape")
}

// stepInStringEscU2 handles after '\uXX'.
func (s *Scanner) stepInStringEscU2(c byte) ScanResult {
	if isHex(c) {
		s.state = StateInStringEscU3
		return ScanContinue
	}
	return s.error(c, "in \\u hexadecimal character escape")
}

// stepInStringEscU3 handles after '\uXXX'.
func (s *Scanner) stepInStringEscU3(c byte) ScanResult {
	if isHex(c) {
		s.state = StateInString
		return ScanContinue
	}
	return s.error(c, "in \\u hexadecimal character escape")
}

// Number parsing states

// stepNeg handles after '-'.
func (s *Scanner) stepNeg(c byte) ScanResult {
	if c == '0' {
		s.state = State0
		return ScanContinue
	}
	if c >= '1' && c <= '9' {
		s.state = State1
		return ScanContinue
	}
	return s.error(c, "in numeric literal")
}

// step0 handles after leading '0'.
func (s *Scanner) step0(c byte) ScanResult {
	if c == '.' {
		s.state = StateDot
		return ScanContinue
	}
	if c == 'e' || c == 'E' {
		s.state = StateE
		return ScanContinue
	}
	// The number is complete; transition out of the numeric state before
	// delegating, so that a whitespace byte here causes subsequent bytes
	// to be checked against the enclosing context (e.g. ',' or '}') rather
	// than being treated as more of this number.
	s.state = StateEndValue
	return s.stepEndValue(c)
}

// step1 handles after non-zero digit.
func (s *Scanner) step1(c byte) ScanResult {
	if isDigit(c) {
		return ScanContinue
	}
	return s.step0(c)
}

// stepDot handles after decimal point.
func (s *Scanner) stepDot(c byte) ScanResult {
	if isDigit(c) {
		s.state = StateDot0
		return ScanContinue
	}
	return s.error(c, "after decimal point in numeric literal")
}

// stepDot0 handles after decimal digits.
func (s *Scanner) stepDot0(c byte) ScanResult {
	if isDigit(c) {
		return ScanContinue
	}
	if c == 'e' || c == 'E' {
		s.state = StateE
		return ScanContinue
	}
	// See comment in step0: transition out of the numeric state first.
	s.state = StateEndValue
	return s.stepEndValue(c)
}

// stepE handles after 'e' or 'E'.
func (s *Scanner) stepE(c byte) ScanResult {
	if c == '+' || c == '-' {
		s.state = StateESign
		return ScanContinue
	}
	return s.stepESign(c)
}

// stepESign handles after exponent sign.
func (s *Scanner) stepESign(c byte) ScanResult {
	if isDigit(c) {
		s.state = StateE0
		return ScanContinue
	}
	return s.error(c, "in exponent of numeric literal")
}

// stepE0 handles after exponent digits.
func (s *Scanner) stepE0(c byte) ScanResult {
	if isDigit(c) {
		return ScanContinue
	}
	// See comment in step0: transition out of the numeric state first.
	s.state = StateEndValue
	return s.stepEndValue(c)
}

// Literal parsing states: true

// stepT handles after 't'.
func (s *Scanner) stepT(c byte) ScanResult {
	if c == 'r' {
		s.state = StateTr
		return ScanContinue
	}
	return s.error(c, "in literal true (expecting 'r')")
}

// stepTr handles after 'tr'.
func (s *Scanner) stepTr(c byte) ScanResult {
	if c == 'u' {
		s.state = StateTru
		return ScanContinue
	}
	return s.error(c, "in literal true (expecting 'u')")
}

// stepTru handles after 'tru'.
func (s *Scanner) stepTru(c byte) ScanResult {
	if c == 'e' {
		s.state = StateEndValue
		return ScanContinue
	}
	return s.error(c, "in literal true (expecting 'e')")
}

// Literal parsing states: false

// stepF handles after 'f'.
func (s *Scanner) stepF(c byte) ScanResult {
	if c == 'a' {
		s.state = StateFa
		return ScanContinue
	}
	return s.error(c, "in literal false (expecting 'a')")
}

// stepFa handles after 'fa'.
func (s *Scanner) stepFa(c byte) ScanResult {
	if c == 'l' {
		s.state = StateFal
		return ScanContinue
	}
	return s.error(c, "in literal false (expecting 'l')")
}

// stepFal handles after 'fal'.
func (s *Scanner) stepFal(c byte) ScanResult {
	if c == 's' {
		s.state = StateFals
		return ScanContinue
	}
	return s.error(c, "in literal false (expecting 's')")
}

// stepFals handles after 'fals'.
func (s *Scanner) stepFals(c byte) ScanResult {
	if c == 'e' {
		s.state = StateEndValue
		return ScanContinue
	}
	return s.error(c, "in literal false (expecting 'e')")
}

// Literal parsing states: null

// stepN handles after 'n'.
func (s *Scanner) stepN(c byte) ScanResult {
	if c == 'u' {
		s.state = StateNu
		return ScanContinue
	}
	return s.error(c, "in literal null (expecting 'u')")
}

// stepNu handles after 'nu'.
func (s *Scanner) stepNu(c byte) ScanResult {
	if c == 'l' {
		s.state = StateNul
		return ScanContinue
	}
	return s.error(c, "in literal null (expecting 'l')")
}

// stepNul handles after 'nul'.
func (s *Scanner) stepNul(c byte) ScanResult {
	if c == 'l' {
		s.state = StateEndValue
		return ScanContinue
	}
	return s.error(c, "in literal null (expecting 'l')")
}

// Completion states

// stepEndValue handles after completing a value.
func (s *Scanner) stepEndValue(c byte) ScanResult {
	n := len(s.parseState)
	if n == 0 {
		// Top-level value complete
		s.state = StateEndTop
		s.endTop = true
		return s.stepEndTop(c)
	}

	if isSpace(c) {
		return ScanSkipSpace
	}

	ctx := s.parseState[n-1]
	switch ctx {
	case ParseObjectKey:
		// Expecting ':'
		if c == ':' {
			s.parseState[n-1] = ParseObjectValue
			s.state = StateBeginValue
			return ScanObjectKey
		}
		return s.error(c, "after object key")

	case ParseObjectValue:
		// Expecting ',' or '}'
		if c == ',' {
			s.parseState[n-1] = ParseObjectKey
			s.state = StateBeginString
			return ScanObjectValue
		}
		if c == '}' {
			s.popParseState()
			return ScanEndObject
		}
		return s.error(c, "after object key:value pair")

	case ParseArrayValue:
		// Expecting ',' or ']'
		if c == ',' {
			s.state = StateBeginValue
			return ScanArrayValue
		}
		if c == ']' {
			s.popParseState()
			return ScanEndArray
		}
		return s.error(c, "after array element")
	}

	return s.error(c, "unknown parse context")
}

// stepEndTop handles after top-level value complete.
func (s *Scanner) stepEndTop(c byte) ScanResult {
	if isSpace(c) {
		return ScanSkipSpace
	}
	return ScanEnd
}
