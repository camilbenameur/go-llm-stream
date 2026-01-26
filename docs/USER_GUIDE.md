# go-llm-stream User Guide

`go-llm-stream` is an $O(n)$ resumable Go library for incremental JSON parsing of Large Language Model (LLM) streams. This guide covers common tasks, including tokenization, handling malformed outputs, and OpenAI-compatible streaming.

---

## Installation

```bash
go get github.com/camilbenameur/go-llm-stream
```

Requires Go 1.23+.

---

## Core Concepts

Most LLM streaming solutions experience $O(n^2)$ performance overhead because they re-parse the entire accumulated response string every time a new chunk arrives. `go-llm-stream` addresses this with a **byte-level state machine** that processes each byte exactly once, providing **$O(n)$ performance** and zero extra memory allocations in the core scanner.

---

## Basic Usage: Streaming JSON

Use the `stream.NewReader` for incremental JSON tokenization.

```go
package main

import (
    "context"
    "fmt"
    "github.com/camilbenameur/go-llm-stream/stream"
)

func main() {
    ctx := context.Background()
    // 'r' is an io.Reader (e.g., from http.Response.Body)
    reader := stream.NewReader(ctx, r)
    defer reader.Close()

    for token := range reader.Tokens() {
        if token.Kind == stream.TokenError {
            fmt.Printf("Error: %v\n", token.Err)
            break
        }
        fmt.Printf("Token: %s, Data: %s\n", token.Kind, token.Raw)
    }
}
```

---

## JSON Healing: Fixing Malformed JSON

LLMs often truncate outputs or wrap them in markdown. The `stream.NewHealer` corrects these issues in real-time.

### Common Healer Tasks:
1.  **Strip Markdown**: Removes ` ```json ` and ` ``` ` delimiters.
2.  **Auto-Close**: Automatically adds missing `}`, `]`, or `"` at the end of a stream.
3.  **Literal Completion**: Completes partial keywords like `tru` → `true`.

```go
healer := stream.NewHealer(ctx, r,
    stream.WithStripMarkdown(true),
    stream.WithAutoClose(true),
)
defer healer.Close()

for token := range healer.Tokens() {
    // These tokens are guaranteed to form a valid JSON structure
}
```

---

## SSE Decoding

If you are working with APIs that return `text/event-stream` (like OpenAI or Anthropic), use the `sse` package.

```go
package main

import (
    "github.com/camilbenameur/go-llm-stream/sse"
    "io"
)

func main() {
    decoder := sse.NewDecoder(r)
    for {
        event, err := decoder.Next()
        if err == io.EOF {
            break
        }
        // event.Data, event.Event, etc.
    }
}
```

---

## OpenAI Streaming Adapter

For an experience similar to `go-openai`, use the `openai` package. It abstracts away the SSE decoding and JSON path extraction.

```go
package main

import (
    "context"
    "fmt"
    "github.com/camilbenameur/go-llm-stream/openai"
)

func main() {
    s := openai.NewStream(context.Background(), r)

    // Method 1: Get content deltas directly
    for delta := range s.Deltas() {
        fmt.Print(delta)
    }

    // Method 2: Access full metadata chunks
    for chunk := range s.Chunks() {
        fmt.Printf("ID: %s, Model: %s\n", chunk.ID, chunk.Model)
    }
}
```

---

## Configuration Options

| Option | Package | Description | Default |
| :--- | :--- | :--- | :--- |
| `WithStripMarkdown(bool)` | `stream` | Removes markdown code block wrappers | `true` |
| `WithAutoClose(bool)` | `stream` | Automatically closes uncompleted JSON | `true` |
| `WithBufferSize(int)` | `stream` | Internal buffer size for reading | `4096` |
| `WithHealJSON(bool)` | `openai` | Attempt to heal individual SSE payloads | `false` |
| `WithDeltaPath(string)` | `openai` | JSON path to extract delta content from | `choices.0.delta.content` |
| `WithDoneMarker(string)` | `openai` | SSE data string that signals completion | `[DONE]` |

---

## Performance & Memory

- **$O(n)$ Throughput**: Performance remains constant as the stream grows.
- **Zero Allocations**: The core scanner uses `sync.Pool` to reuse buffers.
- **Resumable**: Use `scanner.Snapshot()` to save state and resume from any byte offset.

---

## FAQ

**Q: Can I use this with any LLM?**  
A: Yes. While there are specific adapters for OpenAI-style APIs, the core `scanner` and `healer` work with any JSON-producing stream.

**Q: How do I handle non-JSON data?**  
A: The scanner will report `TokenError`. You can use the `healer` with `WithIgnoreTrailingJunk(true)` if you expect noise after the JSON object.

**Q: Is it concurrent safe?**  
A: A single `Reader` or `Healer` instance is not concurrent safe, but you can run many instances in parallel across different goroutines.
