# Yu

Minimal terminal LLM agent.

Yu is a small Go REPL for chatting with OpenAI-compatible model providers. It
keeps conversation history in memory or PostgreSQL, streams model output into
the terminal, and can show vendor reasoning deltas when the selected model
supports thinking.

## Configure models

Define the models you can pick from in `~/.yu/models.yaml`. API keys are **not**
stored in this file — each entry references an environment variable by name via
`api_key_env`.

```yaml
models:
  - name: deepseek
    model: deepseek-v4-flash
    base_url: https://api.deepseek.com/v1
    api_key_env: DEEPSEEK_API_KEY
    supports_thinking: true
    thinking_style: deepseek
    reasoning_path: choices.0.delta.reasoning_content
```

`thinking_style` controls how thinking is enabled for a provider:

- empty / `deepseek`: sends `thinking: {type: enabled|disabled}`
- `qwen`: sends `enable_thinking: true|false`

`reasoning_path` is optional. It defaults to
`choices.0.delta.reasoning_content`, but can be set per model if a compatible
provider streams reasoning text under another raw JSON field.

Put the actual keys in `~/.yu/.env`:

```env
DEEPSEEK_API_KEY=...
MIMO_API_KEY=...
```

## Run

```sh
go run ./cmd/yu
```

At startup Yu uses the first configured model. Switch models inside the REPL
with `/model <name|number>`.

Commands inside the REPL:

- `/model`: switch to another configured model while keeping conversation history
- `/think`: toggle thinking mode for models that support it
- `/exit` or `/quit`: exit

`~/.yu/models.yaml` is required — Yu exits with an error if it's missing.

## Session database

The CLI stores sessions with GORM. Set `YU_SESSION_DSN` before starting Yu;
`YU_SESSION_DRIVER` is optional and defaults to `postgres` when omitted:

```env
YU_SESSION_DRIVER=postgres
YU_SESSION_DSN=postgres://yu:yu@localhost:5432/yu?sslmode=disable
```

`YU_SESSION_DRIVER` supports `postgres`, `sqlite`, and `mysql`. If
`YU_SESSION_DSN` is set and the driver is omitted, Yu defaults to `postgres`.
You can put these values in `~/.yu/.env` next to your provider API keys. Yu
creates its session tables automatically on startup. Custom integrations can
still choose their own session service, such as `session.NewInMemoryService()`
or `session/database.Open(...)`.

For SQLite:

```env
YU_SESSION_DRIVER=sqlite
YU_SESSION_DSN=yu.db
```

For local PostgreSQL development, start the included database:

```sh
docker compose up -d postgres
```

The included PostgreSQL container uses:

```text
host: localhost
port: 5432
database: yu
user: yu
password: yu
```

## Structure

```text
config/                 # ~/.yu/models.yaml profiles and config paths
cmd/yu/                 # terminal REPL frontend and default app assembly
runner/                 # execution engine: session lifecycle + persistence
agent/agent.go          # agent interface and invocation context
agent/llmagent/         # LLM-backed agent: model→tool→model loop
session/                # event-sourced history and session service
llm/                    # shared model/message abstractions
llm/openai/             # OpenAI-compatible streaming client
tool/                   # tool interface; tool/fstool file tools
render/                 # event renderers (CLI)
```
