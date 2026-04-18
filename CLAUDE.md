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
2. A `source.Source` goroutine reads lines and pushes `source.NewLineMsg` into the Bubble Tea event loop via `program.Send`
3. `Update()` in `internal/tui/update.go` appends each line to the `buffer.Buffer` and calls `refreshLogsContent()` to update the viewport
4. Chat requests are fired as `tea.Cmd` (off the event loop), returning `ChatReplyMsg` or `ChatErrMsg`

**Key design rule:** The `buffer.Buffer` is owned exclusively by the Bubble Tea event loop — only mutated inside `Update()`. No locking needed.

**On exit:** `main.go` sends `SIGTERM` to the process group (`-pgid`) so upstream pipeline processes (e.g. `./start_api.sh | logwiz`) are cleaned up.

## Package map

- `internal/buffer` — fixed-size ring buffer of log lines
- `internal/source` — stdin and file readers; emit `NewLineMsg`/`EOFMsg`/`ErrMsg`
- `internal/config` — flag + env var resolution (flag > env > `~/.logwiz/config.json` > default)
- `internal/chat` — OpenAI-compatible HTTP client (`Complete` is non-streaming)
- `internal/tui` — Bubble Tea model split across:
  - `model.go` — state struct + `New`
  - `update.go` — `Update()` dispatch, key handling, `refreshLogsContent`, status bar
  - `view.go` — `View()` composition, sidebar rendering, help overlay
  - `layout.go` — logs/sidebar width split calculation
  - `styles.go` — all lipgloss styles
  - `keys.go` — key name constants
  - `prompt.go` — pure function `buildChatMessages()` that constructs the API message list (system prompt + history + fenced log context + question)
  - `command.go` — command palette dispatcher (`dispatchCommand`) and handlers (`:chat`, `:search`, `:grep`, `:clear`, `:help`, `:model`, `:quit`)
  - `search.go` — regex search helpers: `runSearch`, `updateSearch`, `nextMatch`, `prevMatch`, `clearSearch`
  - `filter.go` — view-layer filter: `runFilter`, `updateFilter`, `clearFilter`, `intersectSearch`

## TUI focus model

The model tracks a `focusArea` (either `focusLogs` or `focusChat`). All key routing in `onKey()` branches on this first, so adding keybindings requires handling both states. Sidebar open/closed is independent of focus — the sidebar can be open with focus on logs (scrollable, non-editable) or on chat input.

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
