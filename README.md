# logwiz

A small Go TUI that behaves like `tee` with an integrated chat sidebar
for asking questions about the current log buffer, backed by any
OpenAI-compatible chat completions endpoint.

See [PLAN.md](./PLAN.md) for the v1 design and post-v1 roadmap.

## Install

```
go install github.com/nqbao/logwiz/cmd/logwiz@latest
```

Or build from source:

```
go build -o logwiz ./cmd/logwiz
```

## Usage

Stream from a running command:

```
some_command 2>&1 | logwiz
```

Read a file:

```
logwiz server.log
```

## Keys

| Key          | Action                                |
| ------------ | ------------------------------------- |
| `/`          | open chat sidebar and focus input     |
| `esc`        | blur input; again closes sidebar      |
| `tab`        | switch focus between logs and input   |
| `enter`      | send the current chat message         |
| `c`          | clear chat history                    |
| `?`          | toggle help overlay                   |
| `j`/`k`/pgup/pgdn | scroll the log pane              |
| `q` / `ctrl+c` | quit                                |

## Configuration

Flags override env vars, which override defaults.

| Flag              | Env var            | Default                      |
| ----------------- | ------------------ | ---------------------------- |
| `--api-key`       | `OPENAI_API_KEY`   | —                            |
| `--base-url`      | `OPENAI_BASE_URL`  | `https://api.openai.com/v1`  |
| `--model`         | `OPENAI_MODEL`     | `gpt-4o-mini`                |
| `--max-lines`     | —                  | `5000`                       |
| `--context-lines` | —                  | `300`                        |
| `--sidebar-width` | —                  | `50`                         |
| `--system-prompt` | —                  | log-analysis default         |

Any OpenAI-compatible server works — point `--base-url` at it (e.g.
`http://localhost:11434/v1` for an Ollama OpenAI shim).

## Tests

```
go test ./...
```
