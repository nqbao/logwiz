# LogViewport Implementation Plan

Replace `viewport.Model` in the logs pane with a custom `LogViewport` that
renders directly from `*buffer.Buffer` — no intermediate copy, no double
storage.

---

## Why

`viewport.Model` (v1) stores a full `[]string` copy of the content internally.
Every new log line triggers:

```
buf.Snapshot() → joined string → viewport.SetContent → split back to []string
```

Three allocations per line. With a custom viewport that holds a pointer to the
buffer and reads from it at render time, that drops to zero extra copies.

---

## Buffer change (internal/buffer/buffer.go)

Add a single O(1) random-access method. Everything else stays the same.

```go
// Line returns the i-th line in chronological order (0 = oldest).
// Returns "" for out-of-range indices.
func (b *Buffer) Line(i int) string {
    if i < 0 || i >= b.size {
        return ""
    }
    start := (b.head - b.size + b.cap) % b.cap
    return b.lines[(start+i)%b.cap]
}
```

No slice returned, no allocation. LogViewport calls `buf.Line(i)` for each
visible row.

---

## New file: internal/tui/logviewport.go

```go
package tui

type LogViewport struct {
    Width  int
    Height int

    YOffset  int // top visible logical row index
    xOffset  int // horizontal scroll in terminal columns
    Selected int // selected row index; -1 = none
}
```

### Core idea: visibleRows

Mirrors viewport's `visibleLines()`. Iterates only the rows that fit on screen:

```
top    = clamp(YOffset, 0, buf.Len())
bottom = clamp(top + Height, top, buf.Len())
rows   = buf.Line(top) … buf.Line(bottom-1)   ← O(1) each, no copy
```

### Horizontal scroll

Same mechanism as viewport v1: `ansi.Cut(line, xOffset, xOffset+Width)`.
ANSI-safe, no manual escape parsing needed.

`longestLineWidth` is tracked as a running max updated during `View()` while
iterating visible rows. No global scan, no stale-on-eviction problem — it
reflects what is currently on screen, which is where scroll makes sense.

### Row selection

```go
// SelectAt maps a screen-relative Y coordinate to a logical row index.
// Call this from the tea.MouseMsg handler.
func (v *LogViewport) SelectAt(screenY int) {
    row := v.YOffset + screenY
    v.Selected = clamp(row, 0, buf.Len()-1)
}
```

No prefix-sum needed because we don't wrap. One line = one screen row, so
`YOffset + screenY` is exact.

### Methods (mirrors viewport API shape)

```go
func (v *LogViewport) maxYOffset(buf *buffer.Buffer) int
func (v *LogViewport) AtTop() bool
func (v *LogViewport) AtBottom(buf *buffer.Buffer) bool
func (v *LogViewport) ScrollUp(n int)
func (v *LogViewport) ScrollDown(n int, buf *buffer.Buffer)
func (v *LogViewport) PageUp()
func (v *LogViewport) PageDown(buf *buffer.Buffer)
func (v *LogViewport) GotoTop()
func (v *LogViewport) GotoBottom(buf *buffer.Buffer)
func (v *LogViewport) ScrollLeft(n int)
func (v *LogViewport) ScrollRight(n int)
func (v *LogViewport) ScrollPercent(buf *buffer.Buffer) float64
func (v *LogViewport) View(buf *buffer.Buffer, selectedStyle lipgloss.Style) string
```

### View()

```go
func (v *LogViewport) View(buf *buffer.Buffer, selectedStyle lipgloss.Style) string {
    n   := buf.Len()
    top := clamp(v.YOffset, 0, max(0, n-1))
    bot := clamp(top+v.Height, top, n)

    var sb strings.Builder
    v.longestLineWidth = 0

    for i := top; i < bot; i++ {
        line := buf.Line(i)

        // track max for horizontal scroll clamping
        if w := ansi.StringWidth(line); w > v.longestLineWidth {
            v.longestLineWidth = w
        }

        // horizontal scroll
        if v.xOffset > 0 || ansi.StringWidth(line) > v.Width {
            line = ansi.Cut(line, v.xOffset, v.xOffset+v.Width)
        }

        // row highlight
        if i == v.Selected {
            line = selectedStyle.Width(v.Width).Render(line)
        }

        if i > top {
            sb.WriteByte('\n')
        }
        sb.WriteString(line)
    }

    return lipgloss.NewStyle().
        Width(v.Width).
        Height(v.Height).
        MaxHeight(v.Height).
        MaxWidth(v.Width).
        Render(sb.String())
}
```

