# Performance Benchmarks

This document contains benchmark results for go-llm-stream v1.1.0.

## Test Environment

- **OS**: Linux (Debian GNU/Linux 12)
- **CPU**: AMD Ryzen 5 7600X 6-Core Processor
- **Environment**: Developed and benchmarked within a **Dev Container** (performance may be slightly lower than bare metal).
- **Go Version**: 1.24+
- **Date**: January 25, 2026

## Benchmark Results

### Core Scanner (`scanner` package)

| Benchmark | Operations | ns/op | MB/s | B/op | allocs/op |
|:----------|:----------:|------:|-----:|-----:|----------:|
| ScannerSmall | 13,636,113 | 85.37 | 316.28 | **0** | **0** |
| ScannerMedium | 131,680 | 8,481 | 318.60 | **0** | **0** |
| TokenizerSmall | 3,910,642 | 316.9 | 85.20 | 184 | 9 |
| TokenizerChunked | 1,056,482 | 1,124 | 47.15 | 608 | 27 |
| StreamReaderSmall | 418,035 | 2,953 | 14.56 | 5,061 | 25 |

**Key Insight**: The core byte-level scanner achieves **zero allocations** and processes JSON at over **316 MB/s**, validating the O(n) design goal.

### Healer (`healer` package)

| Benchmark | Operations | ns/op | MB/s | B/op | allocs/op |
|:----------|:----------:|------:|-----:|-----:|----------:|
| Closer_Feed | 6,425,085 | 180.5 | 448.84 | **0** | **0** |
| Heal | 5,225,101 | 233.4 | 342.72 | 160 | 2 |
| Healer_Healing | 1,317,870 | 898.1 | 89.08 | 160 | 2 |
| StripMarkdown | 1,709,116 | 700.4 | 115.64 | **0** | **0** |
| MarkdownFilter_Streaming | 2,501,712 | 495.9 | 131.08 | **0** | **0** |

**Key Insight**: Markdown stripping and container closure tracking operate with **zero allocations**, maintaining the O(n) performance guarantee.

### SSE Decoder (`sse` package)

| Benchmark | Operations | ns/op | B/op | allocs/op |
|:----------|:----------:|------:|-----:|----------:|
| Decoder_SimpleEvent | 1,476,576 | 806.6 | 4,168 | 4 |
| Decoder_MultiLineData | 941,223 | 1,074 | 4,496 | 12 |
| Decoder_OpenAIPayload | 1,445,895 | 831.8 | 4,320 | 4 |

### OpenAI Adapter (`openai` package)

| Benchmark | Operations | ns/op | B/op | allocs/op |
|:----------|:----------:|------:|-----:|----------:|
| Stream_NextDelta | 289,761 | 3,735 | 6,600 | 51 |
| Stream_NextChunk | 434,751 | 2,797 | 5,376 | 23 |

### Unified API (`stream` package)

| Benchmark | Operations | ns/op | B/op | allocs/op |
|:----------|:----------:|------:|-----:|----------:|
| Reader_SmallJSON | 190,840 | 5,470 | 6,519 | 26 |
| Healer_SmallJSON | 116,216 | 9,910 | 19,426 | 39 |

## Memory Stability

The stress test (`docs/examples/stress_test/main.go`) processes 100,000 JSON objects (800,001 tokens) with:

- **Total Duration**: ~700ms
- **Heap In Use**: ~4 MB (constant)
- **Total Allocations**: ~890 MB throughput with pool reuse

This demonstrates constant memory overhead regardless of input size.

## Comparison with Standard Library

The `encoding/json` package's streaming approach accumulates the entire buffer and re-parses on each chunk, resulting in O(n²) complexity for streaming scenarios. In contrast, go-llm-stream:

1. Processes each byte exactly once: O(n)
2. Uses sync.Pool for buffer reuse
3. Maintains constant memory footprint

For a 1MB streaming response, this can mean the difference between:
- **encoding/json**: ~500 full parses (assuming 2KB average chunks)
- **go-llm-stream**: 1 incremental parse

## Running Benchmarks

```bash
go test -bench=. -benchmem ./...
```

For the stress test:

```bash
go run docs/examples/stress_test/main.go
```
