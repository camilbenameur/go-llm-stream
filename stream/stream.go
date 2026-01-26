package stream

import (
	"context"
	"io"

	"github.com/camilbenameur/go-llm-stream/healer"
	"github.com/camilbenameur/go-llm-stream/scanner"
)

// Re-export token types from scanner for convenience
type (
	// Token represents a single JSON token with metadata.
	Token = scanner.Token

	// TokenKind represents the type of a JSON token.
	TokenKind = scanner.TokenKind
)

// Token kind constants
const (
	TokenObjectStart = scanner.TokenObjectStart
	TokenObjectEnd   = scanner.TokenObjectEnd
	TokenArrayStart  = scanner.TokenArrayStart
	TokenArrayEnd    = scanner.TokenArrayEnd
	TokenString      = scanner.TokenString
	TokenNumber      = scanner.TokenNumber
	TokenBool        = scanner.TokenBool
	TokenNull        = scanner.TokenNull
	TokenColon       = scanner.TokenColon
	TokenComma       = scanner.TokenComma
	TokenEOF         = scanner.TokenEOF
	TokenIncomplete  = scanner.TokenIncomplete
	TokenError       = scanner.TokenError
)

// Options configures the stream reader and healer behavior.
type Options struct {
	// BufferSize is the size of the read buffer in bytes.
	// Default: 4096
	BufferSize int

	// StripMarkdown enables stripping of markdown code block delimiters.
	// Only applies to healer streams.
	// Default: true (for healer)
	StripMarkdown bool

	// AutoClose enables automatic closure of unclosed JSON containers.
	// Only applies to healer streams.
	// Default: true (for healer)
	AutoClose bool

	// IgnoreTrailingJunk ignores content after the root JSON value closes.
	// Only applies to healer streams.
	// Default: true (for healer)
	IgnoreTrailingJunk bool

	// CompleteStrings automatically closes unterminated strings.
	// Only applies to healer streams.
	// Default: true (for healer)
	CompleteStrings bool

	// CompleteLiterals automatically completes partial literals.
	// Only applies to healer streams.
	// Default: true (for healer)
	CompleteLiterals bool
}

// DefaultOptions returns the recommended default options.
func DefaultOptions() Options {
	return Options{
		BufferSize:         4096,
		StripMarkdown:      true,
		AutoClose:          true,
		IgnoreTrailingJunk: true,
		CompleteStrings:    true,
		CompleteLiterals:   true,
	}
}

// Option is a functional option for configuring streams.
type Option func(*Options)

// WithBufferSize sets the read buffer size.
func WithBufferSize(size int) Option {
	return func(o *Options) {
		if size > 0 {
			o.BufferSize = size
		}
	}
}

// WithStripMarkdown enables or disables markdown stripping.
func WithStripMarkdown(enable bool) Option {
	return func(o *Options) {
		o.StripMarkdown = enable
	}
}

// WithAutoClose enables or disables automatic container closure.
func WithAutoClose(enable bool) Option {
	return func(o *Options) {
		o.AutoClose = enable
	}
}

// WithIgnoreTrailingJunk enables or disables ignoring trailing content.
func WithIgnoreTrailingJunk(enable bool) Option {
	return func(o *Options) {
		o.IgnoreTrailingJunk = enable
	}
}

// WithCompleteStrings enables or disables string completion.
func WithCompleteStrings(enable bool) Option {
	return func(o *Options) {
		o.CompleteStrings = enable
	}
}

// WithCompleteLiterals enables or disables literal completion.
func WithCompleteLiterals(enable bool) Option {
	return func(o *Options) {
		o.CompleteLiterals = enable
	}
}

// Reader is a streaming JSON tokenizer.
// It wraps scanner.StreamReader with a simplified interface.
type Reader struct {
	stream *scanner.StreamReader
}

// NewReader creates a new streaming JSON reader.
// The reader tokenizes JSON from the provided io.Reader.
func NewReader(ctx context.Context, r io.Reader, opts ...Option) *Reader {
	// Apply options (currently unused for base reader, reserved for future)
	_ = applyOptions(opts...)

	return &Reader{
		stream: scanner.NewStreamReader(ctx, r),
	}
}

// NextToken returns the next JSON token from the stream.
// Returns TokenEOF when the stream ends normally.
// Returns TokenError if an error occurs.
func (r *Reader) NextToken() Token {
	return r.stream.NextToken()
}

// Tokens returns a channel of tokens from the stream.
// The channel is closed when the stream ends or an error occurs.
func (r *Reader) Tokens() <-chan Token {
	return r.stream.Tokens()
}

// BytesConsumed returns the total bytes read from the stream.
func (r *Reader) BytesConsumed() int64 {
	return r.stream.BytesConsumed()
}

// Close releases resources associated with the reader.
func (r *Reader) Close() error {
	return r.stream.Close()
}

// Healer is a streaming JSON tokenizer with automatic healing.
// It handles truncated streams, incomplete tokens, and markdown wrappers.
type Healer struct {
	healer *healer.Healer
}

// NewHealer creates a new healing JSON stream reader.
// It automatically fixes common LLM output issues like truncated JSON,
// unclosed containers, and markdown code block delimiters.
func NewHealer(ctx context.Context, r io.Reader, opts ...Option) *Healer {
	options := applyOptions(opts...)

	// Convert to healer options
	healerOpts := []healer.Option{
		healer.WithStripMarkdown(options.StripMarkdown),
		healer.WithAutoClose(options.AutoClose),
		healer.WithIgnoreTrailingJunk(options.IgnoreTrailingJunk),
	}

	return &Healer{
		healer: healer.NewFromReader(ctx, r, healerOpts...),
	}
}

// NextToken returns the next healed JSON token from the stream.
// Returns TokenEOF when the stream ends normally.
// Returns TokenError if an error occurs.
func (h *Healer) NextToken() Token {
	return h.healer.NextToken()
}

// Tokens returns a channel of healed tokens from the stream.
// The channel is closed when the stream ends or an error occurs.
func (h *Healer) Tokens() <-chan Token {
	return h.healer.Tokens()
}

// Close releases resources associated with the healer.
func (h *Healer) Close() error {
	return h.healer.Close()
}

// applyOptions applies functional options and returns the final Options.
func applyOptions(opts ...Option) Options {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(&options)
	}
	return options
}
