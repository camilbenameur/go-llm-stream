package scanner

import (
	"context"
	"io"
	"sync"
)

// readResult carries the outcome of a single Read call back from the
// cancellable read goroutine.
type readResult struct {
	n   int
	err error
}

// StreamReader wraps an io.Reader and provides streaming JSON tokenization.
// It is the primary interface for consuming LLM streams.
type StreamReader struct {
	reader   io.Reader
	tok      *Tokenizer
	buf      []byte
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	done     bool
	mu       sync.Mutex
	resultCh chan readResult
}

// Default buffer size for reading from the underlying reader
const defaultBufSize = 4096

// NewStreamReader creates a new StreamReader that reads from r.
// Use context for cancellation support.
func NewStreamReader(ctx context.Context, r io.Reader) *StreamReader {
	ctx, cancel := context.WithCancel(ctx)
	return &StreamReader{
		reader:   r,
		tok:      NewTokenizer(),
		buf:      make([]byte, defaultBufSize),
		ctx:      ctx,
		cancel:   cancel,
		resultCh: make(chan readResult, 1),
	}
}

// Close releases resources associated with the StreamReader.
func (sr *StreamReader) Close() error {
	sr.cancel()
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if sr.tok != nil {
		sr.tok.Free()
		sr.tok = nil
	}
	sr.buf = nil
	return nil
}

// NextToken returns the next complete token from the stream.
// It blocks until a complete token is available, an error occurs, or the context is cancelled.
// Returns TokenEOF when the stream ends normally.
// Returns TokenError if an error occurs (check Err() for details).
func (sr *StreamReader) NextToken() Token {
	for {
		// Check context cancellation
		select {
		case <-sr.ctx.Done():
			sr.mu.Lock()
			sr.err = sr.ctx.Err()
			sr.mu.Unlock()
			return Token{Kind: TokenError, Err: sr.err, Completed: true}
		default:
		}

		// Try to get a token from the tokenizer
		sr.mu.Lock()
		token := sr.tok.NextToken()
		sr.mu.Unlock()

		// If we got a complete token (not incomplete), return it
		if token.Kind != TokenIncomplete {
			return token
		}

		sr.mu.Lock()
		// If already done reading, return what we have
		if sr.done {
			// We might have an incomplete token at end of stream
			if len(token.Raw) > 0 {
				// Return the incomplete data as-is (could be handled by healer later)
				sr.mu.Unlock()
				return token
			}
			sr.mu.Unlock()
			return Token{Kind: TokenEOF, Completed: true}
		}

		// Need more data, read from the underlying reader
		if len(sr.buf) == 0 {
			// Buffer was closed
			sr.mu.Unlock()
			return Token{Kind: TokenEOF, Completed: true}
		}

		buf := sr.buf
		resultCh := sr.resultCh
		sr.mu.Unlock()

		// Drain any stale result left over from a previously cancelled
		// read before issuing a new one (the channel is reused to avoid
		// allocating a fresh one on every call).
		select {
		case <-resultCh:
		default:
		}

		// Use a goroutine to make Read cancellable
		go func() {
			n, err := sr.reader.Read(buf)
			resultCh <- readResult{n, err}
		}()

		// Wait for read or cancellation
		select {
		case <-sr.ctx.Done():
			sr.mu.Lock()
			sr.err = sr.ctx.Err()
			sr.mu.Unlock()
			return Token{Kind: TokenError, Err: sr.err, Completed: true}
		case result := <-resultCh:
			sr.mu.Lock()
			// If closed while reading, just return whatever state we are in
			if sr.tok == nil {
				sr.mu.Unlock()
				return Token{Kind: TokenEOF, Completed: true}
			}

			if result.n > 0 {
				sr.tok.Append(buf[:result.n])
			}

			if result.err != nil {
				if result.err == io.EOF {
					sr.done = true
					// Continue to try to get any remaining tokens
					if result.n == 0 {
						// No data and EOF, return EOF if no incomplete token
						token := sr.tok.NextToken()
						if token.Kind != TokenIncomplete {
							sr.mu.Unlock()
							return token
						}
						if len(token.Raw) > 0 {
							sr.mu.Unlock()
							return token
						}
						sr.mu.Unlock()
						return Token{Kind: TokenEOF, Completed: true}
					}
					sr.mu.Unlock()
					continue
				}
				sr.err = result.err
				sr.mu.Unlock()
				return Token{Kind: TokenError, Err: result.err, Completed: true}
			}
			sr.mu.Unlock()
		}
	}
}

// Tokens returns a channel that yields tokens from the stream.
// The channel is closed when the stream ends or an error occurs.
// This is the idiomatic way to consume tokens in Go.
func (sr *StreamReader) Tokens() <-chan Token {
	ch := make(chan Token, 16) // Buffer some tokens
	go func() {
		defer close(ch)
		for {
			token := sr.NextToken()
			select {
			case ch <- token:
			case <-sr.ctx.Done():
				return
			}
			if token.Kind == TokenEOF || token.Kind == TokenError {
				return
			}
		}
	}()
	return ch
}

// Err returns any error that occurred during reading.
func (sr *StreamReader) Err() error {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	return sr.err
}

// Depth returns the current nesting depth.
func (sr *StreamReader) Depth() int {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	return sr.tok.Depth()
}

// BytesConsumed returns the total bytes processed.
func (sr *StreamReader) BytesConsumed() int64 {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	return sr.tok.BytesConsumed()
}
