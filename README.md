# go-llm-stream

[![CI Status](https://github.com/camilbenameur/go-llm-stream/actions/workflows/go.yml/badge.svg)](https://github.com/camilbenameur/go-llm-stream/actions/workflows/go.yml)

A resumable O(n) Go library for incremental JSON parsing of LLM streams.

## Why go-llm-stream?

The Go LLM ecosystem (OpenAI, LangChainGo) can encounter O(n^2) performance overhead when parsing streams. This occurs because existing implementations typically accumulate and re-parse the entire response buffer on every new chunk. 

go-llm-stream provides a byte-level, resumable state machine that processes each byte exactly once, ensuring O(n) parsing complexity with constant memory overhead.

## Features

- **O(n) Performance**: Constant memory overhead, no re-parsing as streams grow.
- **Automatic Healing**: Repairs truncated or malformed JSON from LLM outputs.
- **SSE Decoder**: Native Server-Sent Events (SSE) parser for OpenAI and Anthropic.
- **OpenAI Compatible**: Adapters for common streaming APIs.
- **Resumable**: Save and restore parsing state using snapshots.

## Installation

```bash
go get github.com/camilbenameur/go-llm-stream
```

## Quick Start

```go
import "github.com/camilbenameur/go-llm-stream/stream"

// Automatically fix truncated JSON and strip markdown
healer := stream.NewHealer(ctx, responseBody)
defer healer.Close()

for token := range healer.Tokens() {
    // Process tokens as they arrive
}
```

## Documentation

- **[User Guide](docs/USER_GUIDE.md)**: Tutorials and reference.
- **[Examples](docs/examples/)**: Implementation samples.
- **[Performance](docs/PERFORMANCE.md)**: Benchmark results.
- **[Library Spec](docs/LIBRARY_SPEC.md)**: Technical architecture details.
- **[Changelog](CHANGELOG.md)**: Release history.

## Status

- **v1.0.0**: Stable release. Verified core, healer, and SSE adapters.

## Technical Details

### The O(n^2) Overhead in Streaming

Typical streaming JSON parsers re-parse the entire accumulated buffer on every new chunk:

```
Chunk 1: {"content": "H"           → Parse 17 bytes
Chunk 2: {"content": "He"          → Parse 18 bytes  
Chunk 3: {"content": "Hel"         → Parse 19 bytes
...
Chunk N: {"content": "Hello..."}   → Parse 17+N bytes
```

This leads to O(n^2) operations relative to the total number of chunks and response size. For a 10KB response in 100 chunks, this results in approximately 5,000 redundant operations.

### O(n) Implementation

This library uses a state machine to process each byte once:

```
Chunk 1: Process bytes 0-16    → State: InString
Chunk 2: Process byte 17       → State: InString (resumed)
Chunk 3: Process byte 18       → State: InString (resumed)
...
```

The total work is linear O(n) relative to the total response size.

### Benchmarks

```
BenchmarkScannerSmall-12    13,636,113    85.37 ns/op    316 MB/s    0 B/op    0 allocs/op
```

The core scanner achieves zero allocations and processes JSON at 316+ MB/s.

## License

MIT License - see [LICENSE](LICENSE) for details.
