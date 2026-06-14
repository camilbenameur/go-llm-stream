package healer

import (
	"bytes"
	"io"
	"sync"
)

// Common markdown delimiters used by LLMs
var (
	markdownJSONStart = []byte("```json")
	markdownStart     = []byte("```")
	markdownEnd       = []byte("```")
)

// MarkdownState represents the current state of markdown filtering.
type MarkdownState uint8

const (
	StateNormal       MarkdownState = iota // Normal JSON content
	StateLookingStart                      // Looking for opening ```
	StateInCodeBlock                       // Inside a code block
	StateLookingEnd                        // Looking for closing ```
)

// MarkdownFilter strips markdown code block delimiters from LLM output.
// It handles the common case where LLMs wrap JSON in ```json ... ``` blocks.
type MarkdownFilter struct {
	// Buffer for accumulating data
	buf bytes.Buffer

	// State of the filter
	state MarkdownState

	// Buffer for potential delimiter detection
	pending []byte

	// Whether we've seen the opening delimiter
	sawOpening bool

	// Output buffer for filtered content
	output bytes.Buffer
}

// Pool for filter reuse
var filterPool = sync.Pool{
	New: func() any {
		return &MarkdownFilter{
			pending: make([]byte, 0, 32),
		}
	},
}

// NewMarkdownFilter returns a new filter from the pool.
func NewMarkdownFilter() *MarkdownFilter {
	f := filterPool.Get().(*MarkdownFilter)
	f.Reset()
	return f
}

// Free returns the filter to the pool.
func (f *MarkdownFilter) Free() {
	if f.buf.Cap() > 65536 {
		f.buf = bytes.Buffer{}
	}
	if f.output.Cap() > 65536 {
		f.output = bytes.Buffer{}
	}
	filterPool.Put(f)
}

// Reset resets the filter to its initial state.
func (f *MarkdownFilter) Reset() {
	f.buf.Reset()
	f.state = StateLookingStart
	f.pending = f.pending[:0]
	f.sawOpening = false
	f.output.Reset()
}

// Filter processes input and returns filtered output.
// Call this incrementally as data arrives.
func (f *MarkdownFilter) Filter(data []byte) []byte {
	f.output.Reset()

	for _, b := range data {
		f.processByte(b)
	}

	return f.output.Bytes()
}

// processByte handles a single byte of input.
func (f *MarkdownFilter) processByte(b byte) {
	switch f.state {
	case StateLookingStart:
		f.pending = append(f.pending, b)

		// Check if we might be seeing a markdown opening
		if bytes.HasPrefix(markdownJSONStart, f.pending) {
			if bytes.Equal(f.pending, markdownJSONStart) {
				// Found ```json, now skip until newline
				f.state = StateInCodeBlock
				f.sawOpening = true
				f.pending = f.pending[:0]
			}
			return
		}
		if bytes.HasPrefix(markdownStart, f.pending) {
			if len(f.pending) < len(markdownStart) {
				// Still matching ```
				return
			}
			// We have ``` but not ```json
			if len(f.pending) == len(markdownStart) {
				// Check if next char is not 'j'
				if b != 'j' {
					// Just ``` without json
					f.state = StateInCodeBlock
					f.sawOpening = true
					f.pending = f.pending[:0]
					// Don't emit this byte if it's a newline after ```
					if b != '\n' {
						f.output.WriteByte(b)
					}
				}
				return
			}
		}

		// Not a markdown delimiter, flush pending and switch to normal
		if !bytes.HasPrefix(markdownStart, f.pending) {
			f.state = StateNormal
			f.output.Write(f.pending)
			f.pending = f.pending[:0]
		}

	case StateInCodeBlock:
		// Skip until newline (the language identifier line)
		if b == '\n' {
			f.state = StateNormal
		}

	case StateNormal:
		// Look for closing ``` or just pass through
		f.pending = append(f.pending, b)

		if bytes.HasPrefix(markdownEnd, f.pending) {
			if bytes.Equal(f.pending, markdownEnd) {
				// Found closing ```, skip it and any trailing content
				f.state = StateLookingEnd
				f.pending = f.pending[:0]
			}
			return
		}

		// Not a markdown delimiter, flush pending
		if !bytes.HasPrefix(markdownEnd, f.pending) {
			f.output.Write(f.pending)
			f.pending = f.pending[:0]
		}

	case StateLookingEnd:
		// After closing ```, skip everything
		// (handles the "junk after JSON" problem)
		return
	}
}

// Flush returns any pending bytes that haven't been emitted.
// Call this when the stream ends.
func (f *MarkdownFilter) Flush() []byte {
	result := make([]byte, len(f.pending))
	copy(result, f.pending)
	f.pending = f.pending[:0]
	return result
}

// FilterReader wraps an io.Reader and filters out markdown delimiters.
type FilterReader struct {
	reader io.Reader
	filter *MarkdownFilter
	buf    []byte
	rawBuf []byte
	pos    int
	len    int
}

// NewFilterReader creates a new FilterReader.
func NewFilterReader(r io.Reader) *FilterReader {
	return &FilterReader{
		reader: r,
		filter: NewMarkdownFilter(),
		buf:    make([]byte, 4096),
		rawBuf: make([]byte, 4096),
	}
}

// Read implements io.Reader, filtering out markdown delimiters.
func (fr *FilterReader) Read(p []byte) (n int, err error) {
	for {
		// Try to fill p with filtered data
		if fr.pos < fr.len {
			n = copy(p, fr.buf[fr.pos:fr.len])
			fr.pos += n
			return n, nil
		}

		// Read more data
		rawN, err := fr.reader.Read(fr.rawBuf)

		if rawN > 0 {
			filtered := fr.filter.Filter(fr.rawBuf[:rawN])
			if len(filtered) > 0 {
				fr.pos = 0
				fr.len = copy(fr.buf, filtered)
				continue
			}
		}

		if err != nil {
			// On EOF, flush any pending
			if err == io.EOF {
				flushed := fr.filter.Flush()
				if len(flushed) > 0 {
					fr.pos = 0
					fr.len = copy(fr.buf, flushed)
					// Return what we have, without EOF yet
					n = copy(p, fr.buf[:fr.len])
					fr.pos = n
					return n, nil
				}
			}
			return 0, err
		}
	}
}

// Close releases resources.
func (fr *FilterReader) Close() error {
	if fr.filter != nil {
		fr.filter.Free()
		fr.filter = nil
	}
	return nil
}

// StripMarkdown removes markdown code block delimiters from bytes.
// This is a convenience function for one-shot processing.
func StripMarkdown(data []byte) []byte {
	f := NewMarkdownFilter()
	defer f.Free()

	result := f.Filter(data)
	flushed := f.Flush()

	if len(flushed) == 0 {
		return result
	}

	combined := make([]byte, len(result)+len(flushed))
	copy(combined, result)
	copy(combined[len(result):], flushed)
	return combined
}

// StripMarkdownString is a convenience wrapper for string input.
func StripMarkdownString(s string) string {
	return string(StripMarkdown([]byte(s)))
}
