package stream

import (
	"github.com/camilbenameur/go-llm-stream/scanner"
)

// Writer is a push-based streaming JSON tokenizer.
// It implements io.Writer, allowing chunks of JSON data to be fed in as
// they arrive (e.g. from an HTTP handler, a websocket callback, or any
// other "push" source) rather than pulled from an io.Reader.
//
// Each call to Write feeds the bytes to an internal scanner.Tokenizer and
// immediately extracts every token that is now complete. Tokens that
// straddle a Write boundary (e.g. a string literal or number split across
// two chunks) are buffered internally by the tokenizer and only emitted
// once they are complete - see scanner.Tokenizer.Append /
// scanner.TokenIncomplete for details.
//
// Completed tokens are made available in two complementary ways:
//
//   - OnToken, if set, is invoked synchronously (from within Write/Close)
//     for every completed token, in order.
//   - NextToken drains an internal buffer of completed tokens that have
//     not yet been delivered via NextToken.
//
// Both mechanisms see the same tokens; OnToken is called first for a given
// token, immediately followed by it becoming available via NextToken. A
// caller may use either, both, or neither (in which case tokens accumulate
// in the internal buffer until Close/Flush, or are simply dropped if never
// drained).
//
// Writer is not safe for concurrent use. Callers needing concurrency should
// serialize access (e.g. with their own mutex or a single dedicated
// goroutine).
//
// # Healing
//
// Writer intentionally does not offer a "healing" variant. healer.Healer
// (and stream.Healer) are built around scanner.StreamReader, which pulls
// from an io.Reader and decides for itself when the stream has ended
// (io.EOF) before applying closure/healing logic. A push-based equivalent
// would need to either (a) run the healer in a background goroutine reading
// from an io.Pipe fed by Write - adding a goroutine, synchronization, and
// shutdown-on-Close complexity that doesn't compose cleanly with a
// synchronous, allocation-light push API - or (b) duplicate the healer's
// closure/markdown logic against the tokenizer directly, forking behavior
// from the canonical implementation in package healer.
//
// Both options add significant complexity for a feature that is easy to
// layer on top: callers that need healed output from pushed chunks can
// write chunks into an io.Pipe and run stream.NewHealer (or
// healer.NewFromReader) on the read side in a goroutine, e.g.:
//
//	pr, pw := io.Pipe()
//	h := stream.NewHealer(ctx, pr)
//	go func() {
//	    for tok := range h.Tokens() { ... }
//	}()
//	// elsewhere, as chunks arrive:
//	pw.Write(chunk)
//	// when done:
//	pw.Close()
//
// This keeps Writer itself simple, synchronous, and dependency-free while
// still making healing achievable for push-style callers.
type Writer struct {
	tok *scanner.Tokenizer

	// OnToken, if non-nil, is called for each completed token as soon as
	// it becomes available, in token order. It may be set at any time
	// before the relevant Write/Close/Flush call.
	OnToken func(Token)

	// buffered holds completed tokens that have been produced but not yet
	// returned via NextToken.
	buffered []Token

	// closed indicates Close has been called. Further Write calls return
	// an error.
	closed bool

	// eofEmitted tracks whether the terminal TokenEOF (or TokenError) has
	// already been produced, so Close/Flush don't emit it twice.
	eofEmitted bool
}

// NewWriter creates a new push-based JSON token writer.
//
// Unlike NewReader/NewHealer, NewWriter takes no io.Reader: data is
// supplied later via Write. It also takes no context, since there is no
// blocking read to cancel - Write is always synchronous and non-blocking
// with respect to I/O.
func NewWriter() *Writer {
	return &Writer{
		tok: scanner.NewTokenizer(),
	}
}

