# AGENTS.md

This file gives coding agents concise guidance for working in this repository.

## Project

This fork is maintained by Aono255. Its maintenance/default branch is `master`; the upstream remote is `whoamihappyhacking/just-talk-go` and updates should be merged from `upstream/master`, never force-synced over local fixes. Commit messages and PR titles/bodies must be in Chinese, with Conventional Commit prefixes.

Just Talk is a desktop voice input tool. It records with a global hotkey, sends audio to streaming ASR, then copies recognized text to the clipboard or submits it into the focused input field.

The supported desktop targets are Linux, macOS, and Windows.

## Build And Test

This project uses native platform APIs. Linux and macOS require cgo; Windows uses direct Win32 calls and builds with `CGO_ENABLED=0`.

```bash
make build              # Build for the current platform
make run                # Run on the current platform
make test               # Run all tests
go test ./...           # Faster default test command
go test ./... -tags no_x11
goreleaser check
CGO_ENABLED=1 go build -o build/just-talk ./cmd/just-talk
go build -o build/just-talk.exe ./cmd/just-talk  # Windows
JUST_TALK_TEST_WINDOWS_AUDIO=1 go test ./plugins/voice -run TestWindowsRecorderIntegration -v
JUST_TALK_TEST_WINDOWS_HOTKEY=1 go test ./hotkey -run TestWindowsHookIntegration -v
```

Release builds are configured by `.goreleaser.yaml` and `.github/workflows/release.yml`, using GoReleaser v2 through the official `goreleaser/goreleaser-action`. Pushing a `v*` tag builds and publishes Linux, macOS, and Windows archives for amd64 and arm64, plus `SHA256SUMS.txt`. Linux and macOS release binaries must remain native cgo builds on their respective GitHub-hosted runners. GoReleaser OSS split/merge is not available, so each native matrix runner creates one archive and the final job only merges those archives into the GitHub Release.

Use stable `vMAJOR.MINOR.PATCH` release tags. The workflow passes `RELEASE_VERSION` to GoReleaser to embed the real tag in `--version`, then generates `Formula/just-talk.rb` from published checksums and pushes it to `master` using the built-in token. Update `scripts/update_homebrew.py` rather than hand-editing the generated formula. Homebrew and macOS prebuilt releases require macOS 15 or later; keep the cgo deployment flags and formula requirement aligned. Forks do not deploy the copied upstream website automatically.

`.github/workflows/homebrew.yml` validates real installation on both macOS architectures after formula publication, and supports manual dispatch. Keep those installation checks when changing packaging.

Do not add or preserve non-cgo macOS fallback builds. A build that compiles but cannot provide native hotkeys, recording, clipboard, auto-submit, or overlay is worse than an explicit build failure.

Useful runtime commands:

```bash
just-talk               # TUI mode, default
just-talk --no-tui      # daemon mode
just-talk --doctor      # startup environment check
just-talk --check-context # context source and input binding check; currently macOS Codex Desktop
just-talk --backend x11
just-talk --backend wayland
```

`JUST_TALK_BACKEND` or `--backend` can force `x11`, `wayland`, or `darwin`. `mock` exists for internal provider testing but is not part of the normal user path.

## Platform Dependencies

Linux:

- Wayland hotkeys use evdev and require readable `/dev/input/event*`, usually via the `input` group.
- Wayland clipboard and auto-submit use `wl-clipboard` and `wtype`.
- X11 hotkeys and overlay use native X11 through cgo.
- X11 auto-submit uses XTest and clipboard tools.

macOS:

- Global hotkeys use CGEventTap through `ApplicationServices`.
- The normal TUI/daemon entry runs the Core Foundation event loop continuously on the locked process main thread, with the Go application on a background goroutine. This is required to refresh AVFoundation device objects after idle headset disconnect/reconnect. Keep helper and one-shot CLI dispatch before this entry, keep the loop alive before native sources arrive, and queue its stop request so immediate application return cannot lose shutdown.
- Recording uses AVFoundation with the selected default microphone and 16 kHz, 16-bit mono PCM. CoreAudio only queries the selected device UID; it does not change system devices or hardware formats. Preserve the first-buffer readiness check, bounded in-memory capture buffer, reader wakeup on stop, and unread tail audio. Do not restore an automatic AudioQueue fallback.
- Clipboard uses NSPasteboard.
- Auto-submit posts native keyboard events.
- Overlay uses an AppKit `NSPanel` helper process.
- Users grant Accessibility and Microphone permissions to the terminal app that launches Just Talk, not to a separate `.app` bundle.
- Full Xcode is not required, but Apple Command Line Tools must provide `clang` and the macOS SDK.

