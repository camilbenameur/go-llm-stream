# Performance Benchmarks

This document contains benchmark results for go-llm-stream v1.2.0.

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

## Comparison with the standard library (corrected)

> An earlier version of this document claimed `encoding/json` is inherently O(n²) for
> streaming. The head-to-head benchmark below **disproves that** and the claim has been
> corrected here for accuracy.

`encoding/json.Decoder` — the idiomatic stdlib streaming API — is **O(n)** and, in the
benchmark below, is actually **2–3× faster per op** than this library's `StreamReader` on
raw chunked token scanning. The quadratic blow-up only appears in the *accumulate-and-
re-parse-the-whole-buffer-on-every-chunk* anti-pattern (`NaiveReparse`), which is **not**
how the stdlib decoder is meant to be used.

So the honest positioning is:

1. If you just need to parse a stream of JSON fast, `encoding/json.Decoder` is excellent —
   use it.
2. go-llm-stream earns its place when you need things the stdlib does **not** offer:
   **resumability / snapshot-restore** across reconnects, **byte-level healing** of
   truncated or markdown-wrapped LLM JSON, and **zero-allocation scanning** when feeding a
   contiguous buffer to the low-level `Scanner`.

See the measured head-to-head below.

## Head-to-head: large streamed JSON value

**Date**: June 15, 2026
**Machine**: Windows 11, amd64, AMD Ryzen 5 7600X 6-Core Processor (`go env GOOS=windows GOARCH=amd64`, `go version go1.26.4`). Note: this is a different machine from the dev-container results above (Windows host, not Linux container), so absolute ns/op numbers are not directly comparable across sections — only the *scaling trends within this section* should be compared to each other.

This benchmark (`scanner/comparison_bench_test.go`) targets the scenario where the README's O(n²) claim actually applies: **one large JSON value (a ~4KB / ~16KB / ~64KB object containing an array of records) delivered across many small chunks (32 or 64 bytes each)**, as happens with structured-output / tool-call-argument streaming. It compares three approaches that all receive the exact same chunked input:

