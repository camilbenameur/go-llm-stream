package openai

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/camilbenameur/go-llm-stream/healer"
	"github.com/camilbenameur/go-llm-stream/sse"
)

// Common delta paths for the providers this adapter is known to work with.
// Pass them to WithDeltaPath / WithDeltaPaths, or use the convenience options.
const (
	// OpenAIDeltaPath is the content path for OpenAI-style chat completion chunks.
	OpenAIDeltaPath = "choices.0.delta.content"
	// AnthropicDeltaPath is the text path for Anthropic content_block_delta events.
	AnthropicDeltaPath = "delta.text"
)

// Stream provides a streaming content extractor for LLM SSE APIs.
// It reads SSE events and pulls out content deltas by JSON path. The default
// path targets OpenAI-style chunks, but the path(s) are configurable so the same
// type can consume other shapes (e.g. Anthropic) — see WithDeltaPaths and
// WithAnthropicFormat.
type Stream struct {
	decoder    *sse.Decoder
	ctx        context.Context
	opts       Options
	deltaPaths [][]string // cached, split candidate paths (lazy)
	done       bool
	err        error
}

// Options configures the stream behavior.
type Options struct {
	// HealJSON enables JSON healing for malformed/truncated payloads.
	HealJSON bool

	// DeltaPath is the JSON path to the content delta.
	// Default: "choices.0.delta.content". Ignored if DeltaPaths is set.
	DeltaPath string

	// DeltaPaths is an ordered list of candidate JSON paths. For each event the
	// first path that yields content wins. Use this for streams whose shape
	// varies (e.g. mixed OpenAI/Anthropic events). When non-empty it takes
	// precedence over DeltaPath.
	DeltaPaths []string

	// DoneMarker is the string that signals end of stream.
	// Default: "[DONE]"
	DoneMarker string
}

// DefaultOptions returns the recommended stream options.
func DefaultOptions() Options {
	return Options{
		HealJSON:   false,
		DeltaPath:  "choices.0.delta.content",
		DoneMarker: "[DONE]",
	}
}

// Option is a functional option for configuring the Stream.
type Option func(*Options)

// WithHealJSON enables or disables JSON healing.
func WithHealJSON(enable bool) Option {
	return func(o *Options) {
		o.HealJSON = enable
	}
}

// WithDeltaPath sets the JSON path to extract content from.
func WithDeltaPath(path string) Option {
	return func(o *Options) {
		o.DeltaPath = path
	}
}

// WithDeltaPaths sets multiple candidate JSON paths, tried in order; the first
// path that yields content for a given event wins. Use this to consume streams
// whose shape varies. It overrides WithDeltaPath.
//
// Example — accept either OpenAI or Anthropic deltas from the same stream:
//
//	openai.NewStream(ctx, r, openai.WithDeltaPaths(openai.OpenAIDeltaPath, openai.AnthropicDeltaPath))
func WithDeltaPaths(paths ...string) Option {
	return func(o *Options) {
		o.DeltaPaths = paths
	}
}

// WithAnthropicFormat configures the stream for Anthropic-style streaming, where
// content arrives as content_block_delta events with the text at "delta.text".
//
// Scope: this maps the delta *content shape* only. Anthropic-specific event
// semantics (event: lines, message_stop, ping) are not interpreted — non-content
// events are simply skipped and the stream ends when the reader closes.
func WithAnthropicFormat() Option {
	return func(o *Options) {
		o.DeltaPaths = []string{AnthropicDeltaPath}
	}
}

// WithDoneMarker sets the end-of-stream marker.
func WithDoneMarker(marker string) Option {
	return func(o *Options) {
		o.DoneMarker = marker
	}
}

// NewStream creates a new OpenAI-compatible stream from an io.Reader.
// The reader should provide SSE-formatted data from an OpenAI-compatible API.
func NewStream(ctx context.Context, r io.Reader, options ...Option) *Stream {
	opts := DefaultOptions()
	for _, opt := range options {
		opt(&opts)
	}

	s := &Stream{
		decoder: sse.NewDecoder(r),
		ctx:     ctx,
		opts:    opts,
	}

	return s
}

// NextDelta returns the next content delta from the stream.
// Returns io.EOF when the stream ends (after receiving [DONE] or stream closes).
// Returns an error if parsing fails.
func (s *Stream) NextDelta() (string, error) {
	if s.done {
		return "", io.EOF
	}

	// Check context
	select {
	case <-s.ctx.Done():
		s.err = s.ctx.Err()
		return "", s.err
	default:
	}

	for {
		event, err := s.decoder.Next()
		if err == io.EOF {
			s.done = true
			return "", io.EOF
		}
		if err != nil {
			s.err = err
			return "", err
		}

		// Check for done marker
		if strings.TrimSpace(event.Data) == s.opts.DoneMarker {
			s.done = true
			return "", io.EOF
		}

		// Parse the JSON payload
		delta, err := s.extractDelta(event.Data)
		if err != nil {
			// Skip events that don't have content (could be metadata)
			continue
		}

		return delta, nil
	}
}

