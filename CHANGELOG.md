# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

_Nothing yet._

## [1.3.0] - 2026-06-15

### Added

- **Push API** (`stream.Writer`): an `io.Writer`-based entry point so callers can write
  chunks as they arrive instead of supplying an `io.Reader`. Provides an `OnToken`
  callback, `NextToken` pull-drain, and `Flush`/`Close`. (Addresses community feedback.)
- **Multi-shape delta extraction** (`openai`): `WithDeltaPaths(...)` tries several
  candidate JSON paths in order, and `WithAnthropicFormat()` plus `OpenAIDeltaPath` /
  `AnthropicDeltaPath` constants let one `Stream` consume OpenAI- and Anthropic-style
  events. Unexpected/empty/metadata events are skipped gracefully.
- **Fuzz testing** for the `scanner` and `healer` packages (`FuzzScanner`, `FuzzHeal`),
  cross-checked against `encoding/json.Valid`, with seed and regression corpora.
- **Head-to-head streaming benchmark** (`scanner/comparison_bench_test.go`) comparing the
  naive accumulate-and-re-parse anti-pattern, `encoding/json.Decoder`, and `StreamReader`.

### Fixed

- **Scanner**: numbers inside containers now finalize correctly on trailing whitespace
  (previously the scanner could stay stuck in a numeric state). Found by fuzzing.
- **Tokenizer**: `Append` no longer corrupts `Token.Start` for an in-progress literal
  when fed many small chunks. Found while testing the push API.
- **Healer**: `Closer.Closure()` now completes a dangling/just-closed object key with
  `:"null"` instead of emitting invalid JSON like `{""}`. Found by fuzzing.
- **OpenAI adapter**: healing now routes through the real `healer` package instead of a
  naive `strings.Count` bracket balancer, removing an internal contradiction with the
  library's own thesis.
- **`IgnoreTrailingJunk` is now actually honored** (it was previously a no-op). With the
  default (`true`) trailing content after the root value is still ignored; setting it
  `false` now makes the healer surface a `TokenError` for content after the root closes.

### Performance

- Reduced allocations in hot paths: reuse the `FilterReader` read buffer, cache the split
  delta path per `Stream`, and reuse the `StreamReader` read channel
  (`BenchmarkStreamReaderSmall`: 25 → 23 allocs/op, 5126 → 4990 B/op). No API change.

### Changed

- **Docs**: the O(n²) performance claim has been corrected. It applies only to the
  re-parse-the-whole-buffer-on-every-chunk anti-pattern, not to `encoding/json.Decoder`
  (which is O(n) and faster than `StreamReader` on raw token scanning). README and
  PERFORMANCE.md reframed around the library's real differentiators: healing,
  incremental field access, and resumability.

### Removed

- Non-functional configuration that silently had no effect: `stream.WithBufferSize`
  /`Options.BufferSize` (the underlying `StreamReader` takes no buffer size),
  `stream.WithCompleteStrings`/`WithCompleteLiterals` (never forwarded to the healer),
  and the unused `CompleteStrings`/`CompleteLiterals` fields on `healer.HealerOptions`.
  These knobs were dead weight and misleading — removing them keeps the API honest.

## [1.2.0] - 2026-01-26

### Changed

- First clean tagged release. Retracts the messy intermediate versions `v1.0.1` through
  `v1.1.2` (see `retract` directive in `go.mod`); functionally equivalent to the
  `v1.1.2` content with the retraction recorded.

## [1.1.2] - 2026-01-26

> **Retracted.** Use `v1.2.0` or later.

### Added

- **Core Scanner** (`scanner` package)
  - Byte-level, resumable JSON scanner with O(n) performance
  - Full JSON tokenization (objects, arrays, strings, numbers, bools, null)
  - Stream processing with `StreamReader` for io.Reader integration
  - Token-based API with `Tokenizer` wrapper
  - Snapshot/restore for resumable parsing
  - sync.Pool-based memory management

- **JSON Healer** (`healer` package)
  - Automatic healing of truncated JSON from LLM outputs
  - Markdown code block stripping (```json support)
  - Unclosed container closure (objects, arrays, strings)
  - Partial literal completion (true/false/null)
  - Configurable via functional options

- **SSE Decoder** (`sse` package)
  - Server-Sent Events parser for text/event-stream
  - Handles partial frames across reads
  - Support for OpenAI and Anthropic event formats
  - Comment and keep-alive handling
  - Multi-line data field support
  - Channel-based `EventStream` for concurrent consumption

- **OpenAI Compatibility Layer** (`openai` package)
  - Drop-in streaming adapter for OpenAI-style APIs
  - `NextDelta()` for simple content extraction
  - `NextChunk()` for full response metadata access
  - Channel-based `Deltas()` and `Chunks()` APIs
  - Configurable JSON path for delta extraction
  - Optional JSON healing for truncated payloads

- **Unified API** (`stream` package)
  - Single import path for common use cases
  - `NewReader()` for basic JSON tokenization
  - `NewHealer()` for healed JSON streaming
  - Unified `Option` type for configuration
  - Re-exported token types for convenience

- **Documentation**
  - Comprehensive README with quick start examples
  - Example code for all major use cases
  - Library specification document
  - Research reports and implementation guides

### Performance

- O(n) streaming parse performance
- Constant memory overhead regardless of input size
- sync.Pool buffer reuse to minimize allocations
- Zero-copy token access where possible

## [0.3.0] - 2026-01-18

### Added

- Phase 3: Buffer management and concurrency
- Improved markdown filtering
- Enhanced error handling

## [0.2.0] - 2026-01-11

### Added

- Phase 2: Core scanner implementation
- Tokenizer wrapper
- StreamReader for io.Reader integration

## [0.1.0] - 2026-01-04

### Added

- Initial research and audit (Phase 1)
- Project structure and documentation
