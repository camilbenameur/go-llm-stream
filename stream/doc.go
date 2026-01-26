// Package stream provides a unified API for streaming JSON tokenization and healing.
//
// This is the primary entry point for go-llm-stream. It re-exports key types
// from the scanner and healer packages with a simplified, ergonomic interface.
//
// # Quick Start
//
//	import "github.com/camilbenameur/go-llm-stream/stream"
//
//	// Create a stream reader for JSON tokenization
//	reader := stream.NewReader(ctx, responseBody)
//	defer reader.Close()
//
//	for token := range reader.Tokens() {
//	    fmt.Printf("%s: %s\n", token.Kind, token.Raw)
//	}
//
//	// Or use the healer for malformed LLM output
//	healer := stream.NewHealer(ctx, responseBody)
//	defer healer.Close()
//
//	for token := range healer.Tokens() {
//	    // Automatically healed tokens
//	}
//
// # SSE + OpenAI Streaming
//
//	import "github.com/camilbenameur/go-llm-stream/openai"
//
//	stream := openai.NewStream(ctx, resp.Body)
//	for delta := range stream.Deltas() {
//	    fmt.Print(delta)
//	}
package stream
