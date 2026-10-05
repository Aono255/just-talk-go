# Changelog

All notable project changes are tracked here.

## Unreleased

- Add multi-engine ASR support behind a pluggable engine registry. `[voice] engine` selects `doubao-stream` (default), `dashscope` (Qwen Fun-ASR realtime streaming), or batch engines `openai`, `auralwise`, `mimo-asr`; per-engine credentials live under `[voice.engine_configs.<engine>]` and are kept separately. Streaming engines keep live captions; batch engines produce only the final result. Existing Doubao credentials keep working unchanged.
- TUI gains an engine picker and per-engine config modal: select 引擎 → Enter to choose a channel, then configure its keys in the follow-up modal; `o` opens the engine's documentation.
- Add a macOS notch overlay. It is the new macOS default (`[overlay] position = "notch"`) and shows live recognized text, mic level, and a short pasted/copied result after each finished session. Cancelled or empty sessions show no result. The overlay follows the screen under the mouse pointer, picked at the start of each recording and kept for that recording, so no extra permission is needed. Screens without a notch show a top-center status capsule instead. Existing configs that set `position` explicitly keep it until changed in the TUI or by hand. Linux, Windows, and other macOS positions keep the original status capsule.
- Add `[overlay] show_text` (default `true`) to hide recognized text in the notch overlay.
- Refresh the macOS notch text as soon as a new partial ASR result arrives instead of on the next polling tick. New text replaces the old with a roughly 100 ms fade and no typewriter delay. ASR service latency itself is unchanged.
- Add an overlay position option (`提示位置`) to the TUI. It writes `[overlay] position` (`notch`, `top-left`, `top-center`, `top-right`, `bottom-left`, `bottom-center`, `bottom-right`) when saved. Saving other TUI settings keeps an explicitly configured position.
- Apply `[overlay]` config changes without restarting: `enabled` starts or stops the overlay, `show_text` and `idle_visible` apply on the next refresh, and `position` or `scale` changes replace the overlay window, which then shows the current recording state.
- Add macOS and Linux `Fn`/`Function` hotkey support. `Fn` can combine with ordinary keys, while `Fn+F1` through `Fn+F24` remain unsupported because macOS may treat them as system controls.
- Fix TUI configuration reloads retaining old ASR credentials: after saving with `s`, the next recording uses the updated App Key and Access Key without restarting.
- Add a responsive Chinese project introduction website with a simulated recording and R-triggered retry demo, platform-specific launch commands, static HTTP preview on port 7788, and automated GitHub Pages deployment linked from both READMEs.

## v0.0.3 - 2026-07-29

- Reworked Windows modifier-only hotkeys so the low-level hook only observes key edges and never consumes or replays modifiers. `Alt+Super` works through exact physical-state matching, stale hook state is cleared without combining separate single-key presses, and system shortcuts such as `Alt+Tab` remain untouched.

## v0.0.2 - 2026-07-21

- Fixed intermittent Windows `Alt+Super` false activations where stale low-level hook state could make a later Alt-only press look like the full shortcut; Windows modifier combinations now require an exact modifier set and reset stale suppressed state after physical release.

## v0.0.1 - 2026-07-17

- Added a tag-triggered GoReleaser v2 pipeline using the official GitHub Action to build Linux, macOS, and Windows amd64/arm64 archives on native runners and publish SHA256 checksums.
- Added Windows 10/11 support with global key-state monitoring, native WinMM microphone capture, Unicode clipboard access, SendInput auto-submit, a click-through Win32 status overlay, and Windows environment checks.
- Added Windows-standard config, state, log, and per-user installation paths. Windows builds no longer require external recording or clipboard commands.
- Added an opt-in Windows microphone integration test using `JUST_TALK_TEST_WINDOWS_AUDIO=1`.
- Added a low-level Windows keyboard-hook fallback so modifier-only shortcuts such as `Alt+Super` still work when `GetAsyncKeyState` misses the physical Windows key.
- Consume Windows modifier-only voice shortcuts such as `Alt+Super` so they do not activate menus or toolbars in the focused application, while replaying unrelated modifier shortcuts normally.
- Fixed recording shutdown getting stuck while ASR audio writes were still in flight, and accept empty final ASR responses without waiting for a false timeout.
- Redesigned the Windows status overlay with per-pixel transparency, antialiased animated waveform bars, smoother capsule edges, and a soft shadow.
- Clarified README build and install setup steps for the repository directory and `~/.local/bin` PATH.
- Restricted voice hotkeys to non-text global shortcut keys, rejecting letters, digits, punctuation, Space, and similar text-producing keys.
- Avoid duplicate auto-submit on KDE Plasma by using uinput directly and not writing the Wayland primary selection there.
- TUI is now the default startup mode.
- Added persistent usage statistics for total sessions, recognized characters, average speed, and recent speed.
- Added configurable ASR hotwords.
- Added TUI help toggle with `h`.
- Improved Wayland clipboard and auto-submit behavior with `wl-copy` and `wtype`.
- Added Linux recording status overlay for X11 and Wayland.
- Added macOS support for global hotkeys, native recording, clipboard, auto-submit, recording status overlay, and environment checks.
- Removed non-cgo macOS fallback builds; Just Talk now requires cgo for native platform integration.
- Replaced the old Claude-specific agent guide with `AGENTS.md` and clarified build documentation.
- Improved toggle and hold hotkey behavior for fast repeated key presses.
- Show ASR connection and final-result timeout errors in the status UI/overlay instead of immediately falling back to idle.
- Added transient `Esc` cancel and `R` retry hotkeys while recording or showing retryable errors.
- Improved X11 overlay placement on multi-monitor setups and switched X11 rendering to an ARGB window for smoother rounded corners.
- Fixed a Wayland overlay shutdown race that could crash while closing the app, and surfaced Linux `arecord` microphone/device failures in the UI.
- Made `Esc` cancel active overlay states, including the final ASR wait state, and suppress output from canceled pending sessions.
- Improved Wayland overlay rounded-corner antialiasing, especially on KDE Plasma.
- Added `just-talk --install` and `make install` to install the binary into `~/.local/bin`.

## 2026-05-30

- Initial Linux-focused development snapshot.
- Supported Linux Wayland hotkeys via evdev.
- Supported Linux X11 hotkeys via native X11 grabs.
- Added Doubao streaming ASR integration.
- Added TUI configuration interface.
- Added automatic clipboard copy and auto-submit.
