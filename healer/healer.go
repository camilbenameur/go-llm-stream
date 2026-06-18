package healer

import (
	"context"
	"io"
	"sync"

	"github.com/camilbenameur/go-llm-stream/scanner"
)

// Healer wraps a StreamReader and provides automatic healing of malformed JSON.
// It handles truncated streams, incomplete tokens, and other common LLM output issues.
type Healer struct {
	// The underlying stream reader
	stream *scanner.StreamReader

	// Closer for tracking parse state
	closer *Closer

	// Markdown filter for stripping code block delimiters
	markdownFilter *MarkdownFilter

	// Configuration options
	opts HealerOptions

	// Token buffer for healing operations
	tokenBuf []scanner.Token

	// Whether we've signaled EOF
	eof bool

	// Closure tokens to emit after stream ends
	closureTokens []scanner.Token
	closureIdx    int

	// Error if any
	err error

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// HealerOptions configures the healer behavior.
type HealerOptions struct {
	// StripMarkdown enables markdown code block filtering
	StripMarkdown bool

	// AutoClose enables automatic closure of unclosed containers
	AutoClose bool

	// IgnoreTrailingJunk ignores content after the root JSON value closes
	IgnoreTrailingJunk bool
}

// DefaultOptions returns the recommended healer options.
func DefaultOptions() HealerOptions {
	return HealerOptions{
		StripMarkdown:      true,
		AutoClose:          true,
		IgnoreTrailingJunk: true,
	}
}

// Pool for healer reuse
var healerPool = sync.Pool{
	New: func() any {
		return &Healer{
			tokenBuf:      make([]scanner.Token, 0, 16),
			closureTokens: make([]scanner.Token, 0, 16),
		}
	},
}

// Option is a functional option for configuring the Healer.
type Option func(*HealerOptions)

// WithStripMarkdown enables or disables markdown stripping.
func WithStripMarkdown(enable bool) Option {
	return func(o *HealerOptions) {
		o.StripMarkdown = enable
	}
}

// WithAutoClose enables or disables automatic container closure.
func WithAutoClose(enable bool) Option {
	return func(o *HealerOptions) {
		o.AutoClose = enable
	}
}

// WithIgnoreTrailingJunk enables or disables ignoring content after JSON.
func WithIgnoreTrailingJunk(enable bool) Option {
	return func(o *HealerOptions) {
		o.IgnoreTrailingJunk = enable
	}
}

// New creates a new Healer wrapping the given StreamReader.
func New(ctx context.Context, stream *scanner.StreamReader, options ...Option) *Healer {
	opts := DefaultOptions()
	for _, opt := range options {
		opt(&opts)
	}

	ctx, cancel := context.WithCancel(ctx)

	h := healerPool.Get().(*Healer)
	h.stream = stream
	h.closer = NewCloser()
	h.markdownFilter = NewMarkdownFilter()
	h.opts = opts
	h.tokenBuf = h.tokenBuf[:0]
	h.eof = false
	h.closureTokens = h.closureTokens[:0]
	h.closureIdx = 0
	h.err = nil
	h.ctx = ctx
	h.cancel = cancel

	// When trailing junk is not ignored, ask the underlying reader to surface
	// content after the root value as an error instead of a clean end.
	if stream != nil && !opts.IgnoreTrailingJunk {
		stream.SetRejectTrailing(true)
	}

	return h
}

// NewFromReader creates a Healer directly from an io.Reader.
func NewFromReader(ctx context.Context, r io.Reader, options ...Option) *Healer {
	opts := DefaultOptions()
	for _, opt := range options {
		opt(&opts)
	}

	var reader io.Reader = r
	if opts.StripMarkdown {
		reader = NewFilterReader(r)
	}

	stream := scanner.NewStreamReader(ctx, reader)
	return New(ctx, stream, options...)
}

// Close releases resources associated with the Healer.
func (h *Healer) Close() error {
	h.cancel()
	if h.stream != nil {
		h.stream.Close()
	}
	if h.closer != nil {
		h.closer.Free()
		h.closer = nil
	}
	if h.markdownFilter != nil {
		h.markdownFilter.Free()
		h.markdownFilter = nil
	}
	// Don't pool healers that might have large buffers
	if cap(h.tokenBuf) <= 64 && cap(h.closureTokens) <= 64 {
		healerPool.Put(h)
	}
	return nil
}

// NextToken returns the next token from the healed stream.
// When the stream ends prematurely, it automatically generates
// closure tokens to complete the JSON structure.
func (h *Healer) NextToken() scanner.Token {
	// Check for context cancellation
	select {
	case <-h.ctx.Done():
		h.err = h.ctx.Err()
		return scanner.Token{Kind: scanner.TokenError, Err: h.err, Completed: true}
	default:
	}

	// Return closure tokens if we have them
	if h.closureIdx < len(h.closureTokens) {
		token := h.closureTokens[h.closureIdx]
		h.closureIdx++
		return token
	}

	// If we've already returned EOF, keep returning it
	if h.eof {
		return scanner.Token{Kind: scanner.TokenEOF, Completed: true}
	}

	// Get next token from the underlying stream
	token := h.stream.NextToken()

	switch token.Kind {
	case scanner.TokenEOF:
		return h.handleEOF()

	case scanner.TokenError:
		// On error, try to heal if AutoClose is enabled
		if h.opts.AutoClose {
			return h.handleError(token)
		}
		return token

	case scanner.TokenIncomplete:
		// Incomplete token at end of stream, try to complete it
		return h.handleIncomplete(token)

	case scanner.TokenObjectStart:
		h.closer.Feed([]byte{'{'})
		return token

	case scanner.TokenObjectEnd:
		h.closer.Feed([]byte{'}'})
		return token

	case scanner.TokenArrayStart:
		h.closer.Feed([]byte{'['})
		return token

	case scanner.TokenArrayEnd:
		h.closer.Feed([]byte{']'})
		return token

	case scanner.TokenString:
		h.closer.Feed(token.Raw)
		return token

	case scanner.TokenNumber, scanner.TokenBool, scanner.TokenNull:
		h.closer.Feed(token.Raw)
		return token

	case scanner.TokenColon:
		h.closer.Feed([]byte{':'})
		return token

	case scanner.TokenComma:
		h.closer.Feed([]byte{','})
		return token

	default:
		return token
	}
}

// handleEOF processes end of stream and generates closure tokens if needed.
func (h *Healer) handleEOF() scanner.Token {
	if !h.opts.AutoClose {
		h.eof = true
		return scanner.Token{Kind: scanner.TokenEOF, Completed: true}
	}

	// Generate closure tokens
	closure := h.closer.Closure()
	if len(closure) == 0 {
		h.eof = true
		return scanner.Token{Kind: scanner.TokenEOF, Completed: true}
	}

	// Parse the closure bytes into tokens
	h.closureTokens = h.parseClosureTokens(closure)
	h.closureIdx = 0
	h.eof = true // Mark EOF so after closure tokens we return EOF

	if len(h.closureTokens) > 0 {
		token := h.closureTokens[0]
		h.closureIdx = 1
		return token
	}

	return scanner.Token{Kind: scanner.TokenEOF, Completed: true}
}

// handleError processes errors and attempts to heal.
func (h *Healer) handleError(token scanner.Token) scanner.Token {
	// Generate closure tokens based on current state
	closure := h.closer.Closure()
	if len(closure) == 0 {
		return token // Can't heal, return the error
	}

	// Parse the closure bytes into tokens
	h.closureTokens = h.parseClosureTokens(closure)
	h.closureIdx = 0
	h.eof = true // After closure tokens, we're done

	if len(h.closureTokens) > 0 {
		token := h.closureTokens[0]
		h.closureIdx = 1
		return token
	}

	return scanner.Token{Kind: scanner.TokenEOF, Completed: true}
}

// handleIncomplete processes incomplete tokens at end of stream.
func (h *Healer) handleIncomplete(token scanner.Token) scanner.Token {
	// If we have raw data, feed it to the closer for context
	if len(token.Raw) > 0 {
		h.closer.Feed(token.Raw)
	}

	// For incomplete tokens, the stream hasn't ended yet.
	// But if we're here from a finished reader, just handle EOF.
	// Check if context is cancelled or if stream is done
	select {
	case <-h.ctx.Done():
		return h.handleEOF()
	default:
		// Try to get more data, but don't block indefinitely
		// Return EOF handling since we have incomplete data
		return h.handleEOF()
	}
}

// parseClosureTokens converts closure bytes into token objects.
func (h *Healer) parseClosureTokens(closure []byte) []scanner.Token {
	tokens := make([]scanner.Token, 0, len(closure))

	for i := 0; i < len(closure); i++ {
		b := closure[i]
		switch b {
		case '}':
			tokens = append(tokens, scanner.Token{
				Kind:      scanner.TokenObjectEnd,
				Raw:       []byte{'}'},
				Completed: true,
			})
		case ']':
			tokens = append(tokens, scanner.Token{
				Kind:      scanner.TokenArrayEnd,
				Raw:       []byte{']'},
				Completed: true,
			})
		case '"':
			tokens = append(tokens, scanner.Token{
				Kind:      scanner.TokenString,
				Raw:       []byte{'"'},
				Completed: true,
			})
		case 'n':
			// null
			if i+3 < len(closure) && string(closure[i:i+4]) == "null" {
				tokens = append(tokens, scanner.Token{
					Kind:      scanner.TokenNull,
					Raw:       []byte("null"),
					Completed: true,
				})
				i += 3
			}
		case 't':
			// true completion
			end := i
			for end < len(closure) && closure[end] >= 'a' && closure[end] <= 'z' {
				end++
			}
			tokens = append(tokens, scanner.Token{
				Kind:      scanner.TokenBool,
				Raw:       closure[i:end],
				Completed: true,
			})
			i = end - 1
		case 'f':
			// false completion
			end := i
			for end < len(closure) && closure[end] >= 'a' && closure[end] <= 'z' {
				end++
			}
			tokens = append(tokens, scanner.Token{
				Kind:      scanner.TokenBool,
				Raw:       closure[i:end],
				Completed: true,
			})
			i = end - 1
		case 'r', 'u', 'e', 'a', 'l', 's':
			// Part of a literal completion, skip (already handled)
			continue
		}
	}

	return tokens
}

// Tokens returns a channel that yields healed tokens from the stream.
func (h *Healer) Tokens() <-chan scanner.Token {
	ch := make(chan scanner.Token, 16)
	go func() {
		defer close(ch)
		for {
			token := h.NextToken()
			select {
			case ch <- token:
			case <-h.ctx.Done():
				return
			}
			if token.Kind == scanner.TokenEOF || token.Kind == scanner.TokenError {
				return
			}
		}
	}()
	return ch
}

// Err returns any error that occurred during healing.
func (h *Healer) Err() error {
	return h.err
}

// Depth returns the current nesting depth.
func (h *Healer) Depth() int {
	return h.closer.Depth()
}

// HealBytes processes incomplete JSON bytes and returns healed JSON.
// This is a convenience function for one-shot processing.
func HealBytes(data []byte) []byte {
	// First strip markdown if present
	data = StripMarkdown(data)

	// Then apply closure
	return Heal(data)
}

// HealJSON is an alias for HealBytes for discoverability.
func HealJSON(data []byte) []byte {
	return HealBytes(data)
}
