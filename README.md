# go-llm-stream

[![CI Status](https://github.com/camilbenameur/go-llm-stream/actions/workflows/go.yml/badge.svg)](https://github.com/camilbenameur/go-llm-stream/actions/workflows/go.yml)

A resumable, byte-level Go library for **incrementally parsing and repairing JSON that streams out of LLMs** — so you can act on fields the moment they complete and recover cleanly when a model truncates its own output.

## What this actually gives you

The Go standard library already streams JSON well (`encoding/json.Decoder` is O(n) and fast — see [Performance](#performance)). go-llm-stream is **not** trying to beat it on raw speed. It exists for the things the stdlib does *not* do, which come up constantly when the producer is a language model:

- **Healing of malformed / truncated JSON.** Models hit token limits, drop connections, or wrap output in ```` ```json ```` fences. The [`healer`](healer/) closes unterminated strings, containers, and partial literals (`tru` → `true`) and strips markdown — turning "almost-JSON" into valid JSON you can unmarshal.
- **Incremental, field-at-a-time consumption.** A single large structured-output object (or a tool call's `arguments`) streams across hundreds of chunks. The [`scanner`](scanner/) processes each byte **exactly once** and emits tokens as they complete, so you can render `title` before the model has finished writing `body`.
- **Resumability.** `Snapshot`/`Restore` lets you persist parser state mid-stream and resume after a reconnect — no re-reading from the top.
- **Pull *or* push.** Supply an `io.Reader` (`stream.NewReader`/`NewHealer`) or write chunks in as they arrive via the `io.Writer`-based `stream.Writer`.
- **Provider-flexible deltas.** The `openai` adapter extracts content by configurable JSON path; `WithDeltaPaths`/`WithAnthropicFormat` let one stream consume OpenAI- and Anthropic-shaped events.
- **Zero-allocation core.** Fed a contiguous buffer, the low-level `Scanner` runs at 300+ MB/s with **0 allocations** (verified — see [Performance](#performance)).

## When to use it (and when not to)

| Use go-llm-stream when… | Just use `encoding/json` when… |
|---|---|
| You need to repair truncated/markdown-wrapped model JSON | Your JSON is well-formed |
| You want to emit sub-fields of one big streamed object early | You can wait for the whole value |
| You need snapshot/resume across reconnects | A single pass is enough |
| You're parsing tool-call arguments as they stream | You're decoding a normal HTTP body |

Being honest about this is the point — reach for the stdlib (or the official provider SDK) when it fits.

## Installation

```bash
go get github.com/camilbenameur/go-llm-stream
```

## Quick start — heal a truncated stream

```go
import "github.com/camilbenameur/go-llm-stream/stream"

// responseBody is an io.Reader of (possibly truncated / fenced) model output.
h := stream.NewHealer(ctx, responseBody)
defer h.Close()

for token := range h.Tokens() {
    if token.Kind == stream.TokenString {
        fmt.Print(string(token.Raw)) // strings arrive (and complete) incrementally
    }
}
// If the stream ends mid-object, the healer emits the closing tokens so the
// JSON you reconstruct is always valid.
```

## Field-at-a-time structured streaming

See [docs/examples/structured_streaming/](docs/examples/structured_streaming/) for a runnable demo (no API key required) that streams one structured-output object in small chunks, prints each field as it completes, and shows the healer recovering a deliberately truncated tail that `json.Unmarshal` rejects outright.

```bash
go run ./docs/examples/structured_streaming
```

## Documentation

- **[User Guide](docs/USER_GUIDE.md)** — tutorials and reference.
- **[Examples](docs/examples/)** — runnable samples for every package.
- **[Performance](docs/PERFORMANCE.md)** — benchmarks, including the honest head-to-head vs the stdlib.
- **[Library Spec](docs/LIBRARY_SPEC.md)** — architecture details.
- **[Roadmap](ROADMAP.md)** — what's planned and why.
- **[Changelog](CHANGELOG.md)** — release history.

## Performance

The core byte-level scanner is genuinely fast and allocation-free on contiguous input:

```
BenchmarkScannerSmall-12    13,636,113    85.37 ns/op    316 MB/s    0 B/op    0 allocs/op
```

On the **O(n²) question**: a head-to-head benchmark ([details](docs/PERFORMANCE.md#head-to-head-large-streamed-json-value)) measured three ways of parsing one large JSON value delivered in small chunks:

| Approach | Scaling per 4× input | Verdict |
|---|---|---|
| Naive accumulate-and-re-parse every chunk | **~15×** (quadratic) | the anti-pattern to avoid |
| `encoding/json.Decoder` (idiomatic streaming) | ~4× (linear) | fast, correct |
| go-llm-stream `StreamReader` | ~4× (linear) | linear, plus healing/resume |

**The takeaway, stated honestly:** O(n²) is real *only* for code that re-parses the whole accumulated buffer on each chunk — it is **not** an inherent property of `encoding/json`. Don't pick this library for raw speed; pick it for healing, incremental field access, and resumability.

## Reliability

The scanner and healer are **fuzz-tested** (`go test -fuzz`) and cross-checked against `encoding/json.Valid`. Fuzzing has already caught and fixed real edge cases (number finalization inside containers; closure of dangling object keys). Seed/regression corpora live alongside the tests.

## Status

- **Current release: v1.3.0** (push `io.Writer` API, multi-shape OpenAI/Anthropic deltas, fuzzing, perf; earlier `v1.0.1`–`v1.1.2` are [retracted](go.mod)).
- See the [Changelog](CHANGELOG.md) for the full history.

## License

MIT License — see [LICENSE](LICENSE).
