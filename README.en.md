<p align="center">
  <img src="docs/assets/brand/icon-d.svg" alt="Just Talk" width="96">
</p>

# Just Talk

[中文](README.md) · [English](README.en.md)

[![Release](https://img.shields.io/github/v/release/wakaka6/just-talk-go?label=release)](https://github.com/wakaka6/just-talk-go/releases)
[![Release workflow](https://github.com/wakaka6/just-talk-go/actions/workflows/release.yml/badge.svg)](https://github.com/wakaka6/just-talk-go/actions/workflows/release.yml)
[![Go version](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](go.mod)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)
[![Homebrew](https://img.shields.io/badge/Homebrew-wakaka6%2Ftap-FBB040?logo=homebrew&logoColor=white)](https://github.com/wakaka6/homebrew-tap)

Type less. Talk instead.

Just Talk is a desktop voice input tool. Press a global hotkey to record; audio is streamed to the Doubao large-model ASR, and the recognized text is copied to the clipboard or submitted directly into the focused input field. It is built for coding, chatting, note-taking, and long-form text entry.

![Just Talk TUI](docs/screenshot-tui.png)

## Features

- Global hotkey recording with `toggle` (press to start, press again to stop) and `hold` (push-to-talk) modes.
- Voice hotkeys are restricted to keys suitable for global shortcuts: modifier-only combos, function keys, Tab, CapsLock, arrow and navigation keys. Letters, digits, punctuation, Space, and other text-producing keys are rejected.
- Doubao large-model streaming ASR (optimized bidirectional streaming plus a second pass) with hotword boosting.
- Automatic clipboard copy, with optional auto-submit into the focused field.
- Recording overlay: on macOS it expands from the notch and shows live recognized text, mic level, and the final result; Linux (Wayland/X11) and Windows get an always-on-top status capsule.
- TUI configuration for hotkeys, mode, auto-submit, stop delay, hotwords, and overlay position — saved changes apply immediately without a restart.
- Usage statistics: total sessions, recognized characters, average speed, and recent speed.
- `Esc` cancels a recording; `R` retries after a retryable error.

## Platform Support

| Platform | Status | Notes |
| --- | --- | --- |
| Linux Wayland | Supported | Verified on Sway / wlroots; hotkeys use evdev and require `input` group access; clipboard/auto-submit need `wl-clipboard` plus `wtype` or writable `/dev/uinput` |
| Linux X11 | Supported | Native X11 global hotkeys and overlay, XTest auto-submit |
| macOS | Supported | CGEventTap hotkeys, CoreAudio recording, NSPasteboard clipboard, AppKit NSPanel overlay; requires Accessibility and Microphone permissions |
| Windows 10/11 | Supported | Global key polling plus a low-level keyboard-hook edge fallback, WinMM recording, Unicode clipboard, SendInput auto-submit, and a topmost Win32 overlay |

## Installation

### Homebrew (macOS / Linux)

```bash
brew install wakaka6/tap/just-talk
```

Tap: [wakaka6/homebrew-tap](https://github.com/wakaka6/homebrew-tap). The formula covers macOS (Apple Silicon / Intel) and Linux (amd64 / arm64).

### Prebuilt binaries

<details>
<summary>Per-platform archive download and checksum verification</summary>

Download an archive for your platform from [GitHub Releases](https://github.com/wakaka6/just-talk-go/releases), extract it, and put `just-talk` on your `PATH` (for example `~/.local/bin`). Windows ships as `.zip`; other platforms as `.tar.gz`. Verify downloads with the included `SHA256SUMS.txt`.

Available archives: Linux amd64 / arm64, macOS Intel / Apple Silicon, Windows amd64 / arm64.

</details>

### Build from source

<details>
<summary>Build dependencies and install steps</summary>

Just Talk relies on native platform APIs. Linux and macOS builds require cgo; Windows calls Win32 directly from Go and does not need cgo.

Linux build dependencies:

```bash
# Arch Linux
sudo pacman -S --needed go gcc libx11 libxtst libxext wayland

# Debian / Ubuntu
sudo apt install golang-go build-essential libx11-dev libxtst-dev libxext-dev libxinerama-dev libwayland-dev
```

macOS needs Apple Command Line Tools (clang and the macOS SDK; full Xcode is not required):

```bash
xcode-select --install
```

Windows only needs Go 1.25 or later — no ffmpeg, SoX, or C compiler:

```powershell
winget install --id GoLang.Go --exact
```

Build:

```bash
CGO_ENABLED=1 go build -o build/just-talk ./cmd/just-talk
# Windows PowerShell:
# go build -o build\just-talk.exe .\cmd\just-talk
```

Install for the current user (`~/.local/bin` on Linux/macOS, `%LOCALAPPDATA%\Programs\Just Talk` on Windows):

```bash
build/just-talk --install
# or
make install
```

If the install directory is not on your `PATH`, follow the hint printed by the command.

</details>

## Quick Start

1. Obtain Doubao large-model ASR credentials (App Key / Access Key).
2. Run `just-talk --doctor` to check the microphone, clipboard tools, and permissions.
3. Run `just-talk` to open the TUI, fill in the App Key and Access Key fields, and press `s` to save.
4. Press the `push_to_talk` hotkey (default `Alt+Super`; `Option+Command` on macOS) to start/stop recording. Recognized text is copied to the clipboard and, when `auto_submit` is on, submitted into the focused field.

### Permissions

<details>
<summary>Per-platform permission requirements</summary>

- **Linux Wayland**: global hotkeys read `/dev/input/event*` through evdev. Add your user to the `input` group and log back in:

  ```bash
  sudo usermod -aG input $USER
  ```

  Clipboard needs `wl-clipboard`; auto-submit needs `wtype` or write access to `/dev/uinput` (uinput is recommended on KDE Plasma — `--doctor` prints the full setup steps).

- **macOS**: grant **Accessibility** and **Microphone** permissions to the terminal app that launches Just Talk (Terminal, iTerm2, etc.) under System Settings → Privacy & Security. No `.app` signing or full Xcode is needed.
- **Windows**: no administrator rights required. If the microphone is unavailable, allow desktop apps under Settings → Privacy & security → Microphone.

</details>

## Hotkeys

### Voice hotkeys

| Key | Action |
| --- | --- |
| Configured `push_to_talk` | Start/stop recording in `toggle` mode; hold to record in `hold` mode |
| `Esc` | Cancel the current recording or dismiss an error |
| `R` | Retry recognition after a retryable error |

Voice hotkeys only accept keys suitable for global shortcuts:

- Modifier-only combos: `Alt+Super`, `Ctrl+Alt+Shift`, `Option+Command`, etc.
- `Fn` (macOS / Linux): also accepted as `Function`; can combine with ordinary keys but not with `F1`–`F24`. On Linux, `Fn` is read through evdev; when no backend is specified, a hotkey using `Fn` automatically selects the Wayland/evdev backend.
- Function keys `F1`–`F24`, such as `F9` or `Alt+F8`.
- Non-text control and navigation keys: `Tab`, `Enter`, `Escape`, `Backspace`, `CapsLock`, arrow keys, `Home`, `End`, `PageUp`, `PageDown`, `Insert`, `Delete`.
- Not supported: `Fn+F1`–`Fn+F24`, and letters, digits, punctuation, `Space`, numpad keys, or other text-producing keys without `Fn` (e.g. `Alt+G`, `G`, `Alt+1`, `Alt+Space`).

On Windows, the low-level keyboard hook only observes key edges and never consumes or replays modifiers, so standalone `Alt`, `Super`, and system shortcuts such as `Alt+Tab` keep their normal behavior.

### TUI keys

| Key | Action |
| --- | --- |
| `j` / `k` / arrows | Move selection |
| `e` / `i` / `Enter` | Edit the selected item (text input for strings; `j`/`k` cycles options; `Space` toggles booleans) |
| `Enter` | Leave edit mode and save; `Esc` only leaves edit mode |
| `s` | Save all settings (applies immediately, no restart) |
| `l` | Expand/collapse the log area |
| `h` | Toggle help |
| `q` / `Ctrl+C` | Quit |

After changing the App Key or Access Key in the TUI, press `s` to save — the next recording uses the new credentials. An in-progress session keeps the credentials used to establish its connection.

## Configuration

<details>
<summary>Config file paths, recommended config, and macOS notch overlay behavior</summary>

Default config path:

```text
# Linux / macOS
~/.config/just-talk/config.toml

# Windows
%APPDATA%\just-talk\config.toml
```

`./config.toml` in the working directory and `$XDG_CONFIG_HOME/just-talk/config.toml` take precedence when present.

Recommended config:

```toml
[voice]
enabled = true
mode = "toggle"                    # toggle | hold
push_to_talk = "Alt+Super"
language = "zh-CN"
auto_submit = true                 # auto-submit recognized text
stop_delay_ms = 0                  # release delay in hold mode
device = ""                        # recording device (empty = default)
gain = 0                           # microphone gain
app_key = "your-app-key"
access_key = "your-access-key"
resource_id = "volc.bigasr.sauc.duration"
hotwords = ["Wayland", "Sway", "wl-copy", "wtype", "just-talk-go"]

[overlay]
enabled = true
# Defaults to "notch" on macOS and "bottom-center" elsewhere.
# Values: notch, top-left, top-center, top-right, bottom-left, bottom-center, bottom-right
position = "notch"
show_text = true                   # show live/final recognized text in the notch
idle_visible = false               # keep the indicator visible when idle
scale = 1.0

[debug]
enabled = false
hotkeys = []                       # extra hotkeys registered by the debug plugin
```

### macOS notch overlay

Real-device preview of the macOS notch overlay (overlay area only, no desktop content):

![macOS notch overlay on real hardware: live recognized text, timer, and mic level while recording](docs/assets/notch-live.gif)

![macOS notch overlay on real hardware: copied indicator after recognition finishes](docs/assets/notch-live-final.png)

- With `position = "notch"`, the indicator expands
- Notch text is driven by recognition results: each new partial result refreshes immediately, replacing the old text whole with a roughly 100 ms fade — no typewriter effect. This only removes overlay-side waiting; ASR latency is unchanged.
- Screen selection: the overlay picks the screen under the mouse pointer on the first display of each recording and sticks to it for that recording. If the display is disconnected it re-picks by pointer position, falling back to the main screen. On a screen without a notch (external displays, older Macs) the status capsule appears at top-center instead, without text.
- Also adjustable in the TUI: select the overlay position field, press `e` to edit, `j`/`k` to cycle, `Enter` to save — it takes effect immediately.
- `[overlay]` changes apply without restarting: `enabled` toggles the overlay instantly, `show_text`/`idle_visible` apply on the next refresh, and `position`/`scale` rebuild the overlay window.
- Existing configs that set `position` explicitly are not migrated; set it to `"notch"` in the TUI or by hand. On Linux/Windows `"notch"` behaves as `top-center`, and other positions keep the original capsule with no text display.

</details>

## CLI

<details>
<summary>Command-line flags</summary>

```text
just-talk [flags]

-tui                 Run with the TUI (default)
-no-tui              Daemon mode; logs to stderr
-backend <name>      Force a backend: x11 | wayland | darwin (or JUST_TALK_BACKEND)
-config <path>       Use a specific config file
-doctor              Run startup environment checks and exit
-install             Install the executable for the current user
-debug               Enable the debug plugin (with [debug] config)
-verbose             Verbose logging
```

</details>

## Development

<details>
<summary>Build, test, and debug commands</summary>

```bash
make build          # Build for the current platform
make run            # Build and run
go test ./...       # Run all tests
go test ./... -tags no_x11    # Test without X11
goreleaser check    # Validate .goreleaser.yaml
```

Optional Windows integration tests (need a real device/hook environment):

```bash
JUST_TALK_TEST_WINDOWS_AUDIO=1 go test ./plugins/voice -run TestWindowsRecorderIntegration -v
JUST_TALK_TEST_WINDOWS_HOTKEY=1 go test ./hotkey -run TestWindowsHookIntegration -v
```

In TUI mode, normal logs go to the log file and the in-app log area so they do not corrupt the Bubble Tea layout; debug details are only visible with `--debug`.

</details>

## Release Process

<details>
<summary>Release pipeline and Homebrew auto-update</summary>

Releases are driven by `.goreleaser.yaml` and `.github/workflows/release.yml` using GoReleaser v2 with the official `goreleaser/goreleaser-action`. Pushing a `v*` tag builds Linux, macOS, and Windows binaries natively on matching GitHub-hosted runners (cgo on Linux/macOS), then a publish job merges the archives and attaches them plus `SHA256SUMS.txt` to a GitHub Release:

```bash
git tag v0.4.0
git push origin v0.4.0
```

### Homebrew auto-update (optional)

Building and publishing the GitHub Release does not depend on the Homebrew tap; the two are independent. To have the release workflow also update the formula in [wakaka6/homebrew-tap](https://github.com/wakaka6/homebrew-tap) (covering macOS and Linux, arm64/amd64), configure the following on the repository:

1. Create (or reuse) the tap repository `wakaka6/homebrew-tap` with a `Formula/` directory.
2. In `wakaka6/just-talk-go`, under Settings → Secrets and variables → Actions, create the secret `HOMEBREW_TAP_TOKEN`: a token with `contents: write` permission on the tap repository (a fine-grained PAT scoped to that repo is enough).
3. Optionally create the repository variable `HOMEBREW_TAP_REPOSITORY` set to `<owner>/<repo>`; when omitted it defaults to `wakaka6/homebrew-tap`.

If `HOMEBREW_TAP_TOKEN` is not configured, the workflow skips the Homebrew update with a warning and the GitHub Release is still published normally.

</details>

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

## Maintenance and Contributing

This repository is an independent fork of [whoamihappyhacking/just-talk-go](https://github.com/whoamihappyhacking/just-talk-go). The upstream project is still maintained by its original author; this repository evolves independently with a goal of community-driven development, and all contributors are welcome.

[Issues](https://github.com/wakaka6/just-talk-go/issues) are welcome for bug reports, usage feedback, and feature discussion — and pull requests are welcome too. See [CONTRIBUTING.md](CONTRIBUTING.md) for the workflow and requirements: open an issue first to discuss larger changes, and run `gofmt`, `go vet`, and `go test ./...` before submitting.

## License

Just Talk is licensed under the [GNU General Public License v3.0](LICENSE) (GPL-3.0-only).
