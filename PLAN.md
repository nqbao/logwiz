# logwiz v1 — Implementation Plan

A small Go TUI that behaves like `tee` with an attached chat panel for
asking questions about the current log buffer. Backed by any
OpenAI‑compatible chat completions endpoint.

This plan turns the v1 design doc into a concrete build order with
package layout, data model, and milestone checklists.

---

## 1. Tech choices

| Concern       | Choice                                      |
| ------------- | ------------------------------------------- |
| Language      | Go 1.22+                                    |
| TUI runtime   | `github.com/charmbracelet/bubbletea`        |
| TUI widgets   | `github.com/charmbracelet/bubbles` (viewport, textarea) |
| Styling       | `github.com/charmbracelet/lipgloss`         |
| HTTP client   | stdlib `net/http` + `encoding/json`         |
| Flag parsing  | stdlib `flag` (small surface, no cobra)     |
| Module path   | `github.com/nqbao/logwiz`                   |

No vendoring, no generator, no ORM-like abstractions. One binary.

---

## 2. Package layout

```
.
├── cmd/
│   └── logwiz/
│       └── main.go          # entry point, flag parsing, wiring
├── internal/
│   ├── config/
│   │   └── config.go        # flags + env var resolution
│   ├── buffer/
│   │   └── buffer.go        # fixed-size rolling line buffer
│   ├── source/
│   │   ├── source.go        # Source interface + NewLineMsg
│   │   ├── stdin.go         # stdin streaming reader
│   │   └── file.go          # file reader that stops at EOF
│   ├── chat/
│   │   ├── client.go        # OpenAI-compatible HTTP client
│   │   └── types.go         # request/response structs
│   └── tui/
│       ├── model.go         # top-level bubbletea.Model
│       ├── update.go        # Update() dispatch
│       ├── view.go          # View() composition
│       ├── logs.go          # log pane (viewport wrapper)
│       ├── sidebar.go       # chat sidebar rendering
│       ├── keys.go          # keymap
│       └── styles.go        # lipgloss styles
├── go.mod
├── go.sum
└── README.md
```

Everything not in `cmd/` is under `internal/` so nothing accidentally
becomes public API.

---

## 3. Data model

```go
// internal/buffer
type Buffer struct {
    lines []string   // ring
    head  int
    size  int        // len(lines) once filled
    cap   int
}
func New(capLines int) *Buffer
func (b *Buffer) Append(line string)
func (b *Buffer) LastN(n int) []string
func (b *Buffer) Snapshot() []string  // in order
func (b *Buffer) Len() int

// internal/source
type Source interface {
    // Start begins reading and sends NewLineMsg via the returned tea.Cmd.
    // Returns a command that produces the next message each time it's run.
    Start(program *tea.Program)  // pushes messages via program.Send
    Close() error
}

type NewLineMsg struct{ Line string }
type EOFMsg struct{}
type SourceErrMsg struct{ Err error }

// internal/chat
type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}
type Request struct {
    Model    string    `json:"model"`
    Messages []Message `json:"messages"`
    Stream   bool      `json:"stream,omitempty"`
}
type Client struct {
    BaseURL string
    APIKey  string
    Model   string
    HTTP    *http.Client
}
func (c *Client) Complete(ctx context.Context, msgs []Message) (string, error)

// tui messages for chat
type ChatReplyMsg struct{ Content string }
type ChatErrMsg struct{ Err error }

// internal/tui
type focus int
const (
    focusLogs focus = iota
    focusChat
)

type Model struct {
    cfg          config.Config
    buf          *buffer.Buffer
    eof          bool
    sourceKind   string // "stdin" | "file"

    logs         viewport.Model
    autoScroll   bool     // stick to bottom when user hasn't scrolled up

    sidebarOpen  bool
    sidebarWidth int
    input        textarea.Model
    chatMsgs     []chat.Message // includes system prompt at index 0
    chatLoading  bool
    chatErr      string

    focus        focus
    width, height int
    quitting     bool

    chatClient   *chat.Client
}
```

The rolling buffer stores strings, not bytes. Lines are appended after
being split on `\n`. ANSI escapes are passed through untouched and let
lipgloss / terminal handle rendering.

---

## 4. CLI surface

