// Package scanner provides a resumable, O(n) JSON scanner for LLM streams.
// It is based on the Go standard library's encoding/json/scanner.go but adapted
// for streaming scenarios where data arrives in arbitrary chunks.
package scanner

// State represents a serializable scanner state.
// This replaces the function pointer pattern from the stdlib with an enum
// that can be saved/restored for resumability.
type State uint8

// All 27 scanner states mapped from the Go stdlib's function pointer states.
// These are organized into logical groups for clarity.
const (
	// Value Entry States
	StateBeginValue         State = iota // Start of any value
	StateBeginValueOrEmpty               // After '[', expecting value or ']'
	StateBeginStringOrEmpty              // After '{', expecting key or '}'
	StateBeginString                     // Expecting object key

	// String Parsing States (6)
	StateInString      // Inside string literal
	StateInStringEsc   // After '\' in string
	StateInStringEscU  // After '\u'
	StateInStringEscU1 // After '\uX'
	StateInStringEscU2 // After '\uXX'
	StateInStringEscU3 // After '\uXXX'

	// Number Parsing States (8)
	StateNeg   // After '-'
	State0     // After leading '0'
	State1     // After non-zero digit
	StateDot   // After decimal point '.'
	StateDot0  // After decimal digits
	StateE     // After 'e' or 'E'
	StateESign // After exponent sign
	StateE0    // After exponent digits

	// Literal Parsing States - true (3)
	StateT   // After 't'
	StateTr  // After 'tr'
	StateTru // After 'tru'

	// Literal Parsing States - false (4)
	StateF    // After 'f'
	StateFa   // After 'fa'
	StateFal  // After 'fal'
	StateFals // After 'fals'

	// Literal Parsing States - null (3)
	StateN   // After 'n'
	StateNu  // After 'nu'
	StateNul // After 'nul'

	// Completion States
	StateEndValue // After completing a value
	StateEndTop   // After top-level value complete
	StateError    // Error state (terminal)
)

// String returns a human-readable name for the state.
func (s State) String() string {
	switch s {
	case StateBeginValue:
		return "BeginValue"
	case StateBeginValueOrEmpty:
		return "BeginValueOrEmpty"
	case StateBeginStringOrEmpty:
		return "BeginStringOrEmpty"
	case StateBeginString:
		return "BeginString"
	case StateInString:
		return "InString"
	case StateInStringEsc:
		return "InStringEsc"
	case StateInStringEscU:
		return "InStringEscU"
	case StateInStringEscU1:
		return "InStringEscU1"
	case StateInStringEscU2:
		return "InStringEscU2"
	case StateInStringEscU3:
		return "InStringEscU3"
	case StateNeg:
		return "Neg"
	case State0:
		return "0"
	case State1:
		return "1"
	case StateDot:
		return "Dot"
	case StateDot0:
		return "Dot0"
	case StateE:
		return "E"
	case StateESign:
		return "ESign"
	case StateE0:
		return "E0"
	case StateT:
		return "T"
	case StateTr:
		return "Tr"
	case StateTru:
		return "Tru"
	case StateF:
		return "F"
	case StateFa:
		return "Fa"
	case StateFal:
		return "Fal"
	case StateFals:
		return "Fals"
	case StateN:
		return "N"
	case StateNu:
		return "Nu"
	case StateNul:
		return "Nul"
	case StateEndValue:
		return "EndValue"
	case StateEndTop:
		return "EndTop"
	case StateError:
		return "Error"
	default:
		return "Unknown"
	}
}

// ParseContext represents the context for parsing within objects and arrays.
// This is used for the push-down automaton stack.
type ParseContext uint8

const (
	ParseObjectKey   ParseContext = iota // Parsing object key (before colon)
	ParseObjectValue                     // Parsing object value (after colon)
	ParseArrayValue                      // Parsing array value
)

// String returns a human-readable name for the parse context.
func (p ParseContext) String() string {
	switch p {
	case ParseObjectKey:
		return "ObjectKey"
	case ParseObjectValue:
		return "ObjectValue"
	case ParseArrayValue:
		return "ArrayValue"
	default:
		return "Unknown"
	}
}

// ScanResult represents the result of scanning a single byte.
type ScanResult int

const (
	ScanContinue     ScanResult = iota // Continue scanning
	ScanBeginLiteral                   // Beginning of a literal (string, number, bool, null)
	ScanBeginObject                    // Beginning of object '{'
	ScanObjectKey                      // Scanned object key
	ScanObjectValue                    // Scanned object value
	ScanEndObject                      // End of object '}'
	ScanBeginArray                     // Beginning of array '['
	ScanArrayValue                     // Scanned array value
	ScanEndArray                       // End of array ']'
	ScanSkipSpace                      // Whitespace, skip
	ScanEnd                            // Top-level value ended
	ScanError                          // Hit an error
	ScanIncomplete                     // Need more data (buffer exhausted mid-token)
)

// String returns a human-readable name for the scan result.
func (r ScanResult) String() string {
	switch r {
	case ScanContinue:
		return "Continue"
	case ScanBeginLiteral:
		return "BeginLiteral"
	case ScanBeginObject:
		return "BeginObject"
	case ScanObjectKey:
		return "ObjectKey"
	case ScanObjectValue:
		return "ObjectValue"
	case ScanEndObject:
		return "EndObject"
	case ScanBeginArray:
		return "BeginArray"
	case ScanArrayValue:
		return "ArrayValue"
	case ScanEndArray:
		return "EndArray"
	case ScanSkipSpace:
		return "SkipSpace"
	case ScanEnd:
		return "End"
	case ScanError:
		return "Error"
	case ScanIncomplete:
		return "Incomplete"
	default:
		return "Unknown"
	}
}
