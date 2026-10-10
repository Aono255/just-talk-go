# Just Talk

After editing the App Key or Access Key in the TUI, leave edit mode and press `s` to save. The next recording uses the new credentials without restarting. An ongoing recognition session keeps the credentials used to establish its connection.

[中文](README.md) · [Website](https://whoamihappyhacking.github.io/just-talk-go/)

Just Talk is a desktop voice input tool. It records audio with a global hotkey, sends it to streaming ASR, and then copies the recognized text to the clipboard or submits it directly into the focused input field.

It is built for people who want to type less and speak more while coding, chatting, writing notes, or working with long text.

This [fork maintained by Aono255](https://github.com/Aono255/just-talk-go) is based on [the original project's master](https://github.com/whoamihappyhacking/just-talk-go/tree/master). Its maintenance branch is `master`, with independent releases and Homebrew updates.

## Screenshot

![Just Talk TUI](docs/screenshot-tui.png)

## Features

- Global hotkey recording with `toggle` and `hold` modes.
- Voice hotkeys are limited to keys suitable for global shortcuts: modifiers, function keys, Tab, CapsLock, arrow/navigation keys, and similar non-text keys. Letters, digits, punctuation, Space, and other text-producing keys are rejected.
- Doubao streaming ASR with optimized bidirectional streaming and second-pass recognition.
- Clipboard copy and automatic text submission.
- Always-on-top recording status overlay for Wayland, X11, macOS, and Windows.
- The macOS overlay uses Chinese status labels, an audio-level waveform and an AI correction animation, keeping the final result visible for 3 seconds. Recording, recognition, correction and completion share the same expanded 448×104-point size, adjusted by the configured scale. The AI stage does not enlarge the overlay. Long transcripts retain the latest text that fits in a fixed two-line preview; the complete result is pasted normally. The `notch` position sits at the top center below the menu bar, including on external displays without a notch.
- TUI configuration for hotkeys, mode, auto-submit, stop delay, hotwords, and related settings.
- ASR hotwords for project names, people names, English terms, and domain-specific vocabulary.
- Usage statistics for total sessions, total recognized characters, average speed, and recent speed.

## Platform Status

Linux, macOS, and Windows desktops are supported:

| Platform | Status | Notes |
| --- | --- | --- |
| Linux Wayland | Supported | Works with Sway / wlroots; hotkeys use evdev and require input permissions |
| Linux X11 | Supported | Uses native X11 global hotkeys |
| macOS | Supported | Global hotkeys use CGEventTap, recording uses AVFoundation, clipboard uses NSPasteboard, and overlay uses AppKit NSPanel |
| Windows 10/11 | Supported | Global key polling with a low-level keyboard-hook edge fallback, WinMM recording, Unicode clipboard, SendInput auto-submit, and a Win32 status overlay |

## macOS: Homebrew Installation And Updates

Prebuilt releases are available for Apple Silicon and Intel on macOS 15 or later:

```bash
brew tap Aono255/just-talk https://github.com/Aono255/just-talk-go
brew install Aono255/just-talk/just-talk
just-talk --version
```

If `wakaka6/tap/just-talk` is already installed, stop its running process with `Ctrl+C`, run `brew uninstall wakaka6/tap/just-talk`, and then install the formula above. Do not use `--zap`; the user configuration at `~/.config/just-talk/config.toml` is preserved. Run only one Just Talk process at a time.

For subsequent updates:

```bash
brew update
brew upgrade Aono255/just-talk/just-talk
```

Restart Just Talk after upgrading. `brew update` refreshes the formula and `brew upgrade` installs the new release; an already running process keeps using the old binary.

This fork re-enables the macOS hotkey tap after the system disables it due to a timeout, while respecting user-requested disabling. A native callback regression test covers this path; long-running background hotkey behavior still requires manual trial.

v0.1.3 opens the microphone in a background worker and immediately shows an opening status. Key release and cancellation remain responsive; an abandoned start cannot begin recognition later or create duplicate capture sessions. On macOS, a scoped system activity identifies user-initiated recording and latency-sensitive I/O, and ends when the microphone is released.

macOS uses AVFoundation with the currently selected default microphone and outputs 16 kHz, 16-bit mono PCM. Recording becomes active only after the first audio buffer arrives. A running session reports an error if no buffer arrives within 3 seconds; this timeout does not bound the system startup call itself. System input/output defaults and the microphone's hardware sample rate are unchanged. Unavailable devices fail explicitly without selecting another microphone. On macOS, `voice.device` accepts only an empty value or `default`.

If a Bluetooth microphone remains at `CON`, check other applications' system-audio capture features. During device testing on macOS 26.5.1, both AudioQueue and AVFoundation stalled in the Core Audio device-start wait while microphone authorization remained granted. After disabling Codex Settings → General → Toys → Audio visualizer and reopening JustTalk, repeated recording, recognition, and auto-paste succeeded; startup to the first audio buffer took about 162–453 ms. This supports investigating interactions between system-audio capture and Bluetooth device reconfiguration. Changing the recording API alone does not guarantee removal of such system waits. Microphone permissions need not be changed, and JustTalk must not automatically stop another application's capture.

Captured audio waits in memory without blocking a full pipe while recognition connects. More than 1 MiB of unread PCM (about 32 seconds) fails explicitly. Stop preserves captured tail audio, wakes blocked readers, and releases native resources after reading has stopped. `--verbose` reports device lookup, configuration, startup, and first-buffer timings separately. Native regression tests cover format, buffering, and stop lifecycle; other headset and OS combinations require their own validation.

## Build

Just Talk uses native platform APIs. Linux and macOS builds require cgo; Windows uses direct Win32 calls from Go and does not require cgo.

Linux build dependencies:

```bash
# Arch Linux
sudo pacman -S --needed go gcc libx11 libxtst libxext wayland

# Debian / Ubuntu
sudo apt install golang-go build-essential libx11-dev libxtst-dev libxext-dev libxinerama-dev libwayland-dev
```

macOS build dependencies:

```bash
# Apple Command Line Tools provide clang and the macOS SDK. Full Xcode is not required.
xcode-select --install
```

Windows build dependency:

```powershell
# Install Go 1.25 or later. No ffmpeg, SoX, or C compiler is required.
winget install --id GoLang.Go --exact
```

Build for the current platform:

```bash
cd just-talk-go
CGO_ENABLED=1 go build -o build/just-talk ./cmd/just-talk
```

Windows PowerShell:

```powershell
cd just-talk-go
go build -o build\just-talk.exe .\cmd\just-talk
```

Install to `~/.local/bin/just-talk`:

```bash
# Make sure ~/.local/bin is in PATH. If not, add this line to ~/.bashrc or ~/.zshrc.
# export PATH="$HOME/.local/bin:$PATH"
build/just-talk --install
# or
make install
```

macOS must be built on macOS. The project does not provide a non-cgo build.

Install on Windows to `%LOCALAPPDATA%\Programs\Just Talk\just-talk.exe`:

```powershell
.\build\just-talk.exe --install
# If the directory is not already in PATH, follow the note printed by the command.
```

## Release Downloads

[This fork's GitHub Releases](https://github.com/Aono255/just-talk-go/releases) provide prebuilt archives for:

- Linux amd64 / arm64
- macOS Intel / Apple Silicon
- Windows amd64 / arm64
- `SHA256SUMS.txt` checksum verification

The release workflow uses GoReleaser v2 with the official `goreleaser/goreleaser-action`. Linux, macOS, and Windows binaries are built natively on matching GitHub-hosted runners. Maintainers can build and publish a release by pushing a `v*` tag, for example:

```bash
git tag v0.1.3
git push origin master v0.1.3
```

Use stable `vMAJOR.MINOR.PATCH` tags. After all native builds and tests succeed, the workflow publishes archives and checksums, then commits the updated `Formula/just-talk.rb` to this repository using the built-in GitHub token. No extra PAT is required. The formula currently supports macOS; Linux and Windows users can use release archives or build from source.

After publishing, clean Apple Silicon and Intel macOS runners execute real `brew tap`, `brew install`, and `brew test` commands. The Homebrew installation validation workflow can also be dispatched manually for the current formula.

To follow upstream, point the `upstream` remote at `https://github.com/whoamihappyhacking/just-talk-go.git`, then run `git fetch upstream master` and `git merge upstream/master`. Validate the result before pushing and releasing a new tag. Do not force-sync over this fork's fixes.

## Usage

Start the TUI:

```bash
just-talk
```

Run without the TUI:

```bash
just-talk --no-tui
```

Force a backend:

```bash
just-talk --backend wayland
just-talk --backend x11
```

Windows selects its native backend automatically. Check the microphone and configuration before first use:

```powershell
.\build\just-talk.exe --doctor
```

## Configuration

Default config path:

```text
# Linux / macOS
~/.config/just-talk/config.toml

# Windows
%APPDATA%\just-talk\config.toml
```

Recommended hotkey config:

```toml
[voice]
mode = "toggle"
push_to_talk = "Alt+Super"
```

`Alt+Super` with `toggle` mode is recommended. Press once to start recording, then press again to stop. This avoids hold-mode key conflicts with desktop environments or focused input fields. On Windows, the low-level keyboard hook only observes key edges and never consumes or replays modifiers, so standalone `Alt`, `Super`, and shortcuts such as `Alt+Tab` retain their normal system behavior. Modifier combinations require the exact configured set, and hook fallback state is checked against the unsuppressed physical key state so separate single-key presses cannot be combined into the shortcut.

Voice hotkeys only support keys suitable for global shortcuts:

- Supported: modifier-only combinations, such as `Alt+Super` and `Ctrl+Alt+Shift`.
- Supported: function keys `F1` through `F24`, such as `F9` and `Alt+F8`.
- Supported: non-text control and navigation keys, such as `Tab`, `Enter`, `Escape`, `Backspace`, `CapsLock`, `Up`, `Down`, `Left`, `Right`, `Home`, `End`, `PageUp`, `PageDown`, `Insert`, and `Delete`.
- Not supported: letters, digits, punctuation, Space, numpad digits, and numpad symbols that can enter text, such as `Alt+G`, `G`, `Alt+1`, and `Alt+Space`.

Hotword example:

```toml
[voice]
hotwords = ["Wayland", "Sway", "wl-copy", "wtype", "just-talk-go"]
```

macOS hotkey example:

```toml
[voice]
# Option is Alt; Command/Cmd is Super.
push_to_talk = "Option+Command"
```

On Windows, `Win` and `Super` both refer to the Windows logo key. If recording is unavailable, allow desktop applications to access the microphone under Windows Settings > Privacy & security > Microphone.


## Codex context correction (macOS)

The optional, disabled-by-default correction step reuses the recording hotkey: finish ASR → read the current Codex conversation → correct/organize with the configured model → paste. The macOS overlay displays “AI 整理中” and a context-correction badge with a blue-purple animation inside the container. Completion displays the corrected result and highlights the visible changed region. Other applications keep their ordinary voice-input behavior.

The macOS waveform uses the existing recording stream and does not open another microphone or system-audio capture. Animations run in the separate AppKit helper, outside hotkey callbacks. The overlay does not take focus or receive mouse input, stops its timer when hidden, and respects the system Reduce Motion setting. Preparation and idle use a small capsule; recording expands it, after which recognition, correction and completion retain the same dimensions and text position.

Use `j/k` in the TUI to reach the correction settings, enter the provider Base URL, model name and API key, enable correction, then press `s`. The list scrolls with the cursor. The API key is masked both when viewing and editing. Providers must support OpenAI-compatible `POST /chat/completions`, Bearer authentication and `choices[].message.content`. The defaults prefill DeepSeek’s Base URL and `deepseek-flash`, explicitly disable thinking and enable JSON output; the API key remains empty. Select `openai-compatible` for another compatible service, and supply its own URL/model. No model switching occurs. See the [DeepSeek API reference](https://api-docs.deepseek.com/api/create-chat-completion/).

“AI 纠错术语” provides an independent glossary of project names, technical terms and abbreviations, separated by English or Chinese commas in the TUI. It is sent only to the correction model, while the existing ASR hotwords also remain available as supplementary correction vocabulary. The glossary defaults to empty and is independent of the ASR hotword budget; it still consumes model input and can affect latency and cost. Saved changes apply to the next recording; recordings already started keep their captured glossary.

```toml
[correction]
enabled = false
provider = "deepseek"                    # Also supports openai-compatible
base_url = "https://api.deepseek.com"
model = "deepseek-flash"
api_key = ""                           # Enter locally
timeout_ms = 8000
terms = ["ClawOps", "ReadModel", "两高一弱"] # Independent AI correction glossary
```

Context comes only from the conversation main region containing the focused Codex draft. The latest six loaded user messages and three loaded assistant replies are selected by their accessible role headings. Each user message is limited to 1,200 characters and each assistant reply to 2,800, with a total limit of 15,600. Long messages retain their beginning and end with an omission marker; the original conversation order is preserved. Only these messages, the current ASR transcript and configured hotwords are sent. The TUI shows actual sent message and character counts; logs also report the loaded message counts without recording context text. Missing context fails explicitly; unrelated recently active chats are never substituted. History not loaded into the virtualized view is unavailable. Grant Accessibility to the terminal that launches JustTalk.

When DeepSeek is enabled, ASR requests append `DeepSeek` to the configured hotwords without rewriting preferences. Normal assistant streaming does not count as a chat switch; input changes, a changed user-message anchor or a changed draft prevent paste.

Correction preserves intent while fixing transcription errors, technical terms, grammar, punctuation and paragraphs, and removing meaningless hesitation/repetition. Changes to numbers, code, URLs or attachment markers are rejected. Service errors, timeouts and changes to the chat/draft preserve the raw transcript in the clipboard, show an error and prevent automatic paste. Escape or a new recording cancels an in-flight model request. Config changes apply on the next recording. Model requests run in background finishing work, not recording/hotkey callbacks.

Run `just-talk --check-codex-context`, then focus the Codex draft within five seconds. It reports message counts, input binding and timing without recording, contacting models or saving message text. Real conversation access and model latency still require validation against the specific Codex version/provider/model.

Config files are saved with mode `0600` on macOS/Linux. Keys remain in local configuration and are not logged. Launching with `--config /path/trial.toml` makes the TUI save to that exact file. See the [Chat Completions API documentation](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create) for the protocol.

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

## Maintenance And Contributions

The original Just Talk project was created and is maintained by `whoamihappyhacking`; this fork is maintained by `Aono255`. Upstream does not accept pull requests. Report issues for this fork in [this repository's Issues](https://github.com/Aono255/just-talk-go/issues).

## License

Just Talk is licensed under the GNU General Public License v3.0.

## Project website

Visit the [original Just Talk website](https://whoamihappyhacking.github.io/just-talk-go/). This fork retains the website source; automatic Pages deployment is enabled only in the original repository.

The static introduction in `website/` includes features, platform support, quick-start instructions, and a simulated demo that does not use the microphone. Preview it with:

```bash
python3 -m http.server 7788 --bind 0.0.0.0 --directory website
```