```
logwiz [flags] [file]

  --model         model name (env OPENAI_MODEL, default "gpt-4o-mini")
  --base-url      API base URL (env OPENAI_BASE_URL, default https://api.openai.com/v1)
  --api-key       API key (env OPENAI_API_KEY)
  --max-lines     rolling buffer size (default 5000)
  --context-lines lines of log context sent per chat turn (default 300)
  --sidebar-width chat sidebar width in cols (default 50)
  --system-prompt override system prompt
```

Resolution order: flag > env > default.

If `[file]` is given → file mode. Otherwise → stdin mode. If stdin is a
TTY and no file is given, print usage and exit 2.

---

## 5. Milestones

### M1 — Log viewer skeleton
- `go mod init github.com/nqbao/logwiz`, add bubbletea/bubbles/lipgloss
- `internal/buffer`: ring buffer + unit tests (Append wrap, LastN, Snapshot)
- `internal/source/stdin.go`: goroutine reading `bufio.Scanner` on os.Stdin,
  pushes `NewLineMsg` via `program.Send`; emits `EOFMsg` on stdin close
- `internal/source/file.go`: same shape, reads file start→EOF, then `EOFMsg`
- `internal/tui` minimal: single `viewport.Model` displaying buffer snapshot
- Alt screen (`tea.WithAltScreen()`), quit on `q`/`ctrl+c`
- Window-resize handling

**Done when:** `cat big.log | go run ./cmd/logwiz` scrolls live with j/k/PgUp/PgDn.

### M2 — Chat sidebar UI (no network yet)
- Add `textarea` input + message list view
- Keybindings: `/` open sidebar + focus input, `Esc` blur / close,
  `Tab` switch focus, `Enter` submit (in chat focus), `c` clear chat
- Split view: left = logs, right = sidebar when open; recompute viewport
  widths on resize and on sidebar toggle
- Stub submit: echoes back "[stub] you asked: …" as assistant reply
- Auto-scroll: logs stick to bottom unless user scrolled up

**Done when:** sidebar toggles cleanly, input works, layout survives resize.

### M3 — OpenAI-compatible chat
- `internal/config`: flag + env resolution, validation (api key required)
- `internal/chat.Client.Complete`: POST `${base}/chat/completions`,
  non-streaming, parses `choices[0].message.content`, 60s timeout,
  surfaces HTTP + JSON errors as `ChatErrMsg`
