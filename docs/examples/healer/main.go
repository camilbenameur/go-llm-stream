// Example: Healing truncated JSON from LLM outputs.
//
// This example demonstrates how to use the stream.Healer to automatically
// fix malformed JSON, including markdown wrappers and unclosed containers.
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/camilbenameur/go-llm-stream/stream"
)

func main() {
	// Simulated truncated LLM output with markdown wrapper
	truncatedJSON := "```json\n{\"name\": \"Alice\", \"items\": [1, 2, 3"

	fmt.Println("Input (truncated):")
	fmt.Println(truncatedJSON)
	fmt.Println()

	// Create a healer that strips markdown and closes containers
	ctx := context.Background()
	healer := stream.NewHealer(ctx, strings.NewReader(truncatedJSON),
		stream.WithStripMarkdown(true), // Remove ```json blocks
		stream.WithAutoClose(true),     // Close unclosed containers
	)
	defer healer.Close()

	fmt.Println("Healed tokens:")
	fmt.Println("==============")

	var result strings.Builder
	for token := range healer.Tokens() {
		switch token.Kind {
		case stream.TokenObjectStart:
			result.WriteString("{")
		case stream.TokenObjectEnd:
			result.WriteString("}")
		case stream.TokenArrayStart:
			result.WriteString("[")
		case stream.TokenArrayEnd:
			result.WriteString("]")
		case stream.TokenString:
			result.Write(token.Raw)
		case stream.TokenNumber:
			result.Write(token.Raw)
		case stream.TokenBool:
			result.Write(token.Raw)
		case stream.TokenNull:
			result.WriteString("null")
		case stream.TokenColon:
			result.WriteString(":")
		case stream.TokenComma:
			result.WriteString(",")
		case stream.TokenEOF:
			// Done
		case stream.TokenError:
			fmt.Printf("Error: %v\n", token.Err)
		}
	}

	fmt.Println("Reconstructed JSON:")
	fmt.Println(result.String())
}
