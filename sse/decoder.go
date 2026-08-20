package sse

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"sync"
)

// Event represents a single Server-Sent Event.
type Event struct {
	// Event is the event type (from "event:" field). Defaults to "message".
	Event string

	// ID is the event ID (from "id:" field).
	ID string

	// Data is the event data (from "data:" field(s)).
	// Multiple data fields are joined with newlines.
	Data string

	// Retry is the reconnection time in milliseconds (from "retry:" field).
	// Zero if not specified.
	Retry int
}

// Decoder reads and parses Server-Sent Events from an io.Reader.
// It handles partial frames across reads, comment lines, and multi-line data.
type Decoder struct {
	scanner *bufio.Scanner
	lastID  string
	err     error
}

// DefaultMaxLineSize is the largest single SSE line NewDecoder accepts.
//
// bufio.Scanner defaults to 64 KiB, which is too small in practice: a single
// data: line carrying tool-call arguments routinely exceeds it, and the
// failure is unrecoverable (the decoder latches the error and the stream
// ends). The cap still has to exist, because an SSE line that never
// terminates would otherwise grow without bound.
const DefaultMaxLineSize = 8 << 20 // 8 MiB

// NewDecoder creates a new SSE decoder that reads from r.
// Lines longer than DefaultMaxLineSize are rejected; use NewDecoderSize to
// choose a different limit.
func NewDecoder(r io.Reader) *Decoder {
	return NewDecoderSize(r, DefaultMaxLineSize)
}

// NewDecoderSize creates an SSE decoder that accepts lines up to maxLineSize
// bytes. A non-positive maxLineSize selects DefaultMaxLineSize.
func NewDecoderSize(r io.Reader, maxLineSize int) *Decoder {
	if maxLineSize <= 0 {
		maxLineSize = DefaultMaxLineSize
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineSize)
	return &Decoder{
		scanner: sc,
	}
}

// Next returns the next SSE event from the stream.
// Returns io.EOF when the stream ends.
// Returns an error if the stream cannot be read.
func (d *Decoder) Next() (Event, error) {
	if d.err != nil {
		return Event{}, d.err
	}

	var event Event
	var dataLines []string
	hasData := false

	for d.scanner.Scan() {
		line := d.scanner.Text()

		// Empty line signals end of event
		if line == "" {
			if hasData || event.Event != "" || event.ID != "" || event.Retry != 0 {
				// Finalize the event
				event.Data = strings.Join(dataLines, "\n")
				if event.Event == "" {
					event.Event = "message"
				}
				if event.ID == "" && d.lastID != "" {
					event.ID = d.lastID
				}
				return event, nil
			}
			// Empty event, continue reading
			continue
		}

		// Comment lines start with ':'
		if strings.HasPrefix(line, ":") {
			// Keep-alive or comment, ignore
			continue
		}

		// Parse field:value pairs
		field, value := parseField(line)

		switch field {
		case "event":
			event.Event = value
		case "data":
			dataLines = append(dataLines, value)
			hasData = true
		case "id":
			// ID must not contain null
			if !strings.Contains(value, "\x00") {
				event.ID = value
				d.lastID = value
			}
		case "retry":
			// Retry must be digits only
			if n, err := strconv.Atoi(value); err == nil && n >= 0 {
				event.Retry = n
			}
		}
		// Unknown fields are ignored per SSE spec
	}

	// Check for scanner error
	if err := d.scanner.Err(); err != nil {
		d.err = err
		return Event{}, err
	}

	// End of stream - emit any pending event
	if hasData || event.Event != "" || event.ID != "" || event.Retry != 0 {
		event.Data = strings.Join(dataLines, "\n")
		if event.Event == "" {
			event.Event = "message"
		}
		if event.ID == "" && d.lastID != "" {
			event.ID = d.lastID
		}
		d.err = io.EOF
		return event, nil
	}

	d.err = io.EOF
	return Event{}, io.EOF
}

// LastEventID returns the last seen event ID.
// This can be used for reconnection with the Last-Event-ID header.
func (d *Decoder) LastEventID() string {
	return d.lastID
}

// parseField parses an SSE line into field and value.
// Format: "field: value" or "field:value" or "field"
func parseField(line string) (field, value string) {
	colonIdx := strings.Index(line, ":")
	if colonIdx == -1 {
		return line, ""
	}
	field = line[:colonIdx]
	value = line[colonIdx+1:]
	// Remove leading space after colon if present
	if len(value) > 0 && value[0] == ' ' {
		value = value[1:]
	}
	return field, value
}

// EventStream represents a streaming SSE event iterator.
// It provides a channel-based interface for consuming events.
type EventStream struct {
	decoder   *Decoder
	events    chan Event
	mu        sync.Mutex
	err       error
	done      chan struct{}
	closeOnce sync.Once
}

// NewEventStream creates a channel-based SSE event stream.
func NewEventStream(r io.Reader) *EventStream {
	s := &EventStream{
		decoder: NewDecoder(r),
		events:  make(chan Event, 8),
		done:    make(chan struct{}),
	}
	go s.run()
	return s
}

func (s *EventStream) run() {
	defer close(s.events)
	for {
		event, err := s.decoder.Next()
		if err != nil {
			if err != io.EOF {
				s.mu.Lock()
				s.err = err
				s.mu.Unlock()
			}
			return
		}
		select {
		case s.events <- event:
		case <-s.done:
			return
		}
	}
}

// Events returns a channel of SSE events.
func (s *EventStream) Events() <-chan Event {
	return s.events
}

// Close stops the event stream. It is safe to call more than once and from
// multiple goroutines.
func (s *EventStream) Close() {
	s.closeOnce.Do(func() { close(s.done) })
}

// Err returns any error encountered during streaming.
// It is safe to call concurrently with a drain of Events.
func (s *EventStream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}