- On submit: build `[]Message`:
  1. system prompt (default: "You are helping analyze terminal logs.
     Be concise and grounded in the provided log text.")
  2. prior turns from `chatMsgs`
  3. new user message that prepends a fenced code block with
     `buf.LastN(cfg.ContextLines)`
- Set `chatLoading=true`, disable input, show spinner in sidebar header
- On `ChatReplyMsg`: append assistant message, clear loading
- On `ChatErrMsg`: show inline error, keep input populated for retry

**Done when:** real questions get real answers from an OpenAI-compatible endpoint.

### M4 — Polish
- EOF indicator in status line ("[eof]" badge) for file mode
- Help overlay on `?`
- Graceful shutdown: cancel in-flight request on quit, close source,
  `tea.ExitAltScreen` via bubbletea lifecycle
- Basic logging of errors to stderr *after* alt screen is torn down
- README with usage examples

---

## 6. Concurrency model

- The bubbletea `Program` is the single source of truth for UI state.
- Source readers run in their own goroutines and push messages with
  `program.Send(msg)`. They never touch model state directly.
- Chat requests are kicked off from `Update` via `tea.Cmd` that runs
  `client.Complete` with a `context.Context` stored on the model so
  quit can cancel it.
- Buffer is owned by the model and only mutated inside `Update` when a
  `NewLineMsg` arrives. No locks needed.

This keeps every state transition on the bubbletea event loop.

---

## 7. Rendering details

- Log pane uses `viewport.Model` with content = `strings.Join(snapshot, "\n")`.
  Recompute only on buffer change or resize; do not rebuild on every tick.
- Sidebar is a plain `lipgloss` vertical stack:
  `header` / `messages (scrollable viewport)` / `input (textarea)` /
  `footer (hint or error)`.
- Sidebar width is `min(cfg.SidebarWidth, width/2)`; logs take the rest
  minus a 1‑col vertical rule.
- Assistant messages wrap at sidebar width with `lipgloss.NewStyle().Width(...)`.

---

## 8. Explicit non-goals for v1

Per the design doc — do **not** build these yet, even if tempting:

- Search / filtering
- Line selection or annotations
- MCP / tool calling / agent loop
- Multiple sessions or tabs
- Structured log parsing, syntax colors
- Response streaming (optional nice-to-have, but deferred)
- `--tee`/`--out` duplication to stdout or file

---

## 8a. Post-v1 roadmap

Deferred but intentionally kept on the roadmap. Ordered roughly by
expected priority.

### Follow mode (`tail -f` semantics) — planned

File mode in v1 stops at EOF. Post-v1, a `--follow` / `-f` flag should
keep the file open and stream appended lines into the same rolling
buffer and UI.

Sketch:

- New source implementation `internal/source/follow.go` that:
  - opens the file, reads start → EOF (same as file mode),
  - then switches to watch mode instead of emitting `EOFMsg`.
- Watch strategy, in order of preference:
  1. `fsnotify` on the file for `Write` / `Rename` / `Remove` events
     (fast, event-driven).
  2. Fallback poll loop (~250ms) that `Stat`s size + inode and reads
     the delta — needed on filesystems where fsnotify is unreliable
     (network mounts, some containers).
- **Rotation / truncation handling**: if size shrinks or inode changes,
  reopen from offset 0 and emit a `SourceRotatedMsg` so the UI can
  show a subtle marker in the log stream.
- Keep the `EOFMsg` for the *initial* read pass only when `--follow`
  is off; in follow mode it's replaced by a `FollowingMsg` status
  that drives a `[following]` badge in place of `[eof]`.
- CLI: `logwiz -f app.log` or `logwiz --follow app.log`. `-F` (capital)
  could later mean "retry on missing file" like GNU tail.
- Chat behavior is unchanged — it already reads from the rolling buffer,
  so follow mode gets chat-over-live-logs for free.

Tests to add with the feature: append after EOF, truncate-in-place,
rotate-by-rename, file deletion + recreation.

### Other post-v1 items

- **Response streaming** from the chat endpoint (SSE) so answers render
  token-by-token.
- **`--tee` / `--out`** to also write raw stdin through to stdout and/or
  a file while the TUI runs.
- **Search / filter** over the buffer (`/` is already taken by chat —
  maybe `Ctrl+F`).
- **Copy answer to clipboard** and **retry last prompt**.
- **Line selection** to scope a chat question to a specific range
  instead of "last N lines".

---

## 9. Risks & mitigations

| Risk                                                | Mitigation                                                        |
| --------------------------------------------------- | ----------------------------------------------------------------- |
| Terminal corruption if we crash mid-render          | Alt screen + deferred `Program.Quit`; panic handler restores term |
| Huge log bursts starve the UI                       | Coalesce `NewLineMsg`: reader batches up to N lines per Send      |
| Chat request blocks UI                              | Run via `tea.Cmd`, show spinner, cancel on quit                   |
| API key leakage in crash logs                       | Never log request bodies; redact Authorization header             |
| Very long single lines break layout                 | `viewport` wraps; we don't truncate in buffer storage             |
| Stdin that never closes (pipe from long-running cmd)| Expected — `EOFMsg` only on actual close; don't block quit on it  |

---

## 10. Test plan

- Unit tests:
  - `buffer`: wrap behavior, `LastN` when `n > size`, empty buffer
  - `config`: flag > env > default precedence
  - `chat.Client`: happy-path + error-body parsing via `httptest.Server`
- Manual smoke tests documented in README:
  - `yes | head -1000 | logwiz` — streaming mode scroll
  - `logwiz testdata/sample.log` — file mode EOF
  - `/ what errors do you see?` against a local OpenAI-compatible server
    (e.g. `ollama` with an OAI shim) to avoid burning API credits

---

## 11. Build order summary (one-line per step)

1. `go mod init` + add deps
2. `internal/buffer` + tests
3. `internal/source` (stdin, file) producing tea messages
4. Minimal `internal/tui` with viewport, alt screen, quit
5. Wire `cmd/logwiz/main.go` for both modes
6. Sidebar UI with stub chat
7. `internal/config` flag/env plumbing
8. `internal/chat` HTTP client
9. Connect real chat to sidebar
10. EOF badge, help overlay, error surface, README