// NextEvent returns the next raw SSE event from the stream.
// This is useful for accessing metadata or handling custom event types.
func (s *Stream) NextEvent() (sse.Event, error) {
	if s.done {
		return sse.Event{}, io.EOF
	}

	// Check context
	select {
	case <-s.ctx.Done():
		s.err = s.ctx.Err()
		return sse.Event{}, s.err
	default:
	}

	event, err := s.decoder.Next()
	if err == io.EOF {
		s.done = true
	}
	if err != nil {
		s.err = err
	}
	return event, err
}

// Err returns any error encountered during streaming.
func (s *Stream) Err() error {
	return s.err
}

// extractDelta extracts the content delta from a JSON payload.
func (s *Stream) extractDelta(data string) (string, error) {
	// Parse JSON
	var payload map[string]any
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		// Try to heal the JSON if enabled
		if s.opts.HealJSON {
			healed := healer.HealJSON([]byte(data))
			if err := json.Unmarshal(healed, &payload); err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	// Try each candidate path in order; the first that yields content wins.
	// This lets one Stream tolerate shape variation (e.g. OpenAI's
	// choices.0.delta.content vs Anthropic's delta.text) and gracefully skip
	// events that don't carry content (returning io.EOF so NextDelta moves on).
	lastErr := error(io.EOF)
	for _, parts := range s.candidatePaths() {
		delta, err := navigatePath(payload, parts)
		if err == nil {
			return delta, nil
		}
		lastErr = err
	}
	return "", lastErr
}

// candidatePaths returns the split delta paths to try, computed once and cached.
func (s *Stream) candidatePaths() [][]string {
	if s.deltaPaths != nil {
		return s.deltaPaths
	}
	raw := s.opts.DeltaPaths
	if len(raw) == 0 {
		raw = []string{s.opts.DeltaPath}
	}
	s.deltaPaths = make([][]string, 0, len(raw))
	for _, p := range raw {
		if p == "" {
			continue
		}
		s.deltaPaths = append(s.deltaPaths, strings.Split(p, "."))
	}
	return s.deltaPaths
}

// navigatePath navigates a dot-separated path through a JSON object.
func navigatePath(data map[string]any, parts []string) (string, error) {
	var current any = data

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]any:
			var ok bool
			current, ok = v[part]
			if !ok {
				return "", io.EOF // Field not present
			}
		case []any:
			// Parse array index
			var idx int
			if err := json.Unmarshal([]byte(part), &idx); err != nil {
				return "", err
			}
			if idx < 0 || idx >= len(v) {
				return "", io.EOF
			}
			current = v[idx]
		default:
			return "", io.EOF
		}
	}

	// Convert to string
	switch v := current.(type) {
	case string:
		return v, nil
	case nil:
		return "", io.EOF // null content
	default:
		// Marshal other types to string
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}

// Chunk represents a parsed chunk from the OpenAI stream.
type Chunk struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

// Choice represents a choice in an OpenAI response.
type Choice struct {
	Index        int         `json:"index"`
	Delta        Delta       `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
	Logprobs     interface{} `json:"logprobs"`
}

// Delta represents the content delta in a streaming response.
type Delta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// NextChunk returns the next fully parsed chunk from the stream.
// This provides more structure than NextDelta for advanced usage.
func (s *Stream) NextChunk() (*Chunk, error) {
	if s.done {
		return nil, io.EOF
	}

	// Check context
	select {
	case <-s.ctx.Done():
		s.err = s.ctx.Err()
		return nil, s.err
	default:
	}

	for {
		event, err := s.decoder.Next()
		if err == io.EOF {
			s.done = true
			return nil, io.EOF
		}
		if err != nil {
			s.err = err
			return nil, err
		}

		// Check for done marker
		if strings.TrimSpace(event.Data) == s.opts.DoneMarker {
			s.done = true
			return nil, io.EOF
		}

		// Parse the JSON payload
		var chunk Chunk
		if err := json.Unmarshal([]byte(event.Data), &chunk); err != nil {
			// Try to heal if enabled
			if s.opts.HealJSON {
				healed := healer.HealJSON([]byte(event.Data))
				if err := json.Unmarshal(healed, &chunk); err != nil {
					continue // Skip malformed chunks
				}
			} else {
				continue // Skip malformed chunks
			}
		}

		return &chunk, nil
	}
}

// Deltas returns a channel that yields content deltas.
// The channel is closed when the stream ends or an error occurs.
func (s *Stream) Deltas() <-chan string {
	ch := make(chan string, 8)
	go func() {
		defer close(ch)
		for {
			delta, err := s.NextDelta()
			if err != nil {
				return
			}
			select {
			case ch <- delta:
			case <-s.ctx.Done():
				return
			}
		}
	}()
	return ch
}

// Chunks returns a channel that yields parsed chunks.
// The channel is closed when the stream ends or an error occurs.
func (s *Stream) Chunks() <-chan *Chunk {
	ch := make(chan *Chunk, 8)
	go func() {
		defer close(ch)
		for {
			chunk, err := s.NextChunk()
			if err != nil {
				return
			}
			select {
			case ch <- chunk:
			case <-s.ctx.Done():
				return
			}
		}
	}()
	return ch
}
