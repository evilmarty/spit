# spit

`spit` is a small Go CLI that sends chat-completion requests to an OpenAI-compatible endpoint and prints the assistant output.

## Features

- Endpoint host and port from CLI flags or environment variables
- Any number of `system` and `user` prompts via args, preserved in payload order
- Optional stdin input appended as the final `user` message
- Streams assistant output as it is received
- One API request per invocation

## Configuration

Flags override environment variables.

| Flag | Env var | Required | Default |
| --- | --- | --- | --- |
| `--endpoint` / `-e` | `OPENAI_ENDPOINT` | Yes | none |
| `--port` | `OPENAI_PORT` | No | inferred from endpoint (`443` for `https`, else `80`) |
| `--api-key` | `OPENAI_API_KEY` | No | unset |
| `--model` / `-m` | `OPENAI_MODEL` | No | `gpt-4o-mini` |
| `--format` / `-f` | n/a | No | `text` (`text` or `json`) |
| `--temperature` | `OPENAI_TEMPERATURE` | No | unset |
| `--top-p` | `OPENAI_TOP_P` | No | unset |
| `--max_tokens` | `OPENAI_MAX_TOKENS` | No | unset |
| `--reasoning-effort` | `OPENAI_REASONING_EFFORT` | No | unset |

Message args:

- `--system "<text>"` or `-s "<text>"` (repeatable)
- `--prompt "<text>"` or `-p "<text>"` (repeatable)
- positional args are combined into one `user` message
- stdin, when present, is appended as the last `user` message
- endpoint can include a path; if omitted, `/v1/chat/completions` is used

## Build

Build the binary in the current directory:

```bash
go build .
```

Build with an explicit output binary name:

```bash
go build -o spit .
```

## Test

Run all tests:

```bash
go test ./...
```

Run tests with verbose output:

```bash
go test -v ./...
```

## Usage examples

Run the built binary with explicit flags:

```bash
./spit \
  -e localhost \
  --port 1234 \
  --api-key test-key \
  -m gpt-4o-mini \
  -f json \
  --temperature 0.7 \
  --top-p 0.9 \
  --max_tokens 512 \
  --reasoning-effort medium \
  -s "You are concise." \
  -p "Summarize this repository."
```

Multiple ordered messages in one payload:

```bash
./spit \
  --endpoint http://127.0.0.1 \
  --api-key test-key \
  --system "You are a coding assistant." \
  --prompt "First question" \
  -s "Use short answers." \
  -p "Second question"
```

Append stdin as the last user message:

```bash
echo "extra context from stdin" | ./spit \
  --endpoint localhost \
  --port 1234 \
  --api-key test-key \
  --prompt "Use the following context:"
```

Use environment variables:

```bash
export OPENAI_ENDPOINT=localhost
export OPENAI_PORT=1234
export OPENAI_API_KEY=test-key
./spit --prompt "Hello"
```
