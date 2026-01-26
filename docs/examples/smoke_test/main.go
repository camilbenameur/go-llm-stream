package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/camilbenameur/go-llm-stream/scanner"
)

func main() {
	// Example: Parse a streaming JSON response
	jsonData := `{
		"model": "gpt-4",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Hello! How can I help you today?"
				},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 15,
			"total_tokens": 25
		}
	}`

	// Create a stream reader from the JSON
	reader := strings.NewReader(jsonData)
	stream := scanner.NewStreamReader(context.Background(), reader)
	defer stream.Close()

	fmt.Println("Parsing JSON tokens:")
	fmt.Println("====================")

	depth := 0
	for token := range stream.Tokens() {
		indent := strings.Repeat("  ", depth)

		switch token.Kind {
		case scanner.TokenObjectStart:
			fmt.Printf("%s{\n", indent)
			depth++
		case scanner.TokenObjectEnd:
			depth--
			indent = strings.Repeat("  ", depth)
			fmt.Printf("%s}\n", indent)
		case scanner.TokenArrayStart:
			fmt.Printf("%s[\n", indent)
			depth++
		case scanner.TokenArrayEnd:
			depth--
			indent = strings.Repeat("  ", depth)
			fmt.Printf("%s]\n", indent)
		case scanner.TokenString:
			if token.IsKey {
				fmt.Printf("%sKey: %s\n", indent, token.Raw)
			} else {
				fmt.Printf("%sString: %s\n", indent, token.Raw)
			}
		case scanner.TokenNumber:
			fmt.Printf("%sNumber: %s\n", indent, token.Raw)
		case scanner.TokenBool:
			fmt.Printf("%sBool: %s\n", indent, token.Raw)
		case scanner.TokenNull:
			fmt.Printf("%sNull\n", indent)
		case scanner.TokenEOF:
			fmt.Println("\nParsing complete!")
		case scanner.TokenError:
			fmt.Printf("Error: %v\n", token.Err)
		}
	}

	fmt.Printf("\nTotal bytes processed: %d\n", stream.BytesConsumed())
}
