# Contributing to Just Talk

This repository is an independent fork of
[whoamihappyhacking/just-talk-go](https://github.com/whoamihappyhacking/just-talk-go),
which is still maintained by its original author. The fork is maintained by
[wakaka6](https://github.com/wakaka6) and has diverged over differences in
project direction and community collaboration — it aims to be community-driven,
with pull requests welcome.

Thanks for your interest in contributing! Pull requests are welcome.

## Before you start

- For bug reports, usage questions, and feature ideas, open an
  [Issue](https://github.com/wakaka6/just-talk-go/issues).
- For non-trivial changes (new features, behavior changes, new dependencies,
  or anything touching hotkey/ASR/audio paths), please open an issue or
  discussion first so we can agree on the approach before you write code.
- Small, obvious fixes (typos, clear-cut bugs, docs) can go straight to a PR.

## Working on a change

1. Fork the repository and create a topic branch from `main`
   (for example `fix/wayland-hotkey-race` or `feat/asr-timeout-option`).
2. Keep commits focused; prefer
   [Conventional Commits](https://www.conventionalcommits.org/) style as used
   in this repository (`feat:`, `fix:`, `docs:`, `ci:`, …).
3. Follow the existing package boundaries and platform-specific files with
   build tags — see [AGENTS.md](AGENTS.md) for architecture notes. Platform
   behavior differs by design; keep Linux/macOS/Windows paths consistent with
   the documented notes.

## Checks before submitting

```bash
gofmt -l .                # must print nothing (run gofmt -w . to fix)
go vet ./...
go test ./...
# On machines without X11:
go test ./... -tags no_x11
```

If your change touches release packaging, also run `goreleaser check`.

## Pull requests

- Describe **what behavior changed** and **how you verified it** (commands run,
  manual steps, platform tested on). Platform-specific changes should state
  which OS/backend you exercised — maintainers cannot assume every
  contributor can test all three platforms.
- Update `CHANGELOG.md` for user-visible changes and keep both
  `README.md` and `README.en.md` in sync when you change documented behavior.
- Do not commit generated binaries, build output, or secrets.

## License

By contributing, you agree that your contributions are licensed under the
project's license: [GPL-3.0-only](LICENSE).
