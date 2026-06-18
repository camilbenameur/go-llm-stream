# Structured streaming + healing demo

Runs with **no API key** — it uses a mock, deliberately *truncated*, markdown-fenced
structured-output response to show the two things go-llm-stream does that the standard
library does not:

1. **Field-at-a-time consumption** — each field is surfaced the moment it completes, with
   the byte offset at which it arrived, so you can act on early fields before the stream
   finishes.
2. **Healing** — the mock stream is cut off mid-`body` (no closing quote, brace, or
   ```` ``` ````), exactly like a model that hit its token limit. `encoding/json` rejects it;
   the healer recovers valid JSON.

## Run

```bash
go run ./docs/examples/structured_streaming
```

## Expected output

```
=== 1. Field-at-a-time streaming (with healing) ===
Streaming a truncated, markdown-fenced structured output in 16-byte chunks...

  [byte  43] title      = "The Rise of Incremental Parsing"
  [byte  53] score      = 9
  [byte  70] published  = true
  [byte 106] summary    = "Process each byte once."
  [in-flight] body       = <truncated mid-value — recovered by healing in step 2>
  ↳ [healer] injected the closing '}' — the model never sent it

=== 2. stdlib vs healer on the raw truncated bytes ===
  encoding/json.Unmarshal(raw):    FAILED — invalid character '`' looking for beginning of value
  json.Unmarshal(HealBytes(raw)):  OK — recovered 5 fields
  healed JSON: {"title":"...","score":9,"published":true,"summary":"...","body":"Streaming LLMs emit JSON one token at a"}
```

The byte offsets make the point concrete: `title` is usable at byte 43 while `summary`
doesn't arrive until byte 106 — and the truncated `body` is still recovered by healing.
