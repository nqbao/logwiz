# logwiz

A terminal log viewer for structured logs. Filter and highlight with
regex or natural language, and ask the AI questions about your logs.

## Install

```
go install github.com/nqbao/logwiz/cmd/logwiz@latest
```

Or build from source:
```
make build
```

## Setup

```bash
logwiz setup   # interactive first-time setup, asks for base URL, API key, and model
```

Alternatively, use environment variables:

```bash
export OPENAI_API_KEY=sk-...
export OPENAI_BASE_URL=https://api.openai.com/v1   # or Ollama / LM Studio / vLLM
export OPENAI_MODEL=gpt-5.4-nano
```

View, search, filter, and highlight always work without any LLM or API key.

## Usage

```
some_command 2>&1 | logwiz          # stream from pipe
logwiz server.log                   # read a file
logwiz -- your-command --flag arg   # logwiz starts and manages the child
logwiz -h                           # see all other command
```

In managed-command mode, `logwiz` captures both stdout and stderr and sends
`SIGINT` to the child process group when you quit, so wrapper shells and
well-behaved descendants are torn down with it.

Press `/` to open the command palette, then ask questions or run commands:

| Input | Result |
|-------|--------|
| `how many errors since 5min ago` | AI greps, counts, and answers |
| `show me lines with timeout` | AI highlights matching lines |
| `/timeout/` | highlight matching lines directly |
| `/timeout/i` | highlight matching lines case-insensitively |
| `:search timeout` | regex search with n/N navigation |
| `:grep error` | filter to matching lines |
| `:view pretty` | switch to humanlog-style layout |

`qq` quits from the logs view, and `ctrl+c` always quits. Press `?` for all keybindings.

## Features

- Stream from a pipe or read a file; works with JSON, logfmt, pipe-delimited (`ts | LEVEL | … | msg`, e.g. Java, with stack traces kept together), and plain text
- Regex search, filter (`:grep`), and highlight — stackable and composable
- AI command palette: ask questions in plain English, AI inspects the buffer and answers
- Sensitive field redaction before logs are sent to the LLM (enabled by default)
- Pretty-print view for structured logs; raw and colorized modes also available

## Commands & Keys

| Key | Action |
| --- | ------ |
| `/` | open command palette — type a question or a `:command` |
| `/re/[iF]` | highlight matches with Vim-style slash syntax (`i` case-insensitive, `F` literal) |
| `:search <re>` | regex search |
| `:grep [-i] [-F] [-v] <re>` | filter lines by regex (stackable, ANDed) |
| `:hl [-i] [-F] <re>` | highlight matching lines (stackable) |
| `:nohl` | clear all highlights |
| `p` | cycle view: raw → color → pretty |
| `w` | toggle line wrap |
| `n` / `N` | next / prev search match |
| `f` / `F` | pause / clear filters |
| `H` | toggle highlights on/off |
| `?` | toggle help overlay |
| `esc` | cancel palette / clear search |
| `j` / `k` / pgup / pgdn | scroll logs |
| `qq` | quit |

Other commands available inside the palette: `:filter` (attribute/time-based
filtering), `:follow` (live tail a file), `:config` (runtime configuration),
`:view` (set view mode), `:clear` (reset filters/highlights/history).