1. **NaiveReparse**: the textbook anti-pattern — append each chunk to a buffer and call `json.Valid`/`json.Unmarshal` on the *entire accumulated buffer* every time.
2. **StdlibDecoder**: `encoding/json.Decoder` reading tokens directly from the chunked `io.Reader` (a fair "streaming stdlib" baseline — this is NOT the anti-pattern, it's the *correct* way to use the stdlib for this).
3. **StreamReaderScanner**: go-llm-stream's `StreamReader`/scanner, processing each byte once.

### Raw results

```
goos: windows
goarch: amd64
pkg: github.com/camilbenameur/go-llm-stream/scanner
cpu: AMD Ryzen 5 7600X 6-Core Processor
BenchmarkCompareNaiveReparse/size=4196/chunk=32-12         	    2024	    584893 ns/op	   7.17 MB/s	   24557 B/op	     561 allocs/op
BenchmarkCompareNaiveReparse/size=4196/chunk=64-12         	    4142	    314430 ns/op	  13.34 MB/s	   22813 B/op	     489 allocs/op
BenchmarkCompareNaiveReparse/size=16494/chunk=32-12        	     135	   8867734 ns/op	   1.86 MB/s	   94694 B/op	    2165 allocs/op
BenchmarkCompareNaiveReparse/size=16494/chunk=64-12        	     264	   4486347 ns/op	   3.68 MB/s	   87698 B/op	    1877 allocs/op
BenchmarkCompareNaiveReparse/size=65595/chunk=32-12        	       8	 134908188 ns/op	   0.49 MB/s	  373651 B/op	    8489 allocs/op
BenchmarkCompareNaiveReparse/size=65595/chunk=64-12        	      15	  68538907 ns/op	   0.96 MB/s	  346507 B/op	    7368 allocs/op
BenchmarkCompareStdlibDecoder/size=4196/chunk=32-12        	   17635	     67729 ns/op	  61.95 MB/s	   45552 B/op	    1874 allocs/op
BenchmarkCompareStdlibDecoder/size=4196/chunk=64-12        	   16209	     69546 ns/op	  60.33 MB/s	   41456 B/op	    1873 allocs/op
BenchmarkCompareStdlibDecoder/size=16494/chunk=32-12       	    4436	    262476 ns/op	  62.84 MB/s	  172512 B/op	    7248 allocs/op
BenchmarkCompareStdlibDecoder/size=16494/chunk=64-12       	    4806	    277495 ns/op	  59.44 MB/s	  156128 B/op	    7247 allocs/op
BenchmarkCompareStdlibDecoder/size=65595/chunk=32-12       	    1165	   1126186 ns/op	  58.25 MB/s	  699843 B/op	   28331 allocs/op
BenchmarkCompareStdlibDecoder/size=65595/chunk=64-12       	    1116	   1094857 ns/op	  59.91 MB/s	  601538 B/op	   28329 allocs/op
BenchmarkCompareStreamReaderScanner/size=4196/chunk=32-12  	    5722	    205956 ns/op	  20.37 MB/s	   52286 B/op	    1019 allocs/op
BenchmarkCompareStreamReaderScanner/size=4196/chunk=64-12  	    8810	    136532 ns/op	  30.73 MB/s	   35995 B/op	     820 allocs/op
BenchmarkCompareStreamReaderScanner/size=16494/chunk=32-12 	    1526	    780371 ns/op	  21.14 MB/s	  193216 B/op	    3911 allocs/op
BenchmarkCompareStreamReaderScanner/size=16494/chunk=64-12 	    2276	    526559 ns/op	  31.32 MB/s	  129233 B/op	    3136 allocs/op
BenchmarkCompareStreamReaderScanner/size=65595/chunk=32-12 	     385	   3085429 ns/op	  21.26 MB/s	  780502 B/op	   15337 allocs/op
BenchmarkCompareStreamReaderScanner/size=65595/chunk=64-12 	     586	   2068495 ns/op	  31.71 MB/s	  493088 B/op	   12259 allocs/op
PASS
ok  	github.com/camilbenameur/go-llm-stream/scanner	25.812s
```

### Scaling (chunk=32, ~4x size increase each step: 4,196 -> 16,494 -> 65,595 bytes)

| Approach | 4,196 B | 16,494 B (~4x) | 65,595 B (~4x again) | Growth pattern |
|:---|---:|---:|---:|:---|
| NaiveReparse | 584,893 ns | 8,867,734 ns (~15.2x) | 134,908,188 ns (~15.2x) | **Quadratic** — each ~4x size increase costs ~15x time, matching the O(n²) prediction (4² = 16) |
| StdlibDecoder | 67,729 ns | 262,476 ns (~3.9x) | 1,126,186 ns (~4.3x) | **Linear** — cost tracks size almost exactly |
| StreamReaderScanner | 205,956 ns | 780,371 ns (~3.8x) | 3,085,429 ns (~4.0x) | **Linear** — cost tracks size |

### Honest interpretation

The data **proves the quadratic-vs-linear claim, but only for the naive re-parse-on-every-chunk pattern** — and that pattern is *not* how `encoding/json` is normally used for streaming. The naive approach's runtime grows by ~15x for every ~4x increase in input size (consistent with O(n²): 4² ≈ 16), while both `encoding/json.Decoder` and go-llm-stream's scanner scale linearly with input size, as expected for O(n) algorithms.

However, the second, more important finding is that **`encoding/json.Decoder` (the correct, idiomatic stdlib streaming API) is itself O(n) and is, in absolute terms, 2-3x *faster* per op than go-llm-stream's `StreamReader` in this benchmark** (e.g. at 65,595 bytes / chunk=32: 1.13ms for `Decoder` vs 3.09ms for `StreamReader`), though `StreamReader` does proportionally better at larger chunk sizes (64 vs 32 bytes) due to less per-byte overhead relative to read overhead. Both are O(n) and allocate proportionally to input size; neither is allocation-free in this chunked-reader scenario (the zero-allocation `ScannerSmall`/`ScannerMedium` numbers above come from feeding a single contiguous buffer directly to the byte-level `Scanner`, not from `StreamReader` reading from a chunked `io.Reader`).

**Recommendation for the README**: the O(n²) claim should be **softened**. It is true *only* if the comparison is against code that re-parses the full accumulated buffer on every chunk — a real but avoidable anti-pattern, not an inherent property of `encoding/json`. The README should not imply that `encoding/json.Decoder` itself is O(n²); it is O(n) and competitive with (here, faster than) go-llm-stream's `StreamReader` on raw token-scanning throughput for chunked input. go-llm-stream's actual value proposition — resumability/snapshot-restore, byte-level incremental healing, and zero-allocation scanning when fed contiguous buffers — should be the lead, with the O(n²) comparison presented as "avoid this common anti-pattern" rather than "stdlib JSON streaming is inherently quadratic."

## Running Benchmarks

```bash
go test -bench=. -benchmem ./...
```

For the stress test:

```bash
go run docs/examples/stress_test/main.go
```
