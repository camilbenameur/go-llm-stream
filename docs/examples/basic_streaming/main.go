// Example: Basic JSON streaming with the stream package.
//
// This example demonstrates how to parse JSON tokens from a stream
// using the stream.Reader.
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/camilbenameur/go-llm-stream/stream"
)

func main() {
	// Sample JSON data (could be from an HTTP response body)
	jsonData := `{
		"model": "gpt-4",
		"choices": [
			{
				"message": {
					"role": "assistant",
					"content": "Hello! How can I help you?"
				}
			}
		]
	}`

	// Create a stream reader
	ctx := context.Background()
	reader := stream.NewReader(ctx, strings.NewReader(jsonData))
	defer reader.Close()

	fmt.Println("Parsing JSON tokens:")
	fmt.Println("====================")

	depth := 0
	for token := range reader.Tokens() {
		indent := strings.Repeat("  ", depth)

		switch token.Kind {
		case stream.TokenObjectStart:
			fmt.Printf("%s{\n", indent)
			depth++
		case stream.TokenObjectEnd:
			depth--
			fmt.Printf("%s}\n", strings.Repeat("  ", depth))
		case stream.TokenArrayStart:
			fmt.Printf("%s[\n", indent)
			depth++
		case stream.TokenArrayEnd:
			depth--
			fmt.Printf("%s]\n", strings.Repeat("  ", depth))
		case stream.TokenString:
			if token.IsKey {
				fmt.Printf("%sKey: %s\n", indent, token.Raw)
			} else {
				fmt.Printf("%sString: %s\n", indent, token.Raw)
			}
		case stream.TokenNumber:
			fmt.Printf("%sNumber: %s\n", indent, token.Raw)
		case stream.TokenBool:
			fmt.Printf("%sBool: %s\n", indent, token.Raw)
		case stream.TokenNull:
			fmt.Printf("%sNull\n", indent)
		case stream.TokenEOF:
			fmt.Println("\n✓ Parsing complete!")
		case stream.TokenError:
			fmt.Printf("Error: %v\n", token.Err)
		}
	}

	fmt.Printf("\nBytes processed: %d\n", reader.BytesConsumed())
}
