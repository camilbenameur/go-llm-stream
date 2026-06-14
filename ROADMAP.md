# go-llm-stream — Roadmap

> Status: draft for execution by coding agents.
> Source: technical + portfolio review (2026-06-15).
> Goal: turn this from "a clever JSON scanner with a contested benchmark claim" into a **credible, real-world-relevant portfolio centerpiece**.

## North star

Reposition the library around the one thing it does that the ecosystem does *not* do well:

> **Incrementally parse a single large LLM-streamed JSON value (structured outputs / tool-call arguments) and emit sub-fields the moment they complete — plus repair truncated model JSON.**

Stop competing with the official OpenAI SDK on plain chat-delta streaming (we lose: they have auth, retries, types, maintenance). Lead with structured-output streaming + healing, with the O(n) resumable scanner as the *how*, not the headline.

## Why (the problem with the current pitch)

- The README's central claim — "OpenAI/LangChainGo re-parse the whole buffer each chunk → O(n²)" — is a **strawman** for the common case: in SSE chat streaming each `data:` line is already a self-contained small JSON parsed **once**.
- Our own `openai/stream.go` proves it: it uses `json.Unmarshal` per event (the conventional approach) and its `healJSON` is a naive `strings.Count` balancer that **doesn't even use our real `healer` package**.
- The genuine O(n) win is for **one big JSON streamed across many tokens**, not many small deltas. The roadmap makes that case explicit and provable.

---

## Toolchain constraint (read before assigning agents)

The dev host has **no Go toolchain** (Go lives only in `.devcontainer`). Tasks are tagged:

- 🟢 **`no-go`** — pure docs/markdown; can be completed and "done" anywhere.
- 🔴 **`needs-go`** — writes Go code; **must be compiled/tested/benchmarked in the devcontainer** before it counts as done. An agent on the host may *draft* it, but a human or in-container agent must run `go test ./...` (and `go test -bench` for T2) to verify.

---

## Workstreams

Each task: **goal · files · acceptance criteria · depends-on · parallel-safe · toolchain · suggested agent.**

### T1 — Reframe the README & pitch  🟢 `no-go`
- **Goal:** Rewrite the README to lead with streaming structured-output parsing + healing. Demote the O(n²) claim: either back it with T2's benchmark or remove it. Keep the resumable O(n) scanner described as the mechanism.
- **Files:** `README.md`, `docs/LIBRARY_SPEC.md` (overview section).
- **Acceptance:**
  - Opening section states the structured-output / tool-arg use case in ≤3 sentences with a concrete "emit `title` before `body` finishes" example.
  - No unqualified "O(n²)" claim remains unless it links to a reproducible benchmark.
  - "When NOT to use this (use the official SDK instead)" section exists — honesty signals seniority.
- **Depends-on:** ideally T2 (for numbers); can be drafted with `<!-- TODO: benchmark link -->` placeholders if T2 isn't done.
- **Parallel-safe:** yes (own file).
- **Suggested agent:** `general-purpose`, sonnet.

