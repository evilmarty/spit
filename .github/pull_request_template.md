## Description

Please describe the purpose of this pull request, the problem it solves, and how the changes were implemented.

## Checklist

Please verify that your contributions follow the development and architectural guidelines of `spit`:

### General
- [ ] All tests pass cleanly (`go test ./...`)
- [ ] Total code coverage remains **>= 90%** (`go test -cover ./...`)
- [ ] Updated `README.md` or other documentation, if necessary

### Architectural Conventions
- [ ] **CLI behavior and input handling**:
  - [ ] Message ordering is deterministic across `--system`/`-s`, `--prompt`/`-p`, positional args, and stdin.
  - [ ] New flags follow existing naming/help conventions and environment variable precedence.
- [ ] **Request/stream robustness**:
  - [ ] Streaming behavior still handles `[DONE]`, multiline `data:` events, and fallback/non-SSE paths correctly.
  - [ ] Timeout/retry behavior remains explicit and bounded (`request-timeout`, `idle-timeout`, `max-retries`).
- [ ] **Interrupt behavior**:
  - [ ] `SIGINT`/`SIGTERM` handling remains graceful (partial output preserved, trailing newline ensured, exit code `130`).
- [ ] **Release/build metadata**:
  - [ ] Build/version behavior (including `--version` and release ldflags) remains correct when touched.

## Verification / Demo (if applicable)

Please describe manual verification steps (CLI invocations and outputs) and/or paste relevant test command output.

Thanks for contributing!
