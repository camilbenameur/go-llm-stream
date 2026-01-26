# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.2] - 2026-01-26

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
