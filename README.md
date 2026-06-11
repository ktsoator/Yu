# Yu

Minimal terminal LLM agent.

Yu is a small Go REPL for chatting with OpenAI-compatible model providers. It
keeps conversation history in memory, streams model output into the terminal,
and can show vendor reasoning deltas when the selected model supports thinking.

## Configure models

Define the models you can pick from in `~/.yu/models.yaml`. You can start from
the repository's `models.yaml`. API keys are **not** stored in this file — each
entry references an environment variable by name via `api_key_env`.

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
go run .
```

At startup you'll be prompted to choose a model (press Enter for the first one).

Commands inside the REPL:

- `/model`: switch to another configured model while keeping conversation history
- `/think`: toggle thinking mode for models that support it
- `/exit` or `/quit`: exit

`~/.yu/models.yaml` is required — Yu exits with an error if it's missing.

## Structure

```text
main.go                 # startup wiring
repl.go                 # terminal REPL and slash commands
config.go               # ~/.yu/models.yaml parsing and model selection
model.go                # OpenAI-compatible model construction
agent/agent.go          # agent interface and config
agent/llmagent/         # LLM-backed agent implementation
llm/                    # shared model/message abstractions
llm/openai/             # OpenAI-compatible streaming client
cmd/probe/              # vendor probing utility for raw streaming chunks
```
