# Engineering Spec: `go-llm-stream`

## 1. Overview
`go-llm-stream` is a Go library for $O(n)$ incremental JSON parsing of LLM streams. It solves the $O(n^2)$ re-allocation problem found in existing libraries and provides real-time "healing" for malformed JSON.

## 2. Technical Goals
1. **O(n) Parsing Complexity:** Process $N$ bytes of stream in $O(N)$ time with constant memory overhead.
2. **Resumable State Machine:** Ability to stop parsing mid-token and resume perfectly when more data arrives.
3. **Zero-Allocation Data Paths:** Extensive use of `sync.Pool` for buffers and tokenizer state.
4. **Auto-Healing:** Detect and automatically correct truncated JSON (missing delimiters).
5. **Streaming-Native API:** Native integration with `io.Reader` and `context.Context`.

## 3. Architecture

### 3.1 Core Components
- **Scanner**: A low-level byte-by-byte JSON state machine based on the Go standard library's scanner.go pattern.
- **Tokenizer**: Consumes scanner output to emit Token objects.
- **StreamReader**: An io.Reader wrapper that orchestrates the buffering and tokenization.
- **Healer**: A post-processor that manages a stack of open JSON delimiters.

### 3.2 State Machine
The scanner implements 27 distinct states including value entry, literal parsing, number components, and Escaped string sequences.

### 3.3 Memory Management
- **Buffer Recycling:** Uses sync.Pool to recycle byte slices.
- **State Serialization:** Tokenizer state can be snapshotted into a compact struct.

## 4. API Design (Target)

\`\`\`go
reader := GetOpenAIStream() // returns io.Reader
stream := gollmstream.NewReader(reader)

for token := range stream.Tokens(ctx) {
    if token.Kind == gollmstream.TokenObjectEnd {
        // Emit sub-object as soon as it's ready
    }
}
\`\`\`

## 5. Performance Targets
- **Memory**: < 2MB total heap allocation for streams up to 1GB.
- **Throughput**: > 500MB/s on a modern CPU core.
- **Latency**: < 100ns per average JSON token transition.
