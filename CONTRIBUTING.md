# Contributing to pomo-time

Thanks for your interest! A few guidelines:

## Getting started

```bash
git clone https://github.com/alexgc96/pomo-time
cd pomo-time
CGO_ENABLED=1 go build -o pomo .
```

Requires Go 1.21+ and CGO enabled (sqlite3). Optional `figlet` for the header.

## Before opening a PR

- Keep changes focused — one thing per PR
- Test your change manually end-to-end (start a session, check the report, etc.)
- If adding a feature, consider whether it needs a config toggle
- Run `go vet ./...` before submitting

## Reporting bugs

Open an issue with:
- What you did
- What you expected
- What actually happened
- Your OS and Go version

## Feature requests

Open an issue first to discuss before building — happy to give feedback on
direction before you invest time in an implementation.
