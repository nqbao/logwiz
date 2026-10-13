# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build        # build ./logwiz binary
make test         # go test ./...
make clean        # remove binary

go test ./internal/buffer/...   # run a single package's tests
go test -run TestName ./...     # run a specific test
```

## Architecture

logwiz is a terminal log viewer with an integrated OpenAI-compatible chat sidebar. It reads from stdin or a file, buffers lines in a ring buffer, and renders them in a Bubble Tea TUI.

**Data flow:**
1. `cmd/logwiz/main.go` wires everything: parses config, picks source, creates buffer, starts TUI program
2. A `source.Source` goroutine reads lines and pushes `source.NewLinesMsg` batches into the Bubble Tea event loop via `program.Send`
3. `Update()` in `internal/tui/update.go` appends each line to the `buffer.Buffer` and calls `refreshLogsContent()` to update the viewport
4. Chat requests and palette agent commands are fired as `tea.Cmd` (off the event loop), returning agent step/done/err messages

**Key design rule:** The `buffer.Buffer` is owned exclusively by the Bubble Tea event loop — only mutated inside `Update()`. No locking needed.

**On exit:** `main.go` sends `SIGINT` to the process group (`-pgid`) so upstream pipeline processes (e.g. `./start_api.sh | logwiz`, `modd | logwiz`) are cleaned up. SIGINT (not SIGTERM) because supervisors like modd put their children in separate process groups and only run cleanup logic on SIGINT — SIGTERM hits the default-terminate path and orphans the grandchildren.

## Package map

- `internal/buffer` — fixed-size ring buffer of log lines
- `internal/source` — stdin and file readers; emit `NewLinesMsg`/`EOFMsg`/`ErrMsg`
- `internal/config` — flag + env var resolution (flag > env > `~/.logwiz/config.json` > default)
- `internal/logmsg` — log message types and parsers (JSON, logfmt, pipe-delimited/Java with stack-trace continuations, auto-detect, passthrough)
- `internal/highlight` — syntax highlighting with a per-message render cache
- `internal/redact` — redacts sensitive fields before sending logs to the LLM
- `internal/tools` — buffer-query tool registry (`search_regex`, `count_regex`, `tail`, `read`) used by the chat agent
- `internal/agent` — LLM client, message types, and agent lifecycle; `command.go` defines the palette agent's tool schemas (`find`, `count`, `tail`, `read`, `pick_tool`)
- `internal/tui` — Bubble Tea model: state, update loop, viewport, layout, rendering, search/filter, agent bridging, and command palette
- `cmd/logsim` — log simulator for development and testing

## TUI focus model

The model tracks a `focusArea` (either `focusLogs` or `focusChat`). All key routing in `onKey()` branches on this first, so adding keybindings requires handling both states. Sidebar open/closed is independent of focus — the sidebar can be open with focus on logs (scrollable, non-editable) or on chat input.

## Command palette agent

Unprefixed palette input (no `:` or `/` prefix) is routed through a short-lived `cmdAgent` defined in `internal/agent/command.go`. The agent runs a two-phase loop:

1. **Inspection** (`find`, `count`, `tail`, `read`) — read-only tools that query the buffer and feed results back into the loop. Budget: 4 rounds (`MaxCmdToolTurns` in `tui/model.go`).
2. **Commit** (`pick_tool`) — a single umbrella call that names the action and closes the loop.

`pick_tool` actions: `grep`, `clear_grep`, `jump`, `highlight`, `clear_highlight`, `set_model`, `clear_chat`, `quit`, `answer`, `reject`.

The `answer` action keeps the palette open, clears the input, and displays the response as a dim info line inside the palette.

Inspection tool details:
- `find(pattern, flags)` — returns count, first/last line, up to 10 samples
- `count(pattern, flags)` — returns match count and total
- `tail(n)` — last N lines (1–200), for questions about recent activity
- `read(start, end)` — inclusive 1-based line range (max 200 lines), for specific ranges

## Input handling notes

- Lines are sanitized in `source/source.go:pump` using `lastCRSegment` — this mimics terminal `\r` overwrite behaviour (keeps only the last segment after splitting on `\r`). This handles spinner/progress output from tools like vite and uvicorn.
- Tools writing to a pipe disable color output automatically (`isatty == false`). Use `FORCE_COLOR=1` upstream to get colors.
- Mouse scrolling is enabled via `tea.WithMouseCellMotion()`.

## Streaming / live-tail design considerations

logwiz is frequently used in streaming mode (stdin pipe from a live process). Any feature that maintains derived state from the buffer — such as search match indices, statistics, or highlights — **must handle the `NewLinesMsg` path** in `Update()`, not just the initial computation. Key points:

- **Ring-buffer evictions:** when the buffer is full, every `buf.Append` evicts the oldest line and returns `(evictedID, true)`. The `NewLinesMsg` handler counts evictions per batch. Derived index lists (e.g. `searchMatches`) must shift their stored indices down by the eviction count and drop entries that go negative.
- **Incremental updates:** re-scanning the full buffer on every incoming batch is O(N) per batch — avoid it. Prefer scanning only the newly appended tail (`buf.Len()-newLines` → `buf.Len()`) and adjusting existing state for evictions.
- **Viewport auto-scroll:** `LogViewport.autoScroll` follows the tail by default. Operations that jump to a specific line (e.g. `JumpTo`) disable `autoScroll`; reaching the bottom re-enables it. Incremental updates that sync `Selected` should not call `JumpTo` (which moves the viewport) — set `logs.Selected` directly to avoid disrupting the user's scroll position.
- **The wrap-height cache** (`LogViewport.wrapCache`) is the canonical example of this pattern: `OnNewLines` calls `updateWrapCache` which evicts front entries and appends heights for new lines in O(newLines).

## Help overlay

The help overlay in `view.go:renderHelpOverlay` is the canonical user-facing reference for all commands and keybindings. **Keep it in sync** whenever you add, remove, or change a command, keybinding, or usage tip. The overlay is opened with `?`.
