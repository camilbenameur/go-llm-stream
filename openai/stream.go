package openai

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/camilbenameur/go-llm-stream/sse"
)

// Stream provides an OpenAI-compatible streaming interface.
// It reads SSE events from an LLM API and extracts content deltas.
type Stream struct {
	decoder   *sse.Decoder
	ctx       context.Context
	opts      Options
	pathParts []string
	done      bool
	err       error
}

// Options configures the OpenAI stream behavior.
type Options struct {
	// HealJSON enables JSON healing for malformed/truncated payloads.
	HealJSON bool

	// DeltaPath is the JSON path to the content delta.
	// Default: "choices.0.delta.content"
	DeltaPath string

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
			healed := s.healJSON(data)
			if err := json.Unmarshal([]byte(healed), &payload); err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	// Navigate the path to extract content
	if s.pathParts == nil {
		s.pathParts = strings.Split(s.opts.DeltaPath, ".")
	}
	return s.navigatePath(payload, s.pathParts)
}

// navigatePath navigates a dot-separated path through a JSON object.
func (s *Stream) navigatePath(data map[string]any, parts []string) (string, error) {
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

// healJSON attempts to heal truncated JSON.
func (s *Stream) healJSON(data string) string {
	// Simple healing: ensure balanced brackets
	openBraces := strings.Count(data, "{") - strings.Count(data, "}")
	openBrackets := strings.Count(data, "[") - strings.Count(data, "]")

	var sb strings.Builder
	sb.WriteString(data)

	// Close any open strings (simple heuristic)
	quoteCount := strings.Count(data, `"`) - strings.Count(data, `\"`)
	if quoteCount%2 != 0 {
		sb.WriteByte('"')
	}

	// Close brackets and braces
	for i := 0; i < openBrackets; i++ {
		sb.WriteByte(']')
	}
	for i := 0; i < openBraces; i++ {
		sb.WriteByte('}')
	}

	return sb.String()
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
				healed := s.healJSON(event.Data)
				if err := json.Unmarshal([]byte(healed), &chunk); err != nil {
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
