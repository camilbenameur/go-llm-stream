// Package scanner provides a resumable, O(n) JSON scanner for LLM streams.
//
// This package implements the core tokenization layer for go-llm-stream,
// designed to handle streaming JSON data that arrives in arbitrary chunks.
// It is based on the Go standard library's encoding/json/scanner.go but
// adapted for streaming scenarios.
//
// # Key Features
//
//   - O(n) parsing complexity: Each byte is examined exactly once
//   - Zero-allocation scanner: Core scanner has 0 B/op in benchmarks
//   - Resumable state machine: Can pause and resume parsing at any byte boundary
//   - Sync.Pool integration: Efficient memory reuse for high-throughput scenarios
//   - Context support: Full cancellation support for streaming operations
//
// # Architecture
//
// The package is organized in three layers:
//
//   - Scanner: Low-level byte-by-byte state machine (27 states)
//   - Tokenizer: Buffers bytes and emits complete Token objects
//   - StreamReader: io.Reader wrapper with context support
//
// # Usage
//
// For streaming data from an io.Reader:
//
//	reader := getOpenAIStream() // returns io.Reader
//	stream := scanner.NewStreamReader(ctx, reader)
//	defer stream.Close()
//
//	for token := range stream.Tokens() {
//	    switch token.Kind {
//	    case scanner.TokenObjectStart:
//	        fmt.Println("Object started")
//	    case scanner.TokenString:
//	        fmt.Printf("String: %s\n", token.Raw)
//	    case scanner.TokenEOF:
//	        fmt.Println("Done")
//	    case scanner.TokenError:
//	        log.Fatal(token.Err)
//	    }
//	}
//
// For low-level control, use the Tokenizer directly:
//
//	tok := scanner.NewTokenizer()
//	defer tok.Free()
//
//	tok.Append([]byte(`{"name": "John"`))
//	for {
//	    token := tok.NextToken()
//	    if token.Kind == scanner.TokenIncomplete {
//	        // Need more data
//	        tok.Append(moreData)
//	        continue
//	    }
//	    // Process token...
//	}
//
// # State Machine
//
// The scanner implements a 27-state machine based on the Go stdlib's
// function pointer pattern, but using a serializable uint8 enum instead.
// This allows the scanner state to be saved and restored for resumability.
//
// States are organized into groups:
//   - Value entry states (4): Handle start of values
//   - String parsing states (6): Handle string literals and escapes
//   - Number parsing states (8): Handle all numeric formats
//   - Literal parsing states (10): Handle true, false, null
//   - Completion states (3): Handle value completion
//
// # Performance
//
// The low-level Scanner processes JSON at ~300 MB/s with zero allocations when
// fed a contiguous buffer (BenchmarkScannerSmall: ~85 ns/op, 316 MB/s, 0
// allocs/op). The Tokenizer and StreamReader allocate proportionally to input
// as they materialize tokens. See docs/PERFORMANCE.md for the full, reproducible
// benchmark suite.
package scanner