---

## Changes to existing files

### internal/tui/model.go

```diff
- logs       viewport.Model
+ logs       LogViewport
```

Remove the `viewport` import if it's no longer used by convo (check: `convo`
still uses `viewport.Model` for the chat pane — keep that import).

### internal/tui/update.go

Remove `refreshLogsContent()` entirely. The log pane now renders live from the
buffer on every `View()` call — there is nothing to refresh.

```diff
  case source.NewLinesMsg:
      for _, line := range msg.Lines {
          m.buf.Append(line)
      }
-     m.refreshLogsContent()
      return m, nil
```

Mouse handling — add left-click row selection alongside existing wheel scroll:

```go
case tea.MouseMsg:
    if msg.Action != tea.MouseActionPress {
        return m, nil
    }
    switch msg.Button {
    case tea.MouseButtonLeft:
        m.logs.SelectAt(msg.Y)   // msg.Y is 0-relative to terminal top
    case tea.MouseButtonWheelUp:
        m.logs.ScrollUp(3)
        m.autoScroll = false
    case tea.MouseButtonWheelDown:
        m.logs.ScrollDown(3, m.buf)
        if m.logs.AtBottom(m.buf) {
            m.autoScroll = true
        }
    }
```

Key handling — replace `m.logs, cmd = m.logs.Update(msg)` delegation with
direct calls (we no longer forward tea.Msg to the viewport):

```go
case keyScrollDown, keyArrowDown:
    m.logs.ScrollDown(1, m.buf)
case keyScrollUp, keyArrowUp:
    m.logs.ScrollUp(1)
case keyPageDown:
    m.logs.PageDown(m.buf)
case keyPageUp:
    m.logs.PageUp()
```

autoScroll logic stays the same, just using `m.logs.AtBottom(m.buf)`.

`onResize` — set `m.logs.Width` / `m.logs.Height` directly, no
`refreshLogsContent()` call needed.

### internal/tui/view.go

```diff
- logsView := lipgloss.NewStyle()...Render(m.logs.View())
+ logsView := m.logs.View(m.buf, styleSelectedRow)
```

The `View()` call does the full render in one pass.

### internal/tui/styles.go

Add `styleSelectedRow` — a lipgloss style for the highlighted row, e.g.:

```go
var styleSelectedRow = lipgloss.NewStyle().
    Background(lipgloss.Color("237")).
    Foreground(lipgloss.Color("255"))
```

---

## What does NOT change

- `buffer.Buffer` ring buffer logic (only adding `Line(i)`)
- `convo` chat pane (still `viewport.Model`)
- `source` package
- `chat` package
- `prompt.go` / `buildChatMessages` (already uses `buf.LastN`)
- Layout, styles (except adding `styleSelectedRow`)
- All existing tests (buffer tests get one new test for `Line()`)

---

## Testing

- `buffer_test.go`: add `TestLine` covering in-range, out-of-range, and
  ring-wrap cases
- `logviewport_test.go`: unit test `View()`, `ScrollDown/Up`, `SelectAt`,
  `AtBottom`, horizontal scroll — pass a real `*buffer.Buffer`, no mocking
  needed
- Existing `model_test.go` tests will need minor updates: remove any assertion
  on `refreshLogsContent` side effects, update mouse msg handling if needed

---

## Non-goals (deferred)

- Soft wrap (one logical row → multiple screen rows): deferred; would require
  prefix-sum of wrap heights for `SelectAt` to work correctly
- Search/highlight across rows: can be added later using the `Selected` field
  pattern or a separate `[]int` of matching row indices
