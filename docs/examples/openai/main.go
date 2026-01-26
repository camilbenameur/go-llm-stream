// Example: OpenAI-compatible streaming.
//
// This example demonstrates how to use the openai.Stream adapter
// to parse streaming responses from OpenAI's chat completions API.
package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/camilbenameur/go-llm-stream/openai"
)

func main() {
	// Simulated OpenAI streaming response
	sseStream := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":" there"},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`

	fmt.Println("OpenAI Streaming Example")
	fmt.Println("========================")
	fmt.Println()

	// Method 1: Using NextDelta for simple content extraction
	fmt.Print("Response: ")
	ctx := context.Background()
	stream := openai.NewStream(ctx, strings.NewReader(sseStream))

	for {
		delta, err := stream.NextDelta()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("\nError: %v\n", err)
			break
		}
		fmt.Print(delta)
	}
	fmt.Println()
	fmt.Println()

	// Method 2: Using channel-based API
	fmt.Println("Using channel API:")
	stream2 := openai.NewStream(ctx, strings.NewReader(sseStream))
	var fullResponse strings.Builder
	for delta := range stream2.Deltas() {
		fullResponse.WriteString(delta)
	}
	fmt.Printf("Full response: %s\n", fullResponse.String())
	fmt.Println()

	// Method 3: Using NextChunk for full chunk access
	fmt.Println("Using NextChunk for metadata:")
	stream3 := openai.NewStream(ctx, strings.NewReader(sseStream))
	for {
		chunk, err := stream3.NextChunk()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			break
		}
		fmt.Printf("  Model: %s, ID: %s\n", chunk.Model, chunk.ID)
		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta
			if delta.Role != "" {
				fmt.Printf("    Role: %s\n", delta.Role)
			}
			if delta.Content != "" {
				fmt.Printf("    Content: %q\n", delta.Content)
			}
		}
	}
}
