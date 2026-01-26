// Example: Parsing Server-Sent Events (SSE).
//
// This example demonstrates how to use the sse.Decoder to parse
// SSE streams from LLM APIs like OpenAI and Anthropic.
package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/camilbenameur/go-llm-stream/sse"
)

func main() {
	// Simulated SSE stream (OpenAI format)
	sseStream := `event: message
data: {"id":"chatcmpl-123","choices":[{"delta":{"role":"assistant"}}]}

data: {"id":"chatcmpl-123","choices":[{"delta":{"content":"Hello"}}]}

data: {"id":"chatcmpl-123","choices":[{"delta":{"content":" World"}}]}

data: {"id":"chatcmpl-123","choices":[{"delta":{"content":"!"}}]}

data: [DONE]

`

	fmt.Println("Parsing SSE stream:")
	fmt.Println("===================")

	decoder := sse.NewDecoder(strings.NewReader(sseStream))

	for {
		event, err := decoder.Next()
		if err == io.EOF {
			fmt.Println("\n✓ Stream complete!")
			break
		}
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			break
		}

		fmt.Printf("Event: %-10s Data: %s\n", event.Event, event.Data)
	}

	fmt.Printf("\nLast Event ID: %q\n", decoder.LastEventID())
}