### T2 — Reproducible head-to-head benchmark  🔴 `needs-go`
- **Goal:** Prove (or retract) the performance claim with a real comparison on the *correct* scenario: parsing one large JSON value streamed in small chunks. Compare `scanner` vs `encoding/json.Decoder` (and, if feasible, the official OpenAI Go SDK's accumulation pattern).
- **Files:** new `docs/examples/benchmark_comparison/` + a `Benchmark*` in `scanner/` or a `bench/` package; update `docs/PERFORMANCE.md`.
- **Acceptance:**
  - A single `go test -bench=. -benchmem` run reproduces the headline numbers; raw output pasted into `docs/PERFORMANCE.md` with date + machine.
  - The benchmark isolates the *large-single-value* case (where we win), not many-small-deltas (where we don't).
  - If O(n²) does NOT materialize vs the real competitor, the claim is **removed** from README/spec and this is noted in the doc. Credibility over hype.
- **Depends-on:** none.
- **Parallel-safe:** yes.
- **Suggested agent:** in-devcontainer agent or human; sonnet drafts, must run Go.

### T3 — Fix the OpenAI adapter to use the real healer  🔴 `needs-go`
- **Goal:** Make `openai/stream.go` use the `healer` package instead of the naive `strings.Count` `healJSON`; or delete `healJSON` if healing isn't appropriate per-event. Adapter must not contradict the library thesis.
- **Files:** `openai/stream.go`, `openai/stream_test.go`.
- **Acceptance:**
  - `healJSON` (the `strings.Count` version) is gone; healing (if kept) routes through `healer`.
  - Existing tests pass; add a test where a truncated event is healed correctly via the real healer path.
  - `go vet ./openai/...` and `golangci-lint` clean.
- **Depends-on:** none.
- **Parallel-safe:** yes (own package).
- **Suggested agent:** in-devcontainer; sonnet drafts + must `go test ./openai/...`.

### T4 — Add fuzz testing to the scanner  🔴 `needs-go`
- **Goal:** Add `FuzzScanner` that feeds random bytes and asserts the scanner never panics and agrees with `encoding/json.Valid` on validity. Highest credibility-per-effort item; we currently have **zero** fuzz functions.
- **Files:** `scanner/fuzz_test.go` (and optionally `healer/fuzz_test.go`).
- **Acceptance:**
  - `go test -run=Fuzz -fuzz=FuzzScanner -fuzztime=60s ./scanner/` runs clean (no crashes) in the devcontainer.
  - For inputs `encoding/json.Valid` accepts, the scanner reaches a valid end state; divergences are documented or fixed.
  - A seed corpus covers nested objects/arrays, escapes, `\u`, numbers with exponents, truncation.
- **Depends-on:** none.
- **Parallel-safe:** yes.
- **Suggested agent:** in-devcontainer; sonnet drafts + must run fuzzing.

### T5 — Clean the release / version story  🟢 `no-go`
- **Goal:** Reconcile the inconsistent version state. Docs say `v1.1.2`; git tags include `v1.2.0`; `go.mod` retracts `[v1.0.1, v1.1.2]`. Pick one coherent story.
- **Files:** `CHANGELOG.md`, `README.md` (Status line), `docs/PERFORMANCE.md` header, `go.mod` (retract block sanity).
- **Acceptance:**
  - Single source of truth for "current version" across README, CHANGELOG, PERFORMANCE, spec.
  - `CHANGELOG.md` has a real `v1.2.0` entry describing what changed since `v1.0.0`.
  - Retract directives in `go.mod` match the CHANGELOG narrative (no dangling/contradictory retracts).
- **Depends-on:** ideally last (after T1–T4 land, so v1.2.0 notes are accurate).
- **Parallel-safe:** partial (touches README/PERFORMANCE — coordinate with T1/T2).
- **Suggested agent:** `general-purpose`, sonnet.

### T6 — One killer demo example  🔴 `needs-go`
- **Goal:** A runnable example that streams a real structured output and renders each field to the terminal as it completes — the "I get it in 10 seconds" demo.
- **Files:** new `docs/examples/structured_streaming/main.go` + short README in that dir.
- **Acceptance:**
  - Builds with `go build ./docs/examples/structured_streaming/`.
  - Works against a real or mocked SSE source (mock fixture committed so it runs without an API key in CI).
  - README shows expected terminal output / asciinema-style snippet.
- **Depends-on:** T3 (clean adapter) preferred.
- **Parallel-safe:** yes once T3 merged.
- **Suggested agent:** in-devcontainer; sonnet drafts + must `go build`/run.

---

## Suggested execution order

1. **Parallel, immediately:** T2 (benchmark — settles the thesis), T4 (fuzz), T3 (adapter). These are independent Go tasks → separate worktrees.
2. **After T2 lands:** T1 (README, now with real numbers).
3. **After T3 lands:** T6 (demo).
4. **Last:** T5 (version/changelog reconciliation, capturing everything above).

## Orchestration notes

- Each `needs-go` task → its own git worktree/branch so verification + merge stay clean.
- Definition of done for `needs-go`: `go test ./...` green + `golangci-lint run` clean **in the devcontainer**. Host-drafted code is "proposed," not "done."
- Keep the O(n) scanner core (`scanner/`) untouched except for T4 — it's the strongest asset.

## Done = portfolio-ready when

- [ ] README leads with a defensible, real-world use case (T1)
- [ ] Performance claim is either proven or removed (T2)
- [ ] No internal contradiction between thesis and adapter (T3)
- [ ] Fuzz testing demonstrates rigor (T4)
- [ ] Version/release story is coherent (T5)
- [ ] A 10-second "I get it" demo exists (T6)
