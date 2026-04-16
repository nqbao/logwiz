# logwiz — Roadmap

## Completed

### ✅ v1.0 — Core TUI
Ring buffer, stdin/file sources, Bubble Tea TUI, chat sidebar, layout, keybindings,
focus model, help overlay, EOF badge, `\r` handling, SIGTERM to process group.

### ✅ Source batching (PR #2)
`source.pump` batches lines into `NewLinesMsg{ Lines []string }`, fixing O(n)-per-line
UI refresh on high-volume streams.

---

## v1.1 — Message foundation + agentic chat + file tail-seek

### New: `internal/logrec`

```go
type LogMessage struct {
    Lines      []string
    Timestamp  time.Time         // zero until format profiles (v1.2)
    Level      string            // "" until format profiles (v1.2)
    Fields     map[string]string // nil until format profiles (v1.2)
    SourceLine int64
}

type Parser interface {
    Feed(line string) []LogMessage
    Flush() []LogMessage  // EOF / idle timeout
}
```

`LineParser` (1 line = 1 message) is the only parser in v1.1. `AnchorParser` ships in v1.2.

Pipeline: `pump → NewLinesMsg → parser.Feed() → []LogMessage → buf.Append(msg)`

### Buffer retyped to `[]LogMessage`

`buffer.Buffer` stores `[]logrec.LogMessage`. New method `LineCount()` alongside `Len()`.
`--max-lines` means messages; `--max-bytes` (~50 MB) caps memory regardless of message size.

### Viewport rendering

Walks messages, joins `msg.Lines` fields. Output identical to today with `LineParser`.

### Agentic chat

Always inject current viewport messages as system context. Expose two tools:
- `get_viewport()` — re-fetch viewport snapshot (if logs moved during in-flight request)
- `search(pattern string)` — regex over lines in full buffer, returns matching messages

`internal/chat.Client` gains tool-call support: send `tools`, handle `tool_calls` response,
run locally, append `tool` role message, re-request until done.

### File-mode tail-seek

Byte-seek backward from EOF, advance to first message boundary. Emit `[truncated]` marker
when seek skips content. `--max-lines` / `--max-bytes` enforced at seek time.

---

## v1.2 — Navigation + filter + format profiles

- **Find** (`Ctrl+F`): regex over `LogMessage.Lines`; `n`/`N` jump message-to-message
- **Filter toggle**: whole-message show/hide (stack traces stay intact)
- **Highlight rules**: substring + `level=` field matching
- **Format profiles** (JSON, logfmt, anchor/timestamp): auto-detect → pick parser +
  populate `Level`, `Timestamp`, `Fields`. Ships `AnchorParser` for multi-line grouping.

---

## v1.3 — Follow mode

- `--follow` / `-f`: keep file open after EOF, stream appended lines
- `fsnotify` watch with polling fallback; rotation/truncation handling
- `[following]` status badge

---

## v1.4+ — Deferred

- Response streaming (SSE)
- `--tee` / `--out` to mirror stdin to stdout/file
- Copy answer to clipboard, retry last prompt
- SQL-over-fields, bookmarks, timeline navigation

---

## Risks

| Risk | Mitigation |
| ---- | ---------- |
| Huge log bursts | Source batching (shipped) |
| AnchorParser holds message open | 200ms idle timeout + EOF flush |
| Tail-seek lands mid-message | `[truncated]` marker; at most one message lost |
| Huge single messages (10k-line trace) | `--max-bytes` cap |
| API key in crash logs | Never log request bodies |