Windows:

- Global hotkeys poll `GetAsyncKeyState` at 5 ms intervals and use `WH_KEYBOARD_LL` as a short-lived physical-edge fallback. Providers emit state edges without key-repeat events.
- The low-level hook is observational: it must always pass events to the next hook and must not consume or replay modifier keys. This keeps normal `Alt`, `Super`, and combinations such as `Alt+Tab` untouched. Windows modifier combinations require an exact modifier set; hook fallback state is reconciled against the unsuppressed physical key state on new modifier presses and after release.
- Recording uses native `winmm` wave input at 16 kHz, 16-bit mono PCM.
- Clipboard operations use the Win32 Unicode clipboard through the existing clipboard dependency.
- Auto-submit uses `SendInput` to post Ctrl+V.
- Overlay uses a no-activate, topmost, click-through Win32 window.
- Config is stored under `%APPDATA%\just-talk`; logs and stats use `%LOCALAPPDATA%`.
- No external ffmpeg, SoX, clipboard tool, cgo toolchain, or administrator privilege is required.

## Architecture

```text
cmd/just-talk/main.go
  -> config.Load
  -> doctor.Run
  -> hotkey.NewProvider
  -> engine.New
  -> load voice + overlay plugins
  -> TUI or daemon mode
```

Core packages:

- `hotkey/`: platform global hotkey providers plus shared combo/event types.
- `engine/`: plugin lifecycle and config reload orchestration.
- `plugins/voice/`: recorder, ASR streaming, hotkey behavior, clipboard/auto-submit dispatch, stats.
- `plugins/overlay/`: recording status capsule for Linux, macOS, and Windows.
- `internal/autotype/`: platform paste/auto-submit implementation.
- `internal/clipboard/`: platform clipboard implementation.
- `internal/correction/`: context source adapters, model client, glossary and local learning memory.
- `internal/doctor/`: startup environment checks.
- `internal/tui/`: Bubble Tea configuration UI.

## Hotkey Notes

`Combo` is `{Mods Modifier, Key KeyCode}`. Modifier-only hotkeys use `KeyNone`, for example `Option+Command` on macOS maps to `ModAlt|ModSuper` with `KeyNone`.

Providers should emit key down/up events promptly. Fast repeated toggle presses and hold-mode release handling are user-visible and have historically been fragile, so avoid changes that add blocking work to provider event loops or hotkey handlers.

Plugin hotkey registration must happen in `Plugin.Init()`, not in `Plugin.Start()`. The registry starts dispatch goroutines after plugin loading; registering late can drop events.

macOS translation uses a third modifier above the base two-modifier shortcut. Native events carry a monotonically increasing sequence, current modifiers and the registered combo's hold history. Multiple combo channels can dispatch out of order: route each native snapshot once, retain translation intent on release, and reset hold history for the next press. Do not start a second recorder or let release of the extra modifier downgrade the session. Translation is currently macOS-only; do not grab plain application keys on other platforms.

## Voice Pipeline Notes

Stopping a recording is intentionally split from hotkey handling:

- Hotkey handlers update state quickly.
- Recorder stop, final audio send, ASR final wait, clipboard writes, and auto-submit happen in background finish work.
- Debug logs should identify whether a stop is waiting on recorder, final audio send, ASR final, clipboard, or ASR client close.
- Stop timers must validate both recording generation and per-delay epoch. An expired AfterFunc may survive Stop and acquire the mutex after resume plus another release.
- pendingDone counts tracked, non-canceled finish sessions. A canceled/unknown/duplicate completion must not decrement another session; keep session identity through completion.
- Finish workers retain ownership through ordinary as well as AI output. Do not launch an untracked paste goroutine. Recheck cancellation after acquiring the output lock and at the platform paste guard; completed OS key posting cannot be recalled.
- Formal models are bounded abstractions, not proofs of the native UI or model response quality. Pair safety checks with deliberate fault mutations and successful-path reachability checks; retain deterministic code regressions for counterexamples.

Avoid double-output bugs: recognized final text should be dispatched once per user-stopped session. If changing ASR result handling, check both `Final()` and `Done()` paths.

The ASR generation and billing resource use the existing `voice.resource_id` field. `config.ASRResources()` defines the four Doubao 1.0/2.0 duration/concurrency choices. Preserve custom IDs when saving unrelated TUI fields. A recording keeps its start-time ASR configuration snapshot; saving a new resource only affects subsequent recordings. Both generations use the existing streaming endpoint and `model_name: bigmodel`; do not infer a new wire protocol from the resource prefix or silently switch resources after an error.

