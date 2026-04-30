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

Skip this if you just want the viewer — search, filter, and highlights work
without any API key.

## Usage

```
some_command 2>&1 | logwiz          # stream from pipe
logwiz server.log                   # read a file
logwiz setup                        # configure API endpoint
```

Press `/` to open the command palette, then ask questions or run commands:

| Input | Result |
|-------|--------|
| `how many errors since 5min ago` | AI greps, counts, and answers |
| `show me lines with timeout` | AI highlights matching lines |
| `:search timeout` | regex search with n/N navigation |
| `:grep error` | filter to matching lines |
| `:view pretty` | switch to humanlog-style layout |

`q` quits. Press `?` for all keybindings.

## Commands & Keys

| Key | Action |
| --- | ------ |
| `/` | open command palette — type a question or a `:command` |
| `:chat [q]` | open chat sidebar |
| `:search <re>` | regex search |
| `:grep [-i] [-F] [-v] <re>` | filter lines by regex (stackable, ANDed) |
| `:hl [-i] [-F] <re>` | highlight matching lines (stackable) |
| `:nohl` | clear all highlights |
| `p` | cycle view: raw → color → pretty |
| `w` | toggle line wrap |
| `n` / `N` | next / prev search match |
| `f` / `F` | pause / clear filters |
| `H` | toggle highlights on/off |
| `c` | clear chat history |
| `?` | toggle help overlay |
| `tab` | switch focus (logs / chat input) |
| `esc` | close sidebar / cancel palette / clear search |
| `j` / `k` / pgup / pgdn | scroll logs |
| `q` | quit |

Other commands available inside the palette: `:filter` (attribute/time-based
filtering), `:follow` (live tail a file), `:config` (runtime configuration),
`:view` (set view mode), `:clear` (reset filters/highlights/chat).

## Configuration

Flags > env vars > `~/.logwiz/config.json` > defaults.

Set at runtime with `:config <key> [val]`, persisted to `~/.logwiz/config.json`.

| Flag | Env | Default |
| --- | --- | --- |
| `--api-key` | `OPENAI_API_KEY` | — |
| `--base-url` | `OPENAI_BASE_URL` | `https://api.openai.com/v1` |
| `--model` | `OPENAI_MODEL` | `gpt-5.4-nano` |
| `--max-lines` | — | `50000` |
| `--context-lines` | — | `10` |
| `--sidebar-width` | — | `50` |
| `--view` | — | `raw` |
| `--format` | — | `auto` |

## Tests

```
make test
```
