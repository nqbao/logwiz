# logwiz — Roadmap

## v1.1 — Remaining

- **Multi-line logs (AnchorParser)**: Wraps an inner parser; groups lines into
  single `LogMessage` entries via anchor regex. Anchor line starts a new message,
  non-matching lines are continuations. `Raw` = `\n`-joined text.
  Unstructured: `^\d{4}-\d{2}-\d{2}[T ]` (timestamp prefix). Pretty-printed JSON:
  `^\s*[{\[]`. Logfmt/JSONL are line-oriented — degenerates to 1:1 passthrough.
  `--multiline-pattern <regex>` override; auto-detect or default off.
  50-line stack trace → 1 buffer slot.

- **Command box context carry-over**: When the user submits a command box query
  and gets an inline answer, subsequent command box input should be able to
  reference the previous Q&A (e.g. "why?" or "show me the lines"). The cmd agent
  would receive a short conversation history (last question + answer) as context,
  so follow-up queries feel conversational without requiring the user to open the
  full chat sidebar.

---

## Deferred

- `--follow` / `-f`: keep file open after EOF, stream appended lines (`fsnotify` watch with polling fallback; rotation/truncation handling; `[following]` status badge)
- Response streaming (SSE)
- Copy answer to clipboard, retry last prompt
- Multi-file support: open and merge multiple log files by timestamp
- Filter by field: scoped filters on parsed attributes (e.g. `level=ERROR`, `service=auth`)
- Filter by timeline: `:since` / `:until` commands, `--since`/`--until` CLI flags, relative and absolute time ranges
- More log formats: syslog, glog, Python logging (in addition to existing JSON/logfmt)
- Aggregation / group-by: count by field, error rate, top-N values. Natural language
  queries like "how many errors per service?" or "top 5 endpoints by count" translated
  to internal aggregation queries — no SQL required.
- User knowledge injection: let users share domain-specific context with the agent
  (e.g. "request_id correlates lines across services", "duration_ms > 5000 is a regression").
  Possible approaches: (1) context file (`.logwiz/context.md`) loaded on startup,
  (2) `--context` flag for ad-hoc sessions, (3) in-chat "remember" command that
  persists to the context file.
- Skills: user-authored markdown snippets in `.logwiz/skills/*.md` that the agent
  can load on demand. On startup, skill names/descriptions are auto-discovered and
  injected into the agent context so the LLM knows what's available. A `load_skill(name)`
  inspection tool lets the agent fetch the full content of a relevant skill during
  the inspect phase, without stuffing all skills into every prompt.
- Full log line detail view: expand a line (e.g. Enter key) to show a vertical
  key-value table of all parsed fields, full message, and multi-line stack traces
  without terminal width truncation.
- SQL-over-fields, bookmarks, timeline navigation
- Full large-file navigation: `FileBackedSource` with mmap + lazy line-offset index
  + paged reads on scroll, so users can view any part of a multi-GB file without
  loading it all into the ring buffer. Tail-seek remains the fast-startup default.
- Line folding: collapse N consecutive identical/similar lines into `[×42]`
- Live level stats: status bar shows level distribution (e.g. "3 ERR | 47 WARN"),
  level toggle keys (1-5) to show/hide DEBUG/INFO/WARN/ERROR/FATAL
- Compression support: transparent .gz/.bz2 reading (low priority — users can
  pipe `zcat` manually)

---