// Write feeds p to the tokenizer and emits any tokens that are now
// complete. It always consumes the entire slice and never returns an
// error except after Close has been called.
//
// Write satisfies io.Writer. A literal (string, number, bool, or null)
// split across multiple Write calls is correctly reassembled: it is only
// emitted once enough bytes have arrived to complete it.
func (w *Writer) Write(p []byte) (int, error) {
	if w.closed {
		return 0, errClosedWriter
	}

	if len(p) > 0 {
		w.tok.Append(p)
	}

	w.drain(false)

	return len(p), nil
}

// drain repeatedly calls NextToken on the underlying tokenizer, delivering
// every completed token via OnToken and the internal buffer, until the
// tokenizer reports TokenIncomplete (no more data) or, if final is true,
// until a terminal token (TokenEOF/TokenError) has been produced.
func (w *Writer) drain(final bool) {
	if w.eofEmitted {
		return
	}

	for {
		token := w.tok.NextToken()

		switch token.Kind {
		case scanner.TokenIncomplete:
			if !final {
				return
			}
			// At end of input an incomplete token (e.g. a partial
			// literal with no closing delimiter) is surfaced as-is so
			// callers can detect truncation. Healing such output is the
			// job of a healer-based pipeline, not the raw push Writer.
			if len(token.Raw) > 0 || token.Completed {
				w.emit(token)
			}
			w.emit(Token{Kind: TokenEOF, Completed: true})
			w.eofEmitted = true
			return

		case scanner.TokenEOF, scanner.TokenError:
			w.emit(token)
			w.eofEmitted = true
			return

		default:
			w.emit(token)
		}
	}
}

// emit delivers a single completed token via OnToken (if set) and appends
// it to the internal buffer for NextToken.
func (w *Writer) emit(token Token) {
	if w.OnToken != nil {
		w.OnToken(token)
	}
	w.buffered = append(w.buffered, token)
}

// NextToken returns the next completed token that has been buffered but
// not yet delivered, in the order it was produced. The second return value
// is false if no buffered token is available.
//
// NextToken does not block and does not read any new data - call Write to
// feed more data first. After Close, any final tokens (including a
// trailing TokenEOF) remain available via NextToken until drained.
func (w *Writer) NextToken() (Token, bool) {
	if len(w.buffered) == 0 {
		return Token{}, false
	}
	token := w.buffered[0]
	w.buffered = w.buffered[1:]
	return token, true
}

// Flush forces the tokenizer to produce any tokens that can be determined
// from data written so far, without signaling end-of-input. It is rarely
// needed since Write already drains all currently-decodable tokens, but is
// provided for symmetry with other writer-like APIs and as a no-op-safe
// call before inspecting BytesConsumed or Depth.
func (w *Writer) Flush() error {
	if w.closed {
		return errClosedWriter
	}
	w.drain(false)
	return nil
}

// Close signals end-of-input. Any token that was pending (e.g. a final
// number or literal with no trailing delimiter) is completed and emitted,
// followed by a terminal TokenEOF (or TokenError, if the JSON was
// malformed). After Close, Write returns an error; NextToken and OnToken
// remain usable to drain any final buffered tokens.
//
// Close releases the underlying tokenizer resources. It is safe to call
// Close multiple times.
func (w *Writer) Close() error {
	if w.closed {
		return nil
	}
	w.drain(true)
	w.closed = true
	if w.tok != nil {
		w.tok.Free()
		w.tok = nil
	}
	return nil
}

// Depth returns the current nesting depth of the JSON being parsed.
func (w *Writer) Depth() int {
	if w.tok == nil {
		return 0
	}
	return w.tok.Depth()
}

// BytesConsumed returns the total number of bytes processed so far.
func (w *Writer) BytesConsumed() int64 {
	if w.tok == nil {
		return 0
	}
	return w.tok.BytesConsumed()
}

// errClosedWriter is returned by Write/Flush after Close has been called.
type closedWriterError struct{}

func (closedWriterError) Error() string { return "stream: Write on closed Writer" }

var errClosedWriter error = closedWriterError{}
