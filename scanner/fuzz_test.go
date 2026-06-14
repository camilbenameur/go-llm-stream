package scanner

import (
	"encoding/json"
	"testing"
)

// FuzzScanner feeds arbitrary bytes to the scanner byte-by-byte and checks:
//  1. The scanner never panics, regardless of input.
//  2. If the input is valid standalone JSON according to encoding/json.Valid,
//     the scanner reaches a completed top-level value with no error once a
//     trailing space is appended.
//
// Note on finalization: a bare top-level scalar like "123" or "true" is not
// considered "ended" by the byte-at-a-time Step API until a delimiter or
// extra whitespace is observed after it (mirrors encoding/json/scanner.go).
// Feeding a single trailing space byte after the input is the documented way
// to flush such trailing literals/numbers, so we append one before checking
// EndTop/Err for the json.Valid cross-check.
func FuzzScanner(f *testing.F) {
	seeds := []string{
		// basics
		``,
		`{}`,
		`[]`,
		`null`,
		`true`,
		`false`,
		`"hello"`,
		`123`,
		`-123`,
		`0`,
		`-0`,
		`0.5`,
		`-0.5`,
		`1e10`,
		`1E-10`,
		`1.5e+10`,
		`-1.5e-10`,

		// nested objects/arrays
		`{"a":1}`,
		`{"a":{"b":{"c":[1,2,3]}}}`,
		`[1,2,3,{"nested":[4,5]}]`,
		`{"a":[1,[2,[3,[4,[5]]]]]}`,
		`[[[[[[1]]]]]]`,

		// escapes and unicode
		`"hello\nworld"`,
		`"tab\tend"`,
		`"quote\"inside"`,
		`"backslash\\here"`,
		`"slash\/ok"`,
		`"ABC"`,
		`"😀"`,
		`{"key with space":"valé"}`,

		// whitespace variations
		"  {  \"key\"  :  \"value\"  }  ",
		"\t\n\r {\"a\":\n1,\n\"b\":2}\r\n",

		// numbers, edge cases
		`[0,-0,0.0,-0.0,1e1,1E1,1e+1,1e-1,1.0e10,123456789012345]`,

		// deeply nested
		`{"a":{"a":{"a":{"a":{"a":{"a":{"a":{"a":{"a":1}}}}}}}}}`,

		// truncated / incomplete inputs (must not panic, may be incomplete)
		`{"key": "val`,
		`{"a":`,
		`[1,2,`,
		`{"a":1,"b":`,
		`"unterminated string`,
		`"escape at end\`,
		`"unicode escape \u00`,
		`tru`,
		`fals`,
		`nul`,
		`-`,
		`1.`,
		`1e`,
		`1e+`,
		`{`,
		`[`,
		`{"a":{"b":[1,2,{"c":"d`,

		// invalid inputs
		`{,}`,
		`[,]`,
		`{"a":1,}`,
		`[1,]`,
		`"\x00"`,
		`trux`,
		`[123a]`,
		`123a`,
		`{"a":1} extra`,
		`{"a":}`,
		`nul1`,
	}

	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// 1. The scanner must never panic on arbitrary input.
		s := New()
		defer s.Free()

		errored := false
		for _, c := range data {
			r := s.Step(c)
			if r == ScanError {
				errored = true
				break
			}
		}

		// 2. Cross-check against encoding/json.Valid for inputs that did not
		// already error mid-stream.
		if !errored && json.Valid(data) {
			// Flush any trailing bare literal/number by feeding a space,
			// which is the documented way to signal "no more bytes follow
			// directly" for the byte-at-a-time Step API.
			r := s.Step(' ')
			if r == ScanError {
				t.Fatalf("scanner errored on trailing flush for valid JSON %q: %v", data, s.Err())
			}

			if s.Err() != nil {
				t.Fatalf("scanner has error for valid JSON %q: %v", data, s.Err())
			}
			if !s.EndTop() {
				t.Fatalf("scanner did not reach EndTop for valid JSON %q (state=%v)", data, s.State())
			}
		}
	})
}
