# AGENTS.md

## Application overview

`spit` is a Go CLI that sends prompts to an OpenAI-compatible Chat Completions base URL and streams output to stdout as it is received.

Core behavior:

- Accepts base URL configuration (`--base-url`/`-u`)
- Requires a model via `--model`/`-m` or `OPENAI_MODEL`
- Accepts ordered message inputs from `--system`/`-s`, `--prompt`/`-p`, positional args, and optional stdin
- Sends one chat-completions request with `stream: true`
- Supports optional generation/lifecycle controls (`--format`, `--temperature`, `--top-p`, `--max_tokens`, `--request-timeout`, `--idle-timeout`, `--reasoning-effort`)
- Appends a trailing newline after stream completion (without duplicating an existing newline)

## Testing requirements

Contributors must keep total coverage above 90%.

Required local checks before finishing changes:

```bash
go test ./...
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

Coverage target:

- Total statements coverage must remain **>= 90%**

When changing request/stream behavior or argument parsing, add or update tests in `main_test.go` for:

- Happy path behavior
- Invalid input and parse errors
- API error responses and malformed payloads
- Streaming edge cases (SSE and non-SSE fallback)

## Go best practices for this project

1. Keep changes small, explicit, and type-safe.
2. Prefer standard library solutions unless a dependency is clearly justified.
3. Return explicit errors with actionable messages; avoid silent fallbacks.
4. Validate and normalize CLI input early.
5. Preserve deterministic message ordering in payload construction.
6. Keep streaming robust: handle `[DONE]`, malformed chunks, read/write failures, and final newline behavior.
7. Update `README.md` when flags or runtime behavior change.
8. Keep tests focused and branch-aware; add regression tests for every bug fix.
