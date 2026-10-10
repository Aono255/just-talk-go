# Just Talk

[中文](README.md) · [Downloads and releases](https://github.com/Aono255/just-talk-go/releases) · [Report an issue](https://github.com/Aono255/just-talk-go/issues)

Just Talk is a desktop voice input tool operated with a global hotkey. It records speech, then copies the transcript or pastes it into the focused input. On macOS, it can also use DeepSeek or another compatible model to correct and organize the transcript with the current Codex conversation.

This [fork maintained by Aono255](https://github.com/Aono255/just-talk-go) is based on [whoamihappyhacking/just-talk-go](https://github.com/whoamihappyhacking/just-talk-go/tree/master). Its maintenance branch is `master`, with independent releases and Homebrew updates.

[Quick start](#quick-start) · [Codex correction](#codex-context-correction-macos) · [Vocabulary](#choosing-between-three-vocabulary-settings) · [Overlay settings](#macos-overlay) · [Configuration and commands](#configuration-files-and-commands) · [Troubleshooting](#troubleshooting)

## Current features

| Feature | Behavior and scope |
| --- | --- |
| Global voice input | `toggle`: press to start, press again to stop; `hold`: hold to record and release to stop |
| Streaming recognition and paste | Volcengine Doubao ASR; automatic paste or clipboard-only output. Automatic paste does not press Enter to send a message |
| Codex context correction | **macOS only, disabled by default**. Reads the current conversation after recognition, then corrects with DeepSeek or an OpenAI-compatible service |
| Independent AI glossary | Project names, technical terms and abbreviations for the correction model, maintained separately from recognition hotwords |
| Local and cloud recognition hotwords | Inline hotwords or a Volcengine self-learning hotword table ID; cloud tables must be bound in the client |
| Chinese recording overlay | On macOS: audio level, transcript preview, AI animation, completion result and visible change highlighting |
| TUI settings and statistics | Scrolling configuration, help and logs, save/reload, session count, total characters and average/recent speed |
| Cancel and record again | Escape cancels the current flow; `R` starts a new recording while an error is displayed |
| Installation and diagnostics | macOS Homebrew, release archives for three platforms, environment checks and a Codex context check |

With “Codex 纠错” enabled, **other applications such as WeChat still use ordinary voice input**, without reading Codex conversations or calling the correction model. The switch can stay enabled. Linux and Windows support basic voice input; Codex context correction is currently unavailable there.

Native macOS overlay examples, using demonstration text:

| AI correction in progress | Completion with visible changes highlighted |
| --- | --- |
| ![AI correction overlay](docs/overlay-ai-correction.png) | ![Completed correction overlay](docs/overlay-completed.png) |

## Quick start

### macOS: install and update

Homebrew prebuilt releases require **macOS 15 or later**, on Apple Silicon or Intel.

```bash
brew tap Aono255/just-talk https://github.com/Aono255/just-talk-go
brew install Aono255/just-talk/just-talk
just-talk --version
just-talk
```

Explicitly add the tap using the repository URL on the first installation: the formula lives in this repository under the tap name `Aono255/just-talk`.

Subsequent updates:

```bash
brew update
brew upgrade Aono255/just-talk/just-talk
just-talk --version
```

Exit the old process and start Just Talk again after upgrading. A running process does not switch binaries automatically. Run only one instance at a time.

If `wakaka6/tap/just-talk` is installed, stop it with `Ctrl+C` in its terminal, run `brew uninstall wakaka6/tap/just-talk`, and then install this fork. Normal uninstall preserves the user configuration; check your existing settings after switching.

### Make your first recording

1. Start `just-talk` in a terminal. On macOS, grant Accessibility and Microphone permissions to **the terminal application that launches it**, such as Terminal or iTerm2. Accessibility is under System Settings → Privacy & Security → Accessibility.
2. Enable the required speech recognition service for a Doubao Voice application in Volcengine and obtain its App ID and Access Token. In the TUI, put the App ID in **App Key** and the Access Token in **Access Key**. These are ASR credentials, configured separately from the correction model's API key.
3. Select a field with `j/k`, then edit with `e`. Enter finishes editing and saves; alternatively, Escape finishes editing and `s` saves afterward.
4. Return to the target application and focus its input. The default hotkey is `Alt+Super`: `⌥ Option + ⌘ Command` on macOS; Super means the Windows logo key on Windows.
5. In the default `toggle` mode, press once to start and again to stop. Enable “自动上屏” for automatic paste, or disable it to copy and paste manually.

Saved App Key, Access Key, hotwords and correction settings apply to the next recording. A recording already started keeps its credentials and vocabulary snapshot. A new binary or changed overlay settings requires a restart.

### Linux and Windows

Download the matching OS/architecture archive from [Releases](https://github.com/Aono255/just-talk-go/releases), extract it, and run `just-talk` or `just-talk.exe`. Archives cover amd64 and arm64 for Linux, macOS and Windows, with `SHA256SUMS.txt` for verification.

| Platform | Basic voice input dependencies and permissions |
| --- | --- |
| Linux Wayland | `alsa-utils` for recording, `wl-clipboard` for clipboard operations, and usually `wtype` for paste on Sway/wlroots. Hotkeys need readable `/dev/input/event*`; KDE Plasma paste requires attention to `/dev/uinput` write access |
| Linux X11 | `alsa-utils` and `xclip`; native X11 hotkeys and XTest paste |
| macOS | Native hotkeys, AVFoundation recording, clipboard and AppKit overlay; grant Accessibility and Microphone permissions to the launching terminal |
| Windows 10/11 | Native hotkeys, WinMM recording, Unicode clipboard, SendInput paste and status overlay; allow desktop applications to access the microphone in privacy settings |

Use `just-talk --doctor` to check Linux dependencies and permissions. Windows needs no additional ffmpeg, SoX or C compiler.

## Everyday controls

### TUI keys

| Key | Action |
| --- | --- |
| `j/k` or arrow keys | Select a setting; the list scrolls with the selection |
| `e`, `i` or Enter | Edit the selected setting |
| Space while editing a switch | Toggle on/off |
| `j/k` while editing a selection | Cycle recording mode, provider type and other options |
| Enter while editing | Finish editing and save |
| Escape while editing, then `s` | Finish editing, then save; Escape itself neither saves nor discards the edited field |
| `h` | Show/hide setting help |
| `l` | Expand/collapse logs |
| `q` or `Ctrl+C` | Quit from navigation mode |

During recording, recognition or correction, global Escape cancels the flow. During microphone startup it marks cancellation; resources are released after the system call returns. Errors remain visible for about 10 seconds. Global `R` during that period **starts a new recording**; it does not replay the last audio or retry the last model request.

### Hotkey, recording mode and stop delay

The defaults are `Alt+Super` and `toggle`. Use `hold` to speak while holding the shortcut. Stop delay is the number of milliseconds to keep recording after the stop action; it defaults to `0`.

Supported shortcuts include modifier-only combinations, `F1`–`F24`, Tab, Enter, Escape, Backspace, CapsLock, arrows and navigation keys. Examples: `Alt+Super`, `F9`, `Alt+F8`, `Ctrl+Alt+Tab`. Letters, digits, punctuation, Space and numpad text keys are rejected to avoid registering ordinary typing as a voice shortcut.

Option maps to Alt; Command/Cmd/Win map to Super. Windows modifier combinations require the exact configured modifier set. Its low-level hook only observes key edges and does not consume or replay modifiers, preserving normal Alt, Win and Alt+Tab behavior.

## Codex context correction (macOS)

### Configure DeepSeek or another model

Correction is disabled by default and reuses the recording hotkey when enabled:

```text
Codex: record → ASR → read current conversation → model correction → validation → paste or copy
Other applications: record → ASR → paste or copy
```

Fill these TUI settings, then enable “Codex 纠错” and save:

| TUI setting | DeepSeek default or purpose |
| --- | --- |
| 模型服务类型 | `deepseek`; select `openai-compatible` for another compatible service |
| 模型服务地址 | `https://api.deepseek.com`; other services use their own Base URL, possibly including `/v1`. Do not enter the complete `/chat/completions` URL |
| 纠错模型 | `deepseek-flash`, or the model name provided by your service |
| 模型 API Key | Enter the service's key locally; masked while viewing and editing |
| 纠错超时(ms) | `8000` by default; allowed range `500`–`30000` |
| AI 纠错术语 | English or Chinese comma separators, e.g. `Codex, DeepSeek, SOCKS5, WebShell, ClickHouse` |

DeepSeek mode explicitly disables thinking and enables JSON output; see the [official API reference](https://api-docs.deepseek.com/api/create-chat-completion/). Other services must support `POST /chat/completions`, Bearer authentication and `choices[].message.content`, returning the expected JSON text. Just Talk sends HTTP requests directly and reuses connections, without starting Codex CLI, automatically switching models, or retrying against another service.

The default 8 seconds is the model request's timeout, **not a fixed wait for every correction**. Context capture, ASR finishing and paste add their own time. Actual latency and quality depend on the service, network, transcript and available context. The TUI shows sent message/character counts and successful correction duration.

### Context limits

Only messages **loaded in the current Codex conversation main region containing the focused draft** are selected by accessible role headings:

| Content | Limit |
| --- | --- |
| User messages | Latest 6, up to 1,200 characters each |
| Assistant replies | Latest 3, up to 2,800 characters each |
| Total context | Up to 15,600 characters, preserving conversation order |

Long messages retain their beginning and end with an omission marker. Other conversations and repository files are not collected. History absent from the virtualized view is unavailable. The TUI shows actual sent counts; logs record loaded/sent counts without saving context text.

Focus the current conversation draft before recording. Normal assistant streaming may continue. Switching applications or conversations, a changed user-message anchor, or a changed draft prevents corrected paste. Missing context fails explicitly rather than borrowing another conversation or correcting without context.

Check context access:

```bash
just-talk --check-codex-context
```

Within five seconds, return to the current Codex conversation and focus the draft. The check reports loaded/selected message counts, character count, input binding and timing. It does not record audio, contact the model service or save message text.

### Correction scope and failure handling

The model fixes transcription errors, technical terms, grammar, punctuation and paragraphs, and removes meaningless hesitation/repetition while preserving intent and tone. Configured numbered terms can be restored, for example `SOCKS` → `SOCKS5`.

Existing numbers, identifiers, code, URLs and attachment markers remain checked. Changing `G01` to `M01`, changing a port, deleting or duplicating an existing identifier is rejected. Newly added single-backtick formatting around verifiable terms is removed; existing code formatting is preserved.

Service errors, timeouts, validation failures or target changes preserve the raw transcript in the clipboard, display a specific error and stop automatic paste. You can paste the raw text manually. Escape or a new recording cancels an in-flight model request. Misrecognition and insufficient context can still affect quality; a glossary does not guarantee correct recovery every time.

## Choosing between three vocabulary settings

| Setting | Pipeline stage | Input format |
| --- | --- | --- |
| 热词 / `voice.hotwords` | ASR and supplementary correction vocabulary | English/Chinese commas in the TUI; a string array in TOML |
| 火山热词表 ID / `voice.boosting_table_id` | Volcengine cloud ASR | Create a self-learning table, then enter its ID |
| AI 纠错术语 / `correction.terms` | Correction model only; never sent to ASR | Full project names, abbreviations and numbered standard terms; English/Chinese commas in the TUI |

TUI vocabulary editing removes empty entries and duplicates. Hotwords and AI terms are independent settings maintained separately. AI terms are not constrained by the recognition vocabulary budget, but consume model input and may affect latency and cost. DeepSeek mode appends `DeepSeek` to the in-memory local vocabulary without rewriting saved preferences; it is sent to ASR with inline hotwords only when no cloud ID is configured.

### Volcengine self-learning platform

1. In the Volcengine console, open Doubao Voice → Self-learning Platform → Hotword Management. Select the application matching Just Talk's App Key.
2. Add a hotword file, upload UTF-8 TXT or enter words, then copy the generated table ID.
3. Enter it in “火山热词表 ID” and save. The next recording uses the table. Creating a cloud table alone does not bind Just Talk to it.

Upload **one word per line**, rather than the comma-separated AI glossary. Example:

```text
Codex
DeepSeek
WebShell
```

The official format allows up to 5,000 words per table and fewer than 10 characters per word. Convert Arabic digits or special symbols as required by the format. Optional `word|weight` uses weights 1–10, default 4. Keep standard forms such as `SOCKS5` separately in the AI glossary. See [official hotword management instructions](https://www.volcengine.com/docs/6561/155739?lang=zh).

With a cloud ID configured, recognition sends only that ID, omitting the higher-priority inline hotwords. Clear the ID to restore inline hotwords. Local hotwords still supplement AI correction; cloud table contents are not automatically downloaded into the AI glossary.

## macOS overlay

The Chinese stages are “准备中” (preparing), “录音中” (recording), “收尾中” (finishing), “识别中” (recognizing), “AI 整理中” (correcting), and “已完成” (completed). Errors display “处理失败” with a reason.

- The waveform uses actual recording audio levels without opening another microphone or capturing system audio.
- AI correction shows a blue-purple animation and a context badge; completion highlights visible changes and keeps the result for about 3 seconds.
- Recording, recognition, AI correction and completion share a 448×104-point expanded size, adjusted by `scale`. The AI stage does not enlarge it again.
- Long transcripts keep the latest text that fits in a fixed two-line preview. The full text is still pasted or copied.
- The overlay does not take focus or receive mouse input, stops animation when hidden, and respects Reduce Motion.

Overlay settings currently require editing the configuration file and **restarting Just Talk**:

```toml
[overlay]
enabled = true
position = "notch"
idle_visible = false
scale = 1.0
```

The default position is `bottom-center`. Common positions are `top-left`, `top-center`, `top-right`, `bottom-left`, `bottom-center`, and `bottom-right`. macOS also supports `notch`, centered below the menu bar, including on external displays without a notch. `idle_visible = true` keeps an idle indicator; `scale` changes overall size.

## Configuration files and commands

The default save paths are `~/.config/just-talk/config.toml` on Linux/macOS and `%APPDATA%\just-talk\config.toml` on Windows.

Without `--config`, lookup uses the first existing file in this order: current-directory `config.toml`, the platform default, then `$XDG_CONFIG_HOME/just-talk/config.toml`. An explicit `--config` also controls where the TUI saves changes. Saved files use mode `0600` on macOS/Linux.

This example includes all current configuration fields. Credentials are empty and vocabulary is illustrative. Fill the Volcengine credentials before recognition and the model key before enabling correction:

```toml
[voice]
enabled = true
mode = "toggle"
push_to_talk = "Alt+Super"
app_key = ""                           # Volcengine App ID
access_key = ""                        # Volcengine Access Token
resource_id = "volc.bigasr.sauc.duration"
auto_submit = true
stop_delay_ms = 0
hotwords = ["Codex", "DeepSeek", "WebShell"]
boosting_table_id = ""                 # Empty: use inline hotwords
device = ""                            # macOS: empty/default only
gain = 1                               # Integer PCM gain; values below 1 act as 1
language = "zh-CN"                     # Retained field; see note below

[correction]
enabled = false
provider = "deepseek"                  # Or openai-compatible
base_url = "https://api.deepseek.com"
model = "deepseek-flash"
api_key = ""
timeout_ms = 8000
terms = ["Codex", "DeepSeek", "SOCKS5", "WebShell", "ClickHouse"]

[overlay]
enabled = true
position = "bottom-center"
idle_visible = false
scale = 1.0

[debug]
enabled = false
hotkeys = []
```

`voice.device` accepts an ALSA device name on Linux and default, a device index or a full device name on Windows. macOS uses the system-selected default input without changing input/output defaults or hardware sample rates. `language` is retained in configuration but is not currently sent to ASR, so it cannot force a recognition language.

| Command | Purpose |
| --- | --- |
| `just-talk` | Default TUI mode |
| `just-talk --no-tui` | Run without TUI using the same hotkey; Ctrl+C quits |
| `just-talk --version` | Print binary version and commit |
| `just-talk --config ./trial.toml` | Use the specified file and save TUI edits back to it |
| `just-talk --doctor` | Check platform dependencies and permissions; does not record or verify remote keys/recognition quality |
| `just-talk --check-codex-context` | macOS current Codex context and input binding check |
| `just-talk --verbose` | Detailed hotkey, microphone startup and finishing logs |
| `just-talk --debug` | TUI hotkey receipt, queue and handling diagnostics |
| `just-talk --backend wayland` / `--backend x11` | Force a Linux backend; also supports `JUST_TALK_BACKEND` |
| `just-talk --install` | Install this binary into Linux/macOS `~/.local/bin` or Windows `%LOCALAPPDATA%\Programs\Just Talk` |

Additional hotkey event logging without the TUI requires both `--debug` and `[debug].enabled = true`; `debug.hotkeys` selects the combinations to observe.

Logs are `/tmp/just-talk.log` on Linux/macOS and `%LOCALAPPDATA%\just-talk\just-talk.log` on Windows. Counts and typing speed are statistics, not replayable recording history. The TUI displays recognized text and non-TUI mode prints transcripts; review diagnostic output for text you need to hide before sharing it.

## Troubleshooting

### DeepSeek enabled, but WeChat asks for a Codex draft

Confirm `just-talk --version` is at least `v0.1.8`, exit the old process, and restart. The updated implementation checks live system input focus, allowing ordinary voice input in other applications. If the error occurs inside Codex, focus the current conversation draft before recording.

### Stuck at “准备中” or the older CON label

Check the default microphone, Bluetooth headset readiness, and other applications' system-audio capture. In an earlier headset investigation, repeated recording worked after disabling Codex Audio visualizer. In versions offering the setting, check Settings → General → Toys. This is a verified diagnostic lead, not a guarantee for every device.

macOS enters recording only after receiving its first audio buffer. It waits up to 3 seconds for that buffer after the startup call returns; **that timeout cannot interrupt a blocked system startup call**. Escape during startup marks cancellation and releases resources after return. Avoid starting multiple instances.

### Hotkey does not respond

Confirm voice input is enabled and saved. Check the launching terminal's Accessibility permission on macOS or input device access on Linux Wayland. Use `--debug` for hotkey receipt and `--verbose` to distinguish missing events from a microphone startup wait. This fork re-enables macOS hotkey taps disabled by system timeout, while respecting user-requested disabling.

### Model changed a number, code, URL or attachment

Result validation stopped the output; the raw transcript remains in the clipboard. Add the full standard name, such as `SOCKS5`, to the AI glossary. From `v0.1.7`, configured numbered terms can be restored and extra backticks around verifiable terms handled. Ordinary numbers, existing identifiers and code remain protected, with category-specific errors.

### Overlay truncates text or never shows AI correction

The overlay has a two-line preview; full text is still pasted or copied. AI correction appears only on macOS with correction enabled and Codex owning the input focus. Context failures show an error instead. Restart after changing overlay position, size or visibility settings.

### Where is my content sent

Audio goes to Volcengine ASR. Only the Codex correction flow sends the transcript, selected conversation context, local hotwords and AI terms to the configured model service. The model key stays in local configuration and is used for request authentication, without being written to logs. ASR and correction use separate credentials.

## Build and maintenance

Go 1.25 or later is required. Linux and macOS use native cgo; Windows uses direct Win32 calls. No non-cgo macOS build is provided.

```bash
git clone https://github.com/Aono255/just-talk-go.git
cd just-talk-go
```

Linux compilation libraries; runtime recording/clipboard dependencies are listed in the platform table above:

```bash
# Arch Linux
sudo pacman -S --needed gcc libx11 libxtst libxext libxinerama libxrender wayland

# Debian / Ubuntu
sudo apt install build-essential libx11-dev libxtst-dev libxext-dev libxinerama-dev libxrender-dev libwayland-dev
```

On macOS, install Apple Command Line Tools; full Xcode is unnecessary:

```bash
xcode-select --install
```

Build and install on Linux/macOS:

```bash
CGO_ENABLED=1 go build -o build/just-talk ./cmd/just-talk
build/just-talk --install
# Ensure ~/.local/bin is on PATH
```

Windows PowerShell, with Go 1.25+ and no C compiler:

```powershell
go build -o build\just-talk.exe .\cmd\just-talk
.\build\just-talk.exe --install
# Follow the command's note to add the install directory to PATH
```

You can direct `go build -o`, `GOCACHE` and `GOMODCACHE` to a chosen build disk. macOS must be built on macOS. Validate source with `go test ./...` and `go vet ./...`, and packaging with `goreleaser check`.

Pushing a stable `vMAJOR.MINOR.PATCH` tag runs native tests/builds for six platform/architecture combinations, publishes archives/checksums, updates `Formula/just-talk.rb`, and verifies real Homebrew installation on Intel and Apple Silicon. The formula is generated by `scripts/update_homebrew.py`; see [AGENTS.md](AGENTS.md) for maintenance rules.

Merge `upstream/master` when following upstream, preserving this fork's fixes instead of force-syncing. Upstream does not accept PRs. Report this fork's issues in [Issues](https://github.com/Aono255/just-talk-go/issues); version changes are in [CHANGELOG.md](CHANGELOG.md).

## Upstream website and license

The [original project website](https://whoamihappyhacking.github.io/just-talk-go/) describes upstream functionality. This fork retains `website/` source without enabling its own automatic Pages deployment; use this README for current fork features. Preview the page and microphone-free demo locally with `python3 -m http.server 7788 --bind 127.0.0.1 --directory website`.

Just Talk uses the [GNU General Public License v3.0](LICENSE).
