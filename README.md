<p align="center">
  <img src="assets/banner.png" alt="spit" width="100%" />
</p>

# spit

`spit` is a small Go CLI that sends chat-completion requests to an OpenAI-compatible base URL and prints the assistant output.

![spit demo](assets/demo.gif)

## Features

- Base URL from CLI flags/environment variables, defaulting to `http://localhost:11434/v1`
- Any number of `system` and `user` prompts via args, preserved in payload order
- Optional stdin input appended as the final `user` message
- Streams assistant output as it is received
- One API request per invocation
- Supports `--version` output with build date, version, and commit metadata
- On interrupt (`Ctrl+C`/`SIGTERM`), cancels in-flight work, keeps partial output, appends newline, and exits with code `130`

## Installation

### Using Homebrew

```bash
brew install evilmarty/spit/spit
```

### Using Go

Install the latest version directly using `go install`:

```bash
go install github.com/evilmarty/spit@latest
```

Ensure your `go/bin` directory (typically `$HOME/go/bin` or `$GOPATH/bin`) is in your system's `$PATH`.

### Pre-built Binaries

You can download the latest release from [GitHub Releases](https://github.com/evilmarty/spit/releases/latest).

### Building from Source

Make sure you have [Go](https://golang.org/) installed.

Clone the repository and build the executable:

```bash
git clone https://github.com/evilmarty/spit
cd spit
go build -ldflags="-X main.Version=v1.0.0" -o spit
```

You can then move the `spit` binary to a location in your `$PATH`.

## Configuration

Flags override environment variables.

| Flag | Env var | Required | Default |
| --- | --- | --- | --- |
| `--base-url` / `-u` | `OPENAI_BASE_URL` | No | `http://localhost:11434/v1` |
| `--api-key` | `OPENAI_API_KEY` | No | unset |
| `--model` / `-m` | `OPENAI_MODEL` | No | `llama3` |
| `--format` / `-f` | n/a | No | `text` (`text` or `json`) |
| `--temperature` | `OPENAI_TEMPERATURE` | No | unset (0 to 2) |
| `--top-p` | `OPENAI_TOP_P` | No | unset (0 to 1) |
| `--max-tokens` | `OPENAI_MAX_TOKENS` | No | unset |
| `--request-timeout` | `OPENAI_REQUEST_TIMEOUT` | No | unset (duration, e.g. `10s`) |
| `--idle-timeout` | `OPENAI_IDLE_TIMEOUT` | No | unset (duration, e.g. `30s`) |
| `--reasoning-effort` | `OPENAI_REASONING_EFFORT` | No | unset |
| `--max-retries` | `OPENAI_MAX_RETRIES` | No | `3` |

Message args:

- `--system "<text>"` or `-s "<text>"` (repeatable)
- `--prompt "<text>"` or `-p "<text>"` (repeatable)
- positional args are combined into one `user` message
- stdin, when present, is appended as the last `user` message
- base URL path is used as provided, and requests append `/chat/completions` (for OpenAI-compatible APIs, pass a base URL ending in `/v1`)

Retry behavior:

- Transient errors (429 Too Many Requests, 5xx status codes, timeouts, and network errors) are retried with exponential backoff and jitter.
- Non-transient errors (4xx except 429, malformed payloads) fail immediately without retry.
- Default retry count is 3; set `--max-retries 0` to disable retries or `--max-retries <n>` to customize.

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
  -u http://localhost:1234 \
  --api-key test-key \
  -m gpt-4o-mini \
  -f json \
  --temperature 0.7 \
  --top-p 0.9 \
  --max-tokens 512 \
  --request-timeout 10s \
  --idle-timeout 30s \
  --reasoning-effort medium \
  -s "You are concise." \
  -p "Summarize this repository."
```

Multiple ordered messages in one payload:

```bash
./spit \
  --base-url http://127.0.0.1 \
  --api-key test-key \
  --system "You are a coding assistant." \
  --prompt "First question" \
  -s "Use short answers." \
  -p "Second question"
```

Append stdin as the last user message:

```bash
echo "extra context from stdin" | ./spit \
  --base-url http://localhost:1234 \
  --api-key test-key \
  --prompt "Use the following context:"
```

Use environment variables:

```bash
export OPENAI_BASE_URL=http://localhost:1234
export OPENAI_API_KEY=test-key
export OPENAI_MODEL=gpt-4o-mini
./spit --prompt "Hello"
```
