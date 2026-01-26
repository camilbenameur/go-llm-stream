// Package healer provides semantic healing for malformed JSON from LLM streams.
//
// LLMs often produce structurally broken but semantically clear JSON output.
// This package implements algorithms to repair such output in real-time,
// enabling reliable structured data extraction from streaming responses.
//
// # Key Features
//
//   - Minimal Closure Algorithm: Automatically closes unclosed objects/arrays
//   - Markdown Stripping: Filters out ```json ... ``` code block delimiters
//   - Premature Stop Handling: Gracefully handles truncated responses
//   - String Completion: Closes unterminated string literals
//
// # Architecture
//
// The healer operates as a filter layer on top of the scanner package:
//
//	LLM Stream → Markdown Filter → Scanner → Healer → Valid JSON Tokens
//
// # Usage
//
// Wrap a StreamReader with the healer:
//
//	reader := getOpenAIStream()
//	stream := scanner.NewStreamReader(ctx, reader)
//	healed := healer.New(stream)
//	defer healed.Close()
//
//	for token := range healed.Tokens() {
//	    // Tokens are guaranteed to form valid JSON structure
//	}
//
// Or use the standalone healing function:
//
//	incomplete := []byte(`{"name": "John", "items": [1, 2`)
//	complete := healer.Heal(incomplete)
//	// Result: {"name": "John", "items": [1, 2]}
//
// # Failure Modes Handled
//
// The healer addresses common LLM output issues:
//
//   - Truncation due to max_tokens limits
//   - Markdown code block delimiters in output
//   - Unclosed strings, objects, and arrays
//   - Trailing commas before closure
//   - Junk text after valid JSON (ignored)
package healer
