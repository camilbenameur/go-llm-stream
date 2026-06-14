// Command structured_streaming demonstrates go-llm-stream's two real
// differentiators over the standard library, using a mock LLM response so it
// runs with no API key:
//
//  1. Field-at-a-time consumption: a single structured-output object streams in
//     small chunks, and each field is surfaced the moment it completes — you can
//     act on "title" long before "body" finishes.
//  2. Healing: the mock stream is wrapped in a ```json markdown fence and is
//     TRUNCATED (the closing brace and fence never arrive), exactly like a model
//     that hit its token limit. encoding/json rejects it outright; the healer
//     recovers valid JSON.
//
// Run it:
//
//	go run ./docs/examples/structured_streaming
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/camilbenameur/go-llm-stream/healer"
	"github.com/camilbenameur/go-llm-stream/stream"
)

// mockResponse is what a model might stream for a structured-output request:
// markdown-fenced and cut off mid-stream (no closing `}` and no closing ```).
const mockResponse = "```json\n" +
	`{"title":"The Rise of Incremental Parsing",` +
	`"score":9,` +
	`"published":true,` +
	`"summary":"Process each byte once.",` +
	`"body":"Streaming LLMs emit JSON one token at a`
	// <-- truncated mid-sentence inside "body": no closing quote, no closing
	//     brace, no closing fence. Exactly like a model that hit its token limit.

// chunkedReader hands out the payload in small pieces with a tiny delay, to
// simulate a real network stream rather than one big buffer.
type chunkedReader struct {
	data      string
	pos       int
	chunkSize int
	delay     time.Duration
}

func (c *chunkedReader) Read(p []byte) (int, error) {
	if c.pos >= len(c.data) {
		return 0, io.EOF
	}
	time.Sleep(c.delay)
	end := c.pos + c.chunkSize
	if end > len(c.data) {
		end = len(c.data)
	}
	n := copy(p, c.data[c.pos:end])
	c.pos += n
	return n, nil
}

func main() {
	fmt.Println("=== 1. Field-at-a-time streaming (with healing) ===")
	fmt.Println("Streaming a truncated, markdown-fenced structured output in 16-byte chunks...")
	fmt.Println()

	ctx := context.Background()
	reader := &chunkedReader{data: mockResponse, chunkSize: 16, delay: 8 * time.Millisecond}

	h := stream.NewHealer(ctx, reader) // default opts: strip markdown + auto-close
	defer h.Close()

	var pendingKey string
	for token := range h.Tokens() {
		// Tokens synthesized by the healer to close a truncated stream carry no
		// position (End == 0); real tokens from the wire have a byte offset.
		injected := token.End == 0
		switch token.Kind {
		case stream.TokenString:
			if token.IsKey {
				pendingKey = unquote(token.Raw)
				continue
			}
			if injected {
				continue
			}
			emit(pendingKey, fmt.Sprintf("%q", unquote(token.Raw)), token.End)
			pendingKey = ""
		case stream.TokenNumber, stream.TokenBool, stream.TokenNull:
			if injected {
				continue
			}
			emit(pendingKey, string(token.Raw), token.End)
			pendingKey = ""
		case stream.TokenObjectEnd:
			if pendingKey != "" {
				fmt.Printf("  [in-flight] %-10s = <truncated mid-value — recovered by healing in step 2>\n", pendingKey)
				pendingKey = ""
			}
			fmt.Println("  ↳ [healer] injected the closing '}' — the model never sent it")
		case stream.TokenError:
			fmt.Printf("  ! error: %v\n", token.Err)
		}
	}

	fmt.Println()
	fmt.Println("=== 2. stdlib vs healer on the raw truncated bytes ===")
	raw := []byte(mockResponse)

	var viaStdlib map[string]any
	if err := json.Unmarshal(raw, &viaStdlib); err != nil {
		fmt.Printf("  encoding/json.Unmarshal(raw):    FAILED — %v\n", err)
	}

	healed := healer.HealBytes(raw) // strips the fence + closes the JSON
	var viaHealer map[string]any
	if err := json.Unmarshal(healed, &viaHealer); err != nil {
		fmt.Printf("  json.Unmarshal(HealBytes(raw)):  FAILED — %v\n", err)
	} else {
		fmt.Printf("  json.Unmarshal(HealBytes(raw)):  OK — recovered %d fields\n", len(viaHealer))
		fmt.Printf("  healed JSON: %s\n", healed)
	}
}

// emit prints a field as soon as it has fully arrived, with the stream offset
// (in bytes) at which it completed — making the "early field access" concrete.
func emit(key, value string, offset int64) {
	if key == "" {
		return
	}
	fmt.Printf("  [byte %3d] %-10s = %s\n", offset, key, value)
}

// unquote strips the surrounding double quotes from a raw JSON string token.
func unquote(raw []byte) string {
	s := string(raw)
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		return s[1 : len(s)-1]
	}
	return strings.TrimPrefix(s, `"`) // truncated string: only the opening quote
}
