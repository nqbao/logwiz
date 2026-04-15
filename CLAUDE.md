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

## TUI focus model

The model tracks a `focusArea` (either `focusLogs` or `focusChat`). All key routing in `onKey()` branches on this first, so adding keybindings requires handling both states. Sidebar open/closed is independent of focus — the sidebar can be open with focus on logs (scrollable, non-editable) or on chat input.

## Input handling notes

- Lines are sanitized in `source/source.go:pump` using `lastCRSegment` — this mimics terminal `\r` overwrite behaviour (keeps only the last segment after splitting on `\r`). This handles spinner/progress output from tools like vite and uvicorn.
- Tools writing to a pipe disable color output automatically (`isatty == false`). Use `FORCE_COLOR=1` upstream to get colors.
- Mouse scrolling is enabled via `tea.WithMouseCellMotion()`.