Translation snapshots its output language and model settings when recording begins. Use one model request for optional correction and translation, returning and pasting only the final target-language result. Never chain Correct then Translate or generate an intermediate Chinese result. Ordinary correction memory is source-interpretation data; target-language memory controls translation preferences. With correction disabled, omit chat context, hotwords and ordinary memory. With learning disabled, omit all memory. Reuse the model client, cancellation and destination checks; never paste a Chinese intermediate or silently skip a failed translation. Translation-edit feedback includes the output language so extraction does not turn a shortcut choice into a global preference.

AI input correction and revision learning share model settings and compact local memory, independently of chat adapters. `correction.enabled` controls correction; `correction.context` adds optional chat context (default true for existing configs); `correction.learning` controls independent revision learning and memory references. Currently only Codex Desktop has a context adapter. Unmatched clients use generic correction bound to the actual frontmost application PID and AX window. If an editable input is readable, additionally guard its text and selection. Window-only binding deliberately cannot detect field/chat changes inside that same window; document this limitation rather than pretending a field was captured. Failure from a matched context adapter must remain explicit and never fall through to input-only correction. Check the bound target before the model request and again before paste. A non-nil empty Messages slice denotes a successful empty-chat read, while nil is invalid. Native capture permits no message roles only after a successful bounded main-region scan; headings without any body remain errors. Empty bindings retain app/main/editor/draft guards and reject any later message, including assistant-only messages. Existing user anchors may never disappear into an accepted empty binding. Retain/release a user anchor only when non-null.

Generic macOS output and revision capture are separate capabilities. Output always checks the actual foreground PID and window; optional revision capture requires AX focus, writable text, selected range and containing window. Input capture failure must not prevent window-bound correction, but never synthesize revisions or weaken a matched conversation adapter guard. Do not read terminal history, secure fields or arbitrary neighboring controls. Input learning observes only the exact pasted span and subsequent user revisions in that same bound input; no source ID or sent-message reader is involved. Unchanged model output and clipboard-only delivery cannot create memory. Extract compact notes in a separate background request only after unmodified Enter. Idle pauses, focus loss, clearing without an earlier Enter, expiry and later programmatic paste must never create feedback. Keep the event timestamp in Unix time, modifier filtering and a bounded pre-key snapshot; verify the same input before confirming. Enter is a user confirmation convention, not proof of sending or IME completion. Keep cancellation/generation checks and the learning commit lock so disabling, shutdown or changing model service configuration prevents stale writes. Never hold the voice-state mutex during disk commits. Preserve the v1 local memory file format and do not log or persist complete input samples. Keep correction-memory.json as ordinary input without migration; translation-memory.en.json and translation-memory.ja.json each use independent 32-note budgets. Read/write the language group fixed by the recording/revision, never current settings or a model-chosen group. Translation learning compares final pasted translation with the user revision, not source text with translation. Extract using only same-group notes; never feed English translation preferences into Japanese or ordinary correction. CLI show/clear defaults to ordinary input and --memory-group input|en|ja selects one group only.

Codex exposes elapsed clocks as peer static text beside role headings, outside the grouped message body. End user collection at that boundary; never append peer clocks or following tool activity to user messages. Do not strip clock-like text inside actual message bodies or remove the conversation guard to hide collection errors.

The macOS context reader queries and validates `AXRole == AXApplication` before reading `AXFocusedUIElement`, which activates Chromium's standard accessibility interface. Initial `kAXErrorNoValue` has a 500 ms readiness budget for that same app; this does not interrupt a blocking AX call. Other errors stop immediately. Preserve AX failure codes. Do not substitute another window or unbound input, or force screen-reader attributes such as `AXManualAccessibility` or `AXEnhancedUserInterface`.

## UI And Logging

TUI mode must not write normal logs to stdout/stderr because it corrupts the Bubble Tea layout. TUI logs go through the in-app log/status area; debug event details should only be visible when `--debug` is enabled.

`voice.SetOutput(io.Discard)` is intentional in TUI mode.

## Repository Rules

- Prefer existing package boundaries and platform-specific files with build tags.
- Keep user-facing doctor output short and action-oriented. Do not list implementation details as checks unless the user can act on them.
- README is bilingual: update both `README.md` and `README.en.md`.
- `CHANGELOG.md` should be updated for user-visible behavior changes.
- The original upstream does not accept pull requests; keep fork maintenance in this repository unless the user requests otherwise.
