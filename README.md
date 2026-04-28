# logwiz

A terminal log viewer with an integrated AI chat sidebar, backed by any
OpenAI-compatible endpoint. Read from stdin or a file, search, filter,
highlight, and ask questions about your logs.

## Install

```
go install github.com/nqbao/logwiz/cmd/logwiz@latest
```

Or build from source:
```
make build
```

## Usage

```
some_command 2>&1 | logwiz          # stream from pipe
logwiz server.log                   # read a file
logwiz --version                    # print version
```

## Commands & Keys

| Key | Action |
| --- | ------ |
| `/` | open command palette |
| `:chat [q]` | open chat sidebar |
| `:search` / `:grep` | regex search / filter |
| `:hl` / `:nohl` | highlight / clear highlights |
| `p` / `:pretty` | toggle pretty mode (humanlog-style) |
| `w` | toggle line wrap |
| `n` / `N` | next / prev search match |
| `f` / `F` | pause / clear filter |
| `H` | toggle highlights |
| `c` | clear chat history |
| `?` | toggle help overlay |
| `tab` | switch focus |
| `esc` | close sidebar / cancel palette |
| `j` / `k` / pgup / pgdn | scroll logs |
| `q` | quit |

Unprefixed palette input runs through an AI agent — ask questions like
"how many errors?" or "show me lines with timeout".

## Configuration

Flags > env vars > `~/.logwiz/config.json` > defaults. Runtime config
via `:config <key> [val]`.

| Flag | Env | Default |
| --- | --- | --- |
| `--api-key` | `OPENAI_API_KEY` | — |
| `--base-url` | `OPENAI_BASE_URL` | `https://api.openai.com/v1` |
| `--model` | `OPENAI_MODEL` | `gpt-4o-mini` |
| `--max-lines` | — | `50000` |
| `--context-lines` | — | `10` |
| `--sidebar-width` | — | `50` |
| `--pretty` | — | `false` |
| `--format` | — | `auto` |

## Tests

```
make test
```
